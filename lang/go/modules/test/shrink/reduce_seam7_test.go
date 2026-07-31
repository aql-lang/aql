package shrink

import (
	"testing"

	"github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/stackform"
)

// TestReduce_NilProfileDefaults covers the `profile == nil` guard in
// Reduce: a nil profile is replaced by DefaultProfile() and the
// reduction proceeds normally.
func TestReduce_NilProfileDefaults(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(1)},
		stackform.PushLit{V: eng.NewInteger(7)},
		stackform.PushLit{V: eng.NewInteger(2)},
	}}
	eval := fpEval(func(f *stackform.StackForm) bool {
		return formHasInt(f, 7)
	})
	reduced := Reduce(initial, eval, nil)
	if !formHasInt(reduced, 7) {
		t.Fatal("nil-profile reduction dropped the witness")
	}
	if len(reduced.Ops) != 1 {
		t.Errorf("nil-profile reduction left %d ops, want 1: %s",
			len(reduced.Ops), stackform.Pretty(reduced))
	}
}

// TestReduce_NilInitial covers the `initial == nil` guard: Reduce
// returns nil without calling eval.
func TestReduce_NilInitial(t *testing.T) {
	called := false
	eval := func(*stackform.StackForm) Outcome {
		called = true
		return Fail
	}
	if got := Reduce(nil, eval, DefaultProfile()); got != nil {
		t.Errorf("Reduce(nil) = %v, want nil", got)
	}
	if called {
		t.Error("Reduce(nil) should not invoke eval")
	}
}

// TestGreedyReduce_BreaksOnNonImprovingCandidate covers the greedy
// inner-loop `candCost >= cost` break. A float literal 0.5 shrinks to
// 0.0, and both have literal complexity 0 (int64(0.5)==0), so the
// shrink candidate costs exactly the same as the original. With the
// cheaper drop candidate rejected (it loses the failure), the reducer
// reaches the equal-cost 0.0 candidate and breaks without accepting —
// leaving the form unchanged.
func TestGreedyReduce_BreaksOnNonImprovingCandidate(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewFloat(0.5)},
	}}
	// Failure: the form contains a PushLit op. Dropping it (→ empty
	// form) loses the failure; shrinking 0.5→0.0 keeps a PushLit but
	// does not lower cost.
	eval := fpEval(func(f *stackform.StackForm) bool {
		for _, op := range f.Ops {
			if _, ok := op.(stackform.PushLit); ok {
				return true
			}
		}
		return false
	})
	reduced := Reduce(initial, eval, DefaultProfile())
	if len(reduced.Ops) != 1 {
		t.Fatalf("expected form unchanged (1 op), got %d: %s",
			len(reduced.Ops), stackform.Pretty(reduced))
	}
	lit, ok := reduced.Ops[0].(stackform.PushLit)
	if !ok {
		t.Fatalf("expected PushLit, got %T", reduced.Ops[0])
	}
	f, _ := eng.AsFloat(lit.V)
	if f != 0.5 {
		t.Errorf("expected the equal-cost break to leave 0.5 unchanged, got %v", f)
	}
}

// TestBestFirst_DefaultBeamWidth covers the `beam <= 0` default arm in
// bestFirstReduce: a Profile with BeamWidth unset (0) must default the
// beam to 16 and still reduce a genuine counterexample.
func TestBestFirst_DefaultBeamWidth(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(1)},
		stackform.PushLit{V: eng.NewInteger(7)},
		stackform.PushLit{V: eng.NewInteger(2)},
		stackform.PushLit{V: eng.NewInteger(3)},
		stackform.Call{Name: "add", Arity: 2},
	}}
	eval := fpEval(func(f *stackform.StackForm) bool {
		return formHasInt(f, 7)
	})
	profile := &Profile{
		MaxSteps: 500,
		Policy:   DefaultPolicy(),
		Strategy: BestFirst,
		// BeamWidth deliberately left 0 to exercise the default.
	}
	reduced := Reduce(initial, eval, profile)
	if !formHasInt(reduced, 7) {
		t.Fatal("default-beam best-first dropped the witness")
	}
	if len(reduced.Ops) != 1 {
		t.Errorf("default-beam best-first left %d ops, want 1: %s",
			len(reduced.Ops), stackform.Pretty(reduced))
	}
}

// TestBestFirst_MaxStepsMidLoop covers the inner-loop
// `steps >= MaxSteps` break inside bestFirstReduce. MaxSteps=1 forces
// the break on the second candidate of the first expansion.
func TestBestFirst_MaxStepsMidLoop(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(7)},
		stackform.PushLit{V: eng.NewInteger(3)},
	}}
	eval := fpEval(func(f *stackform.StackForm) bool {
		return formHasInt(f, 7)
	})
	profile := &Profile{
		MaxSteps:  1,
		Policy:    DefaultPolicy(),
		Strategy:  BestFirst,
		BeamWidth: 16,
	}
	reduced := Reduce(initial, eval, profile)
	// Only one candidate evaluation is permitted, so the witness must
	// survive (either the initial is returned or a single accepted
	// failure-preserving candidate).
	if !formHasInt(reduced, 7) {
		t.Fatalf("MaxSteps=1 best-first lost the witness: %s", stackform.Pretty(reduced))
	}
}

// TestInsertSorted_InsertsBeforeCostlier covers insertSorted's
// insertion branch: when an existing queue entry costs MORE than the
// inserted form, the form is spliced in ahead of it.
func TestInsertSorted_InsertsBeforeCostlier(t *testing.T) {
	policy := DefaultPolicy()
	costly := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(5)}, // cost 1+5 = 6
	}}
	cheap := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(2)}, // cost 1+2 = 3
	}}
	// Sanity on the cost ordering the test relies on.
	if ShrinkCost(costly, policy) <= ShrinkCost(cheap, policy) {
		t.Fatalf("test setup wrong: costly (%d) must exceed cheap (%d)",
			ShrinkCost(costly, policy), ShrinkCost(cheap, policy))
	}
	out := insertSorted([]*stackform.StackForm{costly}, cheap, policy)
	if len(out) != 2 {
		t.Fatalf("insertSorted len = %d, want 2", len(out))
	}
	// cheap must land ahead of costly (ascending order).
	if out[0] != cheap || out[1] != costly {
		t.Errorf("insertSorted order wrong: got [%s, %s], want [cheap, costly]",
			stackform.Pretty(out[0]), stackform.Pretty(out[1]))
	}
}

// TestInsertSorted_AppendsWhenNotCostlier is the paired boundary case:
// when no existing entry costs more than the inserted form, it is
// appended at the tail (the fall-through return).
func TestInsertSorted_AppendsWhenNotCostlier(t *testing.T) {
	policy := DefaultPolicy()
	cheap := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(2)}, // cost 3
	}}
	costly := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(5)}, // cost 6
	}}
	out := insertSorted([]*stackform.StackForm{cheap}, costly, policy)
	if len(out) != 2 {
		t.Fatalf("insertSorted len = %d, want 2", len(out))
	}
	if out[0] != cheap || out[1] != costly {
		t.Errorf("insertSorted append order wrong: got [%s, %s], want [cheap, costly]",
			stackform.Pretty(out[0]), stackform.Pretty(out[1]))
	}
}
