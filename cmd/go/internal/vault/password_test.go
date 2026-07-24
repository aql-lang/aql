package vault

import (
	"os"
	"strings"
	"testing"
)

// setPass switches the authenticating passphrase for subsequent runVault
// calls in this test.
func setPass(t *testing.T, p string) { t.Helper(); t.Setenv(EnvPassphrase, p) }

// mustAddPassword runs `password add` with the new password supplied via
// the env override, authenticating as admin with adminPass.
func mustAddPassword(t *testing.T, adminPass, newPass, name string, args ...string) {
	t.Helper()
	setPass(t, adminPass)
	t.Setenv(EnvNewPassphrase, newPass)
	defer t.Setenv(EnvNewPassphrase, "")
	full := append([]string{"password", "add"}, args...)
	full = append(full, name)
	code, _, errOut := runVault(t, "", full...)
	if code != 0 {
		t.Fatalf("password add %q failed: %s", name, errOut)
	}
}

// TestPasswordAddRotate covers `password add --rotate`: re-issuing a temporary
// password under the SAME name mints a fresh password (invalidating the old
// one) and resets the TTL, so an agent token can be rotated instead of forcing
// a new name. A same-name add WITHOUT --rotate is refused and the error names
// the flag; --rotate over an ABSENT name is just a create.
func TestPasswordAddRotate(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// First add migrates the legacy vault (admin stays "test-pass") and seeds
	// the initial agent token with password "agent-v1".
	mustAddPassword(t, "test-pass", "agent-v1", "agent3", "--scope=read", "--namespaces=*", "--ttl=1h")

	// A same-name add WITHOUT --rotate is refused; the error points at --rotate.
	setPass(t, "test-pass")
	t.Setenv(EnvNewPassphrase, "agent-vX")
	if code, _, e := runVault(t, "", "password", "add", "--scope=read", "--namespaces=*", "--ttl=1h", "agent3"); code == 0 ||
		!strings.Contains(e, "already exists") || !strings.Contains(e, "--rotate") {
		t.Fatalf("duplicate add without --rotate: code=%d err=%q", code, e)
	}

	// --rotate replaces it: succeeds and the output says "rotated".
	setPass(t, "test-pass")
	t.Setenv(EnvNewPassphrase, "agent-v2")
	code, out, e := runVault(t, "", "password", "add", "--rotate", "--generate", "--scope=read", "--namespaces=*", "--ttl=1h", "agent3")
	if code != 0 {
		t.Fatalf("rotate agent3: %s", e)
	}
	if !strings.Contains(out, "rotated") {
		t.Errorf("rotate output = %q, want it to say 'rotated'", out)
	}

	// The OLD password no longer authenticates; the freshly-generated slot exists
	// and kept its TTL.
	s, err := requireStore(home)
	if err != nil {
		t.Fatalf("reload store: %s", err)
	}
	if _, err := openSession(s, home, "agent-v1"); err == nil {
		t.Error("old password still authenticates after --rotate — the slot was not replaced")
	}
	if slot, _ := s.FindPasswordSlot("agent3"); slot == nil {
		t.Error("agent3 slot missing after rotate")
	} else if slot.ExpiresAt == "" {
		t.Error("rotated agent3 lost its TTL")
	}

	// --rotate over an ABSENT name is a create — the output says "added".
	setPass(t, "test-pass")
	t.Setenv(EnvNewPassphrase, "agent-new")
	code, out, e = runVault(t, "", "password", "add", "--rotate", "--scope=read", "--namespaces=*", "--ttl=1h", "agent4")
	if code != 0 {
		t.Fatalf("rotate absent agent4: %s", e)
	}
	if strings.Contains(out, "rotated") || !strings.Contains(out, "added") {
		t.Errorf("rotate over absent name output = %q, want 'added'", out)
	}
}

// TestPasswordRotateRejectsSamePassword pins the reuse guard: an interactive
// --rotate whose new passphrase equals the slot's current one is refused before
// the slot is touched (it would leave the old password still valid, breaking the
// "invalidates the old one" contract), while a genuinely different one rotates
// cleanly. --generate sidesteps the check by construction (see TestPasswordAddRotate).
func TestPasswordRotateRejectsSamePassword(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// Seed an interactive (non-generated) temporary slot with a known password.
	mustAddPassword(t, "test-pass", "agent-v1", "agent3", "--scope=read", "--namespaces=*", "--ttl=1h")

	// Rotating to the SAME password is refused, and the original still opens.
	setPass(t, "test-pass")
	t.Setenv(EnvNewPassphrase, "agent-v1")
	if code, _, e := runVault(t, "", "password", "add", "--rotate", "--scope=read", "--namespaces=*", "--ttl=1h", "agent3"); code == 0 ||
		!strings.Contains(e, "identical to the current one") {
		t.Fatalf("rotate to same password: code=%d err=%q, want a reuse refusal", code, e)
	}
	s, err := requireStore(home)
	if err != nil {
		t.Fatalf("reload store: %s", err)
	}
	if _, err := openSession(s, home, "agent-v1"); err != nil {
		t.Errorf("original password should still authenticate after a refused rotate: %v", err)
	}

	// A DIFFERENT password rotates successfully and invalidates the old one.
	setPass(t, "test-pass")
	t.Setenv(EnvNewPassphrase, "agent-v2")
	if code, out, e := runVault(t, "", "password", "add", "--rotate", "--scope=read", "--namespaces=*", "--ttl=1h", "agent3"); code != 0 ||
		!strings.Contains(out, "rotated") {
		t.Fatalf("rotate to a new password: code=%d out=%q err=%q", code, out, e)
	}
	s, err = requireStore(home)
	if err != nil {
		t.Fatalf("reload store: %s", err)
	}
	if _, err := openSession(s, home, "agent-v1"); err == nil {
		t.Error("old password still authenticates after a successful rotate")
	}
	if _, err := openSession(s, home, "agent-v2"); err != nil {
		t.Errorf("new password should authenticate after rotate: %v", err)
	}
}

// TestPasswordRotateAuthenticatingAdmin pins the case where --rotate targets the
// very admin slot that authorized the command: addSlotToEnvelope must reopen its
// session while the old slot still exists, then upsert the fresh one. A
// remove-then-add would strand the re-auth (the passphrase no longer matches any
// slot) and the rotation would fail.
func TestPasswordRotateAuthenticatingAdmin(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// Migrate to envelope: "test-pass" becomes the "admin" slot.
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")

	// Last-admin guard: rotating the sole admin to a NON-admin scope (--scope
	// defaults to read) would leave no admin and lock password management — it
	// must be refused, before the slot is touched.
	setPass(t, "test-pass")
	t.Setenv(EnvNewPassphrase, "x")
	if code, _, e := runVault(t, "", "password", "add", "--rotate", "admin"); code == 0 ||
		!strings.Contains(e, "last admin") {
		t.Fatalf("rotate sole admin to read scope: code=%d err=%q, want a last-admin refusal", code, e)
	}

	// Rotate the admin slot while authenticating AS admin (keeping --scope=admin).
	setPass(t, "test-pass")
	t.Setenv(EnvNewPassphrase, "admin-v2")
	code, out, e := runVault(t, "", "password", "add", "--rotate", "--scope=admin", "admin")
	if code != 0 {
		t.Fatalf("rotate authenticating admin: %s", e)
	}
	if !strings.Contains(out, "rotated") {
		t.Errorf("admin rotate output = %q, want 'rotated'", out)
	}

	// The OLD admin passphrase no longer authenticates; the NEW one opens an
	// admin session.
	s, err := requireStore(home)
	if err != nil {
		t.Fatalf("reload store: %s", err)
	}
	if _, err := openSession(s, home, "test-pass"); err == nil {
		t.Error("old admin passphrase still authenticates after --rotate")
	}
	sess, err := openSession(s, home, "admin-v2")
	if err != nil {
		t.Fatalf("new admin passphrase should authenticate: %v", err)
	}
	defer sess.Close()
	if err := requireScope(sess, OpAdmin); err != nil {
		t.Errorf("rotated admin slot lost admin scope: %v", err)
	}
}

// TestPasswordAddMigratesAndScopes is the headline end-to-end: a legacy
// vault migrates on first `password add`, the admin keeps full access,
// and a read-scoped namespace password can read only its namespace.
func TestPasswordAddMigratesAndScopes(t *testing.T) {
	testHome(t)
	mustInit(t)

	// Two secrets in the legacy vault: one root, one in 'proj'.
	if code, _, e := runVault(t, "sk-root\n", "add", "--from-stdin", "rootkey"); code != 0 {
		t.Fatalf("add rootkey: %s", e)
	}
	if code, _, e := runVault(t, "sk-proj\n", "add", "--from-stdin", "proj:apikey"); code != 0 {
		t.Fatalf("add proj:apikey: %s", e)
	}

	// First password add migrates the vault; admin passphrase stays "test-pass".
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")

	// Admin still reads everything.
	setPass(t, "test-pass")
	if code, out, e := runVault(t, "", "get", "--reveal", "proj:apikey"); code != 0 || strings.TrimSpace(out) != "sk-proj" {
		t.Fatalf("admin get proj:apikey = %d/%q (%s), want sk-proj", code, out, e)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "rootkey"); code != 0 || strings.TrimSpace(out) != "sk-root" {
		t.Fatalf("admin get rootkey = %d/%q, want sk-root", code, out)
	}

	// Reader reads its namespace.
	setPass(t, "reader-pass")
	if code, out, e := runVault(t, "", "get", "--reveal", "proj:apikey"); code != 0 || strings.TrimSpace(out) != "sk-proj" {
		t.Fatalf("reader get proj:apikey = %d/%q (%s), want sk-proj", code, out, e)
	}
	// Reader cannot read the root namespace (not granted).
	if code, _, e := runVault(t, "", "get", "--reveal", "rootkey"); code == 0 {
		t.Fatalf("reader should not read rootkey")
	} else if !strings.Contains(e, "permission denied") {
		t.Errorf("reader rootkey error = %q, want permission denied", e)
	}
	// Reader (read scope) cannot write.
	if code, _, e := runVault(t, "sk-x\n", "add", "--from-stdin", "proj:newkey"); code == 0 {
		t.Fatalf("reader should not be able to add")
	} else if !strings.Contains(e, "permission denied") {
		t.Errorf("reader add error = %q, want permission denied", e)
	}

	// Wrong passphrase is rejected with the non-revealing message.
	setPass(t, "totally-wrong")
	if code, _, e := runVault(t, "", "get", "--reveal", "proj:apikey"); code == 0 {
		t.Fatal("wrong passphrase should fail")
	} else if !strings.Contains(e, "wrong passphrase") {
		t.Errorf("wrong passphrase error = %q", e)
	}

	// Admin can add a new secret post-migration (envelope write path).
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "sk-another\n", "add", "--from-stdin", "proj:another"); code != 0 {
		t.Fatalf("admin add proj:another: %s", e)
	}
	setPass(t, "reader-pass")
	if code, out, _ := runVault(t, "", "get", "--reveal", "proj:another"); code != 0 || strings.TrimSpace(out) != "sk-another" {
		t.Fatalf("reader get proj:another = %d/%q, want sk-another", code, out)
	}
}

// TestPasswordAddRequiresAdmin: a non-admin (reader) password cannot add
// another password.
func TestPasswordAddRequiresAdmin(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")

	// Reader tries to add a password -> denied.
	setPass(t, "reader-pass")
	t.Setenv(EnvNewPassphrase, "x-pass")
	defer t.Setenv(EnvNewPassphrase, "")
	code, _, errOut := runVault(t, "", "password", "add", "--scope=read", "intruder")
	if code == 0 {
		t.Fatal("reader should not be able to add a password")
	}
	if !strings.Contains(errOut, "admin") {
		t.Errorf("error = %q, want an admin-required message", errOut)
	}
}

// TestPasswordListAndRm covers list output (no secret material) and
// admin removal with confirmation, including the last-admin guard.
func TestPasswordListAndRm(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")

	setPass(t, "test-pass")
	code, out, e := runVault(t, "", "password", "list")
	if code != 0 {
		t.Fatalf("password list: %s", e)
	}
	if !strings.Contains(out, "admin") || !strings.Contains(out, "reader") {
		t.Errorf("list missing slots:\n%s", out)
	}
	// list must never leak verifier/salt/key material.
	s, _ := LoadStore(home)
	for _, p := range s.Passwords {
		if p.Verifier != "" && strings.Contains(out, p.Verifier) {
			t.Error("list leaked a verifier")
		}
		if p.Salt != "" && strings.Contains(out, p.Salt) {
			t.Error("list leaked a salt")
		}
	}

	// Non-interactive rm without --yes is refused.
	if code, _, _ := runVault(t, "", "password", "rm", "reader"); code == 0 {
		t.Error("rm without --yes should be refused non-interactively")
	}
	// With --yes it succeeds.
	if code, _, e := runVault(t, "", "password", "rm", "--yes", "reader"); code != 0 {
		t.Fatalf("password rm --yes reader: %s", e)
	}
	if _, out, _ := runVault(t, "", "password", "list"); strings.Contains(out, "reader") {
		t.Errorf("reader still listed after rm:\n%s", out)
	}
	// Removing the last admin is refused.
	if code, _, e := runVault(t, "", "password", "rm", "--yes", "admin"); code == 0 {
		t.Errorf("removing the last admin should be refused (%s)", e)
	}
}

// TestEnvelopeRotateAndVerify proves the converted write (rotate) and
// reconcile (verify) paths work on an envelope vault, and that a
// namespace-scoped reader still reads a rotated value (same NDK).
func TestEnvelopeRotateAndVerify(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v1\n", "add", "--from-stdin", "proj:k1"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")

	// Admin rotates the value on the envelope vault.
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "v2\n", "rotate", "--yes", "--from-stdin", "proj:k1"); code != 0 {
		t.Fatalf("rotate on envelope vault: %s", e)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "proj:k1"); code != 0 || strings.TrimSpace(out) != "v2" {
		t.Fatalf("admin get after rotate = %d/%q, want v2", code, out)
	}
	// The reader sees the rotated value (rotation preserves the NDK).
	setPass(t, "reader-pass")
	if code, out, _ := runVault(t, "", "get", "--reveal", "proj:k1"); code != 0 || strings.TrimSpace(out) != "v2" {
		t.Fatalf("reader get after rotate = %d/%q, want v2", code, out)
	}
	// verify reconciles cleanly (no spurious dangling/orphan issues).
	setPass(t, "test-pass")
	if code, out, e := runVault(t, "", "verify"); code != 0 {
		t.Fatalf("verify on envelope vault: %s / %s", out, e)
	}
}

// TestPasswordAssignNamespaces: an admin widens then narrows a reader's
// namespace access without the reader's passphrase.
func TestPasswordAssignNamespaces(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "p\n", "add", "--from-stdin", "proj:apikey"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")

	// --scope is rejected on assign.
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "", "password", "assign", "--scope=write", "--namespaces=proj", "reader"); code == 0 {
		t.Errorf("assign --scope should be rejected (%s)", e)
	}

	// Widen reader to proj+staging (admin, no reader passphrase).
	if code, _, e := runVault(t, "", "password", "assign", "--yes", "--namespaces=proj,staging", "reader"); code != 0 {
		t.Fatalf("assign widen: %s", e)
	}
	// Admin adds a staging secret; reader can now read it.
	if code, _, e := runVault(t, "s1\n", "add", "--from-stdin", "staging:s"); code != 0 {
		t.Fatalf("admin add staging: %s", e)
	}
	setPass(t, "reader-pass")
	if code, out, _ := runVault(t, "", "get", "--reveal", "staging:s"); code != 0 || strings.TrimSpace(out) != "s1" {
		t.Fatalf("reader get staging after widen = %d/%q, want s1", code, out)
	}

	// Narrow reader back to staging only; proj access is revoked.
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "", "password", "assign", "--yes", "--namespaces=staging", "reader"); code != 0 {
		t.Fatalf("assign narrow: %s", e)
	}
	setPass(t, "reader-pass")
	if code, _, _ := runVault(t, "", "get", "--reveal", "proj:apikey"); code == 0 {
		t.Error("reader should no longer read proj after narrowing")
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "staging:s"); code != 0 || strings.TrimSpace(out) != "s1" {
		t.Errorf("reader should still read staging = %d/%q", code, out)
	}
}

// TestPasswordSetRotates: a holder rotates their own passphrase; the old
// one stops working and the new one reads the same secrets.
func TestPasswordSetRotates(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "p\n", "add", "--from-stdin", "proj:apikey"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")

	// reader rotates its own password: current=reader-pass, new=reader2.
	setPass(t, "reader-pass")
	t.Setenv(EnvNewPassphrase, "reader2")
	if code, _, e := runVault(t, "", "password", "set", "reader"); code != 0 {
		t.Fatalf("password set: %s", e)
	}
	t.Setenv(EnvNewPassphrase, "")

	// Old password no longer authenticates.
	setPass(t, "reader-pass")
	if code, _, _ := runVault(t, "", "get", "--reveal", "proj:apikey"); code == 0 {
		t.Error("old reader password should no longer work after set")
	}
	// New password reads the same secret.
	setPass(t, "reader2")
	if code, out, _ := runVault(t, "", "get", "--reveal", "proj:apikey"); code != 0 || strings.TrimSpace(out) != "p" {
		t.Fatalf("new reader password get = %d/%q, want p", code, out)
	}
}

// TestMigrationTagOnlyNamespaceRoundTrip is the regression for the
// critical migration bug: a LEGACY tag-only alias (bare Name with a
// Namespace metadata tag) must still decrypt after migration, because
// sealing and reading both use the name-derived namespace (root here),
// not the cosmetic tag.
func TestMigrationTagOnlyNamespaceRoundTrip(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// Hand-craft the pre-namespacing shape: value in the legacy keyring,
	// alias with a bare Name but a Namespace tag.
	kr := &fileKeyring{folder: vaultFolder(home), pass: "test-pass"}
	if err := kr.Set("foo", "tagged-secret"); err != nil {
		t.Fatal(err)
	}
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	s.Aliases = append(s.Aliases, Alias{Name: "foo", Namespace: "proj", CreatedAt: "2020-01-01T00:00:00Z"})
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}

	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--namespaces=*")

	setPass(t, "test-pass")
	if code, out, e := runVault(t, "", "get", "--reveal", "foo"); code != 0 || strings.TrimSpace(out) != "tagged-secret" {
		t.Fatalf("tag-only alias after migration = %d/%q (%s), want tagged-secret", code, out, e)
	}
}

// TestEnvelopeAddBrandNewNamespace is the regression for the brand-new
// namespace write: an admin can add a secret to a namespace that had no
// data key at migration (the NDK is auto-created).
func TestEnvelopeAddBrandNewNamespace(t *testing.T) {
	testHome(t)
	mustInit(t)
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--namespaces=*")
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "sk\n", "add", "--from-stdin", "brandnew:key"); code != 0 {
		t.Fatalf("admin add to brand-new namespace: %s", e)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "brandnew:key"); code != 0 || strings.TrimSpace(out) != "sk" {
		t.Fatalf("get brandnew:key = %d/%q, want sk", code, out)
	}
}

// TestEnvelopeMvCrossNamespace proves mv re-keys a value across
// namespaces on an envelope vault (decrypt under source NDK, re-seal
// under destination NDK).
func TestEnvelopeMvCrossNamespace(t *testing.T) {
	testHome(t)
	mustInit(t)
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--namespaces=*")
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "moveme\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	// Move proj:k -> staging:k2 (staging is brand-new; admin auto-creates it).
	if code, _, e := runVault(t, "", "mv", "proj:k", "staging:k2"); code != 0 {
		t.Fatalf("mv across namespaces: %s", e)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "staging:k2"); code != 0 || strings.TrimSpace(out) != "moveme" {
		t.Fatalf("get staging:k2 after mv = %d/%q, want moveme", code, out)
	}
	if code, _, _ := runVault(t, "", "get", "--reveal", "proj:k"); code == 0 {
		t.Error("proj:k should be gone after mv")
	}
}

// TestPasswordRmRekey: removing a slot with --rekey rotates the data
// keys of the namespaces it could reach (re-encrypting their values),
// so the surviving admin still reads them and the on-disk NDK id has
// changed.
func TestPasswordRmRekey(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "secret\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("add proj:k: %s", e)
	}

	// Capture the NDK id the value is currently sealed under.
	kr := &envelopeFileKeyring{folder: vaultFolder(home)}
	before, err := kr.Get("proj:k")
	if err != nil {
		t.Fatal(err)
	}
	oldID, _ := sealedValueNDKID(before)

	// Remove the reader WITH rekey.
	if code, _, e := runVault(t, "", "password", "rm", "--rekey", "--yes", "reader"); code != 0 {
		t.Fatalf("password rm --rekey: %s", e)
	}

	// Reader slot is gone.
	if _, out, _ := runVault(t, "", "password", "list"); strings.Contains(out, "reader") {
		t.Errorf("reader still present after rm --rekey:\n%s", out)
	}
	// Admin still reads the (re-encrypted) value.
	if code, out, _ := runVault(t, "", "get", "--reveal", "proj:k"); code != 0 || strings.TrimSpace(out) != "secret" {
		t.Fatalf("admin get after rekey = %d/%q, want secret", code, out)
	}
	// The value was re-encrypted: its NDK id changed.
	after, err := kr.Get("proj:k")
	if err != nil {
		t.Fatal(err)
	}
	newID, _ := sealedValueNDKID(after)
	if string(oldID) == string(newID) {
		t.Error("NDK id did not change after --rekey (value not re-encrypted)")
	}
	// The former reader passphrase no longer authenticates.
	setPass(t, "reader-pass")
	if code, _, _ := runVault(t, "", "get", "--reveal", "proj:k"); code == 0 {
		t.Error("removed reader passphrase should no longer authenticate")
	}
}

// TestEnvelopeIntegrityKeyedHMAC proves the keyed integrity layer (M0):
// a clean envelope vault verifies "ok", and a tamper that recomputes the
// cheap sidecar checksum (but cannot forge the keyed HMAC) is caught as
// hmac-invalid on an admin command.
func TestEnvelopeIntegrityKeyedHMAC(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--namespaces=*")
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	// Clean vault verifies ok (the add sealed the keyed sidecar).
	if code, out, e := runVault(t, "", "verify"); code != 0 || !strings.Contains(out, "integrity: ok") {
		t.Fatalf("verify on clean envelope vault = %d, out=%q err=%q (want integrity: ok)", code, out, e)
	}

	// Forge a tamper: change the store content AND recompute the cheap
	// sidecar checksum (what a keyless attacker can do), but sign the
	// HMAC with the wrong key (what they cannot forge without the
	// integrity key).
	data, err := os.ReadFile(StorePath(home))
	if err != nil {
		t.Fatal(err)
	}
	tampered := append(append([]byte(nil), data...), ' ') // trailing space: still parses, different bytes
	if err := os.WriteFile(StorePath(home), tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	gen, _ := storeGenerationOnDisk(tampered)
	if err := writeSidecar(home, gen, tampered, mustRandom(t, 32)); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := runVault(t, "", "verify"); code != 0 || !strings.Contains(out, "integrity: hmac-invalid") {
		t.Fatalf("verify on tampered vault out=%q (want integrity: hmac-invalid)", out)
	}
}

// TestScopeFlipFailsAuthentication is the H2 end-to-end: editing a
// slot's scope in vault.jsonic makes that password fail to authenticate
// rather than escalate.
func TestScopeFlipFailsAuthentication(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")

	// Tamper: flip reader's scope to admin directly in the store.
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	slot, idx := s.FindPasswordSlot("reader")
	if slot == nil {
		t.Fatal("reader slot missing")
	}
	s.Passwords[idx].Scope = ScopeAdmin
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}

	// The reader passphrase must now FAIL to authenticate (verifier +
	// AAD bind the scope), not gain admin.
	setPass(t, "reader-pass")
	if code, _, e := runVault(t, "", "get", "--reveal", "proj:k"); code == 0 {
		t.Fatal("scope-flipped slot should fail authentication, not escalate")
	} else if !strings.Contains(e, "wrong passphrase") {
		t.Errorf("error = %q, want the non-revealing wrong-passphrase message", e)
	}
}
