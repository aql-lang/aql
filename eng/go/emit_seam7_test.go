package eng

import "testing"

// This file drives the bytecode recorder (emit.go / emit_recorder.go) for
// coverage of its refusal gates, provenance helpers, and no-op inactive
// recorder. Tests are in-package so they can construct EmitState / Value /
// emitCall / emitBranch values directly and call unexported methods — the
// direct-call seam the fleet brief authorises for arms the recorder cannot
// reach from a valid eng-only token program.

// --- inactiveEmit: the no-op recorder -----------------------------------

// TestInactiveEmitMethods calls every EmitRecorder method on the shared no-op
// recorder. Each returns the inactive/none answer; the point is coverage of
// the method bodies, plus a sanity check that the answers match the "recording
// is not live" contract.
func TestInactiveEmitMethods(t *testing.T) {
	e := theInactiveEmit

	if e.Active() || e.active() || e.Armed() || e.suspendedNow() {
		t.Fatal("inactive recorder reports live/armed")
	}
	if !e.topFrameOnly() {
		t.Fatal("inactive recorder should be top-frame")
	}
	// Suspend / guards return callable no-op funcs.
	e.Suspend()()
	e.bodyAnalysisGuard()()
	e.fnBodyGuard()()
	e.bindRegistry(nil)

	e.MarkUncompilable("x")
	if e.Sites() != nil {
		t.Fatal("inactive Sites should be nil")
	}
	if p, why, ok := e.Finalize(nil); p != nil || ok || why == "" {
		t.Fatalf("inactive Finalize = %v %q %v", p, why, ok)
	}

	e.RecordCall("w", nil, nil, nil, SrcPos{}, false, false)
	e.RecordPoly("w")
	if e.RecordPolyCall("w", nil, nil, SrcPos{}, nil) {
		t.Fatal("inactive RecordPolyCall should refuse")
	}
	e.RecordUserCall(0, nil, nil, SrcPos{})
	e.RecordUserPolyCall("w", nil, nil, nil, nil, nil, nil, SrcPos{})
	if e.RecordDynApply(nil, Value{}, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordDynApply should refuse")
	}
	if e.RecordDynMethod(Value{}, nil, nil, "w", SrcPos{}) {
		t.Fatal("inactive RecordDynMethod should refuse")
	}
	if e.RecordFallback(FallbackSpan{}, nil, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordFallback should refuse")
	}
	if e.RecordTrap("c", "d", "w", "h", SrcPos{}) {
		t.Fatal("inactive RecordTrap should refuse")
	}
	in := NewInteger(3)
	if out, ok := e.RecordTypedBind(TypedBindSpec{}, in, in, SrcPos{}); ok || out.ID != in.ID {
		t.Fatalf("inactive RecordTypedBind = %v %v", out, ok)
	}
	if e.RecordMakeList(nil, nil, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordMakeList should refuse")
	}
	if e.recordMakeListInner(nil, nil, Value{}, SrcPos{}) {
		t.Fatal("inactive recordMakeListInner should refuse")
	}
	if e.RecordMakeMap(nil, nil, nil, false, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordMakeMap should refuse")
	}
	if e.RecordInterp(nil, nil, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordInterp should refuse")
	}
	e.RegisterTrailingApply("id", 1)
	e.noteMemberFnRead("id")
	if e.memberFnRead("id") {
		t.Fatal("inactive memberFnRead should be false")
	}
	if e.dynInputsProven(nil, nil) {
		t.Fatal("inactive dynInputsProven should be false")
	}
	if v, ok := e.materialise(in); ok || v.ID != in.ID {
		t.Fatalf("inactive materialise = %v %v", v, ok)
	}
	if op, ok := e.resolveOperand(in); ok || op.kind != opNone {
		t.Fatalf("inactive resolveOperand = %v %v", op, ok)
	}
	if e.zeroOutProduced("id") || e.alreadyProduced("id") {
		t.Fatal("inactive produced probes should be false")
	}

	e.MarkValueDef(in)
	e.RecordDefRebind("n", in, SrcPos{})
	e.RefuseCarriedUndef("n")
	if e.RegisterLocal("id") != -1 {
		t.Fatal("inactive RegisterLocal should be -1")
	}
	e.RememberOriginal(in)
	e.RememberStrippedOriginals(nil, nil)

	e.ArmBranchCapture()
	e.ArmLoopCapture()
	if e.ConsumeLoopArm() {
		t.Fatal("inactive ConsumeLoopArm should be false")
	}
	if e.TakeFragment() != nil {
		t.Fatal("inactive TakeFragment should be nil")
	}
	e.RecordBranch(BranchRecord{})
	e.RecordLoop(Value{}, Value{}, Value{}, nil, nil, "it", Value{}, SrcPos{})
	e.BeginLoopCarried()
	e.EndLoopCarried()
	e.NoteLoopCarried("n", Value{}, Value{})
	e.Rollback(e.Checkpoint())
	if e.CanSeatAcrossFragment(in) {
		t.Fatal("inactive CanSeatAcrossFragment should be false")
	}

	unit, finish, ok := e.StartFnCompile("k", "n", nil, nil, nil, nil, nil, false, SrcPos{})
	if unit != -1 || finish != nil || ok {
		t.Fatalf("inactive StartFnCompile = %d finish-nil=%v %v", unit, finish == nil, ok)
	}
	e.SetUnitParamTypes(0, nil, nil)
	if e.unitVariadic(0) {
		t.Fatal("inactive unitVariadic should be false")
	}
}

// TestRecorderAccessorFallback covers CheckState.Recorder()'s inactive
// fallback: a nil CheckState and a CheckState with no installed Emit both
// return the shared no-op recorder.
func TestRecorderAccessorFallback(t *testing.T) {
	var c *CheckState
	if c.Recorder() != theInactiveEmit {
		t.Fatal("nil CheckState should yield the inactive recorder")
	}
	c2 := &CheckState{}
	if c2.Recorder() != theInactiveEmit {
		t.Fatal("empty CheckState should yield the inactive recorder")
	}
}

// --- EmitState nil-receiver guards --------------------------------------

// TestEmitStateNilReceiver calls the nil-safe *EmitState methods on a nil
// pointer: every one has an `es == nil` guard and must not panic.
func TestEmitStateNilReceiver(t *testing.T) {
	var es *EmitState

	if es.Armed() {
		t.Fatal("nil EmitState is not armed")
	}
	if es.active() || es.suspendedNow() {
		t.Fatal("nil EmitState is not active")
	}
	if !es.topFrameOnly() {
		t.Fatal("nil EmitState is top-frame")
	}
	es.bindRegistry(nil)
	if es.Sites() != nil {
		t.Fatal("nil Sites should be nil")
	}
	if es.zeroOutProduced("x") || es.alreadyProduced("x") {
		t.Fatal("nil produced probes should be false")
	}
	if es.unitVariadic(0) {
		t.Fatal("nil unitVariadic should be false")
	}
	es.RegisterTrailingApply("id", 1)
	if es.TrailingApplyArity("id") != 0 {
		t.Fatal("nil TrailingApplyArity should be 0")
	}
	es.MarkUncompilable("x")
	es.Suspend()()
	if es.TakeFragment() != nil {
		t.Fatal("nil TakeFragment should be nil")
	}
	es.MarkValueDef(NewInteger(1))
	if es.OperandRepushable(NewInteger(1)) {
		t.Fatal("nil OperandRepushable should be false")
	}
	if es.CanSeatAcrossFragment(NewInteger(1)) {
		t.Fatal("nil CanSeatAcrossFragment should be false")
	}
	if _, ok := es.tryReturnedClosure(NewInteger(1), SrcPos{}); ok {
		t.Fatal("nil tryReturnedClosure should refuse")
	}
}

// TestTrailingApplyArityRoundTrip covers the non-nil happy path: after
// RegisterTrailingApply the arity reads back, and an unregistered id is 0.
func TestTrailingApplyArityRoundTrip(t *testing.T) {
	es := NewEmitState()
	if es.TrailingApplyArity("id") != 0 {
		t.Fatal("unregistered arity should be 0")
	}
	es.RegisterTrailingApply("id", 3)
	if got := es.TrailingApplyArity("id"); got != 3 {
		t.Fatalf("arity = %d, want 3", got)
	}
	// arity < 1 and empty id are rejected.
	es.RegisterTrailingApply("", 2)
	es.RegisterTrailingApply("id2", 0)
	if es.TrailingApplyArity("id2") != 0 {
		t.Fatal("arity<1 should not register")
	}
}

// --- direct-call helpers -------------------------------------------------

func ptrVal(v Value) *Value { return &v }

// seedProduced makes v resolve to an event operand: resolveOperand finds a
// producedBy entry (and v is not a type body, so the events-first arm wins).
func seedProduced(es *EmitState, v Value, seq int) {
	es.producedBy[v.ID] = producer{seq: seq, idx: 0}
}

// carrierVal is an unresolvable operand: a stripped carrier with no recorded
// original, so materialise (and thus resolveOperand) refuses it.
func carrierVal(t *Type) Value { return NewCarrier(t) }

// --- pure helpers: eventDivergesDeep / computedArmCondOK -----------------

func TestEventDivergesDeepArms(t *testing.T) {
	// else arm is a plain value (armValue) → never diverges.
	evVal := emitEvent{kind: evBranch, br: &emitBranch{hasElse: true, elsIsVal: true}}
	if eventDivergesDeep(&evVal) {
		t.Fatal("value-else branch should not diverge")
	}
	// else arm is computed (armComputed) → never diverges.
	evComp := emitEvent{kind: evBranch, br: &emitBranch{hasElse: true, elsIsVal: true, elsComputed: true}}
	if eventDivergesDeep(&evComp) {
		t.Fatal("computed-else branch should not diverge")
	}
	// A non-control event kind falls through to the trailing return false.
	evStoreEv := emitEvent{kind: evStore, store: &emitStore{}}
	if eventDivergesDeep(&evStoreEv) {
		t.Fatal("store event should not diverge")
	}
}

func TestComputedArmCondOKDirect(t *testing.T) {
	tru := true
	// ConstCond set → false (the disjoint const-cond path owns it).
	if computedArmCondOK(BranchRecord{ConstCond: &tru}, emitOperand{}) {
		t.Fatal("const-cond should not be a computed-arm cond")
	}
	// opNone / default cond → false.
	if computedArmCondOK(BranchRecord{}, emitOperand{kind: opNone}) {
		t.Fatal("opNone cond should be rejected")
	}
	// A stack event cond → true.
	if !computedArmCondOK(BranchRecord{}, emitOperand{kind: opEvent}) {
		t.Fatal("event cond should be accepted")
	}
}

func TestStripZeroOutPhantomsKeepsNonPhantom(t *testing.T) {
	es := NewEmitState()
	phantom := NewInteger(1)
	kept := NewInteger(2)
	seedProduced(es, phantom, 1)
	f := es.eventInfo[1]
	f.zeroOut = true
	es.eventInfo[1] = f
	seedProduced(es, kept, 2) // eventInfo[2].zeroOut defaults false
	got := es.stripZeroOutPhantoms([]Value{phantom, kept})
	if len(got) != 1 || got[0].ID != kept.ID {
		t.Fatalf("strip = %v, want just the kept value", got)
	}
}

// --- embedsEnclosingCompound / returnsAllScalar --------------------------

func TestEmbedsEnclosingCompound(t *testing.T) {
	// MapPayload with nil backing map → false (the nil-map guard).
	nilMap := Value{Parent: TMap, Data: MapPayload{M: nil}}
	if embedsEnclosingCompound(nilMap, map[string]bool{}) {
		t.Fatal("nil-map should not embed")
	}
	// A map whose (compound) member is an enclosing binding's value → true.
	member := NewList([]Value{NewInteger(9)})
	om := NewOrderedMap()
	om.Set("k", member)
	mp := NewMap(om)
	if !embedsEnclosingCompound(mp, map[string]bool{member.ID: true}) {
		t.Fatal("map embedding an enclosing compound should report true")
	}
}

func TestReturnsAllScalarEmpty(t *testing.T) {
	if returnsAllScalar(nil) {
		t.Fatal("no declared returns should not be all-scalar")
	}
}

// --- materialise list/map rebuild ---------------------------------------

func TestMaterialiseRebuild(t *testing.T) {
	es := NewEmitState()
	// List whose member is a stripped carrier recoverable to a concrete int.
	member := NewCarrier(TInteger)
	orig := NewInteger(7)
	es.origByID[member.ID] = orig
	lst := NewList([]Value{member})
	got, ok := es.materialise(lst)
	if !ok {
		t.Fatal("list with a recoverable member should materialise")
	}
	lp, isList := got.Data.(ListPayload)
	if !isList || len(lp.Elems) != 1 || lp.Elems[0].Carrier {
		t.Fatalf("rebuilt list member not recovered: %v", got)
	}

	// Map with nil backing → not materialisable.
	nilMap := Value{Parent: TMap, Data: MapPayload{M: nil}}
	if _, ok := es.materialise(nilMap); ok {
		t.Fatal("nil-map should not materialise")
	}

	// Map with a recoverable member gets rebuilt.
	m2 := NewCarrier(TInteger)
	es.origByID[m2.ID] = NewInteger(11)
	om := NewOrderedMap()
	om.Set("k", m2)
	mp := NewMap(om)
	got2, ok2 := es.materialise(mp)
	if !ok2 {
		t.Fatal("map with a recoverable member should materialise")
	}
	mpOut, isMap := got2.Data.(MapPayload)
	if !isMap || mpOut.M == nil {
		t.Fatalf("rebuilt map missing backing: %v", got2)
	}
	if rv, _ := mpOut.M.Get("k"); rv.Carrier {
		t.Fatal("rebuilt map member not recovered")
	}
}

// --- eventPos / eventsThroughSeq ----------------------------------------

func TestEventPosStore(t *testing.T) {
	p := SrcPos{}
	ev := emitEvent{kind: evStore, store: &emitStore{pos: p}}
	if eventPos(ev) != p {
		t.Fatal("eventPos(store) mismatch")
	}
}

func TestEventsThroughSeqMiss(t *testing.T) {
	evs := []emitEvent{{seq: 1}, {seq: 2}}
	// A seq not present returns the whole slice.
	if got := eventsThroughSeq(evs, 99); len(got) != 2 {
		t.Fatalf("miss should return all events, got %d", len(got))
	}
}

// --- record-method guards: inactive / unresolvable ----------------------

func inactiveEmitState() *EmitState {
	es := NewEmitState()
	es.Compilable = false
	return es
}

func TestRecordUserPolyCallGuards(t *testing.T) {
	// Inactive → no-op.
	inactiveEmitState().RecordUserPolyCall("w", nil, nil, nil, nil, nil, nil, SrcPos{})
	// Active with an unresolvable operand → uncompilable.
	es := NewEmitState()
	es.RecordUserPolyCall("w", nil, nil, nil, nil, []Value{carrierVal(TInteger)}, []Value{NewInteger(1)}, SrcPos{})
	if es.Compilable {
		t.Fatal("unresolvable poly operand should refuse")
	}
}

func TestRecordDynApplyDeclines(t *testing.T) {
	// Inactive → false.
	if inactiveEmitState().RecordDynApply(nil, NewCarrier(TFunction), NewInteger(0), SrcPos{}) {
		t.Fatal("inactive RecordDynApply should decline")
	}
	// Non-fn callee → false.
	es := NewEmitState()
	if es.RecordDynApply(nil, NewInteger(5), NewInteger(0), SrcPos{}) {
		t.Fatal("non-fn callee should decline")
	}
	// Fn-typed carrier but unresolvable → false.
	if es.RecordDynApply(nil, NewCarrier(TFunction), NewInteger(0), SrcPos{}) {
		t.Fatal("unresolvable fn callee should decline")
	}
	// Fn resolves (a dynamic fn carrier resolves to an event operand), but an
	// ARG is itself a fn value → false.
	fn := NewDynamicCarrier(TFunction)
	seedProduced(es, fn, 1)
	if es.RecordDynApply([]Value{NewCarrier(TFunction)}, fn, NewInteger(0), SrcPos{}) {
		t.Fatal("fn-valued arg should decline")
	}
	// Fn resolves, an ARG is unresolvable → false.
	if es.RecordDynApply([]Value{carrierVal(TInteger)}, fn, NewInteger(0), SrcPos{}) {
		t.Fatal("unresolvable arg should decline")
	}
}

func TestRecordDynApplyPendingConsume(t *testing.T) {
	es := NewEmitState()
	fn := NewDynamicCarrier(TFunction)
	seedProduced(es, fn, 1)
	es.units[len(es.units)-1].pendingApply = []string{fn.ID}
	out := NewInteger(0)
	if !es.RecordDynApply([]Value{NewInteger(10)}, fn, out, SrcPos{}) {
		t.Fatal("resolvable apply with pending entry should record")
	}
	if len(es.units[len(es.units)-1].pendingApply) != 0 {
		t.Fatal("pending apply entry should be consumed")
	}
}

func TestRecordLoopRefusals(t *testing.T) {
	// body == nil.
	es := NewEmitState()
	es.RecordLoop(NewInteger(0), NewInteger(5), NewInteger(1), nil, nil, "it", NewInteger(0), SrcPos{})
	if es.Compilable {
		t.Fatal("nil loop body should refuse")
	}
	// range of unknown provenance (start unresolvable).
	es = NewEmitState()
	es.RecordLoop(carrierVal(TInteger), NewInteger(5), NewInteger(1), &EmitFragment{}, nil, "it", NewInteger(0), SrcPos{})
	if es.Compilable {
		t.Fatal("unresolvable range should refuse")
	}
	// computed start/step (start resolves to an event, not a const).
	es = NewEmitState()
	start := NewCarrier(TInteger)
	seedProduced(es, start, 1)
	es.RecordLoop(start, NewInteger(5), NewInteger(1), &EmitFragment{}, nil, "it", NewInteger(0), SrcPos{})
	if es.Compilable {
		t.Fatal("computed loop start should refuse")
	}
	// iterator slot not registered (all-const range, empty body).
	es = NewEmitState()
	es.RecordLoop(NewInteger(0), NewInteger(5), NewInteger(1), &EmitFragment{}, nil, "it", NewInteger(0), SrcPos{})
	if es.Compilable {
		t.Fatal("unregistered iterator should refuse")
	}
}

func TestSetLoopBodyApplyDeclines(t *testing.T) {
	es := NewEmitState()
	// bodyStk[0] fn-typed carrier but not an event operand → false.
	if es.setLoopBodyApply(&EmitFragment{}, []Value{NewCarrier(TFunction), NewInteger(1)}) {
		t.Fatal("non-event fn head should decline")
	}
	// bodyStk[0] resolves to an event (dynamic fn carrier); a fn-valued
	// trailing arg → false.
	fn := NewDynamicCarrier(TFunction)
	seedProduced(es, fn, 1)
	if es.setLoopBodyApply(&EmitFragment{}, []Value{fn, NewCarrier(TFunction)}) {
		t.Fatal("fn-valued loop-apply arg should decline")
	}
}

func TestRecordFallbackGuards(t *testing.T) {
	if inactiveEmitState().RecordFallback(FallbackSpan{}, nil, NewInteger(0), SrcPos{}) {
		t.Fatal("inactive RecordFallback should decline")
	}
	es := NewEmitState()
	if es.RecordFallback(FallbackSpan{}, []Value{carrierVal(TInteger)}, NewInteger(0), SrcPos{}) {
		t.Fatal("unresolvable fallback input should decline")
	}
}

func TestRecordTypedBindDeclines(t *testing.T) {
	es := NewEmitState()
	// Concrete body → declines (returns out unchanged, false).
	in := NewInteger(5)
	if out, ok := es.RecordTypedBind(TypedBindSpec{}, in, in, SrcPos{}); ok || out.ID != in.ID {
		t.Fatal("concrete typed-bind body should decline")
	}
	// Non-concrete but unresolvable body → declines.
	c := NewCarrier(TInteger)
	if _, ok := es.RecordTypedBind(TypedBindSpec{}, c, NewInteger(0), SrcPos{}); ok {
		t.Fatal("unresolvable typed-bind body should decline")
	}
}

func TestRecordPolyMarks(t *testing.T) {
	es := NewEmitState()
	es.RecordPoly("someword")
	if es.Compilable {
		t.Fatal("RecordPoly should mark uncompilable")
	}
	if es.SiteCounts[SitePoly] != 1 {
		t.Fatal("RecordPoly should tally a poly site")
	}
}

func TestSetUnitParamTypesGuard(t *testing.T) {
	NewEmitState().SetUnitParamTypes(-1, nil, nil) // out-of-range → no-op
}

func TestStartFnCompileEmptyParamNames(t *testing.T) {
	es := NewEmitState()
	a1, a2 := NewCarrier(TInteger), NewCarrier(TInteger)
	unit, finish, ok := es.StartFnCompile("k", "fn", nil, []Value{a1, a2}, nil, nil, nil, false, SrcPos{})
	if !ok || finish == nil {
		t.Fatalf("StartFnCompile should open a fresh unit: ok=%v", ok)
	}
	finish(nil) // close cleanly with an empty body residual
	_ = unit
}

// --- RecordBranch refusal arms ------------------------------------------

func TestRecordBranchRefusals(t *testing.T) {
	// condFrag present but empty stack → refuse.
	es := NewEmitState()
	es.RecordBranch(BranchRecord{CondFrag: &EmitFragment{}, CondStk: nil})
	if es.Compilable {
		t.Fatal("empty condition body should refuse")
	}
	// condFrag result unresolvable.
	es = NewEmitState()
	es.RecordBranch(BranchRecord{CondFrag: &EmitFragment{}, CondStk: []Value{carrierVal(TInteger)}})
	if es.Compilable {
		t.Fatal("unresolvable condition result should refuse")
	}
	// default cond unresolvable.
	es = NewEmitState()
	es.RecordBranch(BranchRecord{Cond: carrierVal(TInteger)})
	if es.Compilable {
		t.Fatal("unresolvable condition should refuse")
	}
	// const-cond with an uncaptured then arm.
	es = NewEmitState()
	tru := true
	es.RecordBranch(BranchRecord{ConstCond: &tru, Then: nil})
	if es.Compilable {
		t.Fatal("uncaptured taken arm should refuse")
	}
	// then VALUE unresolvable (else-form).
	es = NewEmitState()
	es.RecordBranch(BranchRecord{Cond: NewInteger(1), HasElse: true, ThenValue: ptrVal(carrierVal(TInteger))})
	if es.Compilable {
		t.Fatal("unresolvable then value should refuse")
	}
	// else VALUE unresolvable.
	es = NewEmitState()
	es.RecordBranch(BranchRecord{Cond: NewInteger(1), HasElse: true, ThenValue: ptrVal(NewInteger(2)), ElsValue: ptrVal(carrierVal(TInteger))})
	if es.Compilable {
		t.Fatal("unresolvable else value should refuse")
	}
}

// --- fn-value / container-member classifiers ----------------------------

// fnVal builds a concrete Function value carrying the given signatures.
func fnVal(sigs ...Signature) Value {
	return Value{ID: GenerateID(IDPrefixForType(TFunction)), Parent: TFunction, Data: FnDefInfo{Signatures: sigs}}
}

// zeroArgFn is a genuine 0-param fn value (two 0-arg sigs → real + phantom),
// the shape the interpreter auto-dispatches when it lands with no operands.
func zeroArgFn() Value { return fnVal(Signature{}, Signature{}) }

// instanceVal builds a flat class instance whose fields map is `om`.
func instanceVal(om *OrderedMap) Value {
	return Value{ID: GenerateID(IDPrefixForType(TAny)), Parent: TAny, Data: ClassInstanceInfo{Fields: om}}
}

func s7MapOf(key string, v Value) Value {
	om := NewOrderedMap()
	om.Set(key, v)
	return NewMap(om)
}

func TestFnValueZeroArg(t *testing.T) {
	if !fnValueZeroArg(zeroArgFn()) {
		t.Fatal("two 0-arg sigs should be a genuine 0-param fn")
	}
	if !fnValueZeroArg(fnVal(Signature{})) {
		t.Fatal("single 0-arg sig should be a genuine 0-param fn")
	}
	// A lone 0-arg sig among others is just the phantom.
	if fnValueZeroArg(fnVal(Signature{Args: []*Type{TInteger}}, Signature{})) {
		t.Fatal("phantom-only 0-arg among args should not count")
	}
	if fnValueZeroArg(NewInteger(1)) {
		t.Fatal("non-fn value is not a 0-param fn")
	}
}

func TestZeroArgFnOut(t *testing.T) {
	if !zeroArgFnOut([]Value{NewInteger(1), zeroArgFn()}) {
		t.Fatal("a concrete 0-param fn out should be flagged")
	}
	if zeroArgFnOut([]Value{NewInteger(1)}) {
		t.Fatal("no fn out should not be flagged")
	}
}

func TestInstanceFnFieldRiskUnresolvableKey(t *testing.T) {
	es := NewEmitState()
	inst := instanceVal(mapOfFields())
	es.fnRiskFields = map[string]map[string]bool{inst.ID: {"f": true}}
	// No concrete key among args → any tracked field is risky → true.
	if !es.instanceFnFieldRisk([]Value{inst}) {
		t.Fatal("unresolvable key over a tracked instance should be risky")
	}
	// Concrete key that is NOT risky → false.
	if es.instanceFnFieldRisk([]Value{inst, NewAtom("other")}) {
		t.Fatal("concrete non-risky key should not be risky")
	}
	// Empty risk table → false.
	if (NewEmitState()).instanceFnFieldRisk([]Value{inst}) {
		t.Fatal("no risk table should be false")
	}
}

func mapOfFields() *OrderedMap {
	om := NewOrderedMap()
	om.Set("f", zeroArgFn())
	return om
}

func TestNoteMemberFnReadGuards(t *testing.T) {
	NewEmitState().noteMemberFnRead("") // empty id → no-op
	inactiveEmitState().noteMemberFnRead("id")
	es := NewEmitState()
	es.noteMemberFnRead("id")
	if !es.memberFnRead("id") {
		t.Fatal("a noted member-fn read should be readable")
	}
}

func TestReadsFnMember(t *testing.T) {
	fn := fnVal(Signature{Args: []*Type{TInteger}})
	// list, concrete int key → fn at that index.
	if !readsFnMember([]Value{NewList([]Value{fn}), NewInteger(0)}) {
		t.Fatal("list int-key fn should be reported")
	}
	// list, no key → loop finds the fn.
	if !readsFnMember([]Value{NewList([]Value{fn})}) {
		t.Fatal("list loop fn should be reported")
	}
	// nil-backed map → skipped.
	if readsFnMember([]Value{{Parent: TMap, Data: MapPayload{M: nil}}}) {
		t.Fatal("nil map should be skipped")
	}
	// map, no key → loop finds the fn.
	if !readsFnMember([]Value{s7MapOf("f", fn)}) {
		t.Fatal("map loop fn should be reported")
	}
	// flat instance, concrete key → fn field.
	if !readsFnMember([]Value{instanceVal(mapOf2("f", fn)), NewAtom("f")}) {
		t.Fatal("instance key fn should be reported")
	}
	// flat instance, no key → loop finds the fn.
	if !readsFnMember([]Value{instanceVal(mapOf2("f", fn))}) {
		t.Fatal("instance loop fn should be reported")
	}
}

func mapOf2(key string, v Value) *OrderedMap {
	om := NewOrderedMap()
	om.Set(key, v)
	return om
}

func TestContainerFnAutoDispatchRisk(t *testing.T) {
	fn := zeroArgFn()
	if !containerFnAutoDispatchRisk([]Value{NewList([]Value{fn}), NewInteger(0)}) {
		t.Fatal("list int-key 0-arg fn should be risky")
	}
	if !containerFnAutoDispatchRisk([]Value{NewList([]Value{fn})}) {
		t.Fatal("list loop 0-arg fn should be risky")
	}
	if containerFnAutoDispatchRisk([]Value{{Parent: TMap, Data: MapPayload{M: nil}}}) {
		t.Fatal("nil map should be skipped")
	}
	if !containerFnAutoDispatchRisk([]Value{s7MapOf("f", fn)}) {
		t.Fatal("map loop 0-arg fn should be risky")
	}
	if !containerFnAutoDispatchRisk([]Value{instanceVal(mapOf2("f", fn)), NewAtom("f")}) {
		t.Fatal("instance key 0-arg fn should be risky")
	}
	if !containerFnAutoDispatchRisk([]Value{instanceVal(mapOf2("f", fn))}) {
		t.Fatal("instance loop 0-arg fn should be risky")
	}
}

// --- producerWord / producerReturnedClosureArity ------------------------

func TestProducerWordMiss(t *testing.T) {
	es := NewEmitState()
	v := NewInteger(1)
	seedProduced(es, v, 99) // producedBy hit, but no frame event with seq 99
	if _, ok := es.producerWord(v.ID); ok {
		t.Fatal("no matching frame event should miss")
	}
}

func TestProducerReturnedClosureArity(t *testing.T) {
	// id not produced → false.
	es := NewEmitState()
	if _, ok := es.producerReturnedClosureArity("nope"); ok {
		t.Fatal("unproduced id should decline")
	}
	// produced, but the frame has only a non-matching seq → loop then trailing miss.
	es = NewEmitState()
	es.producedBy["x"] = producer{seq: 99}
	es.frames[0] = []emitEvent{{seq: 1, kind: evCall}}
	if _, ok := es.producerReturnedClosureArity("x"); ok {
		t.Fatal("no matching seq should decline")
	}
	// matching evCallUser with an out-of-range unit → false.
	es = NewEmitState()
	es.producedBy["u"] = producer{seq: 5}
	es.frames[0] = []emitEvent{{seq: 5, kind: evCallUser, uc: emitUserCall{unit: 99}}}
	if _, ok := es.producerReturnedClosureArity("u"); ok {
		t.Fatal("out-of-range unit should decline")
	}
	// matching unit whose rec is not a single closure out → false.
	es = NewEmitState()
	es.producedBy["u"] = producer{seq: 5}
	es.fnRecs = []*fnUnitRec{{}}
	es.frames[0] = []emitEvent{{seq: 5, kind: evCallUser, uc: emitUserCall{unit: 0}}}
	if _, ok := es.producerReturnedClosureArity("u"); ok {
		t.Fatal("non-closure out should decline")
	}
	// closure out whose closureUnit is out of range → false.
	es = NewEmitState()
	es.producedBy["u"] = producer{seq: 5}
	es.fnRecs = []*fnUnitRec{{outOps: []emitOperand{{kind: opClosure, closureUnit: 99}}}}
	es.frames[0] = []emitEvent{{seq: 5, kind: evCallUser, uc: emitUserCall{unit: 0}}}
	if _, ok := es.producerReturnedClosureArity("u"); ok {
		t.Fatal("out-of-range closureUnit should decline")
	}
}

// --- make-list / make-map / interp / closure refusals -------------------

func TestRecordMakeListRefusals(t *testing.T) {
	// Not the top frame → declines.
	es := NewEmitState()
	es.frames = append(es.frames, nil)
	if es.RecordMakeList(nil, []Value{NewInteger(1)}, NewInteger(0), SrcPos{}) {
		t.Fatal("non-top-frame make-list should decline")
	}
	// A bare type node element → declines (type-pattern list, not data).
	es = NewEmitState()
	if es.recordMakeListInner(nil, []Value{NewTypeLiteral(TInteger)}, NewInteger(0), SrcPos{}) {
		t.Fatal("type-pattern list should decline")
	}
}

func TestRecordMakeMapRefusals(t *testing.T) {
	es := NewEmitState()
	// length mismatch → declines.
	if es.RecordMakeMap(nil, []string{"a"}, nil, false, NewInteger(0), SrcPos{}) {
		t.Fatal("key/val mismatch should decline")
	}
	// bare type node value → declines.
	if es.RecordMakeMap(nil, []string{"a"}, []Value{NewTypeLiteral(TInteger)}, false, NewInteger(0), SrcPos{}) {
		t.Fatal("type-pattern map should decline")
	}
}

func TestRecordInterpRefusals(t *testing.T) {
	es := NewEmitState()
	if es.RecordInterp(nil, nil, NewInteger(0), SrcPos{}) {
		t.Fatal("no holes should decline")
	}
}

func TestRecordClosureCallRefusals(t *testing.T) {
	es := NewEmitState()
	// nil sig → declines.
	if es.RecordClosureCall("w", nil, nil, 0, 0, nil, nil, nil, SrcPos{}) {
		t.Fatal("nil sig should decline")
	}
	// an unresolvable non-body operand → declines.
	sig := &Signature{}
	if es.RecordClosureCall("w", sig, []Value{carrierVal(TInteger)}, -1, 0, nil, nil, []Value{NewInteger(0)}, SrcPos{}) {
		t.Fatal("unresolvable closure-call operand should decline")
	}
}

// --- recordCallRefusal classifier arms ----------------------------------

func TestRecordCallRefusalArms(t *testing.T) {
	pos := SrcPos{}
	// sig == nil.
	es := NewEmitState()
	if !es.recordCallRefusal("w", nil, nil, nil, pos, false, false) || es.Compilable {
		t.Fatal("nil sig should refuse")
	}
	// anonymous dispatch (word == "").
	es = NewEmitState()
	if !es.recordCallRefusal("", &Signature{}, nil, nil, pos, false, false) || es.Compilable {
		t.Fatal("empty word should refuse")
	}
	// full-stack word.
	es = NewEmitState()
	fs := &Signature{Impl: &GoImpl{FullStack: true}}
	if !es.recordCallRefusal("depth", fs, nil, nil, pos, false, false) || es.Compilable {
		t.Fatal("full-stack word should refuse")
	}
	// get-family read that may auto-dispatch a fn member.
	es = NewEmitState()
	getArgs := []Value{NewList([]Value{zeroArgFn()})}
	if !es.recordCallRefusal("get", &Signature{}, getArgs, nil, pos, false, false) || es.Compilable {
		t.Fatal("fn-member read should refuse")
	}
	// context-dependent word `args`.
	es = NewEmitState()
	if !es.recordCallRefusal("args", &Signature{}, nil, nil, pos, false, false) || es.Compilable {
		t.Fatal("args should refuse")
	}
}

// --- recordCallOperands fn-inert const bake -----------------------------

func TestRecordCallOperandsInertFnBake(t *testing.T) {
	es := NewEmitState()
	sig := &Signature{Args: []*Type{TFunction}, FnInertArgs: map[int]bool{0: true}}
	fn := fnVal(Signature{Args: []*Type{TInteger}})
	ops, ok := es.recordCallOperands("stash", sig, []Value{fn})
	if !ok {
		t.Fatal("an inert fn operand should bake as a const")
	}
	if len(ops) != 1 || ops[0].kind != opConst {
		t.Fatalf("baked operand = %+v, want a const", ops)
	}
}

// --- RecordPolyCall / RecordDynMethod guards ----------------------------

func TestRecordPolyCallGuard(t *testing.T) {
	es := NewEmitState()
	// more than one output → declines.
	if es.RecordPolyCall("w", nil, []Value{NewInteger(1), NewInteger(2)}, SrcPos{}, nil) {
		t.Fatal("multi-out poly call should decline")
	}
}

func TestRecordDynMethodGuards(t *testing.T) {
	if inactiveEmitState().RecordDynMethod(NewDynamicCarrier(TFunction), nil, nil, "m", SrcPos{}) {
		t.Fatal("inactive RecordDynMethod should decline")
	}
	es := NewEmitState()
	// fn unresolvable → declines.
	if es.RecordDynMethod(carrierVal(TFunction), nil, nil, "m", SrcPos{}) {
		t.Fatal("unresolvable method value should decline")
	}
	// fn resolves, an arg is unresolvable → declines.
	fn := NewDynamicCarrier(TFunction)
	seedProduced(es, fn, 1)
	if es.RecordDynMethod(fn, []Value{carrierVal(TInteger)}, nil, "m", SrcPos{}) {
		t.Fatal("unresolvable method arg should decline")
	}
}

// --- flat-instance non-fn key (the continue arms) -----------------------

func TestFlatInstanceNonFnKey(t *testing.T) {
	// readsFnMember: concrete key present but the field is not a fn → continue.
	inst := instanceVal(mapOf2("k", NewInteger(1)))
	if readsFnMember([]Value{inst, NewAtom("k")}) {
		t.Fatal("non-fn field should not be reported")
	}
	// containerFnAutoDispatchRisk: same, with a non-0-arg field.
	if containerFnAutoDispatchRisk([]Value{inst, NewAtom("k")}) {
		t.Fatal("non-fn field should not be risky")
	}
}

// --- isInertConst / type-body classifier family -------------------------

func TestNoEvalBodiesInertSentinel(t *testing.T) {
	// A body that is inert data but carries a break sentinel is refused.
	sig := &Signature{NoEvalArgs: map[int]bool{0: true}}
	body := NewList([]Value{NewWord("break")})
	if noEvalBodiesInert(sig, []Value{body}) {
		t.Fatal("a break-bearing body should not be inert")
	}
	es := NewEmitState()
	if es.noEvalBodiesInertScoped(sig, []Value{body}) {
		t.Fatal("a break-bearing body should not be scoped-inert")
	}
}

func TestDynInputsProven(t *testing.T) {
	// arity mismatch → false.
	es := NewEmitState()
	sig := &Signature{CompileEffect: CompileRunsBodyIsolated, Args: []*Type{TInteger}}
	if es.dynInputsProven(sig, nil) {
		t.Fatal("arity mismatch should not be proven")
	}
	// dynamic operand with a concrete parent but a nil sig position → false.
	sigNil := &Signature{CompileEffect: CompileRunsBodyIsolated, Args: []*Type{nil}}
	if es.dynInputsProven(sigNil, []Value{NewDynamicCarrier(TInteger)}) {
		t.Fatal("nil sig position should not be proven")
	}
	// dynamic operand whose parent does not conform to the sig position → false.
	sigStr := &Signature{CompileEffect: CompileRunsBodyIsolated, Args: []*Type{TString}}
	if es.dynInputsProven(sigStr, []Value{NewDynamicCarrier(TInteger)}) {
		t.Fatal("non-conforming dynamic operand should not be proven")
	}
}

func TestInterpMemberInert(t *testing.T) {
	// list member not inert → false.
	if interpMemberInert(NewList([]Value{NewCarrier(TInteger)})) {
		t.Fatal("carrier list member should not be inert")
	}
	// nil-backed map → false.
	if interpMemberInert(Value{Parent: TMap, Data: MapPayload{M: nil}}) {
		t.Fatal("nil map should not be inert")
	}
	// map of inert members → true.
	if !interpMemberInert(s7MapOf("k", NewInteger(1))) {
		t.Fatal("map of inert members should be inert")
	}
	// paren-expr with a non-inert token → false.
	if interpMemberInert(NewParenExpr([]Value{NewCarrier(TInteger)})) {
		t.Fatal("carrier paren token should not be inert")
	}
}

func TestValueRefsNameMap(t *testing.T) {
	// nil-backed map → false.
	if valueRefsName(Value{Parent: TMap, Data: MapPayload{M: nil}}) {
		t.Fatal("nil map references no name")
	}
	// map whose member is a Word → true.
	if !valueRefsName(s7MapOf("k", NewWord("x"))) {
		t.Fatal("a Word map member is a name reference")
	}
}

func TestInternCompoundEmptyID(t *testing.T) {
	es := NewEmitState()
	lst := NewList([]Value{NewInteger(1)})
	lst.ID = "" // a compound with no ID takes the un-pooled append path
	if idx := es.intern(lst); idx != 0 || len(es.consts) != 1 {
		t.Fatalf("empty-id compound intern = %d (consts=%d)", idx, len(es.consts))
	}
}

func TestIsTypeBodyPayload(t *testing.T) {
	if !isTypeBodyPayload(Value{Data: ChildTypeInfo{}}) {
		t.Fatal("ChildTypeInfo is a type-body payload")
	}
	if isTypeBodyPayload(NewInteger(1)) {
		t.Fatal("an integer is not a type-body payload")
	}
}

func TestAllInertAndFields(t *testing.T) {
	if allInert([]Value{NewInteger(1)}, func(Value) bool { return false }) {
		t.Fatal("a failing predicate should not be all-inert")
	}
	if allFieldsInert(nil, func(Value) bool { return true }) {
		t.Fatal("a nil field map is not inert")
	}
}

func TestTypeBodyConstOKChildType(t *testing.T) {
	// Child not const-safe → false.
	bad := Value{Data: ChildTypeInfo{Child: NewCarrier(TInteger)}}
	if typeBodyConstOK(bad) {
		t.Fatal("carrier child should refuse")
	}
	// Child ok, but an entry value is not const-safe → false.
	badEntry := Value{Data: ChildTypeInfo{
		Child:   NewInteger(1),
		Entries: []ChildEntry{{Key: "k", Value: NewCarrier(TInteger)}},
	}}
	if typeBodyConstOK(badEntry) {
		t.Fatal("carrier entry should refuse")
	}
}

func TestIsInertConstPayloadArms(t *testing.T) {
	// Micron instance → inert.
	micron := Value{Parent: TAny, Data: MicronPayload{Fields: NewOrderedMap()}}
	if !isInertConst(micron) {
		t.Fatal("a Micron instance should be inert")
	}
	// nil-backed map → not inert.
	if isInertConst(Value{Parent: TMap, Data: MapPayload{M: nil}}) {
		t.Fatal("nil map should not be inert")
	}
	// XML element whose attribute value is a carrier → not inert.
	attr := NewOrderedMap()
	attr.Set("x", NewCarrier(TInteger))
	xml := Value{Parent: TAny, Data: XmlElementPayload{Tag: "a", Attr: attr}}
	if isInertConst(xml) {
		t.Fatal("carrier attribute should refuse")
	}
}

func TestFnSigConstOK(t *testing.T) {
	pat := NewCarrier(TInteger)
	info := FnUndefInfo{Sigs: []FnSigSpec{{Params: []FnParam{{Pattern: &pat}}}}}
	if fnSigConstOK(info) {
		t.Fatal("a carrier param pattern should refuse")
	}
}

func TestSurfaceConstOK(t *testing.T) {
	if surfaceConstOK(nil) {
		t.Fatal("nil surface should refuse")
	}
	if surfaceConstOK(&SurfaceInfo{Type: nil}) {
		t.Fatal("no canonical node should refuse")
	}
	if !surfaceConstOK(&SurfaceInfo{Type: TInteger}) {
		t.Fatal("a surface with no required ops should be const-ok")
	}
}

func TestSchemaConstOK(t *testing.T) {
	if schemaConstOK(nil) {
		t.Fatal("nil schema should refuse")
	}
	if schemaConstOK(&TypeSchemaInfo{Type: nil}) {
		t.Fatal("no canonical node should refuse")
	}
	// A body Word that names no parameter → isParam false, refuses.
	if schemaConstOK(&TypeSchemaInfo{Type: TInteger, Body: NewWord("nope")}) {
		t.Fatal("a non-param body word should refuse")
	}
	// param bound not const-safe → false.
	if schemaConstOK(&TypeSchemaInfo{Type: TInteger, Body: NewInteger(1),
		Params: []GenParam{{HasBound: true, Bound: NewCarrier(TInteger)}}}) {
		t.Fatal("carrier bound should refuse")
	}
	// param default not const-safe → false.
	if schemaConstOK(&TypeSchemaInfo{Type: TInteger, Body: NewInteger(1),
		Params: []GenParam{{HasDefault: true, Default: NewCarrier(TInteger)}}}) {
		t.Fatal("carrier default should refuse")
	}
}

func TestIsInertQuotedParen(t *testing.T) {
	// Quoted but not actually a paren-expr payload → AsParenExpr errors.
	bad := Value{Parent: TParenExpr, Data: IntPayload{N: 1}, Quoted: true}
	if isInertQuotedParen(bad) {
		t.Fatal("a non-paren payload should refuse")
	}
	// Quoted paren with a carrier token → refuses.
	p := NewParenExpr([]Value{NewCarrier(TInteger)})
	p.Quoted = true
	if isInertQuotedParen(p) {
		t.Fatal("a carrier token should refuse")
	}
}

func TestIsInertReach(t *testing.T) {
	// Not a reach → false.
	if isInertReach(NewInteger(1)) {
		t.Fatal("an integer is not an inert reach")
	}
	// A reach with a computed segment → false.
	r := NewReach(ReachInfo{Segments: []ReachSeg{{Computed: true}}})
	if isInertReach(r) {
		t.Fatal("a computed segment should refuse")
	}
}

func TestInertReachMemberSeam7(t *testing.T) {
	// Not a reach → false.
	if inertReachMember(NewInteger(1)) {
		t.Fatal("an integer is not a reach member")
	}
	// IsReach by parent but the payload is not ReachInfo → AsReach errors.
	if inertReachMember(Value{Parent: TReach, Data: IntPayload{N: 1}}) {
		t.Fatal("a non-reach payload should refuse")
	}
	// A reach whose receiver token is a carrier → refuses.
	r := NewReach(ReachInfo{Receiver: []Value{NewCarrier(TInteger)}})
	if inertReachMember(r) {
		t.Fatal("a carrier receiver should refuse")
	}
}

func TestIsInertConstMemberParenErr(t *testing.T) {
	// A value that IsParenExpr by parent but whose payload is not a
	// ParenExprPayload → AsParenExpr errors → not an inert member.
	bad := Value{Parent: TParenExpr, Data: IntPayload{N: 1}}
	if isInertConstMember(bad) {
		t.Fatal("a non-paren paren-typed value should not be an inert member")
	}
}

// --- small direct helpers: thenArm / OperandRepushable / RegisterLocal ---

func TestThenArmComputed(t *testing.T) {
	br := &emitBranch{thenComputed: true}
	if br.thenArm() != armComputed {
		t.Fatal("thenComputed should classify as armComputed")
	}
}

func TestOperandRepushableBareNode(t *testing.T) {
	es := NewEmitState()
	// A bare type node with an ID is freely re-pushable (type operand).
	if !es.OperandRepushable(NewTypeLiteral(TInteger)) {
		t.Fatal("a bare type node should be re-pushable")
	}
}

func TestRegisterLocalReuseAndNil(t *testing.T) {
	var nilES *EmitState
	if nilES.RegisterLocal("x") != -1 {
		t.Fatal("nil RegisterLocal should be -1")
	}
	es := NewEmitState()
	s1 := es.RegisterLocal("x")
	if s2 := es.RegisterLocal("x"); s2 != s1 {
		t.Fatalf("re-registering should reuse slot: %d vs %d", s1, s2)
	}
}

func TestMaterialiseMapPrefixCopy(t *testing.T) {
	es := NewEmitState()
	// A 2-key map whose SECOND key is the changed (recoverable) member: the
	// rebuild copies the unchanged prefix, then patches the second.
	c := NewCarrier(TInteger)
	es.origByID[c.ID] = NewInteger(9)
	om := NewOrderedMap()
	om.Set("a", NewInteger(1)) // unchanged prefix
	om.Set("b", c)             // changed → triggers prefix copy
	got, ok := es.materialise(NewMap(om))
	if !ok {
		t.Fatal("map with a recoverable member should materialise")
	}
	mp := got.Data.(MapPayload)
	if bv, _ := mp.M.Get("b"); bv.Carrier {
		t.Fatal("changed member not recovered")
	}
	if av, _ := mp.M.Get("a"); av.Carrier {
		t.Fatal("unchanged prefix member should survive")
	}
}

func TestTypeBodyConstOKElements(t *testing.T) {
	// Child ok, but an Elements entry is a carrier → allInert fails.
	v := Value{Data: ChildTypeInfo{Child: NewInteger(1), Elements: []Value{NewCarrier(TInteger)}}}
	if typeBodyConstOK(v) {
		t.Fatal("a carrier element should refuse")
	}
}

func TestRecordCallOperandsInertFnBakeCaptured(t *testing.T) {
	es := NewEmitState()
	sig := &Signature{Args: []*Type{TFunction}, FnInertArgs: map[int]bool{0: true}}
	// A captured fn value is NOT an inert const, so resolveOperand declines and
	// the fn-inert-slot path bakes it as a const instead.
	fn := Value{ID: GenerateID(IDPrefixForType(TFunction)), Parent: TFunction,
		Data: FnDefInfo{Signatures: []Signature{{Args: []*Type{TInteger}}},
			Captured: []CapturedBinding{{Name: "c", Value: NewInteger(1)}}}}
	ops, ok := es.recordCallOperands("stash", sig, []Value{fn})
	if !ok || len(ops) != 1 || ops[0].kind != opConst {
		t.Fatalf("captured inert fn should bake as a const: ok=%v ops=%+v", ok, ops)
	}
}

// --- loop-carried def machinery -----------------------------------------

func TestNoteLoopCarriedArms(t *testing.T) {
	// loopCarried empty → no-op (guard).
	NewEmitState().NoteLoopCarried("n", NewInteger(1), NewInteger(2))

	// scope unit-depth mismatch → returns without registering.
	es := NewEmitState()
	es.loopCarried = []*loopCarriedScope{{unitDepth: 99, slots: map[string]int{}}}
	es.NoteLoopCarried("n", NewInteger(1), NewInteger(2))

	// fresh name whose pre value is a fn value → left unregistered.
	es = NewEmitState()
	es.loopCarried = []*loopCarriedScope{{unitDepth: 1, slots: map[string]int{}}}
	es.NoteLoopCarried("n", NewInteger(1), NewCarrier(TFunction))
	if _, seen := es.loopCarried[0].slots["n"]; seen {
		t.Fatal("fn-valued pre should not register a slot")
	}

	// outer scope at a different unit depth breaks the reuse walk; a plain
	// pre value then registers a fresh slot with an init.
	es = NewEmitState()
	es.loopCarried = []*loopCarriedScope{
		{unitDepth: 2, slots: map[string]int{}},
		{unitDepth: 1, slots: map[string]int{}},
	}
	es.NoteLoopCarried("n", NewInteger(1), NewInteger(2))
	if _, seen := es.loopCarried[1].slots["n"]; !seen {
		t.Fatal("a fresh carried name should register a slot")
	}

	// re-resolve path (seen name) whose pre value no longer resolves → refuse.
	es = NewEmitState()
	es.loopCarried = []*loopCarriedScope{{
		unitDepth: 1,
		slots:     map[string]int{"n": 0},
		inits:     []carriedInit{{slot: 0}},
	}}
	es.NoteLoopCarried("n", NewInteger(1), NewCarrier(TInteger))
	if es.Compilable {
		t.Fatal("an unresolvable re-resolved pre value should refuse")
	}
}

func TestRecordDefRebindRefusals(t *testing.T) {
	// fn-valued rebind → refuse.
	es := NewEmitState()
	es.loopCarried = []*loopCarriedScope{{unitDepth: 1, slots: map[string]int{"n": 0}}}
	es.RecordDefRebind("n", NewCarrier(TFunction), SrcPos{})
	if es.Compilable {
		t.Fatal("fn-valued loop-carried rebind should refuse")
	}
	// unresolvable rebind → refuse.
	es = NewEmitState()
	es.loopCarried = []*loopCarriedScope{{unitDepth: 1, slots: map[string]int{"n": 0}}}
	es.RecordDefRebind("n", NewCarrier(TInteger), SrcPos{})
	if es.Compilable {
		t.Fatal("unresolvable loop-carried rebind should refuse")
	}
}

func TestRefuseCarriedUndefFound(t *testing.T) {
	es := NewEmitState()
	es.loopCarried = []*loopCarriedScope{{unitDepth: 1, slots: map[string]int{"n": 0}}}
	es.RefuseCarriedUndef("n")
	if es.Compilable {
		t.Fatal("undef of a loop-carried name should refuse")
	}
}

func TestBeginEndLoopCarriedGuards(t *testing.T) {
	var nilES *EmitState
	nilES.BeginLoopCarried() // nil guard
	es := NewEmitState()
	es.EndLoopCarried() // empty stack → no-op
}

// --- Finalize / dynamic-apply shape guards ------------------------------

func TestFinalizeNilReceiver(t *testing.T) {
	var es *EmitState
	if p, why, ok := es.Finalize(nil); p != nil || ok || why == "" {
		t.Fatalf("nil Finalize = %v %q %v", p, why, ok)
	}
}

func TestMixedDynamicApplyShape(t *testing.T) {
	// Interior dynamic value that is NOT event-produced → declines.
	es := NewEmitState()
	dyn := NewDynamicCarrier(TAny)
	residual := []Value{NewInteger(1), dyn, NewInteger(2)}
	if _, ok := es.mixedDynamicApplyShape(residual); ok {
		t.Fatal("non-event interior dynamic should decline")
	}
	// Same shape, but the carrier carries a method-shape annotation → declines
	// before the event-produced check.
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	es2 := NewEmitState()
	es2.reg = r
	r.Check.MethodShapes = map[string]Value{dyn.ID: NewInteger(0)}
	if _, ok := es2.mixedDynamicApplyShape(residual); ok {
		t.Fatal("annotated interior carrier should decline")
	}
}

// --- shuffle-word gates -------------------------------------------------

// miniShuffleReg registers one shuffle-named native with the given arg types
// and returns the registry plus a pointer to its single signature.
func miniShuffleReg(t *testing.T, name string, argTypes []*Type) (*Registry, *Signature) {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.RegisterNativeFunc(NativeFunc{
		Name: name,
		Signatures: []Signature{{
			Args: argTypes,
			Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return a, nil
			}),
			BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	return r, &r.Lookup(name).Signatures[0]
}

func TestDynamicStackShuffleOK(t *testing.T) {
	// Shuffle word but no registry bound → false.
	if NewEmitState().dynamicStackShuffleOK("dup", &Signature{}) {
		t.Fatal("no registry should decline")
	}
	// Shuffle word whose sig has a non-Any arg → false.
	r, sig := miniShuffleReg(t, "dup", []*Type{TInteger})
	es := NewEmitState()
	es.reg = r
	if es.dynamicStackShuffleOK("dup", sig) {
		t.Fatal("a non-Any shuffle sig should decline")
	}
}

func TestRecordShuffleElidedMismatch(t *testing.T) {
	r, sig := miniShuffleReg(t, "swap", []*Type{TAny, TAny})
	es := NewEmitState()
	es.reg = r
	d1, d2 := NewDynamicCarrier(TAny), NewDynamicCarrier(TAny)
	// outs are not an ID-preserving permutation of args (one fresh id) → false.
	if es.recordShuffleElided("swap", sig, []Value{d1, d2}, []Value{d1, NewInteger(0)}) {
		t.Fatal("non-permutation outputs should not elide")
	}
}

// --- interpMemberInter interp-string error arm --------------------------

func TestInterpMemberInertBadInterp(t *testing.T) {
	// A value whose Parent is TInterpString but whose payload is not an
	// InterpStringPayload → AsInterpString errors → not inert.
	bad := Value{Parent: TInterpString, Data: IntPayload{N: 1}}
	if interpMemberInert(bad) {
		t.Fatal("a malformed interp-string should not be inert")
	}
}

// --- tryReturnedClosure early refusals ----------------------------------

func TestTryReturnedClosureRefusals(t *testing.T) {
	r := covRegistry(t, nil)
	// Anonymous lambda with an unresolvable capture → declines.
	es := NewEmitState()
	es.reg = r
	withCap := Value{ID: GenerateID(IDPrefixForType(TFunction)), Parent: TFunction,
		Data: FnDefInfo{Anonymous: true,
			Captured:   []CapturedBinding{{Name: "c", Value: NewCarrier(TInteger)}},
			Signatures: []Signature{{}}}}
	if _, ok := es.tryReturnedClosure(withCap, SrcPos{}); ok {
		t.Fatal("unresolvable capture should decline")
	}
	// Anonymous lambda with more than one own signature → declines.
	es = NewEmitState()
	es.reg = r
	multiSig := Value{ID: GenerateID(IDPrefixForType(TFunction)), Parent: TFunction,
		Data: FnDefInfo{Anonymous: true, Signatures: []Signature{{}, {}}}}
	if _, ok := es.tryReturnedClosure(multiSig, SrcPos{}); ok {
		t.Fatal("multi-sig lambda should decline")
	}
}

// --- tryReturnedClosure body-compile path -------------------------------

func TestTryReturnedClosureProbeFails(t *testing.T) {
	r := covRegistry(t, nil)
	es := NewEmitState()
	es.reg = r
	// One own sig, a nil-typed param (exercises the t==nil → Any default), and
	// a body that references an unknown word so the probe compile refuses.
	lam := Signature{
		Params:     []FnParam{{Name: "y", Type: nil}},
		Impl:       AQL([]Value{NewWord("no_such_word_zzz9")}),
		Returns:    []*Type{TInteger},
		BarrierPos: -1,
	}
	v := Value{ID: GenerateID(IDPrefixForType(TFunction)), Parent: TFunction,
		Data: FnDefInfo{Anonymous: true, Signatures: []Signature{lam}}}
	if _, ok := es.tryReturnedClosure(v, SrcPos{}); ok {
		t.Fatal("an uncompilable lambda body should decline")
	}
}

// --- StartFnCompile finish residual paths -------------------------------

func TestStartFnCompileFinishZeroOut(t *testing.T) {
	es := NewEmitState()
	_, finish, ok := es.StartFnCompile("k", "fn", nil, nil, nil, nil, nil, false, SrcPos{})
	if !ok {
		t.Fatal("StartFnCompile should open a unit")
	}
	// A body residual entry that is a 0-output statement guard's phantom is
	// skipped in the finish resolution.
	phantom := NewInteger(1)
	es.producedBy[phantom.ID] = producer{seq: 1}
	f := es.eventInfo[1]
	f.zeroOut = true
	es.eventInfo[1] = f
	finish([]Value{phantom})
}

func TestStartFnCompileFinishDynTrail(t *testing.T) {
	es := NewEmitState()
	_, finish, _ := es.StartFnCompile("k", "fn", nil, nil, nil, nil, nil, false, SrcPos{})
	arg := NewInteger(5)
	fn := NewDynamicCarrier(TFunction)
	es.producedBy[fn.ID] = producer{seq: 1} // resolves to an event
	es.RegisterTrailingApply(fn.ID, 1)      // arity 1 == len(bodyStk)-1
	finish([]Value{arg, fn})
}

func TestStartFnCompileFinishPendingApply(t *testing.T) {
	es := NewEmitState()
	_, finish, _ := es.StartFnCompile("k", "fn", nil, nil, nil, nil, nil, false, SrcPos{})
	u := es.units[len(es.units)-1]
	fnHead := NewDynamicCarrier(TFunction)
	fnLast := NewDynamicCarrier(TFunction)
	es.producedBy[fnHead.ID] = producer{seq: 1}
	es.producedBy[fnLast.ID] = producer{seq: 2}
	u.pendingApply = []string{fnLast.ID}
	// bodyStk[:last] carries a fn value, so the pending-apply argsOK scan trips.
	finish([]Value{fnHead, fnLast})
	if es.Compilable {
		t.Fatal("a mid-body pending apply should refuse")
	}
}

// --- quoted-operand refusal ---------------------------------------------

func TestRecordCallRefusalQuotedOperand(t *testing.T) {
	es := NewEmitState()
	sig := &Signature{QuoteArgs: map[int]bool{0: true}}
	if !es.recordCallRefusal("usurp", sig, nil, nil, SrcPos{}, false, false) || es.Compilable {
		t.Fatal("an uncovered quoted-operand word should refuse")
	}
}

// --- Finalize residual arms ---------------------------------------------

func TestFinalizeResidualArms(t *testing.T) {
	// A zeroOut phantom in the residual is skipped, yielding an empty program.
	es := NewEmitState()
	phantom := NewInteger(1)
	es.producedBy[phantom.ID] = producer{seq: 1}
	f := es.eventInfo[1]
	f.zeroOut = true
	es.eventInfo[1] = f
	if p, _, ok := es.Finalize([]Value{phantom}); !ok || p == nil {
		t.Fatal("a zeroOut-only residual should finalize")
	}

	// A bare Word residual materialises but is not an inert const → refuses.
	es = NewEmitState()
	if _, why, ok := es.Finalize([]Value{NewWord("x")}); ok || why == "" {
		t.Fatal("a non-materialisable residual should refuse")
	}

	// An unfinished fn unit with no terminal trap → refuses.
	es = NewEmitState()
	es.fnRecs = []*fnUnitRec{{name: "ghost"}}
	if _, why, ok := es.Finalize(nil); ok || why == "" {
		t.Fatal("an unfinished fn unit should refuse")
	}
}
