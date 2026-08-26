package core

import "testing"

// The one-forward-per-scope invariant, and the `end` that enforces it.
//
// design/FULL-COMPILATION.0.md §6.2 bounds a compiled REGION by hard
// delimiters (`end`, `)`, EOF) and then relies on the region descriptor
// carrying the state that picks the stranded-forward raise text. Those two
// do not compose for free: strandedForwardError selects its Forward by
// scanning BACKWARD from the pointer and stops at an OpenParen — `end` is
// not a stop condition — so a Forward parked in an EARLIER region would be
// reachable from inside this one, and a region-bounded descriptor could not
// carry it.
//
// What closes the gap is that stepEnd runs the SAME backward scan and
// removes what it finds, so nothing survives the delimiter to be found.
// These pin that, because Stage 4's OpCollect has to reproduce the text
// SELECTION and not merely the raise.

// A parked forward strands at a barrier word — the first of the two texts.
func TestRegionParkedForwardStrandsBeforeEnd(t *testing.T) {
	setStrict(t, true)
	e := engWithTape(t, []Value{fwdMarker("g", 0, 1, 0)}, 1)
	if err := e.strandedForwardError("def"); err == nil {
		t.Fatal("a parked forward must strand when a function word begins its dispatch")
	}
}

// …and stepEnd drains it, so the same probe inside the NEXT region finds
// nothing and the ordinary no-match raise fires instead. This is the whole
// reason an `end`-bounded descriptor is sufficient.
func TestRegionEndDrainsForwardSoNextRegionFindsNone(t *testing.T) {
	setStrict(t, true)
	// [ forward(g) , end ] with the pointer on the end.
	e := engWithTape(t, []Value{fwdMarker("g", 0, 0, 0), NewWord("end")}, 1)
	if err := e.stepEnd(); err != nil {
		t.Fatalf("stepEnd: %v", err)
	}
	for i := 0; i < e.Tape.Len(); i++ {
		if IsForward(e.Tape.At(i)) {
			t.Fatalf("stepEnd must drain the parked forward; found one at %d", i)
		}
	}
	e.Pointer = e.Tape.Len()
	if err := e.strandedForwardError("def"); err != nil {
		t.Fatalf("after the end drained it there is nothing to strand, got %v", err)
	}
}

// The scan stops at an OpenParen, so a forward parked OUTSIDE the current
// paren scope is already invisible — the property the region bound leans on
// for the paren case, as `end` does for the statement case.
func TestRegionParenScopeHidesOuterForward(t *testing.T) {
	setStrict(t, true)
	e := engWithTape(t, []Value{fwdMarker("g", 0, 1, 0), NewOpenParen()}, 2)
	if err := e.strandedForwardError("def"); err != nil {
		t.Fatalf("a forward below an OpenParen is out of scope, got %v", err)
	}
}

// --- RegionEnd: the syntactic bound a descriptor spans -------------------

func regionTape(t *testing.T, vals []Value) *Tape {
	t.Helper()
	return NewTape(vals, StackHeadroom)
}

// No delimiter at all: the region runs to the window's end.
func TestRegionEndRunsToWindowEnd(t *testing.T) {
	w := regionTape(t, []Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	if got := RegionEnd(w, 0); got != 3 {
		t.Errorf("RegionEnd = %d, want 3 (no delimiter → window end)", got)
	}
}

// `end` terminates the region, and is itself excluded.
func TestRegionEndStopsAtEnd(t *testing.T) {
	w := regionTape(t, []Value{NewInteger(1), NewEnd(), NewInteger(2)})
	if got := RegionEnd(w, 0); got != 1 {
		t.Errorf("RegionEnd = %d, want 1 (stops AT the end marker)", got)
	}
}

// A close paren belonging to a group opened BEFORE the start ends the
// region: the region sits inside that group and cannot outlive it.
func TestRegionEndStopsAtUnmatchedCloseParen(t *testing.T) {
	w := regionTape(t, []Value{NewInteger(1), NewCloseParen(), NewInteger(2)})
	if got := RegionEnd(w, 0); got != 1 {
		t.Errorf("RegionEnd = %d, want 1 (a depth-0 close paren ends the region)", got)
	}
}

// A nested group is CONTAINED — neither its open nor its close ends the
// region, and an `end` inside it ends that group's statement, not this one.
func TestRegionEndContainsNestedGroup(t *testing.T) {
	w := regionTape(t, []Value{
		NewWord("f"), NewOpenParen(), NewInteger(1), NewEnd(), NewCloseParen(), NewInteger(9),
	})
	if got := RegionEnd(w, 0); got != 6 {
		t.Errorf("RegionEnd = %d, want 6 (nested group and its inner `end` are contained)", got)
	}
}

// …and the delimiter AFTER a contained group still ends the region.
func TestRegionEndStopsAfterContainedGroup(t *testing.T) {
	w := regionTape(t, []Value{
		NewOpenParen(), NewInteger(1), NewCloseParen(), NewEnd(), NewInteger(9),
	})
	if got := RegionEnd(w, 0); got != 3 {
		t.Errorf("RegionEnd = %d, want 3 (the `end` past the group ends it)", got)
	}
}

// A start already on a delimiter yields an empty region rather than
// scanning backward into the previous one.
func TestRegionEndFromDelimiterIsEmpty(t *testing.T) {
	w := regionTape(t, []Value{NewInteger(1), NewEnd(), NewInteger(2)})
	if got := RegionEnd(w, 1); got != 1 {
		t.Errorf("RegionEnd = %d, want 1 (starting ON a delimiter spans nothing)", got)
	}
}

// Bounds: negative clamps to 0, past-the-end clamps to Len, nil declines.
func TestRegionEndClampsOutOfRange(t *testing.T) {
	w := regionTape(t, []Value{NewInteger(1), NewInteger(2)})
	if got := RegionEnd(w, -5); got != 2 {
		t.Errorf("RegionEnd(-5) = %d, want 2 (negative clamps to 0)", got)
	}
	if got := RegionEnd(w, 99); got != 2 {
		t.Errorf("RegionEnd(99) = %d, want 2 (past-end clamps to Len)", got)
	}
	if got := RegionEnd(nil, 0); got != 0 {
		t.Errorf("RegionEnd(nil) = %d, want 0", got)
	}
	// A TYPED nil is a non-nil interface holding a nil pointer, so a plain
	// `w == nil` lets it through and (*Tape).Len dereferences it. Panics are
	// forbidden (ADR-005), and a function advertising nil handling has to
	// deliver it for the shape a caller actually produces.
	var typedNil *Tape
	if got := RegionEnd(typedNil, 0); got != 0 {
		t.Errorf("RegionEnd(typed-nil *Tape) = %d, want 0", got)
	}
	if got := RegionSlotCount(typedNil, 0); got != 0 {
		t.Errorf("RegionSlotCount(typed-nil *Tape) = %d, want 0", got)
	}
}

func TestRegionSlotCount(t *testing.T) {
	w := regionTape(t, []Value{NewInteger(1), NewWord("add"), NewEnd(), NewInteger(2)})
	if got := RegionSlotCount(w, 0); got != 2 {
		t.Errorf("RegionSlotCount = %d, want 2", got)
	}
	if got := RegionSlotCount(w, 2); got != 0 {
		t.Errorf("RegionSlotCount at the delimiter = %d, want 0", got)
	}
	if got := RegionSlotCount(nil, 0); got != 0 {
		t.Errorf("RegionSlotCount(nil) = %d, want 0", got)
	}
	if got := RegionSlotCount(w, -3); got != 2 {
		t.Errorf("RegionSlotCount(-3) = %d, want 2 (negative clamps)", got)
	}
	// Past-the-end: RegionEnd clamps its result to Len while `from` stays
	// where the caller put it, so end < from and the span is empty. This is
	// the one case where the two clamps disagree.
	if got := RegionSlotCount(w, 99); got != 0 {
		t.Errorf("RegionSlotCount(99) = %d, want 0 (past-end spans nothing)", got)
	}
}
