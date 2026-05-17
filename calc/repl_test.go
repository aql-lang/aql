package calc

import (
	"bytes"
	"strings"
	"testing"
)

// runREPL feeds the supplied script (one expression per line) into the
// REPL and returns whatever the REPL wrote to the output buffer.
func runREPL(t *testing.T, script string) string {
	t.Helper()
	c, _ := newCalc(t)
	in := strings.NewReader(script)
	out := &bytes.Buffer{}
	REPL(c, in, out, "calc> ")
	return out.String()
}

func TestREPLEvalAndShow(t *testing.T) {
	out := runREPL(t, "add 2 3\n:stack\n")
	// After `add 2 3` the REPL prints the stack (5). After `:stack` it
	// prints the stack again. The prompt appears before each input.
	if !strings.Contains(out, "5") {
		t.Errorf("REPL output missing 5: %q", out)
	}
	if strings.Count(out, "calc> ") < 2 {
		t.Errorf("expected at least 2 prompts, got %q", out)
	}
}

func TestREPLStackPersists(t *testing.T) {
	// Two lines: push 1 2; then `add`. The stack carries over.
	out := runREPL(t, "1 2\nadd\n")
	// The REPL prints `1 2` after the first line and `3` after the
	// second. Verify both — the test for carry-over is that "3" appears
	// after the second prompt.
	if !strings.Contains(out, "1 2") {
		t.Errorf("REPL output missing 1 2 line: %q", out)
	}
	if !strings.Contains(out, "3") {
		t.Errorf("REPL output missing 3 (the carry-over add result): %q", out)
	}
}

func TestREPLMetaQuit(t *testing.T) {
	out := runREPL(t, ":quit\nadd 99 99\n")
	// After :quit the REPL should return without evaluating the next line.
	if strings.Contains(out, "198") {
		t.Errorf(":quit should stop the REPL, but the next line ran: %q", out)
	}
}

func TestREPLMetaClear(t *testing.T) {
	out := runREPL(t, "1 2 3\n:clear\n:stack\n")
	if !strings.Contains(out, "(empty)") {
		t.Errorf(":clear didn't empty the stack: %q", out)
	}
}

func TestREPLMetaWords(t *testing.T) {
	out := runREPL(t, ":words\n")
	for _, w := range []string{"add", "sub", "mul", "div", "pi", "print"} {
		if !strings.Contains(out, w) {
			t.Errorf(":words missing %q in output: %q", w, out)
		}
	}
}

func TestREPLMetaHelp(t *testing.T) {
	out := runREPL(t, ":help\n")
	for _, marker := range []string{":stack", ":clear", ":quit", ":help"} {
		if !strings.Contains(out, marker) {
			t.Errorf(":help missing %q: %q", marker, out)
		}
	}
}

func TestREPLUnknownMeta(t *testing.T) {
	out := runREPL(t, ":bogus\n")
	if !strings.Contains(out, "unknown meta-command") {
		t.Errorf("unknown meta should be reported: %q", out)
	}
}

func TestREPLErrorIsRecoverable(t *testing.T) {
	// A bad line should be reported, and the REPL should keep going.
	out := runREPL(t, "frobnicate\nadd 2 3\n")
	if !strings.Contains(out, "error") {
		t.Errorf("REPL should report the error from frobnicate: %q", out)
	}
	if !strings.Contains(out, "5") {
		t.Errorf("REPL should keep going and produce 5: %q", out)
	}
}

func TestREPLEmptyLineIgnored(t *testing.T) {
	out := runREPL(t, "\n\nadd 2 3\n")
	if !strings.Contains(out, "5") {
		t.Errorf("REPL should ignore empty lines and still evaluate: %q", out)
	}
}

func TestREPLDefaultPrompt(t *testing.T) {
	// Passing "" for prompt selects the default "calc> ".
	c, _ := newCalc(t)
	out := &bytes.Buffer{}
	REPL(c, strings.NewReader(""), out, "")
	if !strings.HasPrefix(out.String(), "calc> ") {
		t.Errorf("default prompt missing, got %q", out.String())
	}
}
