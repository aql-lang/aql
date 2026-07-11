package run

import (
	"strings"
	"testing"
)

// The -compile-report flag (design/RUNTIME-STAMPING.0.md Phase 5): a
// compiled-mode run that stamps a runtime-constructed callback prints its
// attribution to stderr; without the flag nothing prints; under -no-compile
// the header line explains the empty report.
func TestExecuteCompileReport(t *testing.T) {
	// An UNCOMPILABLE top level (the mid-expression fn-value apply) forces
	// RunCompiled's armed interpreter fallback — the compile-time bake never
	// sees the `add`, so the STORE-SITE stamp fires and records. (A fully
	// compiled program pre-stamps its handlers; the skip is deliberately
	// unrecorded — first stamp wins.)
	src := `def m {f: ([y:Integer] => [y add 1])} add 1 ((m get "f") 5) drop def svc (service {}) add {cmd:"X"} ([req:Map state:Any] => [ 42 ]) svc (call {cmd:"X"} svc)`

	var stdout, stderr strings.Builder
	if code := Execute([]string{"-compile-report", "-e", src}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "42") {
		t.Fatalf("run result missing: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "compile-report: stamped") {
		t.Fatalf("expected a stamped attribution line, got: %q", stderr.String())
	}

	// Without the flag: no report lines.
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"-e", src}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stderr.String(), "compile-report") {
		t.Fatalf("report printed without the flag: %q", stderr.String())
	}

	// -no-compile: never armed → the explanatory header.
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"-no-compile", "-compile-report", "-e", src}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr.String(), "no runtime-stamp attempts") {
		t.Fatalf("expected the empty-report header, got: %q", stderr.String())
	}

	// A refusal (capturing handler) prints its reason.
	stdout.Reset()
	stderr.Reset()
	capSrc := `def m {f: ([y:Integer] => [y add 1])} add 1 ((m get "f") 5) drop def mk (fn [[n:Integer] [Any] [ def svc (service {}) add {cmd:"N"} ([req:Map state:Any] => [ n ]) svc svc ]]) def s (mk 7) (call {cmd:"N"} s)`
	if code := Execute([]string{"-compile-report", "-e", capSrc}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "compile-report: refused") ||
		!strings.Contains(stderr.String(), "lexical captures") {
		t.Fatalf("expected a capture-refusal line, got: %q", stderr.String())
	}
}
