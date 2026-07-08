package eng

import (
	"testing"
)

// emit_dynapply_fnunit_test.go pins the fn-unit dynamic-apply classifier arms
// the corpus rows don't reach: dynFrameWindow's all-prefix decline,
// noteDynFrameReplay's refuse path, noteApplyLoopReplay's shape declines, and
// setLoopBodyApply's source-order gate.

func daUnit(ids ...string) (*emitUnit, *fnUnitRec) {
	u := &emitUnit{localByID: map[string]int{}}
	for i, id := range ids {
		u.localByID[id] = i
	}
	rec := &fnUnitRec{nParams: len(ids), nUnnamed: len(ids), returns: []*Type{TAny}}
	return u, rec
}

func daFnCarrier(id string) Value {
	v := NewCarrier(TFunction)
	v.ID = id
	return v
}

func TestDynFrameWindowArms(t *testing.T) {
	u, rec := daUnit("p0", "p1")
	// All entries are the unnamed-param re-pushes: no token region — decline.
	if _, ok := dynFrameWindow(u, rec, []Value{daFnCarrier("p0"), daFnCarrier("p1")}); ok {
		t.Error("an all-prefix residual has no token region and must decline")
	}
	// A read beyond the prefix forms the token region.
	if w, ok := dynFrameWindow(u, rec, []Value{daFnCarrier("p0"), daFnCarrier("p1"), daFnCarrier("p0")}); !ok || w != 1 {
		t.Errorf("window = %d/%v, want 1/true", w, ok)
	}
	// A NON-param-local at the bottom stops the prefix immediately: the whole
	// residual is the token region.
	if w, ok := dynFrameWindow(u, rec, []Value{daFnCarrier("q9"), daFnCarrier("p0")}); !ok || w != 2 {
		t.Errorf("non-local bottom window = %d/%v, want 2/true", w, ok)
	}
}

func TestNoteDynFrameReplayArms(t *testing.T) {
	u, rec := daUnit("p0", "p1")
	// A fn value beyond the exempt window whose residual is ALL prefix:
	// dynFrameWindow declines, so the scan keeps the refusal (false).
	if noteDynFrameReplay(u, rec, []Value{daFnCarrier("p0"), daFnCarrier("p1")}, 1) {
		t.Error("an undecomposable fn residual must keep the refusal")
	}
	if rec.retReplay {
		t.Error("a declined replay must not mark the rec")
	}
	// No fn value at all: nothing to replay, no refusal.
	if !noteDynFrameReplay(u, rec, []Value{NewInteger(1), NewInteger(2)}, 1) {
		t.Error("a fn-free residual never refuses here")
	}
	// A fn value with a token region seats the replay.
	u2, rec2 := daUnit("p0", "p1")
	if !noteDynFrameReplay(u2, rec2, []Value{daFnCarrier("p0"), daFnCarrier("p1"), daFnCarrier("p0")}, 2) {
		t.Error("a decomposable fn residual must seat the replay")
	}
	if rec2.dynFrameW != 1 || !rec2.retReplay {
		t.Errorf("replay window = %d retReplay=%v, want 1/true", rec2.dynFrameW, rec2.retReplay)
	}
}

func TestNoteApplyLoopReplayArms(t *testing.T) {
	es := NewEmitState()
	es.eventInfo[7] = eventFlags{variadicResult: true, applyLoop: true}
	es.eventInfo[8] = eventFlags{}
	loopOp := eventOperand(7, 0)
	rec := &fnUnitRec{nParams: 1, nUnnamed: 1, returns: []*Type{TAny}}

	// No apply-loop event: unchanged.
	ops := []emitOperand{eventOperand(8, 0)}
	if got := es.noteApplyLoopReplay(rec, ops); len(got) != 1 || rec.retReplay {
		t.Error("a residual without an apply-loop event is untouched")
	}
	// The loop sits deeper than the unnamed window: decline.
	rec0 := &fnUnitRec{nParams: 1, nUnnamed: 0, returns: []*Type{TAny}}
	ops = []emitOperand{localOperand(0), loopOp}
	if got := es.noteApplyLoopReplay(rec0, ops); rec0.retReplay || len(got) != 2 {
		t.Error("a prefix beyond the unnamed window must decline")
	}
	// A non-param-local prefix entry: decline.
	rec1 := &fnUnitRec{nParams: 1, nUnnamed: 1, returns: []*Type{TAny}}
	ops = []emitOperand{constOperand(0), loopOp}
	if got := es.noteApplyLoopReplay(rec1, ops); rec1.retReplay || len(got) != 2 {
		t.Error("a non-param prefix must decline")
	}
	// An EVENT above the loop region: decline (only inert tails seat).
	rec2 := &fnUnitRec{nParams: 1, nUnnamed: 1, returns: []*Type{TAny}}
	ops = []emitOperand{loopOp, eventOperand(8, 0)}
	if got := es.noteApplyLoopReplay(rec2, ops); rec2.retReplay || len(got) != 2 {
		t.Error("an event above the loop region must decline")
	}
	// The corpus shape: [param re-push, loop, inert tail] splits the prefix.
	rec3 := &fnUnitRec{nParams: 1, nUnnamed: 1, returns: []*Type{TAny}}
	ops = []emitOperand{localOperand(0), loopOp, localOperand(2)}
	got := es.noteApplyLoopReplay(rec3, ops)
	if !rec3.retReplay || len(rec3.retPrefix) != 1 || len(got) != 2 {
		t.Errorf("replay split = prefix %d + ops %d (retReplay=%v), want 1+2/true",
			len(rec3.retPrefix), len(got), rec3.retReplay)
	}
}

func TestSetLoopBodyApplySourceOrderGate(t *testing.T) {
	newES := func() (*EmitState, Value) {
		es := NewEmitState()
		es.units = append(es.units, &emitUnit{localByID: map[string]int{}})
		fn := daFnCarrier("fnv")
		es.units[len(es.units)-1].localByID["fnv"] = 0
		return es, fn
	}
	arg := func(row, col int) Value {
		v := NewInteger(1)
		v.Pos = SrcPos{Row: row, Col: col}
		return v
	}
	evAt := func(row, col int) emitEvent {
		return emitEvent{kind: evCall, call: emitCall{pos: SrcPos{Row: row, Col: col}}}
	}

	// No source anchor anywhere: decline.
	es, fn := newES()
	body := &EmitFragment{}
	if es.setLoopBodyApply(body, []Value{fn, NewInteger(1)}) {
		t.Error("an apply with no source anchor cannot be ordered — decline")
	}
	// Events strictly BEFORE the apply: the apply-last hoist.
	es, fn = newES()
	body = &EmitFragment{events: []emitEvent{evAt(1, 5), {kind: evCall, call: emitCall{}}}}
	if !es.setLoopBodyApply(body, []Value{fn, arg(1, 40)}) || body.applyFirst {
		t.Errorf("events before the apply must seat apply-LAST (applyFirst=%v)", body.applyFirst)
	}
	// Events strictly AFTER: apply-FIRST (a zero-pos event counts as neither).
	es, fn = newES()
	body = &EmitFragment{events: []emitEvent{evAt(1, 60)}}
	if !es.setLoopBodyApply(body, []Value{fn, arg(1, 10)}) || !body.applyFirst {
		t.Errorf("events after the apply must seat apply-FIRST (applyFirst=%v)", body.applyFirst)
	}
	// Events on BOTH sides: a mid-body apply — decline.
	es, fn = newES()
	body = &EmitFragment{events: []emitEvent{evAt(1, 5), evAt(1, 60)}}
	if es.setLoopBodyApply(body, []Value{fn, arg(1, 30)}) {
		t.Error("a mid-body apply cannot seat at either end — decline")
	}
	// A fn operand that is neither a frame local nor an event (a baked inert
	// fn CONST recovered via its original) cannot seat the per-iteration
	// apply — decline.
	es, _ = newES()
	cfn := NewCarrier(TFunction)
	cfn.ID = "cfn"
	es.origByID["cfn"] = NewFnDef(FnDefInfo{Name: "", Signatures: []Signature{{Returns: []*Type{TAny}, BarrierPos: -1}}})
	body = &EmitFragment{}
	if es.setLoopBodyApply(body, []Value{cfn, arg(1, 10)}) {
		t.Error("a const fn operand must decline the loop apply")
	}
}
