package vault

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

// testHome sets AQL_HOME to a fresh temp dir and a known passphrase,
// returning the temp dir. The dir is auto-cleaned by t.TempDir().
func testHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(EnvHome, dir)
	t.Setenv(EnvPassphrase, "test-pass")
	return dir
}

func runVault(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func mustInit(t *testing.T) {
	t.Helper()
	code, _, err := runVault(t, "", "init", "--backend=file")
	if code != 0 {
		t.Fatalf("init failed: %s", err)
	}
}

func TestInitCreatesStore(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	if _, err := os.Stat(StorePath(home)); err != nil {
		t.Fatalf("store not created: %s", err)
	}
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.Backend != BackendFile {
		t.Errorf("backend=%q, want %q", s.Backend, BackendFile)
	}
	if s.Version != storeVersion {
		t.Errorf("version=%d, want %d", s.Version, storeVersion)
	}
}

func TestInitRefusesReinitWithoutForce(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, _, errOut := runVault(t, "", "init", "--backend=file")
	if code == 0 {
		t.Fatal("expected non-zero exit on reinit without --force")
	}
	if !strings.Contains(errOut, "already initialized") {
		t.Errorf("missing reinit error in %q", errOut)
	}
}

func TestAddGetListRemove(t *testing.T) {
	testHome(t)
	mustInit(t)

	// Add a secret via stdin.
	code, _, errOut := runVault(t, "sk-test-12345\n", "add", "--from-stdin", "--provider=openai", "openai")
	if code != 0 {
		t.Fatalf("add: %s", errOut)
	}

	// list should show the alias but never the value.
	code, out, errOut := runVault(t, "", "list")
	if code != 0 {
		t.Fatalf("list: %s", errOut)
	}
	if !strings.Contains(out, "openai") {
		t.Errorf("list missing alias: %q", out)
	}
	if strings.Contains(out, "sk-test-12345") {
		t.Errorf("list leaked secret value: %q", out)
	}

	// get without --reveal must redact.
	code, out, errOut = runVault(t, "", "get", "openai")
	if code != 0 {
		t.Fatalf("get: %s", errOut)
	}
	if strings.Contains(out, "sk-test-12345") {
		t.Errorf("get without --reveal leaked secret: %q", out)
	}

	// get --reveal must print the actual value.
	code, out, errOut = runVault(t, "", "get", "--reveal", "openai")
	if code != 0 {
		t.Fatalf("get --reveal: %s", errOut)
	}
	if !strings.Contains(out, "sk-test-12345") {
		t.Errorf("get --reveal missing value: %q", out)
	}

	// rm.
	code, _, errOut = runVault(t, "", "rm", "openai")
	if code != 0 {
		t.Fatalf("rm: %s", errOut)
	}
	s, _ := LoadStore(os.Getenv(EnvHome))
	if a, _ := s.FindAlias("openai"); a != nil {
		t.Errorf("alias still present after rm")
	}
}

func TestAddFromEnv(t *testing.T) {
	testHome(t)
	mustInit(t)
	t.Setenv("MY_KEY", "value-from-env")

	code, _, errOut := runVault(t, "", "add", "--from-env=MY_KEY", "github")
	if code != 0 {
		t.Fatalf("add --from-env: %s", errOut)
	}
	code, out, errOut := runVault(t, "", "get", "--reveal", "github")
	if code != 0 {
		t.Fatalf("get: %s", errOut)
	}
	if !strings.Contains(out, "value-from-env") {
		t.Errorf("missing env-sourced value: %q", out)
	}
}

func TestAddRejectsInvalidAlias(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, _, errOut := runVault(t, "secret\n", "add", "--from-stdin", "bad alias!")
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid alias")
	}
	if !strings.Contains(errOut, "invalid alias") {
		t.Errorf("missing invalid-alias message: %q", errOut)
	}
}

func TestGetRefusesUnknownAlias(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, _, errOut := runVault(t, "", "get", "nope")
	if code == 0 {
		t.Fatal("expected non-zero exit on missing alias")
	}
	if !strings.Contains(errOut, "no alias") {
		t.Errorf("missing alias error: %q", errOut)
	}
}

func TestLockBlocksAdd(t *testing.T) {
	testHome(t)
	mustInit(t)

	code, _, _ := runVault(t, "", "lock")
	if code != 0 {
		t.Fatal("lock failed")
	}
	code, _, errOut := runVault(t, "x\n", "add", "--from-stdin", "k")
	if code == 0 {
		t.Fatal("expected add to fail while locked")
	}
	if !strings.Contains(errOut, "locked") {
		t.Errorf("missing locked message: %q", errOut)
	}

	code, _, _ = runVault(t, "", "unlock")
	if code != 0 {
		t.Fatal("unlock failed")
	}
	code, _, errOut = runVault(t, "x\n", "add", "--from-stdin", "k")
	if code != 0 {
		t.Fatalf("add after unlock failed: %s", errOut)
	}
}

func TestImportDotenv(t *testing.T) {
	home := testHome(t)
	mustInit(t)

	envPath := home + "/sample.env"
	contents := `# comment
export FOO=bar
QUOTED="hello world"
EMPTY_KEY=
SINGLE='single quoted'
trailing=value # with comment
`
	if err := os.WriteFile(envPath, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runVault(t, "", "import", "--namespace=test", envPath)
	if code != 0 {
		t.Fatalf("import: %s", errOut)
	}
	for _, k := range []string{"FOO", "QUOTED", "SINGLE", "trailing"} {
		if !strings.Contains(out, "imported "+k) {
			t.Errorf("missing import of %s in %q", k, out)
		}
	}
	// EMPTY_KEY has an empty value; the keyring still accepts it but
	// it is a weak default — verify we did not crash. The alias may
	// or may not be created depending on policy; current behavior is
	// to accept it because parseDotenv yields val="".
	code, out, _ = runVault(t, "", "get", "--reveal", "QUOTED")
	if code != 0 {
		t.Fatal("get QUOTED")
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("get QUOTED expected 'hello world', got %q", out)
	}
}

func TestGrantAndRevoke(t *testing.T) {
	home := testHome(t)
	mustInit(t)

	code, _, errOut := runVault(t, "v\n", "add", "--from-stdin", "github")
	if code != 0 {
		t.Fatalf("add: %s", errOut)
	}
	code, out, errOut := runVault(t, "", "grant",
		"--agent=claude", "--hosts=api.github.com", "--methods=GET,POST", "--ttl=1h", "github")
	if code != 0 {
		t.Fatalf("grant: %s", errOut)
	}
	if !strings.Contains(out, "capability:") || !strings.Contains(out, "expires:") {
		t.Errorf("grant output missing fields: %q", out)
	}

	s, _ := LoadStore(home)
	if len(s.Capabilities) != 1 {
		t.Fatalf("capabilities=%d, want 1", len(s.Capabilities))
	}
	id := s.Capabilities[0].ID
	active := s.ActiveCapabilities(time.Now())
	if len(active) != 1 {
		t.Errorf("active=%d, want 1", len(active))
	}

	// Revoke via short prefix.
	code, _, errOut = runVault(t, "", "revoke", id[:8])
	if code != 0 {
		t.Fatalf("revoke: %s", errOut)
	}
	s, _ = LoadStore(home)
	if !s.Capabilities[0].Revoked {
		t.Error("capability not marked revoked")
	}
	if len(s.ActiveCapabilities(time.Now())) != 0 {
		t.Error("revoked capability still active")
	}
}

func TestCapabilityExpiry(t *testing.T) {
	s := &Store{}
	c, err := s.NewCapability("openai", "agent", nil, nil, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if c.ExpiresAt == "" {
		t.Fatal("expected ExpiresAt to be set")
	}
	time.Sleep(2 * time.Millisecond)
	if len(s.ActiveCapabilities(time.Now())) != 0 {
		t.Error("expired capability still active")
	}
}

func TestStatus(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, out, errOut := runVault(t, "", "status")
	if code != 0 {
		t.Fatalf("status: %s", errOut)
	}
	for _, want := range []string{"backend:", "aliases:", "capabilities:", "locked:"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q in %q", want, out)
		}
	}
}

func TestConfigSetUnsetList(t *testing.T) {
	testHome(t)
	mustInit(t)

	code, _, _ := runVault(t, "", "config", "--set=default-host=api.openai.com")
	if code != 0 {
		t.Fatal("config --set")
	}
	code, out, _ := runVault(t, "", "config")
	if code != 0 {
		t.Fatal("config list")
	}
	if !strings.Contains(out, "default-host=api.openai.com") {
		t.Errorf("missing config in %q", out)
	}
	code, _, _ = runVault(t, "", "config", "--unset=default-host")
	if code != 0 {
		t.Fatal("config --unset")
	}
	code, out, _ = runVault(t, "", "config")
	if code != 0 {
		t.Fatal("config list (after unset)")
	}
	if strings.Contains(out, "default-host") {
		t.Errorf("expected unset, got %q", out)
	}
}

func TestNoSubcommandPrintsUsage(t *testing.T) {
	testHome(t)
	code, _, errOut := runVault(t, "")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(errOut, "Usage: aql vault") {
		t.Errorf("missing usage banner: %q", errOut)
	}
}

func TestUnknownModeErrors(t *testing.T) {
	testHome(t)
	code, _, errOut := runVault(t, "", "frobnicate")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(errOut, "unknown vault mode") {
		t.Errorf("missing unknown-mode message: %q", errOut)
	}
}

func TestRequireStoreFailsBeforeInit(t *testing.T) {
	testHome(t)
	code, _, errOut := runVault(t, "", "status")
	if code != 0 {
		t.Fatalf("status before init: %s", errOut)
	}
	code, _, errOut = runVault(t, "", "list")
	if code == 0 {
		t.Fatal("expected list to fail before init")
	}
	if !strings.Contains(errOut, "not initialized") {
		t.Errorf("missing not-initialized message: %q", errOut)
	}
}

func TestFileKeyringRoundtripDetectsWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	kr := &fileKeyring{dir: dir, pass: "good"}
	if err := kr.Set("a", "alpha"); err != nil {
		t.Fatal(err)
	}
	v, err := kr.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if v != "alpha" {
		t.Errorf("got %q, want alpha", v)
	}
	bad := &fileKeyring{dir: dir, pass: "wrong"}
	if _, err := bad.Get("a"); err == nil {
		t.Error("expected wrong-passphrase error")
	}
}

func TestParseDotenv(t *testing.T) {
	in := `# header
KEY1=plain
KEY2="quoted value"
KEY3='single'
export KEY4=with-export
KEY5=value # trailing
EMPTY=
=missing-key
malformed-line
`
	got := parseDotenv(in)
	want := map[string]string{
		"KEY1":  "plain",
		"KEY2":  "quoted value",
		"KEY3":  "single",
		"KEY4":  "with-export",
		"KEY5":  "value",
		"EMPTY": "",
	}
	if len(got) != len(want) {
		t.Errorf("parseDotenv returned %d entries, want %d: %#v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("parseDotenv[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestValidAlias(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"github", true},
		{"openai.prod", true},
		{"my-key_1", true},
		{"", false},
		{"a b", false},
		{"$pwn", false},
		{strings.Repeat("a", 129), false},
	}
	for _, tt := range tests {
		if got := validAlias(tt.name); got != tt.ok {
			t.Errorf("validAlias(%q) = %v, want %v", tt.name, got, tt.ok)
		}
	}
}

func TestRedactNeverLeaks(t *testing.T) {
	for _, in := range []string{"", "x", "shortkey", "sk-abcdefghijklmnop"} {
		got := redact(in)
		if in != "" && strings.Contains(got, in) {
			t.Errorf("redact(%q) = %q, leaks original", in, got)
		}
	}
}
