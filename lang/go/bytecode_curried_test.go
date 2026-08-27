package lang

import (
	"fmt"
	"strings"
	"testing"
)

// REFUSAL-CLOSURE §9.2d (landed 2026-07-17) — a factory body RETURNING a
// nameless verbose-`fn` construction compiles exactly like the lambda form:
// tryReturnedClosure's model now admits any NAMELESS fn value (`fn [...]`
// in a body position constructs Anonymous=false but carries no name; a
// NAMED value keeps the decline — registry dispatch/recursion semantics).
// The returned closure compiles to its own unit with the factory's param
// riding as a capture (§7a's machinery), and the outer apply runs it
// VM-native.
func TestCurriedFactoryCompiles(t *testing.T) {
	// The §9.2 fixture (verbose form) and its lambda twin.
	//
	// REGRESSED TO A SOUND REFUSAL 2026-08-27 (NUR101, PAREN-RESTEP-RULE.0.md).
	// These compiled to [3] — the right answer — but by the WRONG mechanism:
	// the outer paren collapses, the pair reaches the PROGRAM residual, and
	// resolveDynamicApply's carrier arm applies it there. That arm cannot tell
	// `((mk 1) 2)` from `(mk 1) 2`, whose residual is identical and which the
	// interpreter PLACES as `fn (Integer) 2`; applying both is how `(mk 1) 2`
	// and `(mk2 5) 10` came to be silent miscompiles. Refusing both is the only
	// sound reading available until the apply is recorded AT the paren.
	//
	// GRADUATION: Stage 3's universal fn values + Apply kernel. Today
	// DynApplyLeadEligible declines an EVENT lead, so parenLeadFnApplyIdx
	// cannot claim this window; when it can, these become native applies again
	// and the rows below go back to mustCompileWithParity with "[3]".
	for _, src := range []string{
		`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] ((mk 1) 2)`,
		`def mk fn [[a:Integer] [Function] [([b:Integer] => [a add b])]] ((mk 1) 2)`,
		`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] (((mk 1)) 2)`,
	} {
		nur101Refusal(t, src, "[3]")
	}

	// Decline fences, each parity-faithful:
	// THREE-level currying (a capture threading through two constructions)
	// keeps the sound refusal.
	{
		src := `def mk3 fn [[a:Integer] [Function] [(fn [[b:Integer] [Function] [(fn [[c:Integer] [Integer] [a add b add c]])]])]] (((mk3 1) 2) 3)`
		a, _ := New()
		prog, _, _, _ := a.CompileCheck(src)
		if prog != nil {
			t.Errorf("three-level currying compiled — graduate this fence to a parity row")
		}
		b, _ := New()
		_, _, errC := b.RunCompiled(src)
		if !strings.Contains(fmt.Sprint(errC), "compile_refused") {
			t.Errorf("three-level currying: err=%v, want compile_refused", errC)
		}
		c, _ := New()
		if out, err := c.RunInterp(src); err != nil || fmt.Sprint(out) != "[6]" {
			t.Errorf("interp = %v (%v), want [6]", out, err)
		}
	}
	// A def-bound returned closure applies with parity through the fallback
	// (the def consumer is a different seam; the value is what matters).
	{
		src := `def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] def f (mk 10) (f 5)`
		b, _ := New()
		gotC, _, errC := b.RunCompiled(src)
		c, _ := New()
		gotI, errI := c.RunInterp(src)
		if errC != nil || errI != nil || fmt.Sprint(gotC) != "[15]" || fmt.Sprint(gotI) != "[15]" {
			t.Errorf("def-bound closure: compiled=%v (%v) interp=%v (%v), want [15]", gotC, errC, gotI, errI)
		}
	}
	// A QUOTED returned fn value stays INERT: lowering it to a closure would
	// drop the Quoted flag and the VM would auto-apply a value the
	// interpreter keeps as data (PR #279 review: compiled [3] vs interp
	// [fn (Integer) 2]).
	//
	// REGRESSED TO A SOUND REFUSAL 2026-08-27 with the rest of the
	// paren-bounded carrier family (NUR101). The quoted-ness lives on the fn
	// value the factory returns, not on the CARRIER the residual leads with,
	// so resolveDynamicApply cannot see it and declines the placed lead like
	// any other. It compiled correctly before only because the apply it
	// emitted was a runtime no-op (isAppliableFn leaves a quoted value as
	// data) — the right answer from an instruction that should not have been
	// there. Graduates with the rest of the family.
	nur101Refusal(t,
		`def mk fn [[] [Function] [quote (fn [[b:Integer] [Integer] [b add 1]])]] ((mk) 2)`,
		"[fn (Integer) 2]")
}
