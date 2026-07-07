package shrink

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/stackform"
)

// TestDropOpCandidates_NilForm covers the nil-guard early return.
func TestDropOpCandidates_NilForm(t *testing.T) {
	if got := dropOpCandidates(nil); got != nil {
		t.Errorf("dropOpCandidates(nil) = %v, want nil", got)
	}
}

// TestLiteralShrinkCandidates_NilForm covers the nil-guard early
// return.
func TestLiteralShrinkCandidates_NilForm(t *testing.T) {
	if got := literalShrinkCandidates(nil); got != nil {
		t.Errorf("literalShrinkCandidates(nil) = %v, want nil", got)
	}
}

// TestQuoteBodyCandidates_NilForm covers the nil-guard early return.
func TestQuoteBodyCandidates_NilForm(t *testing.T) {
	if got := quoteBodyCandidates(nil, DefaultPolicy()); got != nil {
		t.Errorf("quoteBodyCandidates(nil) = %v, want nil", got)
	}
}

// TestShrinkLiteral_NilParent covers the v.Parent == nil guard: a
// zero-value Value has no Parent, so shrinkLiteral returns nil.
func TestShrinkLiteral_NilParent(t *testing.T) {
	if got := shrinkLiteral(eng.Value{}); got != nil {
		t.Errorf("shrinkLiteral(zero Value) = %v, want nil", got)
	}
}

// TestShrinkLiteral_UnhandledTypeReturnsNil covers the final `return
// nil` fallthrough: an Atom conforms to none of Integer/Float/String/
// Boolean/List, so no shrink alternatives are produced.
func TestShrinkLiteral_UnhandledTypeReturnsNil(t *testing.T) {
	if got := shrinkLiteral(eng.NewAtom("foo")); got != nil {
		t.Errorf("shrinkLiteral(atom) = %v, want nil", got)
	}
}

// TestShrinkLiteral_FloatRoutesToShrinkFloat covers the Float arm of
// shrinkLiteral's type switch (dispatch to shrinkFloat).
func TestShrinkLiteral_FloatRoutesToShrinkFloat(t *testing.T) {
	got := shrinkLiteral(eng.NewFloat(4.0))
	// shrinkFloat(4.0) → [0.0, 2.0].
	if len(got) != 2 {
		t.Fatalf("shrinkLiteral(4.0) len = %d, want 2: %v", len(got), got)
	}
	f0, _ := eng.AsFloat(got[0])
	f1, _ := eng.AsFloat(got[1])
	if f0 != 0.0 {
		t.Errorf("first alternative = %v, want 0.0", f0)
	}
	if f1 != 2.0 {
		t.Errorf("second alternative = %v, want 2.0 (halving)", f1)
	}
}

// TestShrinkLiteral_ListRoutesToShrinkList covers the List arm of
// shrinkLiteral's type switch (dispatch to shrinkList).
func TestShrinkLiteral_ListRoutesToShrinkList(t *testing.T) {
	list := eng.NewList([]eng.Value{
		eng.NewInteger(1), eng.NewInteger(2),
		eng.NewInteger(3), eng.NewInteger(4),
	})
	got := shrinkLiteral(list)
	// shrinkList(4-elem) → 3 alternatives (empty, first-half, first-elem).
	if len(got) != 3 {
		t.Fatalf("shrinkLiteral(list) len = %d, want 3: %v", len(got), got)
	}
}

// TestShrinkInteger_ErrorReturnsNil covers the AsInteger error arm:
// an Integer type literal has nil Data so AsInteger fails.
func TestShrinkInteger_ErrorReturnsNil(t *testing.T) {
	if got := shrinkInteger(eng.NewTypeLiteral(eng.TInteger)); got != nil {
		t.Errorf("shrinkInteger(type literal) = %v, want nil", got)
	}
}

// TestShrinkInteger_Negative covers the `else if n < 0` unit-step
// arm (append n+1) alongside the zero and halving arms.
func TestShrinkInteger_Negative(t *testing.T) {
	got := shrinkInteger(eng.NewInteger(-4))
	// n=-4: → 0 (n!=0); → -2 (n/2, since n<-1); → -3 (n+1, since n<0).
	if len(got) != 3 {
		t.Fatalf("shrinkInteger(-4) len = %d, want 3: %v", len(got), got)
	}
	want := []int64{0, -2, -3}
	for i, w := range want {
		n, _ := eng.AsInteger(got[i])
		if n != w {
			t.Errorf("shrinkInteger(-4)[%d] = %d, want %d", i, n, w)
		}
	}
}

// TestShrinkInteger_PositiveBoundary pairs the negative case: a small
// positive integer exercises the n>0 unit-step arm.
func TestShrinkInteger_PositiveBoundary(t *testing.T) {
	got := shrinkInteger(eng.NewInteger(5))
	// n=5: → 0; → 2 (5/2); → 4 (5-1).
	want := []int64{0, 2, 4}
	if len(got) != len(want) {
		t.Fatalf("shrinkInteger(5) len = %d, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		n, _ := eng.AsInteger(got[i])
		if n != w {
			t.Errorf("shrinkInteger(5)[%d] = %d, want %d", i, n, w)
		}
	}
}

// TestShrinkFloat_NonZeroAndHalving covers the float entry, the
// non-zero (→ 0.0) arm, and the halving (→ f/2) arm.
func TestShrinkFloat_NonZeroAndHalving(t *testing.T) {
	got := shrinkFloat(eng.NewFloat(8.0))
	// f=8: → 0.0; → 4.0 (halving).
	if len(got) != 2 {
		t.Fatalf("shrinkFloat(8.0) len = %d, want 2: %v", len(got), got)
	}
	f0, _ := eng.AsFloat(got[0])
	f1, _ := eng.AsFloat(got[1])
	if f0 != 0.0 || f1 != 4.0 {
		t.Errorf("shrinkFloat(8.0) = [%v,%v], want [0,4]", f0, f1)
	}
}

// TestShrinkFloat_ZeroReturnsEmpty is the boundary: 0.0 offers no
// smaller alternative (neither the non-zero nor the halving arm fires).
func TestShrinkFloat_ZeroReturnsEmpty(t *testing.T) {
	if got := shrinkFloat(eng.NewFloat(0.0)); len(got) != 0 {
		t.Errorf("shrinkFloat(0.0) = %v, want empty", got)
	}
}

// TestShrinkFloat_SmallMagnitudeNoHalving covers the case where the
// non-zero arm fires but the |f|>1 halving arm does not.
func TestShrinkFloat_SmallMagnitudeNoHalving(t *testing.T) {
	got := shrinkFloat(eng.NewFloat(0.5))
	if len(got) != 1 {
		t.Fatalf("shrinkFloat(0.5) len = %d, want 1: %v", len(got), got)
	}
	f0, _ := eng.AsFloat(got[0])
	if f0 != 0.0 {
		t.Errorf("shrinkFloat(0.5)[0] = %v, want 0.0", f0)
	}
}

// TestShrinkFloat_ErrorReturnsNil covers the AsFloat error arm via a
// Float type literal (nil Data).
func TestShrinkFloat_ErrorReturnsNil(t *testing.T) {
	if got := shrinkFloat(eng.NewTypeLiteral(eng.TFloat)); got != nil {
		t.Errorf("shrinkFloat(type literal) = %v, want nil", got)
	}
}

// TestShrinkString_ErrorReturnsNil covers the AsString error arm via
// a String type literal (nil Data).
func TestShrinkString_ErrorReturnsNil(t *testing.T) {
	if got := shrinkString(eng.NewTypeLiteral(eng.TString)); got != nil {
		t.Errorf("shrinkString(type literal) = %v, want nil", got)
	}
}

// TestShrinkBoolean_ErrorReturnsNil covers the AsBoolean error arm via
// a Boolean type literal (nil Data).
func TestShrinkBoolean_ErrorReturnsNil(t *testing.T) {
	if got := shrinkBoolean(eng.NewTypeLiteral(eng.TBoolean)); got != nil {
		t.Errorf("shrinkBoolean(type literal) = %v, want nil", got)
	}
}

// TestShrinkList_Empty covers the n == 0 early return. The list must
// be a CONCRETE empty list (non-nil, zero-length backing slice) —
// NewList(nil) is nil-backed and fails RequireConcreteList's IsNil
// check earlier, so it exercises the error arm instead.
func TestShrinkList_Empty(t *testing.T) {
	if got := shrinkList(eng.NewList([]eng.Value{})); got != nil {
		t.Errorf("shrinkList(concrete empty) = %v, want nil", got)
	}
}

// TestShrinkList_NilBackedErrorsOut pairs the above: a nil-backed list
// hits the RequireConcreteList error arm (not the n == 0 path) and
// also returns nil.
func TestShrinkList_NilBackedErrorsOut(t *testing.T) {
	if got := shrinkList(eng.NewList(nil)); got != nil {
		t.Errorf("shrinkList(nil-backed) = %v, want nil", got)
	}
}

// TestShrinkList_MultiElement covers the first-half loop and the
// first-element alternative for a list with more than one element.
func TestShrinkList_MultiElement(t *testing.T) {
	list := eng.NewList([]eng.Value{
		eng.NewInteger(10),
		eng.NewInteger(20),
		eng.NewInteger(30),
		eng.NewInteger(40),
	})
	got := shrinkList(list)
	// n=4: → [] ; → first half [10,20] ; → first element [10].
	if len(got) != 3 {
		t.Fatalf("shrinkList(4-elem) len = %d, want 3: %v", len(got), got)
	}
	lens := []int{0, 2, 1}
	for i, wantLen := range lens {
		rl, err := eng.AsList(got[i])
		if err != nil {
			t.Fatalf("shrinkList result[%d] not a list: %v", i, err)
		}
		if rl.Len() != wantLen {
			t.Errorf("shrinkList result[%d] len = %d, want %d", i, rl.Len(), wantLen)
		}
	}
	// First-half [10,20]: verify elements copied in order.
	half, _ := eng.AsList(got[1])
	if n, _ := eng.AsInteger(half.Get(0)); n != 10 {
		t.Errorf("first-half[0] = %d, want 10", n)
	}
	if n, _ := eng.AsInteger(half.Get(1)); n != 20 {
		t.Errorf("first-half[1] = %d, want 20", n)
	}
}

// TestShrinkList_SingleElement is the boundary: a one-element list
// yields only the empty-list alternative (no first-half / first-elem).
func TestShrinkList_SingleElement(t *testing.T) {
	got := shrinkList(eng.NewList([]eng.Value{eng.NewInteger(9)}))
	if len(got) != 1 {
		t.Fatalf("shrinkList(1-elem) len = %d, want 1: %v", len(got), got)
	}
	rl, _ := eng.AsList(got[0])
	if rl.Len() != 0 {
		t.Errorf("shrinkList(1-elem)[0] len = %d, want 0 (empty list)", rl.Len())
	}
}

// TestGenerateCandidates_QuoteRecursion is an end-to-end check that
// quoteBodyCandidates recurses into a Quote body and substitutes the
// reduced body back, keeping the outer Quote wrapper.
func TestGenerateCandidates_QuoteRecursion(t *testing.T) {
	inner := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(42)},
	}}
	form := &stackform.StackForm{Ops: []stackform.Op{
		stackform.Quote{Body: inner},
	}}
	cands := quoteBodyCandidates(form, DefaultPolicy())
	if len(cands) == 0 {
		t.Fatal("quoteBodyCandidates produced no candidates for a Quote-wrapped body")
	}
	// Every candidate must still be a single Quote op (wrapper kept).
	for _, c := range cands {
		if len(c.Ops) != 1 {
			t.Fatalf("candidate op count = %d, want 1", len(c.Ops))
		}
		if _, ok := c.Ops[0].(stackform.Quote); !ok {
			t.Fatalf("candidate top op = %T, want Quote", c.Ops[0])
		}
	}
}
