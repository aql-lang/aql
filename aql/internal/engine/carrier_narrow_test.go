package engine

import "testing"

// TestApplyComplementNarrowing exercises the disjunction-subtraction
// path directly: a binding of Integer|String with a guard `x is
// Integer` should narrow x to String in the else branch (subtract
// the matched alternative).
func TestApplyComplementNarrowing(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	r.Check.Mode = true

	// Install a disjunct carrier on x: Integer|String.
	disjunct := NewDisjunct([]Value{
		NewTypeLiteral(TInteger),
		NewTypeLiteral(TString),
	})
	disjunct.Carrier = true
	r.DefStacks["x"] = []Value{disjunct}

	// Build a condition list simulating `x is Integer`.
	cond := NewEvalList([]Value{
		NewWord("x"),
		NewWord("is"),
		NewTypeLiteral(TInteger),
	})

	restore := applyComplementNarrowing(r, cond)
	defer restore()

	ds := r.DefStacks["x"]
	if len(ds) < 2 {
		t.Fatalf("expected pushed narrow entry, got stack depth %d", len(ds))
	}
	top := ds[len(ds)-1]
	if !top.VType.Equal(TString) {
		t.Errorf("expected top to narrow to String, got %s", top.VType)
	}
}

// TestApplyComplementNarrowingNoOpOnConcrete verifies that when the
// current binding is NOT a disjunct (e.g. concrete Integer), the
// complement narrower leaves the DefStack untouched — we can't
// subtract a type from a non-disjunct in the current lattice.
func TestApplyComplementNarrowingNoOpOnConcrete(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	r.Check.Mode = true

	r.DefStacks["x"] = []Value{NewCarrier(TInteger)}

	cond := NewEvalList([]Value{
		NewWord("x"),
		NewWord("is"),
		NewTypeLiteral(TString),
	})

	restore := applyComplementNarrowing(r, cond)
	defer restore()

	if len(r.DefStacks["x"]) != 1 {
		t.Errorf("expected DefStack depth to remain 1, got %d", len(r.DefStacks["x"]))
	}
}
