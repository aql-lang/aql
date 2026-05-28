package shrink

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/stackform"
)

// fpEval makes a deterministic EvalFn from a predicate over forms.
// Useful for tests where we want to assert the reducer converges on
// the minimum form satisfying some "failure" condition.
func fpEval(predFail func(*stackform.StackForm) bool) EvalFn {
	return func(f *stackform.StackForm) Outcome {
		if predFail(f) {
			return Fail
		}
		return Pass
	}
}

// formHas returns true if `form` contains a PushLit of the given
// integer value at any nesting level — handy for declaring the
// "failure shape" in test predicates.
func formHasInt(form *stackform.StackForm, n int64) bool {
	if form == nil {
		return false
	}
	for _, op := range form.Ops {
		switch o := op.(type) {
		case stackform.PushLit:
			if got, err := eng.AsInteger(o.V); err == nil && got == n {
				return true
			}
		case stackform.Quote:
			if formHasInt(o.Body, n) {
				return true
			}
		}
	}
	return false
}

// TestReduce_DropsUnrelatedOps confirms the reducer removes ops
// that aren't required to preserve the failure. Failure condition:
// the form contains the integer 7. Start with extra ops; reducer
// should drop them.
func TestReduce_DropsUnrelatedOps(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(1)},
		stackform.PushLit{V: eng.NewInteger(7)}, // the witness
		stackform.PushLit{V: eng.NewInteger(2)},
		stackform.PushLit{V: eng.NewInteger(3)},
		stackform.Call{Name: "add", Arity: 2},
	}}
	eval := fpEval(func(f *stackform.StackForm) bool {
		return formHasInt(f, 7)
	})
	reduced := Reduce(initial, eval, DefaultProfile())
	if !formHasInt(reduced, 7) {
		t.Fatal("reducer dropped the witness — failure no longer preserved")
	}
	// Should now contain ONLY the witness. PushLit(7) is the
	// minimal failing form.
	if len(reduced.Ops) != 1 {
		t.Errorf("reducer left %d ops, want 1 (just the PushLit 7): %s",
			len(reduced.Ops), stackform.Pretty(reduced))
	}
}

// TestReduce_ShrinksIntegerLiterals confirms PushLit(42) is shrunk
// toward 0 when the failure condition is "contains some Integer".
func TestReduce_ShrinksIntegerLiterals(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(42)},
	}}
	// Failure: ANY integer present. Reducer should shrink the
	// value all the way to 0.
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
		t.Fatalf("expected 1 op left, got %d", len(reduced.Ops))
	}
	lit, ok := reduced.Ops[0].(stackform.PushLit)
	if !ok {
		t.Fatalf("expected PushLit, got %T", reduced.Ops[0])
	}
	n, _ := eng.AsInteger(lit.V)
	if n != 0 {
		t.Errorf("expected shrunk to 0, got %d", n)
	}
}

// TestReduce_ShrinksToMinimumNonZero confirms binary-halving works
// when 0 specifically doesn't preserve the failure but a smaller
// positive value does.
func TestReduce_ShrinksToMinimumNonZero(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(100)},
	}}
	// Failure: integer > 0. So 0 doesn't qualify but 1 does.
	eval := fpEval(func(f *stackform.StackForm) bool {
		if len(f.Ops) == 0 {
			return false
		}
		lit, ok := f.Ops[0].(stackform.PushLit)
		if !ok {
			return false
		}
		n, err := eng.AsInteger(lit.V)
		return err == nil && n > 0
	})
	reduced := Reduce(initial, eval, DefaultProfile())
	lit, _ := reduced.Ops[0].(stackform.PushLit)
	n, _ := eng.AsInteger(lit.V)
	if n <= 0 {
		t.Errorf("reducer broke the failure condition: got %d, want >0", n)
	}
	if n > 1 {
		t.Errorf("expected shrinker to converge on 1 via halving, got %d", n)
	}
}

// TestReduce_ShrinksStrings confirms string literals collapse to
// "" (or shortest possible) when the failure is just "has a string".
func TestReduce_ShrinksStrings(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewString("hello world")},
	}}
	eval := fpEval(func(f *stackform.StackForm) bool {
		if len(f.Ops) == 0 {
			return false
		}
		_, ok := f.Ops[0].(stackform.PushLit)
		return ok
	})
	reduced := Reduce(initial, eval, DefaultProfile())
	lit, _ := reduced.Ops[0].(stackform.PushLit)
	s, _ := eng.AsString(lit.V)
	if s != "" {
		t.Errorf("expected shrunk to \"\", got %q", s)
	}
}

// TestReduce_ShrinksBoolean confirms `true` shrinks to `false`
// when both preserve the failure.
func TestReduce_ShrinksBoolean(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewBoolean(true)},
	}}
	eval := fpEval(func(f *stackform.StackForm) bool {
		if len(f.Ops) == 0 {
			return false
		}
		_, ok := f.Ops[0].(stackform.PushLit)
		return ok
	})
	reduced := Reduce(initial, eval, DefaultProfile())
	lit, _ := reduced.Ops[0].(stackform.PushLit)
	b, _ := eng.AsBoolean(lit.V)
	if b {
		t.Error("expected shrunk to false")
	}
}

// TestReduce_RecursesIntoQuoteBody confirms inner Quote bodies are
// shrunk too — the reducer walks the tree, not just the top level.
func TestReduce_RecursesIntoQuoteBody(t *testing.T) {
	inner := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(1)},
		stackform.PushLit{V: eng.NewInteger(99)}, // the witness, nested
		stackform.PushLit{V: eng.NewInteger(2)},
	}}
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.Quote{Body: inner},
	}}
	eval := fpEval(func(f *stackform.StackForm) bool {
		return formHasInt(f, 99)
	})
	reduced := Reduce(initial, eval, DefaultProfile())
	if !formHasInt(reduced, 99) {
		t.Fatal("reducer dropped the nested witness")
	}
	// The Quote body should have just the witness left.
	q, ok := reduced.Ops[0].(stackform.Quote)
	if !ok {
		t.Fatalf("top op no longer Quote: got %T", reduced.Ops[0])
	}
	if len(q.Body.Ops) != 1 {
		t.Errorf("Quote body left with %d ops, want 1: %s",
			len(q.Body.Ops), stackform.Pretty(reduced))
	}
}

// TestReduce_NoOpWhenAllOpsRequired confirms the reducer leaves the
// form alone when every op is essential.
func TestReduce_NoOpWhenAllOpsRequired(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(7)},
	}}
	// Failure: contains 7. Already minimal (single op, 0 wouldn't
	// satisfy the condition).
	eval := fpEval(func(f *stackform.StackForm) bool {
		return formHasInt(f, 7)
	})
	reduced := Reduce(initial, eval, DefaultProfile())
	if len(reduced.Ops) != 1 {
		t.Errorf("minimal form mutated: %s", stackform.Pretty(reduced))
	}
}

// TestReduce_RejectsCandidatesThatBreakFailure confirms the reducer
// won't accept a mutation that loses the failure condition.
func TestReduce_RejectsCandidatesThatBreakFailure(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(7)},
		stackform.PushLit{V: eng.NewInteger(3)},
	}}
	// Failure: top-of-stack is 7 (i.e. the SECOND PushLit's value).
	eval := fpEval(func(f *stackform.StackForm) bool {
		if len(f.Ops) < 1 {
			return false
		}
		lit, ok := f.Ops[len(f.Ops)-1].(stackform.PushLit)
		if !ok {
			return false
		}
		n, _ := eng.AsInteger(lit.V)
		return n == 7
	})
	// Start: top is 3, not 7 — eval returns Pass.
	// Reducer can't make it Fail by dropping/shrinking, so should
	// leave it alone (or at least return SOMETHING that's at most
	// no worse).
	reduced := Reduce(initial, eval, DefaultProfile())
	// Either nothing changed, or whatever changed still leaves the
	// form NOT failing. Confirm reducer didn't accept a Pass.
	if eval(reduced) == Fail {
		t.Errorf("reducer somehow made the form Fail when initial didn't: %s",
			stackform.Pretty(reduced))
	}
}

// TestReduce_BestFirst_FindsMinimum confirms best-first search
// reaches the same minimum as greedy on cases where greedy works.
func TestReduce_BestFirst_FindsMinimum(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(1)},
		stackform.PushLit{V: eng.NewInteger(7)}, // the witness
		stackform.PushLit{V: eng.NewInteger(2)},
		stackform.PushLit{V: eng.NewInteger(3)},
		stackform.Call{Name: "add", Arity: 2},
	}}
	eval := fpEval(func(f *stackform.StackForm) bool {
		return formHasInt(f, 7)
	})
	profile := &Profile{
		MaxSteps:  500,
		Policy:    DefaultPolicy(),
		Strategy:  BestFirst,
		BeamWidth: 16,
	}
	reduced := Reduce(initial, eval, profile)
	if !formHasInt(reduced, 7) {
		t.Fatal("best-first dropped the witness")
	}
	if len(reduced.Ops) != 1 {
		t.Errorf("best-first left %d ops, want 1: %s",
			len(reduced.Ops), stackform.Pretty(reduced))
	}
}

// TestReduce_BestFirst_ExploresWiderThanGreedy demonstrates a case
// where greedy's "commit to the locally lowest-cost candidate"
// behavior misses a better reduction that best-first finds.
//
// Setup: form has two PushLits at cost 4 each. Greedy might commit
// to shrinking one path; best-first explores both. With this
// particular eval predicate, both paths converge on the same
// answer — the test just confirms best-first AT LEAST matches
// greedy and doesn't regress.
func TestReduce_BestFirst_AtLeastMatchesGreedy(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(100)},
		stackform.PushLit{V: eng.NewInteger(200)},
		stackform.Call{Name: "add", Arity: 2},
	}}
	eval := fpEval(func(f *stackform.StackForm) bool {
		// Failure: there's a Call op AND at least one PushLit.
		var hasCall, hasLit bool
		for _, op := range f.Ops {
			if _, ok := op.(stackform.Call); ok {
				hasCall = true
			}
			if _, ok := op.(stackform.PushLit); ok {
				hasLit = true
			}
		}
		return hasCall && hasLit
	})

	greedyProf := DefaultProfile()
	greedyProf.MaxSteps = 500
	greedyReduced := Reduce(initial, eval, greedyProf)

	bestProf := &Profile{
		MaxSteps:  500,
		Policy:    DefaultPolicy(),
		Strategy:  BestFirst,
		BeamWidth: 32,
	}
	bestReduced := Reduce(initial, eval, bestProf)

	greedyCost := ShrinkCost(greedyReduced, DefaultPolicy())
	bestCost := ShrinkCost(bestReduced, DefaultPolicy())
	if bestCost > greedyCost {
		t.Errorf("BestFirst regressed: best=%d greedy=%d (forms: best=%q greedy=%q)",
			bestCost, greedyCost,
			stackform.Pretty(bestReduced), stackform.Pretty(greedyReduced))
	}
}

// TestReduce_BestFirst_NoFailDoesntShrink confirms best-first
// respects the "only shrink genuine counterexamples" contract.
func TestReduce_BestFirst_NoFailDoesntShrink(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(5)},
	}}
	// Eval always returns Pass.
	eval := fpEval(func(_ *stackform.StackForm) bool { return false })
	profile := &Profile{
		MaxSteps: 500,
		Policy:   DefaultPolicy(),
		Strategy: BestFirst,
	}
	reduced := Reduce(initial, eval, profile)
	if !stackform.Equal(reduced, initial) {
		t.Errorf("non-failing initial got shrunk: %s", stackform.Pretty(reduced))
	}
}

// TestReduce_BestFirst_RespectsBeamWidth confirms the queue is
// bounded — the search doesn't blow up with many candidates.
func TestReduce_BestFirst_RespectsBeamWidth(t *testing.T) {
	// Form with many PushLits → many drop candidates per step.
	ops := []stackform.Op{}
	for i := 0; i < 20; i++ {
		ops = append(ops, stackform.PushLit{V: eng.NewInteger(int64(i))})
	}
	initial := &stackform.StackForm{Ops: ops}
	eval := fpEval(func(f *stackform.StackForm) bool {
		// Failure: contains the value 7 anywhere.
		return formHasInt(f, 7)
	})
	profile := &Profile{
		MaxSteps:  100,
		Policy:    DefaultPolicy(),
		Strategy:  BestFirst,
		BeamWidth: 4, // very tight beam
	}
	// Should not panic / OOM. Returned form should still preserve
	// the witness.
	reduced := Reduce(initial, eval, profile)
	if !formHasInt(reduced, 7) {
		t.Fatal("tight beam dropped the witness")
	}
}

// TestReduce_RespectsMaxSteps confirms MaxSteps caps the outer
// loop. Uses a predicate (n > 0) where 0 doesn't qualify, so the
// reducer must halve through intermediate values rather than jump
// straight to 0. With MaxSteps=2 starting from 1024, the reducer
// gets through at most two halvings (1024 → 512 → 256) before the
// loop terminates.
func TestReduce_RespectsMaxSteps(t *testing.T) {
	initial := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(1024)},
	}}
	eval := fpEval(func(f *stackform.StackForm) bool {
		if len(f.Ops) == 0 {
			return false
		}
		lit, ok := f.Ops[0].(stackform.PushLit)
		if !ok {
			return false
		}
		n, err := eng.AsInteger(lit.V)
		return err == nil && n > 0
	})
	profile := &Profile{MaxSteps: 2, Policy: DefaultPolicy()}
	reduced := Reduce(initial, eval, profile)
	lit, _ := reduced.Ops[0].(stackform.PushLit)
	n, _ := eng.AsInteger(lit.V)
	if n == 1 {
		t.Errorf("reducer converged all the way to 1 — MaxSteps=2 not respected (would need ~10 halvings)")
	}
	if n <= 0 {
		t.Errorf("reducer lost the failure condition: n=%d", n)
	}
}
