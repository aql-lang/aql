package vault

// verify_seam7_test.go — runVerify error/repair arms: flag parse, store &
// authenticate refusals, load/list failures under the lock, the non-lister
// backend dangling-check branches, revoked-cap skip, prune-delete failures,
// and the envelope integrity re-seal.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// s7verifyLockWindow seeds a legacy file vault, holds the vault lock in a
// goroutine while `vault verify` blocks on it, mutates the store in that
// window, then releases. verify's requireStore/authenticate run before the
// lock, so mutating after they finish exercises verify's under-lock load.
func s7verifyLockWindow(t *testing.T, mutate func(home string)) (int, string, string) {
	t.Helper()
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed add: %s", e)
	}
	held := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = withVaultLock(home, func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	var code int
	var out, errOut string
	done := make(chan struct{})
	go func() {
		defer close(done)
		code, out, errOut = runVault(t, "", "verify")
	}()
	// Give verify time to clear requireStore+authenticate and park on the
	// held lock, then mutate the store it will re-load once it acquires it.
	time.Sleep(300 * time.Millisecond)
	mutate(home)
	close(release)
	<-done
	return code, out, errOut
}

func TestS7_VerifyFlagParseError(t *testing.T) {
	testHome(t)
	if code, _, _ := runVault(t, "", "verify", "-definitely-not-a-flag"); code != 1 {
		t.Errorf("verify with a bad flag = %d, want 1", code)
	}
}

func TestS7_VerifyRequireStoreError(t *testing.T) {
	testHome(t) // no init
	code, _, errOut := runVault(t, "", "verify")
	if code != 1 || !strings.Contains(errOut, "not initialized") {
		t.Errorf("verify before init = %d, %q", code, errOut)
	}
}

func TestS7_VerifyAuthenticateError(t *testing.T) {
	w4EnvelopeVault(t)
	setPass(t, "s7-wrong-pass")
	code, _, errOut := runVault(t, "", "verify")
	if code != 1 || errOut == "" {
		t.Errorf("verify with a wrong passphrase = %d, %q", code, errOut)
	}
}

func TestS7_VerifyLoadStoreErrorUnderLock(t *testing.T) {
	code, _, errOut := s7verifyLockWindow(t, func(home string) {
		if err := os.WriteFile(StorePath(home), []byte("{corrupt json"), 0600); err != nil {
			t.Error(err)
		}
	})
	if code != 1 || errOut == "" {
		t.Errorf("verify over a corrupted-under-lock store = %d, %q", code, errOut)
	}
}

func TestS7_VerifyStoreVanishedUnderLock(t *testing.T) {
	code, _, errOut := s7verifyLockWindow(t, func(home string) {
		if err := os.Remove(StorePath(home)); err != nil {
			t.Error(err)
		}
	})
	if code != 1 || !strings.Contains(errOut, "not initialized") {
		t.Errorf("verify over a vanished-under-lock store = %d, %q", code, errOut)
	}
}

func TestS7_VerifyKeyringListError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	// Corrupt the keyring so the lister's decrypt (inside the lock) fails.
	kp := filepath.Join(vaultFolder(home), vaultFileName("keyring"))
	if err := os.WriteFile(kp, []byte("garbage"), 0600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runVault(t, "", "verify")
	if code != 1 || !strings.Contains(errOut, "listing keyring") {
		t.Errorf("verify over a corrupt keyring = %d, %q", code, errOut)
	}
}

func TestS7_VerifyNonListerBackend(t *testing.T) {
	stubDir := t.TempDir()
	// secret-tool stub: store succeeds; lookup for s7gone → exit 1 with no
	// stderr (== not found); lookup for s7err → exit 1 WITH stderr (== a
	// generic error).
	stubBin(t, stubDir, "secret-tool", `case "$1" in
store) cat >/dev/null; exit 0;;
lookup)
  for a in "$@"; do last="$a"; done
  case "$last" in
  s7gone) exit 1;;
  s7err) echo boom >&2; exit 1;;
  *) exit 1;;
  esac;;
clear) exit 0;;
esac
exit 0
`)
	prependPath(t, stubDir)
	home := testHome(t)
	if code, _, e := runVault(t, "", "init", "--backend=secret-service"); code != 0 {
		t.Fatalf("init secret-service: %s", e)
	}
	// Inject two metadata aliases (no keyring entries behind them).
	s, _ := LoadStore(home)
	s.UpsertAlias(Alias{Name: "s7gone"})
	s.UpsertAlias(Alias{Name: "s7err"})
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runVault(t, "", "verify")
	if !strings.Contains(out, "dangling metadata") || !strings.Contains(out, "s7gone") {
		t.Errorf("missing dangling report for s7gone: out=%q", out)
	}
	if !strings.Contains(errOut, "could not check s7err") {
		t.Errorf("missing warning for s7err: err=%q", errOut)
	}
	if !strings.Contains(out, "orphan detection is unavailable") {
		t.Errorf("missing non-lister note: out=%q", out)
	}
	if code == 0 {
		t.Errorf("verify with a dangling alias should be non-zero, got %d", code)
	}
}

func TestS7_VerifyRevokedCapabilitySkipped(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	s, _ := LoadStore(home)
	// A revoked cap referencing a missing alias must be skipped (not
	// reported) by the capability check.
	s.Capabilities = append(s.Capabilities, Capability{ID: "s7deadcap00000000", Alias: "ghost-missing", Revoked: true})
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runVault(t, "", "verify")
	if strings.Contains(out, "ghost-missing") {
		t.Errorf("revoked cap should not be reported: out=%q", out)
	}
	if code != 0 {
		t.Errorf("verify with only a revoked cap = %d, want 0; out=%q", code, out)
	}
}

func TestS7_VerifyOrphanDeleteError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	// One orphaned secret with no metadata.
	if err := testKeyring(t).Set("s7orphan", "v"); err != nil {
		t.Fatal(err)
	}
	// Make the keyring save (inside kr.Delete) fail during --prune.
	old := createTemp
	createTemp = func(string, string) (*os.File, error) { return nil, errS7Boom }
	t.Cleanup(func() { createTemp = old })
	_, out, errOut := runVault(t, "", "verify", "--prune")
	_ = home
	if !strings.Contains(out, "orphaned keyring entry") || !strings.Contains(out, "s7orphan") {
		t.Errorf("missing orphan report: out=%q", out)
	}
	if !strings.Contains(errOut, "deleting orphan s7orphan") {
		t.Errorf("missing orphan-delete warning: err=%q", errOut)
	}
}

func TestS7_VerifyStaleTempRemoveError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	stale := filepath.Join(vaultFolder(home), ".vault.jsonic.s7stale.tmp")
	if err := os.WriteFile(stale, []byte("junk"), 0600); err != nil {
		t.Fatal(err)
	}
	// Make the stale-temp removal fail during --prune. (A read-only folder
	// would not stop root, under which these tests run.)
	old := s7osRemove
	s7osRemove = func(string) error { return errS7Boom }
	t.Cleanup(func() { s7osRemove = old })
	_, out, errOut := runVault(t, "", "verify", "--prune")
	if !strings.Contains(out, "stale temp file") {
		t.Errorf("missing stale-temp report: out=%q", out)
	}
	if !strings.Contains(errOut, "removing .vault.jsonic.s7stale.tmp") {
		t.Errorf("missing stale-remove warning: err=%q", errOut)
	}
}

func TestS7_VerifyEnvelopePrunedSealsIntegrity(t *testing.T) {
	home := w4EnvelopeVault(t) // admin test-pass; secret proj:k
	// Delete the secret behind proj:k, leaving dangling metadata that
	// --prune repairs (pruned > 0) on an envelope vault.
	kr := &envelopeFileKeyring{folder: vaultFolder(home)}
	if err := kr.Delete("proj:k"); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runVault(t, "", "verify", "--prune")
	if code != 0 {
		t.Fatalf("verify --prune = %d; out=%q err=%q", code, out, errOut)
	}
	if !strings.Contains(out, "dangling metadata") {
		t.Errorf("missing dangling report: out=%q", out)
	}
	if !strings.Contains(out, "integrity:") {
		t.Errorf("missing envelope integrity report: out=%q", out)
	}
}
