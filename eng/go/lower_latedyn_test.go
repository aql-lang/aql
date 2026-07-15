package eng

import "testing"

// singleOutputCall gates which dyn-bound source promoteLateDynBind can seat as
// a lone frame local. The recursion-blind arms (a nil event from a non-recursed
// multi-value arm, a native evCall, a multi-output producer, a non-call event)
// are off the voxgig corpus path, so pin them directly.
func TestSingleOutputCall(t *testing.T) {
	if singleOutputCall(nil) {
		t.Error("a nil event is not a single-output call")
	}
	if !singleOutputCall(&emitEvent{kind: evCall, call: emitCall{nout: 1}}) {
		t.Error("a single-result native call IS a single-output call")
	}
	if singleOutputCall(&emitEvent{kind: evCall, call: emitCall{nout: 2}}) {
		t.Error("a multi-result native call is not single-output")
	}
	if !singleOutputCall(&emitEvent{kind: evCallUser, uc: emitUserCall{nout: 1}}) {
		t.Error("a single-result user call IS a single-output call")
	}
	if singleOutputCall(&emitEvent{kind: evCallUser, uc: emitUserCall{nout: 0}}) {
		t.Error("a zero-result user call is not single-output")
	}
	if singleOutputCall(&emitEvent{kind: evBranch}) {
		t.Error("a branch value is not a call")
	}
}

// promoteLateDynBind's promotion tail became corpus-invisible once plan-time
// value-def promotion (planValueDefLocals) started seating every dyn-bound
// source the corpus produces BEFORE Finalize — every recorded evDynBind
// arrives with rec.promoted already carrying its srcSeq, so the late pass's
// own seat/rewrite arms no longer fire from any spec row. The pass is still
// the sound backstop for a unit finished before a later tryRecordDynBody
// arms DynEnv, so pin its contract directly: the seat + rewrite, each skip
// gate (fragment result, variadic producer, non-single-output, already
// promoted), and the disarmed no-ops.
func TestPromoteLateDynBind(t *testing.T) {
	// producer(0) — a single-output call whose value def 1 dyn-binds; a
	// consumer call (2) and the unit residual both reference event 0 and must
	// be rewritten to the seated local.
	mkRec := func() *fnUnitRec {
		return &fnUnitRec{
			numLoc: 3,
			frag: &EmitFragment{events: []emitEvent{
				{seq: 0, kind: evCall, call: emitCall{nout: 1}},
				{seq: 1, kind: evDynBind, dyn: &emitDynBind{name: "v", srcSeq: 0}},
				{seq: 2, kind: evCall, call: emitCall{nout: 1, ops: []emitOperand{eventOperand(0, 0)}}},
			}},
			outOps: []emitOperand{eventOperand(0, 0)},
		}
	}
	es := &EmitState{dynEnv: true, eventInfo: map[int]eventFlags{}}

	rec := mkRec()
	es.promoteLateDynBind(rec)
	if got, ok := rec.promoted[0]; !ok || got != 3 {
		t.Fatalf("promoted[0] = %d, %v — want seat at pre-pass numLoc 3", got, ok)
	}
	if rec.numLoc != 4 {
		t.Errorf("numLoc = %d, want 4 (one seat allocated)", rec.numLoc)
	}
	if op := rec.frag.events[2].call.ops[0]; op.kind != opLocal || op.idx != 3 {
		t.Errorf("consumer operand not rewritten to the seat: %+v", op)
	}
	if op := rec.outOps[0]; op.kind != opLocal || op.idx != 3 {
		t.Errorf("residual operand not rewritten to the seat: %+v", op)
	}

	// Skip gates — each leaves the rec unpromoted and unchanged.
	skip := func(name string, prep func(*fnUnitRec, *EmitState)) {
		r, s := mkRec(), &EmitState{dynEnv: true, eventInfo: map[int]eventFlags{}}
		prep(r, s)
		before := r.numLoc
		s.promoteLateDynBind(r)
		if r.numLoc != before {
			t.Errorf("%s: promoted anyway (numLoc %d -> %d)", name, before, r.numLoc)
		}
		if op := r.outOps[0]; op.kind != opEvent {
			t.Errorf("%s: residual operand rewritten: %+v", name, op)
		}
	}
	skip("fragment result", func(r *fnUnitRec, _ *EmitState) {
		// The producer is a branch arm's recorded out — it must stay on its sim.
		r.frag.events = append(r.frag.events, emitEvent{seq: 3, kind: evBranch,
			br: &emitBranch{thenOut: eventOperand(0, 0)}})
	})
	skip("variadic producer", func(_ *fnUnitRec, s *EmitState) {
		s.eventInfo[0] = eventFlags{variadicResult: true}
	})
	skip("multi-output producer", func(r *fnUnitRec, _ *EmitState) {
		r.frag.events[0].call.nout = 2
	})
	skip("already promoted at plan time", func(r *fnUnitRec, _ *EmitState) {
		r.promoted = map[int]int{0: 1}
	})

	// Disarmed / degenerate no-ops.
	off := &EmitState{eventInfo: map[int]eventFlags{}}
	offRec := mkRec()
	off.promoteLateDynBind(offRec)
	if offRec.promoted != nil {
		t.Error("a non-DynEnv program must be byte-for-byte unchanged")
	}
	es.promoteLateDynBind(nil)
	es.promoteLateDynBind(&fnUnitRec{})
}
