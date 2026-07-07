package vault

// password_more_test.go — argument/refusal edges of the password verbs
// and the pure helpers (parseNamespaceList, readNewPassphrase).

import (
	"strings"
	"testing"
)

func TestPasswordVerbDispatch(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, errOut := runVault(t, "", "password"); code == 0 || !strings.Contains(errOut, "usage") {
		t.Errorf("bare password: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "password", "frobnicate"); code == 0 ||
		!strings.Contains(errOut, "unknown password verb") {
		t.Errorf("unknown verb: %q", errOut)
	}
	// The pw alias routes too.
	if code, _, errOut := runVault(t, "", "pw", "list"); code != 0 {
		t.Errorf("pw alias: %q", errOut)
	}
}

func TestPasswordListLegacyVault(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, out, _ := runVault(t, "", "password", "list")
	if code != 0 || !strings.Contains(out, "no password slots") {
		t.Errorf("legacy list = %d %q", code, out)
	}
	// Before init, list requires a store.
	dir2 := t.TempDir()
	t.Setenv(EnvHome, dir2)
	if code, _, errOut := runVault(t, "", "password", "list"); code == 0 ||
		!strings.Contains(errOut, "not initialized") {
		t.Errorf("list before init: %q", errOut)
	}
}

func TestPasswordAddArgumentValidation(t *testing.T) {
	testHome(t)
	mustInit(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no name", []string{"password", "add"}, "usage"},
		{"two names", []string{"password", "add", "a", "b"}, "usage"},
		{"bad name", []string{"password", "add", "with:colon"}, "invalid password name"},
		{"reserved name", []string{"password", "add", "@anchor"}, "invalid password name"},
		{"bad scope", []string{"password", "add", "--scope=root", "x"}, "invalid scope"},
		{"bad namespaces", []string{"password", "add", "--namespaces=bad ns", "x"}, "invalid namespace"},
		{"admin reserved on legacy", []string{"password", "add", "admin"}, `reserves the name "admin"`},
	}
	t.Setenv(EnvNewPassphrase, "np")
	for _, tc := range cases {
		code, _, errOut := runVault(t, "", tc.args...)
		if code == 0 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: code=%d err=%q (want %q)", tc.name, code, errOut, tc.want)
		}
	}
	// A locked vault refuses password add.
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, errOut := runVault(t, "", "password", "add", "x"); code == 0 ||
		!strings.Contains(errOut, "locked") {
		t.Errorf("locked add: %q", errOut)
	}
	if code, _, _ := runVault(t, "", "unlock"); code != 0 {
		t.Fatal("unlock")
	}
	// The wrong current passphrase fails the legacy pre-migration check.
	setPass(t, "wrong-pass")
	// Seed a value so the legacy keyring exists and validates passphrases.
	setPass(t, "test-pass")
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("seed: %s", e)
	}
	setPass(t, "wrong-pass")
	if code, _, errOut := runVault(t, "", "password", "add", "reader"); code == 0 ||
		!strings.Contains(errOut, "passphrase") {
		t.Errorf("wrong current passphrase: %q", errOut)
	}
}

func TestPasswordAddDuplicateAndSecondSlot(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("seed: %s", e)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")

	// A duplicate name on the envelope vault is rejected.
	setPass(t, "test-pass")
	t.Setenv(EnvNewPassphrase, "other")
	if code, _, errOut := runVault(t, "", "password", "add", "reader"); code == 0 ||
		!strings.Contains(errOut, "already exists") {
		t.Errorf("duplicate add: %q", errOut)
	}
	// A second slot on an existing envelope vault, granting a brand-new
	// namespace, exercises addSlotToEnvelope's NDK-minting path.
	mustAddPassword(t, "test-pass", "writer-pass", "writer", "--scope=write", "--namespaces=brandnew")
	setPass(t, "writer-pass")
	if code, _, e := runVault(t, "nv\n", "add", "--from-stdin", "brandnew:k"); code != 0 {
		t.Fatalf("writer add to brand-new ns: %s", e)
	}
	if code, out, _ := runVault(t, "", "get", "--reveal", "brandnew:k"); code != 0 || !strings.Contains(out, "nv") {
		t.Errorf("writer roundtrip = %d %q", code, out)
	}
	// The admin can also read it (the NDK was sealed to admin slots).
	setPass(t, "test-pass")
	if code, out, _ := runVault(t, "", "get", "--reveal", "brandnew:k"); code != 0 || !strings.Contains(out, "nv") {
		t.Errorf("admin read of brand-new ns = %d %q", code, out)
	}
	// An admin-scoped slot receives every namespace.
	mustAddPassword(t, "test-pass", "admin2-pass", "admin2", "--scope=admin")
	setPass(t, "admin2-pass")
	if code, out, _ := runVault(t, "", "get", "--reveal", "proj:k"); code != 0 || !strings.Contains(out, "v") {
		t.Errorf("second admin read = %d %q", code, out)
	}
}

func TestPasswordRmEdges(t *testing.T) {
	testHome(t)
	mustInit(t)
	// No slots yet.
	if code, _, errOut := runVault(t, "", "password", "rm", "--yes", "x"); code == 0 ||
		!strings.Contains(errOut, "no password slots") {
		t.Errorf("rm on legacy vault: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "password", "rm"); code == 0 || !strings.Contains(errOut, "usage") {
		t.Errorf("rm usage: %q", errOut)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=*")
	// Unknown slot name.
	setPass(t, "test-pass")
	if code, _, errOut := runVault(t, "", "password", "rm", "--yes", "ghost"); code == 0 ||
		!strings.Contains(errOut, "not found") {
		t.Errorf("rm unknown: %q", errOut)
	}
	// A non-admin cannot remove passwords.
	setPass(t, "reader-pass")
	if code, _, errOut := runVault(t, "", "password", "rm", "--yes", "reader"); code == 0 ||
		!strings.Contains(errOut, "admin") {
		t.Errorf("reader rm: %q", errOut)
	}
	// The rm note reminds about --rekey.
	setPass(t, "test-pass")
	code, _, errOut := runVault(t, "", "password", "rm", "--yes", "reader")
	if code != 0 {
		t.Fatalf("rm reader: %s", errOut)
	}
	if !strings.Contains(errOut, "--rekey") {
		t.Errorf("rm note missing: %q", errOut)
	}
}

func TestPasswordSetEdges(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, errOut := runVault(t, "", "password", "set"); code == 0 || !strings.Contains(errOut, "usage") {
		t.Errorf("set usage: %q", errOut)
	}
	if code, _, errOut := runVault(t, "", "password", "set", "x"); code == 0 ||
		!strings.Contains(errOut, "no password slots") {
		t.Errorf("set on legacy vault: %q", errOut)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=*")
	// The authenticated slot must be the named one.
	setPass(t, "test-pass") // authenticates as admin, not reader
	t.Setenv(EnvNewPassphrase, "np")
	if code, _, errOut := runVault(t, "", "password", "set", "reader"); code == 0 ||
		!strings.Contains(errOut, "not password") {
		t.Errorf("set with the wrong holder: %q", errOut)
	}
	// A wrong passphrase entirely fails authentication.
	setPass(t, "no-such-pass")
	if code, _, errOut := runVault(t, "", "password", "set", "reader"); code == 0 ||
		!strings.Contains(errOut, "wrong passphrase") {
		t.Errorf("set with a wrong passphrase: %q", errOut)
	}
}

func TestPasswordAssignEdges(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("seed: %s", e)
	}
	mustAddPassword(t, "test-pass", "reader-pass", "reader", "--scope=read", "--namespaces=proj")
	setPass(t, "test-pass")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"usage", []string{"password", "assign", "--namespaces=proj"}, "usage"},
		{"missing namespaces", []string{"password", "assign", "reader"}, "requires --namespaces"},
		{"bad namespaces", []string{"password", "assign", "--namespaces=bad ns", "reader"}, "invalid namespace"},
		{"unknown slot", []string{"password", "assign", "--yes", "--namespaces=proj", "ghost"}, "no password named"},
		{"admin slot", []string{"password", "assign", "--yes", "--namespaces=proj", "admin"}, "admin passwords"},
	}
	for _, tc := range cases {
		code, _, errOut := runVault(t, "", tc.args...)
		if code == 0 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: code=%d err=%q (want %q)", tc.name, code, errOut, tc.want)
		}
	}
	// Without --yes there is no terminal for the tier-1 confirm.
	if code, _, errOut := runVault(t, "", "password", "assign", "--namespaces=proj", "reader"); code == 0 ||
		!strings.Contains(errOut, "confirmation") {
		t.Errorf("assign without --yes: %q", errOut)
	}
	// A non-admin cannot reassign.
	setPass(t, "reader-pass")
	if code, _, errOut := runVault(t, "", "password", "assign", "--yes", "--namespaces=proj", "reader"); code == 0 ||
		!strings.Contains(errOut, "admin") {
		t.Errorf("reader assign: %q", errOut)
	}
	// A locked vault refuses assignment.
	setPass(t, "test-pass")
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	if code, _, errOut := runVault(t, "", "password", "assign", "--yes", "--namespaces=proj", "reader"); code == 0 ||
		!strings.Contains(errOut, "locked") {
		t.Errorf("locked assign: %q", errOut)
	}
	// Widening to '*' (all) drives resolveGrant's star path.
	if code, _, _ := runVault(t, "", "unlock"); code != 0 {
		t.Fatal("unlock")
	}
	if code, _, e := runVault(t, "", "password", "assign", "--yes", "--namespaces=*", "reader"); code != 0 {
		t.Fatalf("assign star: %s", e)
	}
	setPass(t, "reader-pass")
	if code, out, _ := runVault(t, "", "get", "--reveal", "proj:k"); code != 0 || !strings.Contains(out, "v") {
		t.Errorf("reader after star assign = %d %q", code, out)
	}
}

func TestParseNamespaceList(t *testing.T) {
	cases := []struct {
		in      string
		want    string // comma-joined
		wantErr bool
	}{
		{"", "*", false},
		{"*", "*", false},
		{"proj", "proj", false},
		{"proj,staging", "proj,staging", false},
		{"proj,proj", "proj", false}, // deduped
		{"proj,*", "*", false},       // star swallows the rest
		{":", "", false},             // root as the empty stored form
		{"proj,:", "proj,", false},   // named + root
		{" proj , staging ", "proj,staging", false},
		{"bad ns", "", true},
		{",,", "", false}, // empty tokens read as root, deduped to one
	}
	for _, tc := range cases {
		got, err := parseNamespaceList(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseNamespaceList(%q) should error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseNamespaceList(%q): %v", tc.in, err)
			continue
		}
		if joined := strings.Join(got, ","); joined != tc.want {
			t.Errorf("parseNamespaceList(%q) = %q, want %q", tc.in, joined, tc.want)
		}
	}
}

func TestReadNewPassphraseHelpers(t *testing.T) {
	t.Setenv(EnvNewPassphrase, "from-env")
	if p, err := readNewPassphrase(nil, nil); err != nil || p != "from-env" {
		t.Errorf("env path = %q, %v", p, err)
	}
	t.Setenv(EnvNewPassphrase, "")
	if _, err := readNewPassphrase(nil, nil); err == nil ||
		!strings.Contains(err.Error(), EnvNewPassphrase) {
		t.Errorf("nil stdin should name the env var: %v", err)
	}
	var sink strings.Builder
	if _, err := readNewPassphrase(strings.NewReader("a\nb\n"), &sink); err == nil ||
		!strings.Contains(err.Error(), "did not match") {
		t.Errorf("mismatch: %v", err)
	}
	if _, err := readNewPassphrase(strings.NewReader("\n\n"), &sink); err == nil ||
		!strings.Contains(err.Error(), "empty") {
		t.Errorf("empty: %v", err)
	}
	if p, err := readNewPassphrase(strings.NewReader("np\nnp\n"), &sink); err != nil || p != "np" {
		t.Errorf("match = %q, %v", p, err)
	}
}

func TestSlotHasNamespaceWrap(t *testing.T) {
	slot := &PasswordSlot{WrappedKeys: map[string]string{
		"proj#aa11": "w", "@integrity#bb22": "w",
	}}
	if !slotHasNamespaceWrap(slot, "proj") {
		t.Error("proj wrap should be found")
	}
	if slotHasNamespaceWrap(slot, "other") {
		t.Error("other should not be found")
	}
	if !slotHasNamespaceWrap(slot, "@integrity") {
		t.Error("integrity wrap should be found")
	}
}
