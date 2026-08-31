package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// RecordBindTwin's contract (§6.5's inert-emission stage): unconditional
// append — no Active()/Suspend gate, because NoteBindTransition already
// applied every ledger suppression and a second filter would be a second
// source of truth — nil-receiver safe like every EmitState method, and the
// finalized Program carries a COPY, so a later append on the same EmitState
// cannot mutate a delivered Program.
func TestRecordBindTwinAppendsUnconditionally(t *testing.T) {
	var nilES *EmitState
	nilES.RecordBindTwin(core.BindTransition{Name: "x"}) // must not panic

	es := NewEmitState()
	resume := es.Suspend()
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "a", Depth: 1})
	resume()
	es.RecordBindTwin(core.BindTransition{Kind: core.BindUndef, Name: "a", Depth: 0})
	if len(es.bindTwins) != 2 {
		t.Fatalf("twin table = %d entries, want 2 (suspension must not filter)", len(es.bindTwins))
	}

	prog, _, ok := es.Finalize(nil)
	if !ok {
		t.Fatal("an empty compilable state must finalize")
	}
	if len(prog.BindTwins) != 2 || prog.BindTwins[0].Name != "a" ||
		prog.BindTwins[0].Kind != core.BindDef || prog.BindTwins[1].Kind != core.BindUndef {
		t.Fatalf("finalized twin table = %v, want the recorded entries verbatim", prog.BindTwins)
	}
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "late", Depth: 1})
	if len(prog.BindTwins) != 2 {
		t.Fatal("the Program's table is a copy; a later append must not reach it")
	}
}
