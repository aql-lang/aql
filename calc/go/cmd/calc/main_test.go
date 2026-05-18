package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunEFlag(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	code := run([]string{"-e", "add 2 3"}, strings.NewReader(""), out, errOut)
	if code != 0 {
		t.Fatalf("run exit code = %d, want 0; stderr=%q", code, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != "5" {
		t.Errorf("run -e output = %q, want %q", got, "5")
	}
}

func TestRunPositional(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	code := run([]string{"10", "sub", "3"}, strings.NewReader(""), out, errOut)
	if code != 0 {
		t.Fatalf("run exit code = %d, want 0", code)
	}
	if got := strings.TrimSpace(out.String()); got != "7" {
		t.Errorf("run positional output = %q, want %q", got, "7")
	}
}

func TestRunREPL(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	code := run(nil, strings.NewReader(":quit\n"), out, errOut)
	if code != 0 {
		t.Fatalf("run exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "calc> ") {
		t.Errorf("REPL should write a prompt: %q", out.String())
	}
}

func TestRunEvalError(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	code := run([]string{"-e", "frobnicate"}, strings.NewReader(""), out, errOut)
	if code != 1 {
		t.Errorf("run undefined-word exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "calc:") {
		t.Errorf("expected error on stderr, got %q", errOut.String())
	}
}

func TestRunBadFlag(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	code := run([]string{"-no-such-flag"}, strings.NewReader(""), out, errOut)
	if code != 2 {
		t.Errorf("bad flag exit code = %d, want 2", code)
	}
}
