package vault

import (
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// requireSh skips the test if `sh` is not on PATH. Child-process
// tests need a small shell to inspect their environment; the rest
// of the vault test suite is platform-independent so we don't
// require sh package-wide.
func requireSh(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh not available: %s", err)
	}
	return p
}

func TestExecInjectsSecretAsEnvVar(t *testing.T) {
	requireSh(t)
	testHome(t)
	mustInit(t)
	code, _, errOut := runVault(t, "ghp-secret-value\n", "add", "--from-stdin", "github_token")
	if code != 0 {
		t.Fatalf("add: %s", errOut)
	}

	code, out, errOut := runVault(t, "",
		"exec", "github_token", "--", "sh", "-c", "printf %s \"$github_token\"")
	if code != 0 {
		t.Fatalf("exec: %s", errOut)
	}
	if out != "ghp-secret-value" {
		t.Errorf("child env mismatch: got %q, want %q", out, "ghp-secret-value")
	}
}

func TestExecRenameAndUpper(t *testing.T) {
	requireSh(t)
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v1\n", "add", "--from-stdin", "github_token"); code != 0 {
		t.Fatalf("add github_token: %s", e)
	}
	if code, _, e := runVault(t, "v2\n", "add", "--from-stdin", "openai"); code != 0 {
		t.Fatalf("add openai: %s", e)
	}

	// Explicit remap + alias=ENV form combined with --upper for the
	// unmapped alias. Print both vars to confirm both are present.
	code, out, errOut := runVault(t, "",
		"exec", "--upper", "openai,github_token=GH_TOK", "--",
		"sh", "-c", "printf %s:%s \"$OPENAI\" \"$GH_TOK\"")
	if code != 0 {
		t.Fatalf("exec: %s", errOut)
	}
	if out != "v2:v1" {
		t.Errorf("output=%q, want %q", out, "v2:v1")
	}
}

func TestExecPrefix(t *testing.T) {
	requireSh(t)
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "abc\n", "add", "--from-stdin", "api_key"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	code, out, errOut := runVault(t, "",
		"exec", "--prefix=APP_", "--upper", "api_key", "--",
		"sh", "-c", "printf %s \"$APP_API_KEY\"")
	if code != 0 {
		t.Fatalf("exec: %s", errOut)
	}
	if out != "abc" {
		t.Errorf("output=%q, want %q", out, "abc")
	}
}

func TestExecPropagatesExitCode(t *testing.T) {
	requireSh(t)
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	code, _, _ := runVault(t, "", "exec", "k", "--", "sh", "-c", "exit 7")
	if code != 7 {
		t.Errorf("exit code propagation: got %d, want 7", code)
	}
}

func TestExecRequiresSeparator(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	// No `--` separator: should refuse rather than treat "sh" as another flag.
	code, _, errOut := runVault(t, "", "exec", "k", "sh", "-c", "true")
	if code == 0 {
		t.Fatal("expected non-zero exit when `--` separator is missing")
	}
	if !strings.Contains(errOut, "missing command") {
		t.Errorf("missing-command error not in %q", errOut)
	}
}

func TestExecRefusesMissingAlias(t *testing.T) {
	testHome(t)
	mustInit(t)
	code, _, errOut := runVault(t, "", "exec", "nope", "--", "sh", "-c", "true")
	if code == 0 {
		t.Fatal("expected non-zero exit for missing alias")
	}
	if !strings.Contains(errOut, "no alias") {
		t.Errorf("missing alias error not in %q", errOut)
	}
}

func TestExecRefusesInvalidEnvName(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	// Digit-leading env name is not a valid POSIX identifier.
	code, _, errOut := runVault(t, "", "exec", "k=1BAD", "--", "sh", "-c", "true")
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid env name")
	}
	if !strings.Contains(errOut, "invalid env name") {
		t.Errorf("invalid-env-name error not in %q", errOut)
	}
}

func TestExecBlockedWhenLocked(t *testing.T) {
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	if code, _, _ := runVault(t, "", "lock"); code != 0 {
		t.Fatal("lock")
	}
	code, _, errOut := runVault(t, "", "exec", "k", "--", "sh", "-c", "true")
	if code == 0 {
		t.Fatal("expected non-zero exit while locked")
	}
	if !strings.Contains(errOut, "locked") {
		t.Errorf("locked error not in %q", errOut)
	}
}

func TestExecAuditsWithoutValue(t *testing.T) {
	requireSh(t)
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "super-secret-value\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	if code, _, e := runVault(t, "", "exec", "k", "--", "sh", "-c", "true"); code != 0 {
		t.Fatalf("exec: %s", e)
	}
	events, err := ReadAudit(home)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, ev := range events {
		if ev.Action != "vault.exec" {
			continue
		}
		found = true
		if ev.Alias != "k" {
			t.Errorf("audit alias=%q, want %q", ev.Alias, "k")
		}
		if strings.Contains(ev.Reason, "super-secret-value") {
			t.Errorf("audit leaked secret value in reason=%q", ev.Reason)
		}
	}
	if !found {
		t.Errorf("no vault.exec audit event recorded; got %d events", len(events))
	}
}

func TestExecClearEnvDropsAmbient(t *testing.T) {
	requireSh(t)
	testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add: %s", e)
	}
	// Set a non-essential ambient var; with --clear-env it must
	// NOT appear in the child's environment.
	t.Setenv("SOME_RANDOM_AMBIENT_VAR_XYZ", "leaked")
	code, out, errOut := runVault(t, "",
		"exec", "--clear-env", "k", "--",
		"sh", "-c", "printf %s \"${SOME_RANDOM_AMBIENT_VAR_XYZ:-CLEARED}\"")
	if code != 0 {
		t.Fatalf("exec: %s", errOut)
	}
	if out != "CLEARED" {
		t.Errorf("--clear-env did not drop ambient var: got %q", out)
	}
}

func TestParseExecAliases(t *testing.T) {
	type want struct {
		alias, env string
	}
	cases := []struct {
		name    string
		spec    string
		prefix  string
		upper   bool
		want    []want
		wantErr string
	}{
		{name: "single bare", spec: "github", want: []want{{"github", "github"}}},
		{name: "remap", spec: "github=GH", want: []want{{"github", "GH"}}},
		{name: "upper", spec: "github", upper: true, want: []want{{"github", "GITHUB"}}},
		{name: "prefix", spec: "k", prefix: "APP_", upper: true, want: []want{{"k", "APP_K"}}},
		{name: "multiple", spec: "a,b=B,c", upper: true, want: []want{{"a", "A"}, {"b", "B"}, {"c", "C"}}},
		{name: "explicit beats upper", spec: "github=gh_tok", upper: true, want: []want{{"github", "gh_tok"}}},
		{name: "empty", spec: "", wantErr: "no aliases"},
		{name: "invalid alias", spec: "bad alias", wantErr: "invalid alias"},
		{name: "invalid env", spec: "ok=1bad", wantErr: "invalid env name"},
		{name: "dup env", spec: "a=X,b=X", wantErr: "duplicate env name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseExecAliases(tc.spec, tc.prefix, tc.upper)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error=%q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			var gotPairs []want
			for _, m := range got {
				gotPairs = append(gotPairs, want{m.alias, m.envName})
			}
			if !reflect.DeepEqual(gotPairs, tc.want) {
				t.Errorf("got %+v, want %+v", gotPairs, tc.want)
			}
		})
	}
}

func TestBuildExecEnvOverridesCollisions(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("FOO", "from-parent")
	t.Setenv("KEEP", "kept")
	env := buildExecEnv(false, map[string]string{"FOO": "from-vault"})
	got := map[string]string{}
	for _, e := range env {
		if eq := strings.IndexByte(e, '='); eq >= 0 {
			got[e[:eq]] = e[eq+1:]
		}
	}
	if got["FOO"] != "from-vault" {
		t.Errorf("FOO=%q, want %q (vault must shadow parent)", got["FOO"], "from-vault")
	}
	if got["KEEP"] != "kept" {
		t.Errorf("KEEP=%q, want %q (non-colliding ambient vars survive)", got["KEEP"], "kept")
	}
	// Ensure no duplicate FOO= entries leak through.
	var foos []string
	for _, e := range env {
		if strings.HasPrefix(e, "FOO=") {
			foos = append(foos, e)
		}
	}
	sort.Strings(foos)
	if !reflect.DeepEqual(foos, []string{"FOO=from-vault"}) {
		t.Errorf("FOO entries=%v, want exactly [FOO=from-vault]", foos)
	}
}

func TestValidEnvName(t *testing.T) {
	good := []string{"FOO", "foo", "_FOO", "FOO_BAR", "f1", "_1"}
	bad := []string{"", "1FOO", "FOO-BAR", "FOO BAR", "FOO=BAR", "FOO.BAR"}
	for _, s := range good {
		if !validEnvName(s) {
			t.Errorf("validEnvName(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if validEnvName(s) {
			t.Errorf("validEnvName(%q) = true, want false", s)
		}
	}
}
