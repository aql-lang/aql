package do

import (
	"bytes"
	"strings"
	"testing"
)

// `boru do` drives a program just as `boru run` does, so an expression that
// asks to exit sets this process's status rather than being reported as a
// failure. Without this the shell-friendly form would be the one spelling
// of a BORU program that cannot choose its own exit code.
func TestDoExitCodeIsProcessStatus(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`import "boru:io" IO.exit 0`, 0},
		{`import "boru:io" IO.exit 3`, 3},
		{`import "boru:io" IO.exit 125`, 125},
	} {
		var out, errb bytes.Buffer
		got := New().Run([]string{tc.src}, strings.NewReader(""), &out, &errb)
		if got != tc.want {
			t.Errorf("%s: exit status %d, want %d (stderr: %s)", tc.src, got, tc.want, errb.String())
		}
		if errb.Len() != 0 {
			t.Errorf("%s: printed to stderr: %q", tc.src, errb.String())
		}
	}
}

// An out-of-range code is a refusal, not an exit — status 1 with the range
// explained, exactly as `boru run` reports it.
func TestDoExitRangeRefused(t *testing.T) {
	var out, errb bytes.Buffer
	if code := New().Run([]string{`import "boru:io" IO.exit 200`}, strings.NewReader(""), &out, &errb); code != 1 {
		t.Fatalf("exit status %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "0..125") {
		t.Errorf("stderr does not explain the range: %q", errb.String())
	}
}
