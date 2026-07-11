package lang

import (
	"strings"
	"testing"
)

// TestRunCompiledReason pins the third return of RunCompiledReason: the
// whole-program compilation-refusal reason the CLI surfaces as a performance
// warning. A reason is reported ONLY for a genuine refusal (a valid program the
// compiler cannot lower, which silently falls back to the slower interpreter);
// it is EMPTY for a compiled run and for a statically-invalid program (which
// fails in both engines and so is not a performance fallback).
func TestRunCompiledReason(t *testing.T) {
	// POSITIVE — a compiled program reports ran=true and no reason.
	t.Run("compiled", func(t *testing.T) {
		a, _ := New()
		_, ran, reason, err := a.RunCompiledReason("1 add 2")
		if !ran || reason != "" || err != nil {
			t.Fatalf("compiled program: ran=%v reason=%q err=%v (want ran=true, no reason, no error)", ran, reason, err)
		}
	})

	// POSITIVE — a genuine whole-program refusal reports ran=false and names the
	// first offending construct. `5 inc apply` refuses ("unmatched dispatch
	// recovered at apply") and falls back to the interpreter, which raises.
	t.Run("refusal names the offender", func(t *testing.T) {
		const src = `def inc fn [[n:Integer][Integer][n add 1]]  5 inc apply`
		a, _ := New()
		_, ran, reason, _ := a.RunCompiledReason(src)
		if ran {
			t.Fatalf("expected the interpreter fallback (ran=false), got ran=true")
		}
		if !strings.Contains(reason, "apply") {
			t.Fatalf("refusal reason %q does not name the offending word `apply`", reason)
		}
	})

	// NEGATIVE — a statically-invalid program (a SeverityError check diagnostic:
	// undefined word) reports NO reason. It fails in both engines, so its
	// interpreter fallback is not a performance refusal worth warning about.
	t.Run("check diagnostics report no reason", func(t *testing.T) {
		a, _ := New()
		_, ran, reason, err := a.RunCompiledReason("no_such_word")
		if ran {
			t.Fatalf("a check-error program must fall back (ran=false), got ran=true")
		}
		if reason != "" {
			t.Fatalf("a statically-invalid program must report no refusal reason, got %q", reason)
		}
		if err == nil {
			t.Fatalf("an undefined word must still error on the interpreter fallback")
		}
	})

	// NEGATIVE — a parse error (the err != nil compile path) reports NO reason
	// and surfaces the parse error, exactly as RunCompiled does.
	t.Run("parse error reports no reason", func(t *testing.T) {
		a, _ := New()
		_, ran, reason, err := a.RunCompiledReason("(1 add 2")
		if ran || reason != "" || err == nil {
			t.Fatalf("parse error: ran=%v reason=%q err=%v (want ran=false, no reason, an error)", ran, reason, err)
		}
	})

	// RunCompiled is the reason-free wrapper: its three returns must match
	// RunCompiledReason's (minus the reason) for the same source.
	t.Run("RunCompiled delegates", func(t *testing.T) {
		const src = "1 add 2"
		a, _ := New()
		out, ran, err := a.RunCompiled(src)
		if !ran || err != nil || len(out) != 1 {
			t.Fatalf("RunCompiled(%q): out=%v ran=%v err=%v", src, out, ran, err)
		}
	})
}
