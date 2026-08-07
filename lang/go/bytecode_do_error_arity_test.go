package lang

import (
	"fmt"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// bytecode_do_error_arity_test.go pins the §8.2(6) ZERO-NETTING handler
// graduation (completeness-review §9.13): a `do … error` whose body PROVABLY
// raises (a strict Error do-result — the pass-through arm is statically
// dead) and whose handler nets NO value now compiles natively — the
// dispatch records a 0-output call (errorReturnsFn returns the true zero
// arity) and the strip-input shape screen admits the empty residual
// (stripResidualShapeOK's want-0 arm).
func TestZeroNettingHandlerCompiles(t *testing.T) {
	fnValueM2Native(t, "the ledgered frontier row",
		`do [1 div 0] error [drop] end 2 add 3`,
		"[5]")
	fnValueM2Native(t, "the 1-netting twin keeps compiling",
		`do [1 div 0] error [drop 9] end 2 add 3`,
		"[9 5]")
}

// TestZeroNettingHandlerDynamicKeepsRefusal pins the edge the graduation
// keeps: a DYNAMIC Error bound (the body may not raise) has variable arity
// — the pass-through nets one where the caught path nets zero — so the
// refusal stands with faithful interpreter fallback. The divisor must be a
// genuinely DYNAMIC read: a def-bound literal zero const-folds into a
// PROVEN raise, which correctly compiles (the case above).
func TestZeroNettingHandlerDynamicKeepsRefusal(t *testing.T) {
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	fnValueM2Refusal(t, "a maybe-raising body with a zero-netting handler",
		`def xs [0] do [1 div (xs 0 getr)] error [drop] end 2 add 3`,
		"")
}

// TestFullStackHostOverloadParity pins the Codex-review claim (PR #327,
// eng/go/emit.go FoldFullStack) that a HOST-registered overload of a
// full-stack word diverges under compilation. It does not: check-mode
// dispatch and the runtime share one dispatch table, so both engines pick
// the host sig and the fold (gated on the DISPATCHED sig being a FullStack
// sig) never fires. Verified non-reproducing 2026-08-03; this pin keeps it
// that way.
func TestFullStackHostOverloadParity(t *testing.T) {
	host := func(a *Boru) {
		a.Register("depth", Signature{
			Args:       []*core.Type{core.TInteger},
			Returns:    []*core.Type{core.TInteger, core.TInteger},
			BarrierPos: 0,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{args[0], core.NewInteger(99)}, nil
			}),
		})
	}
	src := `10 1 depth`
	a, _ := New()
	host(a)
	gotI, errI := a.RunInterp(src)
	c, _ := New()
	host(c)
	gotC, _, errC := c.RunCompiled(src)
	if errI != nil || errC != nil || fmt.Sprint(gotI) != fmt.Sprint(gotC) {
		t.Errorf("host-overload depth diverged: interp=%v/%v compiled=%v/%v", gotI, errI, gotC, errC)
	}
	if fmt.Sprint(gotI) != "[10 1 99]" {
		t.Errorf("host-overload depth = %v, want [10 1 99] (the host sig owns the dispatch)", gotI)
	}
}

// TestLeadApplyNoMatchTwoReturnParity pins the Codex-review claim (PR #327,
// engine.go leading apply) that a type-mismatched 1-arg callee under a
// TWO-value declared return diverges. It does not: the interpreter's
// no-match leaves [fn, arg] as data — exactly what the compiled trailing
// event nets — and the 2-value return contract accepts it on both engines.
// Verified non-reproducing 2026-08-03; this pin keeps it that way.
func TestLeadApplyNoMatchTwoReturnParity(t *testing.T) {
	fnValueM2Native(t, "type-mismatched 1-arg callee, 2-value declared return",
		`def ld fn [[g:Function x:Integer] [Function Integer] [(g x)]] ld ([k:String] => [k]) 14`,
		"[fn (String) 14]")
}
