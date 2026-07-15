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
	// A program the compiler refuses to lower ("branch leaves extra values"
	// — the variadic-if 1-vs-2 residual; its dispatch half records an
	// offset-form rematch since DispatchSpec.WrittenOff landed). It also
	// errors at runtime, but the warning fires on the REFUSAL, before the
	// error — so -no-check (skip the pre-flight that would reject it) lets
	// the run reach the compile-try fallback. Exit is non-zero (the runtime
	// error), which is expected here.
	refuses := `def n 0 if (n eq 0) [99] [1 2] each [dup mul]`
	const wantWarn = "warning: bytecode compilation refused"

	var stdout, stderr strings.Builder
	Execute([]string{"-no-check", "-e", refuses}, strings.NewReader(""), &stdout, &stderr)
	if !strings.Contains(stderr.String(), wantWarn) {
		t.Fatalf("expected a refusal warning, got stderr: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "branch") {
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
