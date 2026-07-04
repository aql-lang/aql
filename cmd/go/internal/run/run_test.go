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

// CompileForce must run a compilable program through the VM and surface a
// refusal (not silently fall back) for one the emitter cannot lower.
func TestEvalForceCompile(t *testing.T) {
	// A plain arithmetic program is compilable: force mode runs it on the VM
	// and yields the same result the interpreter would.
	var buf bytes.Buffer
	if err := EvalOptionsMode(&buf, "1 add 2", OptionsFor("", 0, nil), CompileForce); err != nil {
		t.Fatalf("force-compile of a compilable program: %v", err)
	}
	if !strings.Contains(buf.String(), "3") {
		t.Errorf("expected '3', got %q", buf.String())
	}

	// An uncompilable program (a loop result consumed by `size`) must ABORT
	// under force mode, carrying the emitter's refusal reason — not fall back.
	buf.Reset()
	err := EvalOptionsMode(&buf, "(size (for 5 [i]))", OptionsFor("", 0, nil), CompileForce)
	if err == nil {
		t.Fatal("force-compile of an uncompilable program: expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "force-compile") {
		t.Errorf("expected a force-compile refusal, got %q", err.Error())
	}

	// The SAME uncompilable program runs fine under best-effort mode (silent
	// fallback to the interpreter): its output matches a plain interpreter run.
	var tryBuf, interpBuf bytes.Buffer
	if err := EvalOptionsMode(&tryBuf, "(size (for 5 [i]))", OptionsFor("", 0, nil), CompileTry); err != nil {
		t.Fatalf("best-effort compile should fall back and succeed: %v", err)
	}
	if err := EvalOptionsMode(&interpBuf, "(size (for 5 [i]))", OptionsFor("", 0, nil), CompileOff); err != nil {
		t.Fatalf("interpreter run: %v", err)
	}
	if tryBuf.String() != interpBuf.String() {
		t.Errorf("fallback output %q != interpreter output %q", tryBuf.String(), interpBuf.String())
	}
}

// ResolveCompileMode applies the bytecode rollout contract: the `--compile`
// flag OR AQL_COMPILE enables best-effort compiled mode; the `--force-compile`
// flag OR AQL_FORCE_COMPILE demands the bytecode path (winning over the
// best-effort form); and AQL_NO_COMPILE is the kill switch that wins over all.
func TestResolveCompileMode(t *testing.T) {
	cases := []struct {
		compileFlag, forceFlag    bool
		compile, force, noCompile string
		want                      CompileMode
	}{
		{false, false, "", "", "", CompileOff},     // default: interpreter
		{true, false, "", "", "", CompileTry},      // --compile flag
		{false, true, "", "", "", CompileForce},    // --force-compile flag
		{false, false, "1", "", "", CompileTry},    // AQL_COMPILE=1
		{false, false, "true", "", "", CompileTry}, // AQL_COMPILE=true
		{false, false, "0", "", "", CompileOff},    // AQL_COMPILE=0 is off
		{false, false, "no", "", "", CompileOff},   // AQL_COMPILE=no is off
		{false, false, "", "1", "", CompileForce},  // AQL_FORCE_COMPILE=1
		{true, true, "", "", "", CompileForce},     // force wins over try (flags)
		{true, false, "", "1", "", CompileForce},   // force env wins over try flag
		{true, false, "", "", "1", CompileOff},     // AQL_NO_COMPILE wins over the flag
		{false, true, "", "", "1", CompileOff},     // AQL_NO_COMPILE wins over force
		{false, false, "1", "1", "1", CompileOff},  // AQL_NO_COMPILE wins over both envs
		{true, false, "", "", "0", CompileTry},     // AQL_NO_COMPILE=0 does not disable
	}
	for _, c := range cases {
		t.Setenv("AQL_COMPILE", c.compile)
		t.Setenv("AQL_FORCE_COMPILE", c.force)
		t.Setenv("AQL_NO_COMPILE", c.noCompile)
		if got := ResolveCompileMode(c.compileFlag, c.forceFlag); got != c.want {
			t.Errorf("ResolveCompileMode(compile=%v, force=%v, AQL_COMPILE=%q, AQL_FORCE_COMPILE=%q, AQL_NO_COMPILE=%q) = %v, want %v",
				c.compileFlag, c.forceFlag, c.compile, c.force, c.noCompile, got, c.want)
		}
	}
}

// Completion plan Phase 5.2 — check-by-default: every Run pre-flights
// unless --no-check / AQL_NO_CHECK; the default gate is quiet for clean
// programs and verbose only under an explicit --check.
func TestCheckByDefault(t *testing.T) {
	run := func(args ...string) (int, string, string) {
		var out, errB bytes.Buffer
		code := Execute(args, strings.NewReader(""), &out, &errB)
		return code, out.String(), errB.String()
	}

	// A clean program runs with ZERO checker noise on stderr.
	code, out, stderr := run("-e", "add 1 2")
	if code != 0 || !strings.Contains(out, "3") {
		t.Fatalf("clean run: code=%d out=%q", code, out)
	}
	if stderr != "" {
		t.Errorf("clean run stderr = %q; want empty (quiet default gate)", stderr)
	}

	// An Error-severity finding aborts BEFORE execution, diagnostics shown.
	code, _, stderr = run("-e", "flurble 1 2")
	if code == 0 {
		t.Fatal("undefined word ran; want pre-flight abort")
	}
	if !strings.Contains(stderr, "undefined_word") || !strings.Contains(stderr, "check failed") {
		t.Errorf("abort stderr = %q; want the diagnostic + check failed", stderr)
	}

	// --no-check skips the pre-flight: the failure is a RUNTIME error.
	code, _, stderr = run("-no-check", "-e", "flurble 1 2")
	if code == 0 {
		t.Fatal("--no-check run of undefined word succeeded")
	}
	if strings.Contains(stderr, "check failed") {
		t.Errorf("--no-check stderr = %q; want runtime error, not the pre-flight gate", stderr)
	}

	// An advisory-only program (forward-strand info) stays QUIET by default…
	code, _, stderr = run("-e", "1 2 add 3 mul")
	if code != 0 {
		t.Fatalf("advisory-only program aborted: %q", stderr)
	}
	if strings.Contains(stderr, "forward_strands_operand") {
		t.Errorf("default gate printed advisory: %q", stderr)
	}
	// …and --check surfaces it.
	code, _, stderr = run("-check", "-e", "1 2 add 3 mul")
	if code != 0 {
		t.Fatalf("--check advisory run aborted: %q", stderr)
	}
	if !strings.Contains(stderr, "forward_strands_operand") {
		t.Errorf("--check stderr = %q; want the advisory", stderr)
	}
}
