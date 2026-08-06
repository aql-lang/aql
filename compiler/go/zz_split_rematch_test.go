package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestW8DispatchRematchDeclines(t *testing.T) {
	// Inactive recorder + interface no-op.
	var nilES *EmitState
	if nilES.RecordDispatchRematchValues("w", []core.Value{core.NewInteger(1)}, 0, 1, core.SrcPos{}) {
		t.Error("inactive EmitState must decline")
	}
	if core.TheInactiveEmit.RecordDispatchRematchValues("w", []core.Value{core.NewInteger(1)}, 0, 1, core.SrcPos{}) {
		t.Error("the inactive recorder must decline")
	}

	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es, _ := r.Check.Recorder().(*EmitState)
	es.BindRegistry(r)
	if es.RecordDispatchRematchValues("w", nil, 0, 1, core.SrcPos{}) {
		t.Error("an empty window must decline")
	}
	// A dynamic value with no provenance fails resolveOperand.
	dyn := core.NewCarrier(core.TAny)
	dyn.Dynamic = true
	dyn.ID = ""
	if es.RecordDispatchRematchValues("w", []core.Value{dyn}, 0, 1, core.SrcPos{}) {
		t.Error("an unresolvable operand must decline")
	}
	if es.RecordDispatchRematch("", []EmitOperand{ConstOperand(0)}, 0, 1, core.SrcPos{}) {
		t.Error("an empty word must decline")
	}
	if es.RecordDispatchRematch("w", nil, 0, 1, core.SrcPos{}) {
		t.Error("no operands must decline")
	}
	// The render bound must be a contiguous in-window slice: a zero length,
	// a negative offset, and a slice past the window all decline.
	if es.RecordDispatchRematch("w", []EmitOperand{ConstOperand(0)}, 0, 0, core.SrcPos{}) {
		t.Error("a zero render bound must decline")
	}
	if es.RecordDispatchRematch("w", []EmitOperand{ConstOperand(0)}, -1, 1, core.SrcPos{}) {
		t.Error("a negative render offset must decline")
	}
	if es.RecordDispatchRematch("w", []EmitOperand{ConstOperand(0)}, 1, 1, core.SrcPos{}) {
		t.Error("a render slice past the window must decline")
	}
	// First trap wins: with a trap latched, a second record reports owned.
	if !es.RecordDispatchRematch("w", []EmitOperand{ConstOperand(0)}, 0, 1, core.SrcPos{}) {
		t.Fatal("the first rematch record must land")
	}
	if !es.RecordDispatchRematch("w2", []EmitOperand{ConstOperand(1)}, 0, 1, core.SrcPos{}) {
		t.Error("a second record after the latch must report owned (true), not re-record")
	}

	// The promoted-operand rewrite reaches a rematch trap's window.
	ev := EmitEvent{kind: evTrap, trap: EmitTrap{
		rematchWord: "w",
		rematchOps:  []EmitOperand{EventOperand(0, 0)},
	}}
	RewritePromotedRefs(&ev, map[int]int{0: 3})
	if op := ev.trap.rematchOps[0]; op.kind != opLocal || op.idx != 3 {
		t.Errorf("rematch operand not rewritten to the promoted local: %+v", op)
	}
}
