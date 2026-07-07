// Tests for the `aql repl` subcommand wrapper (cmd.go) plus the
// remaining meta.go edges the existing meta tests do not reach.

package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdMetadata(t *testing.T) {
	c := New()
	if c.Name() != "repl" {
		t.Errorf("Name() = %q, want repl", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() should not be empty")
	}
}

func TestCmdRunRejectsBadFlag(t *testing.T) {
	var out, stderr bytes.Buffer
	code := New().Run([]string{"-definitely-not-a-flag"}, strings.NewReader(""), &out, &stderr)
	if code != 1 {
		t.Errorf("Run with bad flag = %d, want 1", code)
	}
}

func TestCmdRunPrintsBannerAndEvaluates(t *testing.T) {
	var out, stderr bytes.Buffer
	code := New().Run(nil, strings.NewReader("1 add 2\n"), &out, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "aql repl") {
		t.Errorf("output = %q, want the aql repl banner", out.String())
	}
	if !strings.Contains(out.String(), "3") {
		t.Errorf("output = %q, want the evaluated result 3", out.String())
	}
}

func TestCmdRunAcceptsRegistryFlag(t *testing.T) {
	var out, stderr bytes.Buffer
	code := New().Run([]string{"-r", t.TempDir()}, strings.NewReader(""), &out, &stderr)
	if code != 0 {
		t.Errorf("Run with -r = %d, want 0", code)
	}
}

// --- meta.go edges ---

func TestParseAndRunReportsArgParseError(t *testing.T) {
	mr := NewMetaRegistry()
	ctx := &MetaContext{Out: &bytes.Buffer{}}
	// An unterminated string is one of the few inputs jsonic rejects.
	handled, err := mr.ParseAndRun(`/stack "unterminated`, ctx)
	if !handled {
		t.Fatal("a /-prefixed line must be handled even when its args fail to parse")
	}
	if err == nil {
		t.Fatal("expected an arg parse error")
	}
	if !strings.Contains(err.Error(), "/stack") {
		t.Errorf("error = %q, want it to name the /stack command", err)
	}
}

func TestToInt(t *testing.T) {
	tests := []struct {
		name   string
		in     any
		want   int
		wantOK bool
	}{
		{"float64 whole", float64(4), 4, true},
		{"float64 fractional", 2.5, 0, false},
		{"int", 7, 7, true},
		{"string", "x", 0, false},
		{"nil", nil, 0, false},
	}
	for _, tc := range tests {
		got, ok := toInt(tc.in)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("%s: toInt(%v) = (%d, %v), want (%d, %v)", tc.name, tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}
