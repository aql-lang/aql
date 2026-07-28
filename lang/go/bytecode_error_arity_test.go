package lang

import (
	"fmt"
	"strings"
	"testing"
)

// `do […] error [handler]` where the handler leaves the stack EMPTY used to
// be modelled as producing one value. `errorReturnsFn` runs the handler
// during the compile pass, so it can see the real residual — and on
// `len(stk) != 1` it returned the wide one-value `dynamic(Any)` bound, the
// same answer it gives for the cases where it genuinely does not know.
//
// Everything downstream was faithful to that wrong model: the dispatch was
// recorded as a single-output fallback island, `FallbackSpan` has no
// out-count field to say otherwise, `lowerFallback` pushed exactly one
// simulated slot, and at run time `runFallback` appended the island's real
// ZERO results — leaving the stack one short. The phantom slot then read as
// a runtime fn-value, so the failure surfaced as `CALL_DYNAMIC underflow`
// or `BIND_GLOBAL underflow`: an `internal_error` from ordinary source.
//
// The repair is to refuse rather than widen. A known-wrong arity is not an
// unknown one, the island model cannot express any out-count but 1, and
// under the refusal architecture the interpreter fallback is always sound —
// `tryRecordFallback` already declines `len(outs) != 1` for the same reason,
// and its sibling guard explicitly prefers "a clean refusal" to a new
// island.
//
// For most shapes this buys honesty rather than answers: the whole-program
// fallback caught the underflow and silently re-ran, so DEFAULT mode was
// already returning the interpreter's result at the cost of a wasted
// compile and an unreported runtime bail. The exception is the shape that
// matters most — once the guarded block has emitted OUTPUT the fallback is
// deliberately blocked, because re-running would duplicate it, and the
// `internal_error` reaches the user in the default configuration. That is
// the case TestErrorHandlerFencedFallbackIsUserVisible pins.

// TestErrorHandlerZeroResidualRefuses pins the refusal for every shape that
// produced an underflow, and pins that the answer still matches the
// interpreter.
func TestErrorHandlerZeroResidualRefuses(t *testing.T) {
	cases := []struct{ name, src string }{
		// The reported repro: a trailing expression consumes the phantom slot.
		{"trailing expression", `do [1 div 0] error [drop]
2 add 3`},
		// A LEADING value instead — the phantom lands under it.
		{"leading value", `1
do [1 div 0] error [drop]`},
		// Bound with def: the phantom is consumed by BIND_GLOBAL instead.
		{"def-bound", `def x (do [1 div 0] error [drop])
x`},
		// No raise at all: the handler never runs, but the ReturnsFn still
		// measured its residual, so the same model error applies.
		{"handler never fires", `do [1] error [drop]
2 add 3`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog != nil {
				t.Fatalf("must refuse: a zero-residual error handler cannot be "+
					"modelled by the single-output island, but it compiled to:\n%s",
					prog.Disassemble())
			}
			if !strings.Contains(reason, "single-output") {
				t.Errorf("refusal reason %q should name the island's single-output "+
					"limit so the next reader knows which model is short, not which "+
					"word is unsupported", reason)
			}
			// Refusal is only acceptable because the fallback is sound: the
			// program must still run, and agree with the interpreter. Each
			// arm gets a FRESH instance — a CompileCheck pass installs the
			// program's defs, so reusing `a` here would let the check pass's
			// `def x` satisfy the run's lookup and hide the very divergence
			// this asserts.
			ra, _ := New()
			got, err := ra.Run(c.src)
			b, _ := New()
			want, wantErr := b.RunInterp(c.src)
			if fmt.Sprint(err) != fmt.Sprint(wantErr) {
				t.Errorf("default-mode error %v != interpreter %v", err, wantErr)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("default-mode %v != interpreter %v", got, want)
			}
			// And no internal_error may escape under -force-compile: a refusal
			// there is a refusal, not a bytecode fault.
			if _, ferr := a.RunCompiledStrict(c.src); ferr != nil &&
				strings.Contains(fmt.Sprint(ferr), "internal:") {
				t.Errorf("force-compile leaked an internal error: %v", ferr)
			}
		})
	}
}

// TestErrorHandlerFencedFallbackIsUserVisible is the variant that makes this
// a default-path defect rather than a -force-compile curiosity. A `print`
// inside the guarded block emits output before the underflow, so the
// whole-program fallback refuses to rescue the run — re-running would
// duplicate the output — and the internal_error surfaces to the user with
// the engine's own "report this as a compiler bug" note attached.
//
// Post-fix the program must refuse at COMPILE time, which is before any
// output exists, so the interpreter runs it once and cleanly.
func TestErrorHandlerFencedFallbackIsUserVisible(t *testing.T) {
	const src = `do [1 print  1 div 0] error [drop]
2 add 3`
	a, _ := New()
	if prog, _, _, _ := a.CompileCheck(src); prog != nil {
		t.Fatalf("must refuse before any output is emitted, compiled to:\n%s",
			prog.Disassemble())
	}
	ra, _ := New()
	got, err := ra.Run(src)
	if err != nil {
		t.Fatalf("default mode must not surface an error once the compile is "+
			"refused up front (this is the shape the effects fence made "+
			"user-visible): %v", err)
	}
	b, _ := New()
	want, _ := b.RunInterp(src)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("default-mode %v != interpreter %v", got, want)
	}
}

// TestErrorHandlerOneResidualStillCompiles is the negative half: the fix must
// narrow ONLY the arity it cannot model. A handler that nets exactly one
// value is what the island expresses, so it must keep compiling — and keep
// compiling to the same answer.
func TestErrorHandlerOneResidualStillCompiles(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"handler nets one, raise fires", `do [1 div 0] error [drop 9]
2 add 3`, "[9 5]"},
		{"handler nets one, no raise", `do [7] error [drop 9]
2 add 3`, "[7 5]"},
		{"handler nets one, def-bound", `def x (do [1 div 0] error [drop 9])
x`, "[9]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			if prog, reason, _, _ := a.CompileCheck(c.src); prog == nil {
				t.Fatalf("a one-value handler is exactly what the island models; "+
					"it must still compile, refused: %q", reason)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
