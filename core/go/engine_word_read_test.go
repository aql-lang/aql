package core

import "testing"

// wordReadEmit is the inactive recorder plus the NUR123 read notes: it
// records what noteWordRead classifies as a word dispatch.
type wordReadEmit struct {
	EmitRecorder
	noted []string
	vals  []string
}

func (w *wordReadEmit) Active() bool { return true }
func (w *wordReadEmit) NoteWordRead(v Value, name string, _ SrcPos) {
	w.noted = append(w.noted, name)
}
func (w *wordReadEmit) NoteValRead(id string) { w.vals = append(w.vals, id) }

// TestNoteWordReadClassifies pins the engine-side classification of a bare
// read (NUR123): a fn-typed carrier and a gradual (Any, Dynamic) carrier are
// reads the interpreter would dispatch and are noted; a concrete-typed
// carrier, a quoted value and a read with a Function-expecting forward
// pending (the value-delivery arrival) are not.
func TestNoteWordReadClassifies(t *testing.T) {
	es := &wordReadEmit{EmitRecorder: TheInactiveEmit}
	r := covRegistry(t, nil)
	r.Check.Emit = es
	e := NewTop(r)
	e.Tape = NewTape(nil, StackHeadroom)
	e.noteWordRead(NewCarrier(TFunction), "g", SrcPos{})
	e.noteWordRead(NewDynamicCarrier(TAny), "x", SrcPos{})
	e.noteWordRead(NewCarrier(TInteger), "n", SrcPos{})
	quoted := NewCarrier(TFunction)
	quoted.Quoted = true
	e.noteWordRead(quoted, "q", SrcPos{})
	if len(es.noted) != 2 || es.noted[0] != "g" || es.noted[1] != "x" {
		t.Errorf("fn-typed and gradual reads are noted, a concrete or quoted one is not: %v", es.noted)
	}
	// A pending forward whose next slot expects a Function delivers the
	// value: nothing to dispatch, nothing noted.
	sig := &Signature{Args: []*Type{TFunction}, BarrierPos: BarrierAllForward}
	NormalizeSig(sig)
	fwd := NewForward(ForwardInfo{FuncName: "sort", ExpectedArgs: 1, Sig: sig})
	e.Tape = NewTape([]Value{fwd, NewWord("g")}, StackHeadroom)
	e.Pointer = 1
	e.noteWordRead(NewCarrier(TFunction), "g", SrcPos{})
	if len(es.noted) != 2 {
		t.Errorf("a Function-expecting forward takes the value, no note: %v", es.noted)
	}
}

// TestFailingTupleStopsAtEngineMarkers pins the no-match diagnostic's
// failing-tuple walks (sigError's written tuple and its check-time twin
// rematchWritten): an engine marker after the pointer — a fn frame's tail
// DefCleanup `__dc`, a Mark, a Move — ends the tuple, so a dispatch that
// no-matches at a body's end never lists `__dc (a __DC)` as the argument
// the caller supplied; a plain value before the marker is still collected.
func TestFailingTupleStopsAtEngineMarkers(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	dc := NewDefCleanup(DefCleanupInfo{})
	if !isEngineMarker(dc) || !isEngineMarker(NewMark("m")) || !isEngineMarker(NewMove("t", "r")) || isEngineMarker(NewInteger(5)) {
		t.Fatal("isEngineMarker: markers are markers, a value is not")
	}
	e.Tape = NewTape([]Value{NewWord("g"), NewInteger(3), dc, NewInteger(9)}, StackHeadroom)
	e.Pointer = 0
	if got := reorderForwardCandidates(e.Tape, e.Pointer); len(got) != 1 {
		t.Errorf("the written tuple stops at the frame marker: %v", got)
	}
	if got := e.rematchWritten(); len(got) != 1 {
		t.Errorf("the check-time twin stops there too: %v", got)
	}
	e.Tape = NewTape([]Value{NewWord("g"), dc}, StackHeadroom)
	if got := reorderForwardCandidates(e.Tape, 0); len(got) != 0 {
		t.Errorf("a marker right after the word leaves an empty tuple: %v", got)
	}
}
