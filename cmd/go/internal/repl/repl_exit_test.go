package repl

import (
	"bytes"
	"strings"
	"testing"
)

// `IO.exit N` from a REPL line ends the SESSION — the interactive analogue
// of a program terminating, and the same thing the bare `exit` word does.
// The code is reported rather than silently swallowed: a session's status
// is not any one line's, but an interactive `IO.exit 3` must still be
// distinguishable from `IO.exit 0`.
func TestReplExitEndsTheSession(t *testing.T) {
	in := strings.NewReader("import \"boru:io\"\nIO.exit 3\n1 add 2\n")
	out := &bytes.Buffer{}
	Start(in, out, "")
	if !strings.Contains(out.String(), "exit 3") {
		t.Errorf("the exit was not reported: %q", out.String())
	}
	if strings.Contains(out.String(), "3\n3") || strings.Contains(out.String(), "  3\n") {
		t.Errorf("the session continued past the exit: %q", out.String())
	}
}

// An out-of-range code is an ordinary error: the session survives it and
// the next line still runs.
func TestReplExitRangeRefusedKeepsSession(t *testing.T) {
	in := strings.NewReader("import \"boru:io\"\nIO.exit 200\n1 add 2\n")
	out := &bytes.Buffer{}
	Start(in, out, "")
	if !strings.Contains(out.String(), "0..125") {
		t.Errorf("the range refusal was not reported: %q", out.String())
	}
	if !strings.Contains(out.String(), "3") {
		t.Errorf("the session did not survive the refusal: %q", out.String())
	}
}
