package run

import (
	"strings"
	"testing"
)

// A whole-program compilation refusal silently runs on the slower interpreter
// (design/COMPILABLE-SUBSET.md §1: "slow, not wrong"). The default compile-try
// mode surfaces that as a one-line stderr warning naming the first offending
// construct, so the performance cost is not a surprise. A compiled program, and
// the interpreter (-no-compile) mode, print no warning.
func TestExecuteCompileRefusalWarning(t *testing.T) {
	// A program the compiler refuses to lower ("unmatched dispatch recovered
	// at add" — the wide-window local-add shape; the former `5 inc apply`
	// fixture compiles to a runtime rematch since OpDispatchRematch landed).
	// It also errors at runtime, but the warning fires on the REFUSAL, before
	// the error — so -no-check (skip the pre-flight that would reject it)
	// lets the run reach the compile-try fallback. Exit is non-zero (the
	// runtime error), which is expected here.
	refuses := `def f fn [[x:Boolean] [Boolean] [def add fn [[a:Boolean b:Boolean] [Boolean] [a or b]] add x false]]  (f true) add true false`
	const wantWarn = "warning: bytecode compilation refused"

	var stdout, stderr strings.Builder
	Execute([]string{"-no-check", "-e", refuses}, strings.NewReader(""), &stdout, &stderr)
	if !strings.Contains(stderr.String(), wantWarn) {
		t.Fatalf("expected a refusal warning, got stderr: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "add") {
		t.Fatalf("refusal warning should name the offending construct, got: %q", stderr.String())
	}

	// A compilable program prints no warning.
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"-e", "1 add 2"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "3") {
		t.Fatalf("run result missing: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), wantWarn) {
		t.Fatalf("a compiled program must not warn, got: %q", stderr.String())
	}

	// -no-compile is an explicit opt-out of the bytecode path: no refusal, no
	// warning, even for the program that would otherwise refuse.
	stdout.Reset()
	stderr.Reset()
	Execute([]string{"-no-check", "-no-compile", "-e", refuses}, strings.NewReader(""), &stdout, &stderr)
	if strings.Contains(stderr.String(), wantWarn) {
		t.Fatalf("-no-compile must not warn about a refusal, got: %q", stderr.String())
	}
}
