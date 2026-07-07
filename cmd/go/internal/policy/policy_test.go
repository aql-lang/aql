package policy

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runPolicy invokes the subcommand the way main does.
func runPolicy(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := New().Run(args, nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// writeProfile drops a profile file into a temp dir and returns its path.
func writeProfile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prof.jsonic")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validProfile = `{
  version: 1
  name: "testprof"
  scopes: {
    fileops: {
      words: {
        default: "deny"
        rules: [ { allow: ["read"] } ]
      }
    }
  }
}`

func TestNameAndSynopsis(t *testing.T) {
	c := New()
	if c.Name() != "policy" {
		t.Errorf("Name = %q, want policy", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis should not be empty")
	}
}

func TestRunNoArgs(t *testing.T) {
	code, _, errOut := runPolicy(t)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "Usage: aql policy") {
		t.Errorf("stderr should show usage; got %q", errOut)
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		code, out, _ := runPolicy(t, arg)
		if code != 0 {
			t.Errorf("%s: exit = %d, want 0", arg, code)
		}
		if !strings.Contains(out, "Usage: aql policy") {
			t.Errorf("%s: stdout should show usage; got %q", arg, out)
		}
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	code, _, errOut := runPolicy(t, "frobnicate")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "unknown subcommand: frobnicate") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestListPrintsBuiltins(t *testing.T) {
	code, out, _ := runPolicy(t, "list")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	lines := strings.Fields(out)
	if len(lines) == 0 || lines[0] != "full" {
		t.Errorf("list should start with full (most permissive); got %v", lines)
	}
	for _, want := range []string{"full", "trusted", "sandbox"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q:\n%s", want, out)
		}
	}
}

func TestShowBuiltinProfile(t *testing.T) {
	code, out, errOut := runPolicy(t, "show", "sandbox")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	var view map[string]any
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("show output is not JSON: %v\n%s", err, out)
	}
	if view["name"] != "sandbox" {
		t.Errorf("name = %v, want sandbox", view["name"])
	}
	scopes, ok := view["scopes"].(map[string]any)
	if !ok {
		t.Fatalf("scopes missing: %v", view)
	}
	for _, want := range []string{"fileops", "global", "network"} {
		if _, ok := scopes[want]; !ok {
			t.Errorf("scopes missing %q", want)
		}
	}
}

func TestShowUserProfileFromFile(t *testing.T) {
	path := writeProfile(t, validProfile)
	code, out, errOut := runPolicy(t, "show", path)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "testprof") {
		t.Errorf("show should report the profile name; got %q", out)
	}
}

func TestShowMissingArg(t *testing.T) {
	code, _, errOut := runPolicy(t, "show")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "profile name or path required") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestShowUnknownName(t *testing.T) {
	code, _, errOut := runPolicy(t, "show", "no-such-profile")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "show:") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestValidateOK(t *testing.T) {
	path := writeProfile(t, validProfile)
	code, out, errOut := runPolicy(t, "validate", path)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("stdout = %q, want ok", out)
	}
}

func TestValidateMissingArg(t *testing.T) {
	code, _, errOut := runPolicy(t, "validate")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "file path required") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestValidateMissingFile(t *testing.T) {
	code, _, errOut := runPolicy(t, "validate", filepath.Join(t.TempDir(), "nope.jsonic"))
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if errOut == "" {
		t.Error("stderr should report the read failure")
	}
}

func TestValidateRejectsUnknownScope(t *testing.T) {
	path := writeProfile(t, `{
  version: 1
  name: "bad"
  scopes: { frobnicate: { words: { default: "allow" } } }
}`)
	code, _, errOut := runPolicy(t, "validate", path)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "frobnicate") {
		t.Errorf("stderr should name the bad scope; got %q", errOut)
	}
}

func TestValidateRejectsMalformedProfile(t *testing.T) {
	// jsonic is lenient about syntax, but the profile decoder rejects
	// unknown fields.
	path := writeProfile(t, `{ version: 1, name: "x", frobnicate: true }`)
	code, _, errOut := runPolicy(t, "validate", path)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if errOut == "" {
		t.Error("stderr should report the parse failure")
	}
}

func TestTestAllowed(t *testing.T) {
	code, out, errOut := runPolicy(t, "test", "full", "fileops.read", "path=/tmp/x")
	if code != 0 {
		t.Errorf("exit = %d, want 0 (allow); stderr = %q", code, errOut)
	}
	if out != "" {
		t.Errorf("test (non-verbose) should print nothing; got %q", out)
	}
}

func TestTestDenied(t *testing.T) {
	code, _, _ := runPolicy(t, "test", "sandbox", "fileops.write", "path=/etc/passwd")
	if code != 1 {
		t.Errorf("exit = %d, want 1 (deny)", code)
	}
}

func TestTestUsageErrors(t *testing.T) {
	cases := [][]string{
		{"test"},                                   // no args
		{"test", "full"},                           // missing scope.op
		{"test", "full", "fileopswrite"},           // no dot
		{"test", "full", ".write"},                 // empty scope
		{"test", "full", "fileops."},               // empty op
		{"test", "full", "fileops.write", "pathx"}, // bad k=v
		{"test", "no-such-profile", "fileops.write"},
	}
	for _, args := range cases {
		code, _, errOut := runPolicy(t, args...)
		if code != 1 {
			t.Errorf("%v: exit = %d, want 1", args, code)
		}
		if errOut == "" {
			t.Errorf("%v: stderr should explain the failure", args)
		}
	}
}

func TestExplainAllow(t *testing.T) {
	code, out, _ := runPolicy(t, "explain", "full", "fileops.read", "path=/tmp/x")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, want := range []string{"profile:  full", "scope:    fileops", "op:       read", "decision: ALLOW"} {
		if !strings.Contains(out, want) {
			t.Errorf("explain output missing %q:\n%s", want, out)
		}
	}
}

func TestExplainDeny(t *testing.T) {
	code, out, _ := runPolicy(t, "explain", "sandbox", "fileops.write", "path=/etc/passwd")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	for _, want := range []string{"decision: DENY", "code:", "blame:"} {
		if !strings.Contains(out, want) {
			t.Errorf("explain output missing %q:\n%s", want, out)
		}
	}
}

func TestSplitScopeOp(t *testing.T) {
	cases := []struct {
		raw       string
		scope, op string
		wantErr   bool
	}{
		{"fileops.write", "fileops", "write", false},
		{"modules.aql:math.sin", "modules", "aql:math.sin", false},
		{"nodot", "", "", true},
		{".op", "", "", true},
		{"scope.", "", "", true},
		{"", "", "", true},
	}
	for _, c := range cases {
		scope, op, err := splitScopeOp(c.raw)
		if (err != nil) != c.wantErr {
			t.Errorf("splitScopeOp(%q) err = %v, wantErr %v", c.raw, err, c.wantErr)
			continue
		}
		if scope != c.scope || op != c.op {
			t.Errorf("splitScopeOp(%q) = (%q, %q), want (%q, %q)", c.raw, scope, op, c.scope, c.op)
		}
	}
}
