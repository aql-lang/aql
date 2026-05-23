package native

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
	r.Defs.Push("x", disjunct)

	// Build a condition list simulating `x is Integer`.
	cond := NewEvalList([]Value{
		NewWord("x"),
		NewWord("is"),
		NewTypeLiteral(TInteger),
	})

	restore := ApplyComplementNarrowing(r, cond)
	defer restore()

	if d := r.Defs.Depth("x"); d < 2 {
		t.Fatalf("expected pushed narrow entry, got stack depth %d", d)
	}
	top, _ := r.Defs.Top("x")
	// Narrowing produces a carrier (Parent=String, Carrier=true) —
	// check the Parent, not the value's own identity.
	if !top.Parent.Equal(TString) {
		t.Errorf("expected top to narrow to String, got %s", top.String())
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

	r.Defs.Push("x", NewCarrier(TInteger))

	cond := NewEvalList([]Value{
		NewWord("x"),
		NewWord("is"),
		NewTypeLiteral(TString),
	})

	restore := ApplyComplementNarrowing(r, cond)
	defer restore()

	if d := r.Defs.Depth("x"); d != 1 {
		t.Errorf("expected DefStack depth to remain 1, got %d", d)
	}
}
