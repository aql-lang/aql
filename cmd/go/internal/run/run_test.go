package run

import (
	"bytes"
	"strings"
	"testing"
)

func TestEvalSuccess(t *testing.T) {
	var buf bytes.Buffer
	err := Eval(&buf, "1 add 2", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "3") {
		t.Errorf("expected '3' in output, got %q", buf.String())
	}
}

func TestEvalParseError(t *testing.T) {
	var buf bytes.Buffer
	err := Eval(&buf, `"unterminated`, "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse error") {
		t.Errorf("expected 'parse error', got %q", err.Error())
	}
}

func TestEvalEngineError(t *testing.T) {
	var buf bytes.Buffer
	err := Eval(&buf, "10 div 0", "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

// resolveCompileMode applies the bytecode rollout contract: the
// `--compile` flag OR AQL_COMPILE enables compiled mode, and
// AQL_NO_COMPILE is the kill switch that wins over both.
func TestResolveCompileMode(t *testing.T) {
	cases := []struct {
		flag               bool
		compile, noCompile string
		want               bool
	}{
		{false, "", "", false},    // default: interpreter
		{true, "", "", true},      // --compile flag
		{false, "1", "", true},    // AQL_COMPILE=1
		{false, "true", "", true}, // AQL_COMPILE=true
		{false, "0", "", false},   // AQL_COMPILE=0 is off
		{false, "no", "", false},  // AQL_COMPILE=no is off
		{true, "", "1", false},    // AQL_NO_COMPILE wins over the flag
		{false, "1", "1", false},  // AQL_NO_COMPILE wins over AQL_COMPILE
		{true, "", "0", true},     // AQL_NO_COMPILE=0 does not disable
	}
	for _, c := range cases {
		t.Setenv("AQL_COMPILE", c.compile)
		t.Setenv("AQL_NO_COMPILE", c.noCompile)
		if got := resolveCompileMode(c.flag); got != c.want {
			t.Errorf("resolveCompileMode(flag=%v, AQL_COMPILE=%q, AQL_NO_COMPILE=%q) = %v, want %v",
				c.flag, c.compile, c.noCompile, got, c.want)
		}
	}
}
