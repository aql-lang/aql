package shrink

import (
	"testing"

	"github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/stackform"
)

// TestShrinkCost_DoEval covers the DoEval arm of ShrinkCost's op
// switch: a DoEval op contributes exactly 1.
func TestShrinkCost_DoEval(t *testing.T) {
	form := &stackform.StackForm{Ops: []stackform.Op{
		stackform.DoEval{},
	}}
	if got := ShrinkCost(form, DefaultPolicy()); got != 1 {
		t.Errorf("ShrinkCost([DoEval]) = %d, want 1", got)
	}
	// Pair with a two-DoEval form to confirm it accumulates.
	form2 := &stackform.StackForm{Ops: []stackform.Op{
		stackform.DoEval{}, stackform.DoEval{},
	}}
	if got := ShrinkCost(form2, DefaultPolicy()); got != 2 {
		t.Errorf("ShrinkCost([DoEval DoEval]) = %d, want 2", got)
	}
}

// TestLiteralComplexity_NilParent covers the v.Parent == nil guard: a
// zero-value Value has complexity 0.
func TestLiteralComplexity_NilParent(t *testing.T) {
	if got := literalComplexity(eng.Value{}); got != 0 {
		t.Errorf("literalComplexity(zero Value) = %d, want 0", got)
	}
}

// TestLiteralComplexity_Float covers the Float arm: complexity is the
// integer magnitude of the (truncated) float value.
func TestLiteralComplexity_Float(t *testing.T) {
	if got := literalComplexity(eng.NewFloat(8.0)); got != 8 {
		t.Errorf("literalComplexity(8.0) = %d, want 8", got)
	}
	// Fractional value truncates toward zero before magnitude.
	if got := literalComplexity(eng.NewFloat(3.9)); got != 3 {
		t.Errorf("literalComplexity(3.9) = %d, want 3 (int64 truncation)", got)
	}
	// Confirm it flows through ShrinkCost too (PushLit base + complexity).
	form := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewFloat(8.0)},
	}}
	if got := ShrinkCost(form, DefaultPolicy()); got != 9 {
		t.Errorf("ShrinkCost([PushLit 8.0]) = %d, want 9 (1 base + 8)", got)
	}
}

// TestLiteralComplexity_List covers the List arm: complexity is the
// element count plus the recursive complexity of each element.
func TestLiteralComplexity_List(t *testing.T) {
	list := eng.NewList([]eng.Value{eng.NewInteger(1), eng.NewInteger(2)})
	// c = Len(2) + complexity(1)=1 + complexity(2)=2 = 5.
	if got := literalComplexity(list); got != 5 {
		t.Errorf("literalComplexity([1,2]) = %d, want 5", got)
	}
	// Nested list recurses: [[3]] → Len(1) + (Len(1) + complexity(3)=3).
	nested := eng.NewList([]eng.Value{
		eng.NewList([]eng.Value{eng.NewInteger(3)}),
	})
	// outer: 1 + inner; inner: 1 + 3 = 4; total = 1 + 4 = 5.
	if got := literalComplexity(nested); got != 5 {
		t.Errorf("literalComplexity([[3]]) = %d, want 5", got)
	}
}

// TestLiteralComplexity_UnhandledIsOne covers the final `return 1`
// placeholder for a value whose type matches none of the arms (Atom).
func TestLiteralComplexity_UnhandledIsOne(t *testing.T) {
	if got := literalComplexity(eng.NewAtom("foo")); got != 1 {
		t.Errorf("literalComplexity(atom) = %d, want 1", got)
	}
}

// TestIntMagnitude_Negative covers the n < 0 sign-adjustment arm.
func TestIntMagnitude_Negative(t *testing.T) {
	// |−5| = 5, plus 1 for the sign.
	if got := intMagnitude(-5); got != 6 {
		t.Errorf("intMagnitude(-5) = %d, want 6", got)
	}
	// Zero and positive as the paired non-negative cases.
	if got := intMagnitude(0); got != 0 {
		t.Errorf("intMagnitude(0) = %d, want 0", got)
	}
	if got := intMagnitude(5); got != 5 {
		t.Errorf("intMagnitude(5) = %d, want 5", got)
	}
}

// TestIntMagnitude_Cap covers the n > maxIntCost saturation arm.
func TestIntMagnitude_Cap(t *testing.T) {
	const maxCap = 1 << 20 // maxIntCost
	if got := intMagnitude(1 << 21); got != maxCap {
		t.Errorf("intMagnitude(1<<21) = %d, want %d (capped)", got, maxCap)
	}
	// A value just under the cap is NOT saturated (boundary check).
	if got := intMagnitude(maxCap - 1); got != maxCap-1 {
		t.Errorf("intMagnitude(cap-1) = %d, want %d", got, maxCap-1)
	}
	// Negative beyond the cap saturates and still adds the sign.
	if got := intMagnitude(-(1 << 21)); got != maxCap+1 {
		t.Errorf("intMagnitude(-(1<<21)) = %d, want %d (capped + sign)", got, maxCap+1)
	}
}
