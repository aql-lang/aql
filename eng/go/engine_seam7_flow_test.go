package eng

// Wave-7 coverage, part 3: control-flow (mark/move/loop) resolvers,
// close-paren scanning, implicit-end, and the pending-forward predicate
// probes. Each is exercised by hand-built tape state — these helpers
// read/rewrite the tape directly, so a constructed *Engine reaches their
// edge arms without a full Run(). See design/TEST-SEAMS.10.md.

import "testing"

// --- stepMove orphan ------------------------------------------------------

func TestS7StepMoveOrphanMarkGone(t *testing.T) {
	// The mark id is registered in e.marks but the Mark token is not on
	// the tape (a for-loop controller removed it) → orphan move, removed
	// quietly.
	e := engWithTape(t, []Value{NewMove("m1", "for loop")}, 0)
	e.marks = map[string]bool{"m1": true}
	if err := e.stepMove(e.tape.At(0)); err != nil {
		t.Fatalf("orphan move should be removed quietly, got %v", err)
	}
	if e.tape.Len() != 0 {
		t.Errorf("orphan move not removed: len=%d", e.tape.Len())
	}
}

func TestS7StepMoveMarkNotFoundErrors(t *testing.T) {
	// The mark id is NOT registered → move_error.
	e := engWithTape(t, []Value{NewMove("nope", "for loop")}, 0)
	if err := e.stepMove(e.tape.At(0)); err == nil {
		t.Fatal("move to an unknown mark must error")
	}
}

// --- stepMoveCont next-iteration (nil marks map) --------------------------

func TestS7StepMoveContMoreIterationsNilMarks(t *testing.T) {
	r := covRegistry(t, nil)
	cont := &ForCont{
		Registry: r,
		IterName: "i",
		Current:  0,
		End:      3,
		Step:     1,
		Body:     []Value{NewInteger(1)},
	}
	move := NewMoveCont("m1", "for loop", cont)
	e := NewTop(r)
	e.tape = NewTape([]Value{NewMark("m1"), move}, stackHeadroom)
	e.marks = nil // force the lazy-init arm
	if err := e.stepMoveCont(0, 1, MoveInfo{To: "m1", Cont: cont}); err != nil {
		t.Fatalf("stepMoveCont: %v", err)
	}
	if e.marks == nil {
		t.Error("marks map should have been initialised for the next iteration")
	}
}

// --- handleLoopBreak / handleLoopContinue with missing mark ---------------

func TestS7HandleLoopBreakMissingMark(t *testing.T) {
	r := covRegistry(t, nil)
	cont := &ForCont{Registry: r, IterName: "i"}
	// A MoveCont with no matching Mark on the tape → markIdx<0 → skip.
	e := NewTop(r)
	e.tape = NewTape([]Value{NewMoveCont("gone", "for loop", cont)}, stackHeadroom)
	e.marks = map[string]bool{"gone": true}
	if e.handleLoopBreak() {
		t.Error("break with no mark on tape should not resolve")
	}
}

func TestS7HandleLoopContinueMissingMark(t *testing.T) {
	r := covRegistry(t, nil)
	cont := &ForCont{Registry: r, IterName: "i"}
	e := NewTop(r)
	e.tape = NewTape([]Value{NewMoveCont("gone", "for loop", cont)}, stackHeadroom)
	e.marks = map[string]bool{"gone": true}
	if e.handleLoopContinue() {
		t.Error("continue with no mark on tape should not resolve")
	}
}

// --- cleanMarks -----------------------------------------------------------

func TestS7CleanMarks(t *testing.T) {
	e := engWithTape(t, []Value{NewMark("a"), NewInteger(1), NewMove("a", "r")}, 0)
	e.cleanMarks()
	if e.tape.Len() != 1 {
		t.Errorf("cleanMarks left %d values, want 1 (the Integer)", e.tape.Len())
	}
	if e.marks != nil {
		t.Error("marks map should be cleared")
	}
}

// --- findCloseParenAfter --------------------------------------------------

func TestS7FindCloseParenAfter(t *testing.T) {
	// Nested open with only one close → the outer group is unterminated.
	e := engWithTape(t, []Value{NewOpenParen(), NewOpenParen(), NewCloseParen()}, 0)
	if got := e.findCloseParenAfter(0); got != -1 {
		t.Errorf("findCloseParenAfter = %d, want -1 (unterminated outer)", got)
	}
	// Balanced: the matching close is found.
	e2 := engWithTape(t, []Value{NewOpenParen(), NewInteger(1), NewCloseParen()}, 0)
	if got := e2.findCloseParenAfter(0); got != 2 {
		t.Errorf("findCloseParenAfter = %d, want 2", got)
	}
}

// --- implicitEnd ----------------------------------------------------------

func TestS7ImplicitEndForwardBeforeFunc(t *testing.T) {
	// Forward marker sits BEFORE its function index → the funcIdx-- arm.
	sig := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1}
	fwd := NewForward(ForwardInfo{FuncName: "cadd", Sig: sig, FuncIndex: 2, CollectedArgs: 0})
	e := engWithTape(t, []Value{fwd, NewInteger(1), NewWord("cadd")}, 0)
	if err := e.implicitEnd(0); err != nil {
		t.Fatalf("implicitEnd: %v", err)
	}
}

// --- pending-forward predicates (break arms) ------------------------------

func fwdFull(sig *Signature) Value {
	return NewForward(ForwardInfo{Sig: sig, CollectedArgs: sig.TotalArgs(), FuncIndex: 0})
}

func TestS7PendingForwardPredicatesFullyCollected(t *testing.T) {
	// A pending forward whose slots are all filled (CollectedArgs ==
	// TotalArgs) hits the break in each probe → all report false.
	sig := &Signature{Params: []FnParam{{Type: TInteger}}, BarrierPos: -1}
	e := engWithTape(t, []Value{fwdFull(sig), NewInteger(0)}, 1)
	if e.hasPendingForwardQuoteArg() {
		t.Error("fully-collected forward → no pending quote slot")
	}
	if e.hasPendingForwardFormArg() {
		t.Error("fully-collected forward → no pending form slot")
	}
	if e.hasPendingForwardExpectingFunction() {
		t.Error("fully-collected forward → no pending function slot")
	}
}

func TestS7PendingForwardExpectingFunctionTrue(t *testing.T) {
	// Next slot is a Function type → true (the positive arm).
	sig := &Signature{Params: []FnParam{{Type: TFunction}}, BarrierPos: -1}
	fwd := NewForward(ForwardInfo{Sig: sig, CollectedArgs: 0, FuncIndex: 0})
	e := engWithTape(t, []Value{fwd, NewInteger(0)}, 1)
	if !e.hasPendingForwardExpectingFunction() {
		t.Error("a Function next-slot should be detected")
	}
}

// --- checkModeFallbackPositions (marker skip) -----------------------------

func TestS7CheckModeFallbackPositionsSkipsMarkers(t *testing.T) {
	// A Mark token after the pointer is skipped; the Integer beyond it is
	// gathered.
	e := engWithTape(t, []Value{NewInteger(0), NewMark("m"), NewInteger(9)}, 0)
	got := e.checkModeFallbackPositions(1)
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("positions = %v, want [2] (skipped the Mark)", got)
	}
}
