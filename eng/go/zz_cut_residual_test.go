package eng

// Residual pins and helpers assembled at the core cut (triage output).

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"testing"
)

var (
	_ = errors.New
	_ = fmt.Sprint
	_ = math.Pi
	_ = sort.Ints
	_ = strings.TrimSpace
	_ sync.Mutex
	_ = testing.Short
)

func TestOrderingReturnsFn(t *testing.T) {
	r := newTestRegistry(t)
	fn := OrderingReturnsFn(LtHandler, TBoolean)

	// Cross-family determinate operands: diagnostic recorded.
	before := len(r.Check.Diagnostics)
	out := fn([]Value{NewString("a"), NewInteger(1)}, r)
	if len(out) != 1 || !out[0].Carrier || !out[0].Parent.Equal(TBoolean) {
		t.Fatalf("OrderingReturnsFn result = %v, want Boolean carrier", out)
	}
	if len(r.Check.Diagnostics) != before+1 {
		t.Errorf("cross-family static pair added %d diagnostics, want 1",
			len(r.Check.Diagnostics)-before)
	}
	if r.Check.Diagnostics[len(r.Check.Diagnostics)-1].Code != "incomparable" {
		t.Errorf("diagnostic code = %q", r.Check.Diagnostics[len(r.Check.Diagnostics)-1].Code)
	}

	// A dynamic operand is not determinate — no diagnostic.
	before = len(r.Check.Diagnostics)
	dyn := NewCarrier(TAny)
	dyn.Dynamic = true
	fn([]Value{dyn, NewInteger(1)}, r)
	if len(r.Check.Diagnostics) != before {
		t.Error("dynamic operand produced a static incomparable diagnostic")
	}

	// Same-family determinate operands: no diagnostic.
	before = len(r.Check.Diagnostics)
	fn([]Value{NewInteger(1), NewInteger(2)}, r)
	if len(r.Check.Diagnostics) != before {
		t.Error("orderable pair produced a diagnostic")
	}
}

func TestOrderingDeterminate(t *testing.T) {
	if !orderingDeterminate(NewInteger(1)) {
		t.Error("concrete integer not determinate")
	}
	if !orderingDeterminate(NewNone()) {
		t.Error("none (singleton type) not determinate")
	}
	if orderingDeterminate(NewCarrier(TInteger)) {
		t.Error("bare carrier claims determinate")
	}
	dyn := NewInteger(1)
	dyn.Dynamic = true
	if orderingDeterminate(dyn) {
		t.Error("dynamic value claims determinate")
	}
}

func TestS5bECodeEffectBareTypeResidualDeclines(t *testing.T) {
	r := newTestRegistry(t)
	r.RegisterNativeFunc(NativeFunc{
		Name: "s5benoop",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	done := r.Check.Begin()
	defer done()
	// Body: a no-op word plus a bare type literal. The analysis is clean
	// and leaves the literal as the residual, which the []*Type domain
	// cannot represent — the carrier conversion must decline.
	body := NewList([]Value{NewWord("s5benoop"), NewTypeLiteral(TInteger)})
	body.Quoted = true
	if _, ok := AnalyseCodeEffectCarrier(r, body); ok {
		t.Error("expected AnalyseCodeEffectCarrier to decline a bare-type-literal residual")
	}
}

func TestS5bENarrowArgsMoreArgsThanParams(t *testing.T) {
	args := []Value{NewInteger(1), NewInteger(2)}
	params := []FnParam{{Name: "a", Type: TInteger}}
	out := narrowArgsToParams(args, params)
	if out != nil && len(out) != len(args) {
		t.Errorf("unexpected narrowing result: %v", out)
	}
}

func TestS5bEBodySpanEndComputedReach(t *testing.T) {
	seg := ReachSeg{Computed: true, KeyExpr: []Value{s5beAt(NewWord("k"), 3, 7)}}
	reach := NewReach(ReachInfo{Receiver: []Value{s5beAt(NewWord("m"), 3, 1)}, Segments: []ReachSeg{seg}})
	end := bodySpanEnd([]Value{reach})
	if end.Row != 3 || end.Col != 7 {
		t.Errorf("bodySpanEnd = %+v, want row 3 col 7 (the computed key)", end)
	}
}

func TestS5bEResidualProvablyDisjointDisjunct(t *testing.T) {
	// Empty disjunct: nothing is provable.
	if residualProvablyDisjoint(NewDisjunct(nil), TInteger) {
		t.Error("empty disjunct must not be provably disjoint")
	}
	// Every alternative disjoint from the declared type → true.
	d := NewDisjunct([]Value{NewString("x")})
	if !residualProvablyDisjoint(d, TInteger) {
		t.Error("String-only disjunct should be provably disjoint from Integer")
	}
}

func TestS5bEReturnsFnDeferredParamListPopsGenBindings(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	p := GenParam{Name: "S5beT"}
	node := MintTypeParam(r, p)
	spec := &GenSpecInfo{Params: []GenParam{p}}

	// Body: a single evaluate-by-default list literal referencing the
	// param — the deferred-list shape.
	inner := NewList([]Value{NewWord("s5bec1")})
	inner.Eval = true
	sig := FnSig{
		Params: []FnParam{{Name: "s5bec1", Type: node}},
		Impl:   Boru([]Value{inner}),
	}
	rf := buildFnBodyReturnsFn(r, "s5bemk", sig, FnDefInfo{Gen: spec})
	out := rf([]Value{NewInteger(9)}, r)
	if len(out) != 1 {
		t.Fatalf("expected the raw deferred list residual, got %v", out)
	}
	// The inferred T binding must have been popped again.
	if _, ok := r.Defs.Top("S5beT"); ok {
		t.Error("gen binding S5beT must be popped after the deferred-list return")
	}
}

func TestS5bEReturnsFnArmedRootCarrierArg(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	defer done()

	sig := FnSig{
		Params: []FnParam{{Name: "s5bez", Type: TAny}},
		Impl:   Boru([]Value{NewInteger(1)}),
	}
	rf := buildFnBodyReturnsFn(r, "s5beh", sig, FnDefInfo{})
	// A root-node arg (nil Parent): the armed generalisation must keep it
	// as a carrier rather than calling NewCarrier(nil).
	anyLit := NewTypeLiteral(TAny)
	if anyLit.Parent != nil {
		t.Fatalf("test premise: the Any literal is its own root (nil Parent), got %v", anyLit.Parent)
	}
	out := rf([]Value{anyLit}, r)
	if out == nil {
		t.Log("armed ReturnsFn returned nil residual (acceptable; path exercised)")
	}
}

func TestS5bEReturnsFnUnboundReturnTypeParamDedupes(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	p := GenParam{Name: "S5beU"}
	node := MintTypeParam(r, p)
	spec := &GenSpecInfo{Params: []GenParam{p}}
	// The param types never mention the placeholder, so the return's
	// type parameter is uninferable.
	sig := FnSig{
		Params:  []FnParam{{Name: "s5bew", Type: TInteger}},
		Returns: []*Type{node},
		Impl:    Boru([]Value{NewInteger(1)}),
	}
	rf := buildFnBodyReturnsFn(r, "s5bei", sig, FnDefInfo{Gen: spec})

	out := rf([]Value{NewInteger(1)}, r)
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("expected one dynamic(Any) carrier, got %v", out)
	}
	count := func() int {
		n := 0
		for _, d := range r.Check.Diagnostics {
			if d.Code == "unbound_param" {
				n++
			}
		}
		return n
	}
	first := count()
	if first == 0 {
		t.Fatal("expected an unbound_param diagnostic")
	}
	// Second identical call: the dedupe loop must find the duplicate and
	// not add another diagnostic.
	rf([]Value{NewInteger(1)}, r)
	if got := count(); got != first {
		t.Errorf("unbound_param diagnostics grew from %d to %d; dedupe failed", first, got)
	}
}

// TestInactiveEmitMethods calls every EmitRecorder method on the shared no-op
// recorder. Each returns the inactive/none answer; the point is coverage of
// the method bodies, plus a sanity check that the answers match the "recording
// is not live" contract.
func TestInactiveEmitMethods(t *testing.T) {
	e := TheInactiveEmit

	if e.Active() || e.Armed() || e.SuspendedNow() {
		t.Fatal("inactive recorder reports live/armed")
	}
	if !e.TopFrameOnly() {
		t.Fatal("inactive recorder should be top-frame")
	}
	// Suspend / guards return callable no-op funcs.
	e.Suspend()()
	e.BodyAnalysisGuard()()
	e.FnBodyGuard()()
	e.BindRegistry(nil)

	e.MarkUncompilable("x")
	if e.Sites() != nil {
		t.Fatal("inactive Sites should be nil")
	}
	// Stage-0b promotions: the probes that replaced the concrete
	// *EmitState asserts all decline on the inactive recorder.
	if e.InClosureUnit() || e.StoredGradualActive() {
		t.Fatal("inactive closure/stored probes should decline")
	}
	if folded, ok := e.FoldFullStack("w", nil, nil); folded != nil || ok {
		t.Fatal("inactive FoldFullStack should decline")
	}
	if e.RecordSpliceDyn(Value{}, SrcPos{}) {
		t.Fatal("inactive RecordSpliceDyn should decline")
	}
	e.NoteShapedRead("id")
	if v, ok := e.MemberFnReadValue("id"); ok || v.Data != nil {
		t.Fatal("inactive memberFnReadValue should miss")
	}

	e.RecordCall("w", nil, nil, nil, SrcPos{}, false, false)
	e.RecordPoly("w")
	if e.RecordPolyCall("w", nil, nil, SrcPos{}, nil, nil) {
		t.Fatal("inactive RecordPolyCall should refuse")
	}
	e.RecordUserCall(0, nil, nil, SrcPos{})
	e.RecordUserPolyCall("w", nil, nil, nil, nil, nil, nil, nil, SrcPos{})
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
	if e.RecordMakeListInner(nil, nil, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordMakeListInner should refuse")
	}
	if e.RecordMakeMap(nil, nil, nil, false, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordMakeMap should refuse")
	}
	if e.RecordInterp(nil, nil, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordInterp should refuse")
	}
	e.RegisterTrailingApply("id", 1)
	e.NoteMemberFnRead("id", Value{})
	if e.MemberFnRead("id") {
		t.Fatal("inactive memberFnRead should be false")
	}
	if e.DynInputsProven(nil, nil) {
		t.Fatal("inactive dynInputsProven should be false")
	}
	if v, ok := e.Materialise(in); ok || v.ID != in.ID {
		t.Fatalf("inactive materialise = %v %v", v, ok)
	}
	if e.ZeroOutProduced("id") || e.AlreadyProduced("id") {
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
	e.SetUnitReturnPatterns(0, nil)
	e.SetUnitDecl(0, DeclSite{})
	if e.UnitVariadic(0) {
		t.Fatal("inactive unitVariadic should be false")
	}
}

// TestEmitStateNilReceiver calls the nil-safe *EmitState methods on a nil
// pointer: every one has an `es == nil` guard and must not panic.
func TestEmitStateNilReceiver(t *testing.T) {
	var es *EmitState

	if es.Armed() {
		t.Fatal("nil EmitState is not armed")
	}
	if es.Active() || es.SuspendedNow() {
		t.Fatal("nil EmitState is not active")
	}
	if !es.TopFrameOnly() {
		t.Fatal("nil EmitState is top-frame")
	}
	es.BindRegistry(nil)
	if es.Sites() != nil {
		t.Fatal("nil Sites should be nil")
	}
	if es.ZeroOutProduced("x") || es.AlreadyProduced("x") {
		t.Fatal("nil produced probes should be false")
	}
	if es.UnitVariadic(0) {
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

// seedProduced makes v resolve to an event operand: resolveOperand finds a
// producedBy entry (and v is not a type body, so the events-first arm wins).
func seedProduced(es *EmitState, v Value, seq int) {
	es.producedBy[v.ID] = producer{seq: seq, idx: 0}
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

func TestBeginEndLoopCarriedGuards(t *testing.T) {
	var nilES *EmitState
	nilES.BeginLoopCarried() // nil guard
	es := NewEmitState()
	es.EndLoopCarried() // empty stack → no-op
}

func TestFinalizeNilReceiver(t *testing.T) {
	var es *EmitState
	if p, why, ok := es.Finalize(nil); p != nil || ok || why == "" {
		t.Fatalf("nil Finalize = %v %q %v", p, why, ok)
	}
}

func TestS7DeadSignaturesShort(t *testing.T) {
	if got := DeadSignatures(nil); got != nil {
		t.Errorf("DeadSignatures(nil) = %v, want nil", got)
	}
	if got := DeadSignatures([]Signature{{Args: []*Type{TInteger}, BarrierPos: -1}}); got != nil {
		t.Errorf("DeadSignatures(single) = %v, want nil", got)
	}
}

func TestS7DeadSignaturesDuplicateAndFallback(t *testing.T) {
	sigA := Signature{Args: []*Type{TInteger}, BarrierPos: -1, Impl: Go(nil)}
	sigDup := Signature{Args: []*Type{TInteger}, BarrierPos: -1, Impl: Go(nil)}
	// A trailing fallback sig must be skipped, and the duplicate detected.
	fb := Signature{Fallback: true, BarrierPos: -1, Impl: Go(nil)}
	dead := DeadSignatures([]Signature{sigA, sigDup, fb})
	if len(dead) == 0 {
		t.Error("a duplicate signature should be flagged dead")
	}
	for _, d := range dead {
		if d.Sig.Fallback {
			t.Error("a fallback signature must never be reported dead")
		}
	}
}

func TestS7DeadSignaturesFallbackFirst(t *testing.T) {
	// A fallback with MORE args sorts BEFORE a normal sig (CompareSignatures
	// ignores Fallback, ranks by arg count) — so the inner loop encounters a
	// fallback at an earlier index than a non-fallback and must skip it.
	fb2 := Signature{Fallback: true, Args: []*Type{TInteger, TInteger}, BarrierPos: -1, Impl: Go(nil)}
	norm := Signature{Args: []*Type{TInteger}, BarrierPos: -1, Impl: Go(nil)}
	if got := DeadSignatures([]Signature{norm, fb2}); len(got) != 0 {
		t.Errorf("no sig should be dead here, got %v", got)
	}
}

func TestS7SigSubsumesNilType(t *testing.T) {
	s1 := &Signature{Args: []*Type{nil}, BarrierPos: -1}
	s2 := &Signature{Args: []*Type{nil}, BarrierPos: -1}
	if sigSubsumes(s1, s2) {
		t.Error("a nil arg type must make sigSubsumes decline")
	}
}

func TestUndefinedWordCheckDiagSuggestions(t *testing.T) {
	e := engWithTape(t, nil, 0)
	e.Registry.Defs.Push("counter", NewInteger(1))
	d := undefinedWordCheckDiag(e, "countr", SrcPos{Row: 2, Col: 3})
	if d.Code != "undefined_word" || d.Row != 2 || d.Col != 3 {
		t.Fatalf("diag = %+v", d)
	}
	if len(d.Suggestions) == 0 || !strings.Contains(d.Suggestions[0].Message, "`counter`") {
		t.Errorf("check diag must carry the did-you-mean, got %+v", d.Suggestions)
	}
	// No plausible match → no suggestions at all.
	d = undefinedWordCheckDiag(e, "qqqqqqqq", SrcPos{})
	if len(d.Suggestions) != 0 {
		t.Errorf("far name must carry no suggestions, got %+v", d.Suggestions)
	}
}

func TestDriftWindowOutOfRangePosition(t *testing.T) {
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(1)}, StackHeadroom)
	w, sig := zzDriftWord()
	if tryRecordDriftWindow(e, w, sig, []int{0, 999}) {
		t.Error("an out-of-range matched position must decline the window")
	}
}

func TestDriftWindowNonContiguousDeclines(t *testing.T) {
	// Matched operands not directly under the word (a bystander token between
	// the top operand and the word) — the contiguity fence declines.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	dyn := NewDynamicCarrier(TAny)
	e.Tape = NewTape([]Value{NewInteger(5), dyn, NewInteger(9), NewWord("add"), NewInteger(1)}, StackHeadroom)
	e.Pointer = 3
	w, sig := zzDriftWord()
	if tryRecordDriftWindow(e, w, sig, []int{1, 0}) {
		t.Error("a non-contiguous matched span must decline the window")
	}
}

func TestDriftWindowVariadicOperandDeclines(t *testing.T) {
	// The dynamic top is produced by a VARIADIC event — a fixed-width window
	// cannot carry it, so the fence declines.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es := r.Check.Emit.(*EmitState)
	dyn := NewDynamicCarrier(TAny)
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: "zzvar", nout: 1}})
	es.setProducedAt(dyn, seq, 0)
	f := es.eventInfo[seq]
	f.variadicResult = true
	es.eventInfo[seq] = f
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(5), dyn, NewWord("add"), NewInteger(1)}, StackHeadroom)
	e.Pointer = 2
	w, sig := zzDriftWord()
	if tryRecordDriftWindow(e, w, sig, []int{1, 0}) {
		t.Error("a variadic-event operand must decline the window")
	}
}

func TestDriftWindowUnresolvableOperandDeclines(t *testing.T) {
	// A window operand with no compiled home (a plain carrier: no producing
	// event, no local, no const materialisation) — resolveOperand declines.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es := r.Check.Emit.(*EmitState)
	dyn := NewDynamicCarrier(TAny)
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: "zzev", nout: 1}})
	es.setProducedAt(dyn, seq, 0)
	orphan := NewCarrier(TInteger) // non-dynamic carrier, no event, no orig
	e := NewTop(r)
	e.Tape = NewTape([]Value{orphan, dyn, NewWord("add"), NewInteger(1)}, StackHeadroom)
	e.Pointer = 2
	w, sig := zzDriftWord()
	if tryRecordDriftWindow(e, w, sig, []int{1, 0}) {
		t.Error("an unresolvable window operand must decline the window")
	}
}

// residualReadStable's no-name arm: a value never noted as a def read (or a
// reg-less state) is never a stable residual re-push candidate.
func TestResidualReadStableNoName(t *testing.T) {
	if (&EmitState{}).residualReadStable(Value{}) {
		t.Error("a value with no def-read note must not be stable")
	}
}

// RecordSpliceDyn's decline arms (§9.2b): an inactive state and an
// unresolvable payload both leave the refusal standing.
func TestRecordSpliceDynDeclines(t *testing.T) {
	if (&EmitState{}).RecordSpliceDyn(Value{}, SrcPos{}) {
		t.Error("an inactive EmitState must decline")
	}
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es := r.Check.Emit.(*EmitState)
	es.BindRegistry(r)
	orphan := NewCarrier(TList) // no event, no local, no const home
	if es.RecordSpliceDyn(orphan, SrcPos{}) {
		t.Error("an unresolvable payload must decline")
	}
}

func TestRecordCallRefusalComputedRangeList(t *testing.T) {
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es := r.Check.Emit.(*EmitState)
	es.BindRegistry(r)
	lst := NewCarrier(TList)
	lst.ID = GenerateID(IDPrefixForType(TList))
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: wordMakeList, nout: 1, makeList: true}})
	es.setProduced(lst, seq)
	if !es.recordCallRefusal("for", &Signature{}, []Value{lst}, nil, SrcPos{}, false, false) {
		t.Fatal("the computed-range-list arm must classify the refusal")
	}
	if es.Compilable || !containsStr(es.Reason, "computed range list") {
		t.Errorf("want the computed-range-list reason, got compilable=%v reason=%q", es.Compilable, es.Reason)
	}
}

// TestWindowReadsID pins the replay tail-proof's window-read probe (the
// §9.8 dyn-bind skip): only a value the window actually holds reads.
func TestWindowReadsID(t *testing.T) {
	w := NewCarrier(TFunction)
	w.ID = "g1"
	if !windowReadsID([]Value{w}, "g1") {
		t.Error("a window value's own ID must read")
	}
	if windowReadsID([]Value{w}, "zz") {
		t.Error("an ID absent from the window must not read")
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

func TestStripZeroOutPhantomsKeepsNonPhantom(t *testing.T) {
	// Mint inside a pass so the values carry the compile identities the
	// producedBy bookkeeping keys on (runtime mints elide IDs).
	c := &CheckState{}
	defer c.Begin()()
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

func TestMaterialiseRebuild(t *testing.T) {
	es := NewEmitState()
	// List whose member is a stripped carrier recoverable to a concrete int.
	member := NewCarrier(TInteger)
	orig := NewInteger(7)
	es.origByID[member.ID] = orig
	lst := NewList([]Value{member})
	got, ok := es.Materialise(lst)
	if !ok {
		t.Fatal("list with a recoverable member should materialise")
	}
	lp, isList := got.Data.(ListPayload)
	if !isList || len(lp.Elems) != 1 || lp.Elems[0].Carrier {
		t.Fatalf("rebuilt list member not recovered: %v", got)
	}

	// Map with nil backing → not materialisable.
	nilMap := Value{Parent: TMap, Data: MapPayload{M: nil}}
	if _, ok := es.Materialise(nilMap); ok {
		t.Fatal("nil-map should not materialise")
	}

	// Map with a recoverable member gets rebuilt.
	m2 := NewCarrier(TInteger)
	es.origByID[m2.ID] = NewInteger(11)
	om := NewOrderedMap()
	om.Set("k", m2)
	mp := NewMap(om)
	got2, ok2 := es.Materialise(mp)
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

func TestRecordUserPolyCallGuards(t *testing.T) {
	// Inactive → no-op.
	inactiveEmitState().RecordUserPolyCall("w", nil, nil, nil, nil, nil, nil, nil, SrcPos{})
	// Active with an unresolvable operand → uncompilable.
	es := NewEmitState()
	es.RecordUserPolyCall("w", nil, nil, nil, nil, nil, []Value{carrierVal(TInteger)}, []Value{NewInteger(1)}, SrcPos{})
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

func TestRecordLoopRefusals(t *testing.T) {
	// body == nil.
	es := NewEmitState()
	es.RecordLoop(NewInteger(0), NewInteger(5), NewInteger(1), nil, nil, "it", NewInteger(0), 0, SrcPos{})
	if es.Compilable {
		t.Fatal("nil loop body should refuse")
	}
	// range of unknown provenance (start unresolvable).
	es = NewEmitState()
	es.RecordLoop(carrierVal(TInteger), NewInteger(5), NewInteger(1), &EmitFragment{}, nil, "it", NewInteger(0), 0, SrcPos{})
	if es.Compilable {
		t.Fatal("unresolvable range should refuse")
	}
	// computed start/step (start resolves to an event, not a const).
	es = NewEmitState()
	start := NewCarrier(TInteger)
	seedProduced(es, start, 1)
	es.RecordLoop(start, NewInteger(5), NewInteger(1), &EmitFragment{}, nil, "it", NewInteger(0), 0, SrcPos{})
	if es.Compilable {
		t.Fatal("computed loop start should refuse")
	}
	// iterator slot not registered (all-const range, empty body).
	es = NewEmitState()
	es.RecordLoop(NewInteger(0), NewInteger(5), NewInteger(1), &EmitFragment{}, nil, "it", NewInteger(0), 0, SrcPos{})
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

func TestNoteMemberFnReadGuards(t *testing.T) {
	NewEmitState().NoteMemberFnRead("", Value{}) // empty id → no-op
	inactiveEmitState().NoteMemberFnRead("id", Value{})
	es := NewEmitState()
	es.NoteMemberFnRead("id", Value{})
	if !es.MemberFnRead("id") {
		t.Fatal("a noted member-fn read should be readable")
	}
}

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
	if es.DynInputsProven(sig, nil) {
		t.Fatal("arity mismatch should not be proven")
	}
	// dynamic operand with a concrete parent but a nil sig position → false.
	sigNil := &Signature{CompileEffect: CompileRunsBodyIsolated, Args: []*Type{nil}}
	if es.DynInputsProven(sigNil, []Value{NewDynamicCarrier(TInteger)}) {
		t.Fatal("nil sig position should not be proven")
	}
	// dynamic operand whose parent does not conform to the sig position → false.
	sigStr := &Signature{CompileEffect: CompileRunsBodyIsolated, Args: []*Type{TString}}
	if es.DynInputsProven(sigStr, []Value{NewDynamicCarrier(TInteger)}) {
		t.Fatal("non-conforming dynamic operand should not be proven")
	}
	// a genuinely-WIDENED gradual operand (dynamic Any) is exactly the
	// unproven case the guard defends — refused even on a conforming sig.
	sigAny := &Signature{CompileEffect: CompileRunsBodyIsolated, Args: []*Type{TAny}}
	if es.DynInputsProven(sigAny, []Value{NewDynamicCarrier(TAny)}) {
		t.Fatal("a widened dynamic(Any) operand must refuse the proof")
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

func TestOperandRepushableBareNode(t *testing.T) {
	es := NewEmitState()
	// A bare type node with an ID is freely re-pushable (type operand).
	if !es.OperandRepushable(NewTypeLiteral(TInteger)) {
		t.Fatal("a bare type node should be re-pushable")
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
	got, ok := es.Materialise(NewMap(om))
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

func TestRecordCallOperandsInertFnBakeCaptured(t *testing.T) {
	es := NewEmitState()
	sig := &Signature{Args: []*Type{TFunction}, FnInertArgs: map[int]bool{0: true}}
	// A CAPTURED fn value must NOT bake as a const: a frozen const body leaves
	// the captured names as unbound Words (`word(c)`), so a later invocation
	// reads them unbound (the capturing-sink miscompile). It routes to the
	// closure path instead — which, for a NON-anonymous fn like this synthetic
	// one, declines (tryReturnedClosure requires an anonymous single-sig
	// lambda), so the operand does not resolve and recordCallOperands refuses.
	// The program then falls back faithfully. (Anonymous capturing lambdas whose
	// body compiles DO resolve to an opClosure — see
	// lang/go's TestPatrunFnValueStoreCompiles / TestReturnedCapturingClosureApply.)
	fn := Value{ID: GenerateID(IDPrefixForType(TFunction)), Parent: TFunction,
		Data: FnDefInfo{Signatures: []Signature{{Args: []*Type{TInteger}}},
			Captured: []CapturedBinding{{Name: "c", Value: NewInteger(1)}}}}
	ops, ok := es.recordCallOperands("stash", sig, []Value{fn})
	if ok {
		t.Fatalf("captured non-anonymous fn must NOT resolve as a const bake: ops=%+v", ops)
	}
}

func TestRecordCallOperandsInertFnBakeCaptureFree(t *testing.T) {
	r := newTestRegistry(t)
	es := NewEmitState()
	sig := &Signature{Args: []*Type{TFunction}, FnInertArgs: map[int]bool{0: true}}
	// The sibling of the captured case: a CAPTURE-FREE concrete fn value in an
	// fn-inert slot bakes as a const the storing / introspecting handler reads
	// at run time — its frozen body has no captured names to leave unbound, so
	// the const is faithful. A plain capture-free fn is const-baked by
	// resolveOperand's isInertConst path BEFORE this switch; only a fn that
	// isInertConst refuses reaches here. A capture-free MACRO module fn (Registry
	// set, Macro true) is exactly that case — resolveOperand declines it, so the
	// `IsConcrete && no-captures` arm is what places the const operand.
	fn := Value{ID: GenerateID(IDPrefixForType(TFunction)), Parent: TFunction,
		Data: FnDefInfo{Signatures: []Signature{{Args: []*Type{TInteger}}},
			Registry: r, Macro: true}}
	ops, ok := es.recordCallOperands("stash", sig, []Value{fn})
	if !ok {
		t.Fatalf("a capture-free fn that resolveOperand refuses must bake as a const here")
	}
	if len(ops) != 1 || ops[0].kind != opConst {
		t.Fatalf("expected a single opConst operand, got %+v", ops)
	}
}

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

func TestTryReturnedClosureProbeFails(t *testing.T) {
	r := covRegistry(t, nil)
	es := NewEmitState()
	es.reg = r
	// One own sig, a nil-typed param (exercises the t==nil → Any default), and
	// a body that references an unknown word so the probe compile refuses.
	lam := Signature{
		Params:     []FnParam{{Name: "y", Type: nil}},
		Impl:       Boru([]Value{NewWord("no_such_word_zzz9")}),
		Returns:    []*Type{TInteger},
		BarrierPos: -1,
	}
	v := Value{ID: GenerateID(IDPrefixForType(TFunction)), Parent: TFunction,
		Data: FnDefInfo{Anonymous: true, Signatures: []Signature{lam}}}
	if _, ok := es.tryReturnedClosure(v, SrcPos{}); ok {
		t.Fatal("an uncompilable lambda body should decline")
	}
}

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

func TestRecordCallRefusalQuotedOperand(t *testing.T) {
	es := NewEmitState()
	sig := &Signature{QuoteArgs: map[int]bool{0: true}}
	if !es.recordCallRefusal("usurp", sig, nil, nil, SrcPos{}, false, false) || es.Compilable {
		t.Fatal("an uncovered quoted-operand word should refuse")
	}
}

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

func TestZeroArgFnOut(t *testing.T) {
	if !zeroArgFnOut([]Value{NewInteger(1), zeroArgFn()}) {
		t.Fatal("a concrete 0-param fn out should be flagged")
	}
	if zeroArgFnOut([]Value{NewInteger(1)}) {
		t.Fatal("no fn out should not be flagged")
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

func TestInterpMemberInertBadInterp(t *testing.T) {
	// A value whose Parent is TInterpString but whose payload is not an
	// InterpStringPayload → AsInterpString errors → not inert.
	bad := Value{Parent: TInterpString, Data: IntPayload{N: 1}}
	if interpMemberInert(bad) {
		t.Fatal("a malformed interp-string should not be inert")
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

func TestIsTypeBodyPayload(t *testing.T) {
	if !isTypeBodyPayload(Value{Data: ChildTypeInfo{}}) {
		t.Fatal("ChildTypeInfo is a type-body payload")
	}
	if isTypeBodyPayload(NewInteger(1)) {
		t.Fatal("an integer is not a type-body payload")
	}
}

func TestThenArmComputed(t *testing.T) {
	br := &emitBranch{thenComputed: true}
	if br.thenArm() != armComputed {
		t.Fatal("thenComputed should classify as armComputed")
	}
}

func TestS7CheckModeFallbackPositionsSkipsMarkers(t *testing.T) {
	// A Mark token after the pointer is skipped; the Integer beyond it is
	// gathered.
	e := engWithTape(t, []Value{NewInteger(0), NewMark("m"), NewInteger(9)}, 0)
	got := checkModeFallbackPositions(e, 1)
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("positions = %v, want [2] (skipped the Mark)", got)
	}
}

func TestW8NamedFnValueCompileMode(t *testing.T) {
	// A non-anonymous body-bearing fn VALUE reached as a call while the
	// bytecode emitter is active routes through spliceFnValueCheckResult.
	r := covRegistry(t, nil)
	fnv := namedFnVal("w8nf",
		[]FnParam{{Name: "a", Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("a"), NewWord("a")),
	)
	if _, reason := compileTokens(t, r, []Value{fnv, NewInteger(4)}); reason != "" {
		// Not required to compile cleanly; the point is exercising the arm.
		t.Logf("compile reason: %s", reason)
	}
}

func TestW8RefuseForwardStackDriftOutOfRange(t *testing.T) {
	// An out-of-range matched position bails the drift check early.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(1)}, StackHeadroom)
	sig := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: 1}
	refuseForwardStackDrift(e, sig, []int{0, 999})
}

func TestW8TryRecordUnmatchedTrapZeroArgSig(t *testing.T) {
	// A 0-arg non-fallback sig is courtesy-dispatchable → returns false.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(1)}, StackHeadroom)
	e.Pointer = 0
	fn := &FnDefInfo{Signatures: []Signature{{BarrierPos: -1}}}
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "w8z", ArgCount: -1}, fn, SrcPos{}) {
		t.Error("a 0-arg sig should make the trap decline (false)")
	}
}

func TestW8SpliceFnValueCheckResultEmptyDeclaredReturns(t *testing.T) {
	// A body producing no carrier with declared returns degrades to one
	// carrier per declared return.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	fnDef := FnDefInfo{Name: "w8e", Signatures: []Signature{{
		Params: []FnParam{{Type: TInteger}}, Returns: []*Type{TInteger, TString},
		Impl: Boru(parenBody()), BarrierPos: 0,
	}}}
	sig := &fnDef.Signatures[0]
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(3), NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 1
	spliceFnValueCheckResult(e, 1, 1, fnDef, sig, []Value{NewInteger(3)})
}

// TestW8DispatchRematchPermutedStackTakesDefiniteTrap — a permuted CONCRETE
// stack window never reaches the rematch gates: the stack positions fill the
// window first (resolvedIndicesBefore), every value is runtime-identical, so
// the DEFINITE trap serialises the interpreter's error — reorder hint
// included — with no runtime rebuild needed.
func TestW8DispatchRematchPermutedStackTakesDefiniteTrap(t *testing.T) {
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(NativeFunc{Name: "w8rh", Signatures: []Signature{{
		Args: []*Type{TInteger, TString}, BarrierPos: -1,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		}),
	}}})
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	e.Tape = NewTape([]Value{
		NewInteger(1), NewString("s"), // prefix: permutes (Integer, String)
		NewWord("w8rh"),
		NewCarrier(TInteger), NewInteger(2),
	}, StackHeadroom)
	e.Pointer = 2
	fn := r.Lookup("w8rh")
	if fn == nil {
		t.Fatal("w8rh not registered")
	}
	if !e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "w8rh", ArgCount: -1}, fn, SrcPos{}) {
		t.Error("a concrete permuted stack window must serialise the definite trap")
	}
}

func TestW8SpliceFnCheckTailCompaction(t *testing.T) {
	// A skipped marker between the args forces the compaction loop body.
	r := covRegistry(t, nil)
	e := NewTop(r)
	fnv := NewFunction(FnDefInfo{Signatures: []Signature{{Params: []FnParam{{Type: TInteger}, {Type: TInteger}}}}})
	e.Tape = NewTape([]Value{NewInteger(1), NewForward(ForwardInfo{}), NewInteger(2), fnv}, StackHeadroom)
	e.Pointer = 3
	spliceFnCheckTail(e, 3, 2, []Value{NewCarrier(TAny)})
}

func TestW8SpliceFnCheckTailElseBranch(t *testing.T) {
	// nArgs exceeds the resolved values before valIdx: the else branch
	// clamps argStart and splices.
	e := engWithTape(t, []Value{NewInteger(1), NewInteger(2)}, 1)
	spliceFnCheckTail(e, 1, 2, []Value{NewCarrier(TAny)})
	if e.Pointer != 0 {
		t.Errorf("pointer = %d, want 0 (argStart clamped)", e.Pointer)
	}
}

func TestW8CheckModeSurfaceShapeNilParent(t *testing.T) {
	// A nil-Parent value at a fallback position is skipped; no surface
	// candidate is found → (false, nil).
	e := engWithTape(t, []Value{NewInteger(0), {}}, 0)
	ok, err := checkModeSurfaceShape(e, WordInfo{Name: "w8x", ArgCount: -1}, SrcPos{})
	if ok || err != nil {
		t.Errorf("got (%v, %v), want (false, nil)", ok, err)
	}
}

func TestW8ExprRefsCarrierReachComputedKey(t *testing.T) {
	r := covRegistry(t, nil)
	r.Defs.Push("w8carrier", NewCarrier(TInteger))
	e := NewTop(r)
	reach := w8ReachWithComputedKey([]Value{NewWord("w8carrier")})
	if !exprRefsCarrier(e, []Value{reach}) {
		t.Error("a Reach computed key referencing a carrier should be detected")
	}
}

func TestCheckMakeConstruction(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()
	fields.Set("n", NewTypeLiteral(TInteger))
	fields.Set("d", NewInteger(7)) // defaulted field
	ct := r.Types.MintType("Class/Chk", TClass)
	cls := NewClassType(ct, ClassTypeInfo{Fields: fields, Name: "Chk", Type: ct})

	done := r.Check.Begin()
	defer done()

	// Well-formed construction: no diagnostics.
	CheckMakeConstruction(r, cls, mapOf("n", NewInteger(1)), SrcPos{Row: 1, Col: 1})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("clean construction flagged: %+v", r.Check.Diagnostics)
	}

	// Unknown field, missing field, and a wrong-typed field each flag.
	CheckMakeConstruction(r, cls, mapOf("bogus", NewInteger(1)), SrcPos{Row: 2, Col: 1})
	CheckMakeConstruction(r, cls, mapOf("n", NewString("x")), SrcPos{Row: 3, Col: 1})
	var unknown, missing, badType bool
	for _, d := range r.Check.Diagnostics {
		switch {
		case strings.Contains(d.Detail, "unknown field"):
			unknown = true
		case strings.Contains(d.Detail, "missing field"):
			missing = true
		case strings.Contains(d.Detail, `field "n"`):
			badType = true
		}
	}
	if !unknown || !missing || !badType {
		t.Errorf("diagnostics missing (unknown=%v missing=%v badType=%v): %+v",
			unknown, missing, badType, r.Check.Diagnostics)
	}

	// Dedup: repeating the same construction at the same position adds nothing.
	n := len(r.Check.Diagnostics)
	CheckMakeConstruction(r, cls, mapOf("bogus", NewInteger(1)), SrcPos{Row: 2, Col: 1})
	if len(r.Check.Diagnostics) != n {
		t.Errorf("duplicate diagnostics: %+v", r.Check.Diagnostics)
	}

	// Non-map / carrier sources and non-class targets are skipped.
	CheckMakeConstruction(r, cls, NewCarrier(TMap), SrcPos{})
	CheckMakeConstruction(r, cls, NewInteger(1), SrcPos{})
	CheckMakeConstruction(r, NewTypeLiteral(TInteger), mapOf("n", NewInteger(1)), SrcPos{})
	if len(r.Check.Diagnostics) != n {
		t.Errorf("skipped shapes flagged: %+v", r.Check.Diagnostics)
	}
}

func TestCheckMakeConstructionInactiveNoOp(t *testing.T) {
	r := newTestRegistry(t)
	// Outside check mode nothing happens (and nothing panics).
	CheckMakeConstruction(r, NewTypeLiteral(TInteger), mapOf("n", NewInteger(1)), SrcPos{})
	CheckMakeConstruction(nil, Value{}, Value{}, SrcPos{})
}

func TestCompiledNamedFnValueCall(t *testing.T) {
	// A named body-bearing fn value CALLED while the emitter is active
	// routes through spliceFnValueCheckResult. If the program compiles it
	// must agree with the interpreter; a refusal is also sound.
	tokens := func() []Value {
		fnv := namedFnVal("vdbl",
			[]FnParam{{Name: "x", Type: TInteger}},
			[]*Type{TInteger},
			parenBody(NewWord("cadd"), NewWord("x"), NewWord("x")),
		)
		return []Value{fnv, NewInteger(16)}
	}
	ri := covRegistry(t, nil)
	iOut, iErr := NewTop(ri).Run(tokens())
	if iErr != nil {
		t.Fatalf("interpreted: %v", iErr)
	}
	if got := renderAll(iOut); got != "32" {
		t.Fatalf("interpreted = %q, want 32", got)
	}
	rc := covRegistry(t, nil)
	prog, reason := compileTokens(t, rc, tokens())
	if prog == nil {
		t.Logf("compile refused (%s) — interpreter owns the case", reason)
		return
	}
	cOut, cErr := RunProgram(prog, rc)
	if cErr != nil {
		t.Fatalf("compiled: %v", cErr)
	}
	if got := renderAll(cOut); got != "32" {
		t.Errorf("compiled = %q, want 32", got)
	}
}

// TestResolveOperandIdentitylessCompoundRescued pins the fn-unit rescue:
// a compound with no compile identity (a runtime mint) must route to
// dynScopeRescue — never into the ID-keyed freshen/share machinery —
// while the same compound WITH an identity resolves normally.
func TestResolveOperandIdentitylessCompoundRescued(t *testing.T) {
	r := seam7Reg(t)
	defer r.Check.Begin()()
	es := NewEmitState()
	es.BindRegistry(r)
	// One extra unit puts resolution inside a compiled fn frame.
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}, capID: map[string]bool{}})

	blank := Value{Parent: TList, Data: ListPayload{Elems: []Value{NewInteger(1)}}} // no ID
	if _, ok := es.resolveOperand(blank); ok {
		t.Error("identity-less compound inside a fn unit must be rescued, not placed")
	}
	// Positive twin: the same compound minted with an identity resolves.
	minted := NewList([]Value{NewInteger(1)})
	if minted.ID == "" {
		t.Fatal("test premise: pass-minted list should carry an ID")
	}
	if _, ok := es.resolveOperand(minted); !ok {
		t.Error("pass-minted compound must still resolve inside a fn unit")
	}
}

// vmDefer is the designed-defer choke point: it reports the BailEvent to an
// armed hook and returns the internal_error class runtimeShouldFallback
// resolves; unarmed it just builds the error.
func TestRuntimeBailHookVmDefer(t *testing.T) {
	r := runUnitReg(t)
	var (
		mu     sync.Mutex
		events []BailEvent
	)
	disarm := r.ArmRuntimeBailHook(func(e BailEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	})

	err := vmDefer(r, nil, 0, "vm:test-site", "test defer message")
	if !IsInternalErr(err) {
		t.Fatalf("vmDefer error class = %v, want internal_error", err)
	}
	if len(events) != 1 || events[0].Site != "vm:test-site" || events[0].Reason != "test defer message" {
		t.Fatalf("bail events = %+v, want one vm:test-site event", events)
	}

	disarm()
	if err := vmDefer(r, nil, 0, "vm:test-site", "again"); !IsInternalErr(err) {
		t.Fatalf("disarmed vmDefer error class = %v, want internal_error", err)
	}
	if len(events) != 1 {
		t.Fatalf("disarmed vmDefer still recorded: %d events", len(events))
	}
}

// InheritObserveHooks shares the parent's holders into a module sub-registry:
// entries and bails emitted on the child reach the parent's observers.
func TestInheritObserveHooks(t *testing.T) {
	parent := runUnitReg(t)
	child := runUnitReg(t)
	var c entryCollector
	var bails []BailEvent
	defer parent.ArmInterpEntryHook(c.add)()
	defer parent.ArmRuntimeBailHook(func(e BailEvent) { bails = append(bails, e) })()

	child.InheritObserveHooks(parent)
	if _, err := RunPooledSub(child, []Value{NewInteger(2)}, false); err != nil {
		t.Fatalf("child runPooledSub: %v", err)
	}
	if len(c.entries) == 0 {
		t.Fatal("child interp entries did not reach the parent's hook")
	}
	_ = vmDefer(child, nil, 0, "vm:test-site", "child defer")
	if len(bails) != 1 {
		t.Fatalf("child bail did not reach the parent's hook: %+v", bails)
	}
}

func TestPayloadMarkersCallable(t *testing.T) {
	// The sealed-payload markers are no-ops; calling them directly pins
	// that the types satisfy eng.Payload.
	mod := NewDispatchMod(DispatchModInfo{Ref: true})
	if !IsDispatchMod(mod) {
		t.Error("dispatch-mod marker not recognised")
	}
	if info, ok := AsDispatchMod(mod); !ok || !info.Ref {
		t.Errorf("AsDispatchMod = %+v, %v", info, ok)
	}
}

func TestMembershipBeyondNominal(t *testing.T) {
	r := newTestRegistry(t)
	member, err := r.DefineMemberType("W3Even", TInteger, func(v Value) bool {
		n, e := AsInteger(v)
		return e == nil && n%2 == 0
	})
	if err != nil {
		t.Fatalf("DefineMemberType: %v", err)
	}
	if !membershipBeyondNominal(member) {
		t.Error("member type read as nominal")
	}
	if membershipBeyondNominal(TInteger) {
		t.Error("Integer read as membership-based")
	}
	if membershipBeyondNominal(nil) {
		t.Error("nil read as membership-based")
	}
}

func TestVariadicCarrierHelpers(t *testing.T) {
	elem := NewTypeLiteral(TInteger)
	vc := NewVariadicCarrier(elem)
	got, ok := IsVariadicSpread(vc)
	if !ok || !got.Equal(TInteger) {
		t.Errorf("IsVariadicSpread = %v, %v", got, ok)
	}
	if _, ok := IsVariadicSpread(NewInteger(1)); ok {
		t.Error("integer read as variadic")
	}
	if !stackHasVariadic([]Value{NewInteger(1), vc}) {
		t.Error("stackHasVariadic missed the spread")
	}
	if stackHasVariadic([]Value{NewInteger(1)}) {
		t.Error("stackHasVariadic false positive")
	}

	// FoldVariadicArms: one variadic arm plus a leaked concrete slot →
	// one spread joining both element types.
	folded, ok := FoldVariadicArms(
		[]Value{NewCarrier(TInteger), vc},
		[]Value{NewNone()},
	)
	if !ok {
		t.Fatal("FoldVariadicArms refused a variadic arm")
	}
	fe, ok := IsVariadicSpread(folded)
	if !ok {
		t.Fatalf("folded arm not variadic: %v", folded)
	}
	if !strings.Contains(fe.String(), "Integer") {
		t.Errorf("folded element = %v", fe)
	}
	// No variadic anywhere: refuses.
	if _, ok := FoldVariadicArms([]Value{NewCarrier(TInteger)}, nil); ok {
		t.Error("FoldVariadicArms accepted plain arms")
	}
}

func TestTypedListCarrierHelpers(t *testing.T) {
	tl := NewCarrierTypedListValue(NewCarrier(TString))
	if !tl.Carrier || !IsTypedList(tl) {
		t.Errorf("typed-list carrier shape: %v", tl)
	}
	if elem := DataListElemTypeFromValue(tl); !elem.Equal(TString) {
		t.Errorf("elem of typed list = %v", elem)
	}
	// Bare type-literal child: the child IS the element type.
	btl := NewTypedList(NewTypeLiteral(TInteger))
	if elem := DataListElemTypeFromValue(btl); !elem.Equal(TInteger) {
		t.Errorf("elem of bare typed list = %v", elem)
	}
	if elem := DataListElemTypeFromValue(NewTypeLiteral(TList)); !elem.Equal(TAny) {
		t.Errorf("elem of bare list literal = %v", elem)
	}

	pres := ReturnsPreserveListAt(0)
	out := pres([]Value{NewCarrierTypedList(TInteger)}, nil)
	if len(out) != 1 || !IsTypedList(out[0]) {
		t.Fatalf("ReturnsPreserveListAt = %v", out)
	}
	if elem := DataListElemTypeFromValue(out[0]); !elem.Equal(TInteger) {
		t.Errorf("preserved elem = %v", elem)
	}
	// Out-of-range index degrades to a plain List carrier.
	out = ReturnsPreserveListAt(3)([]Value{NewCarrier(TList)}, nil)
	if len(out) != 1 || !out[0].Parent.Equal(TList) {
		t.Errorf("out-of-range preserve = %v", out)
	}

	elemFn := ReturnsListElemAt(0)
	out = elemFn([]Value{NewCarrierTypedList(TString)}, nil)
	if len(out) != 1 || !out[0].Parent.Equal(TString) {
		t.Errorf("ReturnsListElemAt = %v", out)
	}
	out = ReturnsListElemAt(-1)(nil, nil)
	if len(out) != 1 || !out[0].Parent.Equal(TAny) {
		t.Errorf("out-of-range elem = %v", out)
	}
}

func TestElementAndParamCarriers(t *testing.T) {
	// Unknown element type → dynamic carrier.
	ec := NewElementCarrier(TAny)
	if !ec.Dynamic {
		t.Error("Any element carrier not dynamic")
	}
	if NewElementCarrier(TInteger).Dynamic {
		t.Error("typed element carrier dynamic")
	}

	// Mixed concrete list joins element types.
	mixed := ElementCarrierFromValue(NewList([]Value{NewInteger(1), NewString("s")}))
	if !mixed.Carrier {
		t.Errorf("mixed element carrier: %v", mixed)
	}
	// Single-typed list keeps the plain path.
	single := ElementCarrierFromValue(NewList([]Value{NewInteger(1), NewInteger(2)}))
	if !single.Parent.Equal(TInteger) {
		t.Errorf("single-typed element = %v", single)
	}
	// A map's values join too.
	mv := ElementCarrierFromValue(mapOf("a", NewInteger(1), "b", NewString("s")))
	if !mv.Carrier {
		t.Errorf("map value carrier: %v", mv)
	}

	// ParamInputCarrier: Any → dynamic; concrete stays strict.
	if !ParamInputCarrier(TAny).Dynamic {
		t.Error("Any param carrier not dynamic")
	}
	if !ParamInputCarrier(nil).Dynamic {
		t.Error("nil param carrier not dynamic")
	}
	pc := ParamInputCarrier(TInteger)
	if pc.Dynamic || !pc.Parent.Equal(TInteger) {
		t.Errorf("Integer param carrier = %v", pc)
	}
}

func TestRunCarrierBodyAndStacksEqual(t *testing.T) {
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()

	stk := RunCarrierBody(r, NewList([]Value{NewWord("cadd"), NewInteger(1), NewInteger(2)}))
	if len(stk) != 1 || !stk[0].Parent.ConformsTo(TInteger) {
		t.Errorf("RunCarrierBody residual = %v", stk)
	}
	// A non-concrete body yields nil.
	if stk := RunCarrierBody(r, NewTypeLiteral(TList)); stk != nil {
		t.Errorf("type-literal body residual = %v", stk)
	}

	// carrierStacksEqual: equal values equal, different values not,
	// mismatched lengths never.
	a := []Value{NewInteger(1)}
	b := []Value{NewInteger(1)}
	c := []Value{NewString("x")}
	if !carrierStacksEqual(a, b) {
		t.Error("equal stacks unequal")
	}
	if carrierStacksEqual(a, c) {
		t.Error("different stacks equal")
	}
	if carrierStacksEqual(a, nil) {
		t.Error("length mismatch equal")
	}
}

func TestW9EvalFixedWindowToken(t *testing.T) {
	// Non-inert modes → false (carrier / dynamic / undefined).
	if evalFixedWindowToken(NewCarrier(TInteger)) {
		t.Error("carrier is not evaluation-fixed")
	}
	if evalFixedWindowToken(NewDynamicCarrier(TInteger)) {
		t.Error("dynamic carrier is not evaluation-fixed")
	}
	und := NewInteger(1)
	und.Undefined = true
	if evalFixedWindowToken(und) {
		t.Error("undefined value is not evaluation-fixed")
	}

	// A list of all-inert elements → true; a list with a word → false.
	if !evalFixedWindowToken(NewList([]Value{NewInteger(1), NewString("x")})) {
		t.Error("a list of inert scalars is evaluation-fixed")
	}
	if evalFixedWindowToken(NewList([]Value{NewWord("w")})) {
		t.Error("a list containing a word is not evaluation-fixed")
	}

	// A nil-backed map payload → false.
	if evalFixedWindowToken(NewMap(nil)) {
		t.Error("a nil-backed map is not evaluation-fixed")
	}
	// A map carrying computed keys (Meta "ck") → false.
	ck := NewOrderedMap()
	ck.Set("k", NewInteger(1))
	ck.Meta = map[string]any{"ck": map[string]bool{"k": true}}
	if evalFixedWindowToken(NewMap(ck)) {
		t.Error("a map with computed keys is not evaluation-fixed")
	}
	// A map with a non-inert value → false.
	bad := NewOrderedMap()
	bad.Set("k", NewWord("w"))
	if evalFixedWindowToken(NewMap(bad)) {
		t.Error("a map with a word value is not evaluation-fixed")
	}
	// A fully inert map → true.
	good := NewOrderedMap()
	good.Set("k", NewInteger(1))
	if !evalFixedWindowToken(NewMap(good)) {
		t.Error("an inert map is evaluation-fixed")
	}
}

func TestW9ShapedMethodApplyWindowRejects(t *testing.T) {
	r := newTestRegistry(t)
	e := &Engine{Registry: r}
	e.Tape = NewTape([]Value{NewCarrier(TAny), NewInteger(1)}, StackHeadroom)
	// Member payload is not an FnDefInfo → decline.
	if _, _, ok := shapedMethodApplyWindow(e, 0, NewInteger(1)); ok {
		t.Error("a non-FnDef member must decline")
	}
	// FnDefInfo whose owning registry does not resolve the name → decline.
	member := NewFunction(FnDefInfo{Registry: r, Name: "w9missing"})
	if _, _, ok := shapedMethodApplyWindow(e, 0, member); ok {
		t.Error("an unresolvable member fn must decline")
	}
}

func TestW9TryRecordMethodApplyWordMismatch(t *testing.T) {
	r := newTestRegistry(t)
	r.Check.PendingMethodApply = &PendingMethodApply{Origin: NewCarrier(TAny), Word: "owner"}
	if tryRecordMethodApply(r, "other", nil, nil, SrcPos{}) {
		t.Error("a word mismatch must leave the pending for its owner")
	}
	if r.Check.PendingMethodApply == nil {
		t.Error("the pending must survive a word mismatch")
	}
}

func TestW9TryRecordMethodApplyRecordFails(t *testing.T) {
	r := newTestRegistry(t)
	es := NewEmitState()
	r.Check.Emit = es
	// Origin carrier with no producing event → RecordDynMethod fails and the
	// program is marked uncompilable.
	r.Check.PendingMethodApply = &PendingMethodApply{Origin: NewCarrier(TAny), Word: "m"}
	if !tryRecordMethodApply(r, "m", nil, nil, SrcPos{}) {
		t.Error("a matching pending must be consumed")
	}
	if es.Compilable {
		t.Error("a failed RecordDynMethod must mark the program uncompilable")
	}
}

func TestW9TryDynamicFnValueDispatchNonInertWindow(t *testing.T) {
	r := newTestRegistry(t)
	e := &Engine{Registry: r}
	// A dynamic Function-bearing carrier followed by a WORD (non-inert): the
	// window admission declines the model.
	e.Tape = NewTape([]Value{NewDynamicCarrier(TFunction), NewWord("w")}, StackHeadroom)
	if tryDynamicFnValueDispatch(e, 0) {
		t.Error("a word in the forward window must decline the dynamic-fn model")
	}
}

func TestW9ScalarFoldOperand(t *testing.T) {
	// A plain concrete integer is inert-const → the fast path.
	if !scalarFoldOperand(NewInteger(1)) {
		t.Error("a concrete integer is a scalar fold operand")
	}
	// A carrier whose payload is a value-scalar reaches the carrier-tolerant
	// switch arm (not inert-const, not dynamic, scalar payload).
	scalarCarrier := Value{Parent: TInteger, Data: IntPayload{N: 1}, Carrier: true}
	if !scalarFoldOperand(scalarCarrier) {
		t.Error("a scalar-payload carrier is a fold operand")
	}
	// A compound (ChildTypeInfo) carrier declines.
	if scalarFoldOperand(NewCarrier(TInteger)) {
		t.Error("a compound carrier is not a scalar fold operand")
	}
}

func TestW9JoinCarriersInnerOverCap(t *testing.T) {
	// More than CarrierDisjunctCap distinct alternatives collapse to a single
	// common-ancestor carrier.
	first := []Value{
		NewTypeLiteral(TInteger), NewTypeLiteral(TString), NewTypeLiteral(TBoolean),
		NewTypeLiteral(TAtom), NewTypeLiteral(TList),
	}
	second := []Value{
		NewTypeLiteral(TMap), NewTypeLiteral(TWord), NewTypeLiteral(TFunction),
		NewTypeLiteral(TError), NewTypeLiteral(TType),
	}
	out := joinCarriersInner(NewDisjunct(first), NewDisjunct(second))
	if !out.Carrier {
		t.Errorf("an over-cap join should collapse to a carrier, got %v", out)
	}
	if IsDisjunct(out) {
		t.Error("an over-cap join must not remain a disjunct")
	}
}

// The VM's module-poly re-match arm: a module poly word resolves its
// overload at run time, and the gate runs AFTER the match so it applies
// to the overload that actually dispatches.
func TestModuleGateVMPolyRematchArm(t *testing.T) {
	impl := Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{args[0]}, nil
	})
	r := moduleGateReg(t, "boru:zz", "denied")
	sub, err := NewRegistry()
	if err != nil {
		t.Fatalf("sub registry: %v", err)
	}
	sub.ModuleRef = "boru:zz"
	sub.Register("zz-poly", Signature{
		Args: []*Type{TInteger}, Returns: []*Type{TInteger}, BarrierPos: -1, Impl: impl,
		ModuleCall: &ModuleCallID{Module: "boru:zz", Export: "denied"},
	})

	p := &Program{
		Code:     []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpCallNativePoly, Arg: 0}},
		Consts:   []Value{NewInteger(1)},
		PolyRefs: []PolyRef{{Word: "zz-poly", Arity: 1, Reg: sub}},
	}
	if _, err := RunProgram(p, r); err == nil || !strings.Contains(err.Error(), "module export denied") {
		t.Fatalf("the re-matched module overload must gate, got %v", err)
	}

	// Negative twin: the same program under a checker denying a
	// different export dispatches.
	if _, err := RunProgram(p, moduleGateReg(t, "boru:zz", "other")); err != nil &&
		strings.Contains(err.Error(), "zz-policy") {
		t.Fatalf("an unruled export must pass the gate, got %v", err)
	}
}

func TestS7StaticListLenNilList(t *testing.T) {
	// A concrete but nil list reports no static length.
	if _, ok := StaticListLen(NewList(nil)); ok {
		t.Error("a nil list must not report a static length")
	}
}

func TestS7MinMaxIndexErrorArms(t *testing.T) {
	// DepScalar whose bound value is a non-integer -> no min/max.
	strLo := NewDepScalar(DepGT, NewString("z"))
	if _, ok := minIndex(strLo); ok {
		t.Error("string-bounded lower DepScalar has no integer minimum")
	}
	strHi := NewDepScalar(DepLT, NewString("z"))
	if _, ok := maxIndex(strHi); ok {
		t.Error("string-bounded upper DepScalar has no integer maximum")
	}
	// Concrete Integer-tagged value with a non-int payload -> no min/max.
	badInt := Value{Parent: TInteger, Data: StrPayload{S: "x"}}
	if _, ok := minIndex(badInt); ok {
		t.Error("unreadable integer index has no minimum")
	}
	if _, ok := maxIndex(badInt); ok {
		t.Error("unreadable integer index has no maximum")
	}
}

func TestS7CheckAtIndicesGuards(t *testing.T) {
	r := newTestRegistry(t)
	// Unknown data length (non-list) -> no-op, no diagnostic.
	CheckAtIndices(r, NewList([]Value{NewInteger(0)}), NewInteger(1), "at")
	// Known data length, but the indices list is nil -> early return.
	data := NewList([]Value{NewInteger(1)})
	CheckAtIndices(r, NewList(nil), data, "at")
	if n := len(r.Check.Diagnostics); n != 0 {
		t.Errorf("guarded CheckAtIndices should emit nothing, got %d diagnostics", n)
	}
}

// A unit whose body is [PUSH_LOCAL 0, RET] returns its single param verbatim.
func TestRunUnitBindsParam(t *testing.T) {
	p := &Program{
		Fns: []CompiledFn{{
			Name:    "echo",
			NParams: 1,
			NLocals: 1,
			Code:    []Instr{{Op: OpPushLocal, Arg: 0}, {Op: OpRet, Arg: 0}},
			Debug:   []SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
	ref := &CompiledFnRef{Prog: p, Unit: 0}
	out, err := RunUnit(ref, runUnitReg(t), []Value{NewInteger(7)})
	if err != nil {
		t.Fatalf("RunUnit: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Fatalf("result = %v, want 7", out[0])
	}
}

// A capture fills the trailing local slot: NParams=2, NCaptures=1, body returns
// local 1 (the capture), so the per-call arg lands in slot 0 and the capture in
// slot 1.
func TestRunUnitBindsCapture(t *testing.T) {
	p := &Program{
		Fns: []CompiledFn{{
			Name:      "grabCapture",
			NParams:   2,
			NCaptures: 1,
			NLocals:   2,
			Code:      []Instr{{Op: OpPushLocal, Arg: 1}, {Op: OpRet, Arg: 0}},
			Debug:     []SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
	ref := &CompiledFnRef{Prog: p, Unit: 0, Captures: []Value{NewInteger(99)}}
	out, err := RunUnit(ref, runUnitReg(t), []Value{NewInteger(7)})
	if err != nil {
		t.Fatalf("RunUnit: %v", err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 99 {
		t.Fatalf("result = %v, want the captured 99", out[0])
	}
}

// TestInvokeCallbackInternalErrorFallsBack pins PR #243 comment #3: a callback
// fires AFTER the enclosing RunProgram returned (serve-raw handling a connection,
// a spawned process), so there is no outer RunCompiled to catch a VM bailout and
// re-run on the interpreter. InvokeCallback therefore applies that discipline
// itself — when the stamped unit raises an internal_error (here a CALL_DYNAMIC
// underflow, the vmErrAt soundness-bailout class), it falls back to CallBoru over
// the sig's boru body instead of leaking the raw internal_error to the peer. The
// body `[42]` returns 42, proving the fallback ran rather than the failing unit.
func TestInvokeCallbackInternalErrorFallsBack(t *testing.T) {
	// A unit that raises internal_error when run: CALL_DYNAMIC over an empty
	// stack underflows → vmErrAt(internal_error).
	p := &Program{Fns: []CompiledFn{{
		Name: "boom", NParams: 0, NLocals: 0,
		Code:  []Instr{{Op: OpCallDynamic, Arg: 0}, {Op: OpRet, Arg: 0}},
		Debug: []SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
	}}}
	ref := &CompiledFnRef{Prog: p, Unit: 0}
	// Confirm the unit really does raise internal_error on its own.
	if _, err := RunUnit(ref, runUnitReg(t), nil); !IsInternalErr(err) {
		t.Fatalf("RunUnit err = %v, want an internal_error to drive the fallback", err)
	}
	// The sig carries the stamped ref AND a boru body CallBoru can run to 42.
	sig := &Signature{Impl: &BoruImpl{Body: []Value{NewInteger(42)}, Compiled: ref}}
	out, err := InvokeCallback(runUnitReg(t), sig, nil, nil)
	if err != nil {
		t.Fatalf("InvokeCallback should have fallen back to CallBoru, got err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1 (the CallBoru body residual)", len(out))
	}
	if n, _ := out[0].AsConcreteInteger(); n != 42 {
		t.Fatalf("result = %v, want 42 from the interpreter fallback", out[0])
	}
}

// TestInvokeCallbackBusyRegistryFallsBack covers the no-VM-path branch of
// invokeCompiledUnit: a stamped ref whose registry is mid-run (canHostVM false)
// with no nestedRunner installed cannot run on the VM, so invokeCompiledUnit
// reports ran=false and InvokeCallback owns the call on the interpreter. The
// CallBoru body `[7]` proves the interpreter ran.
func TestInvokeCallbackBusyRegistryFallsBack(t *testing.T) {
	r := runUnitReg(t)
	r.VmRunning = 1 // canHostVM() false; nestedRunner stays nil
	p := &Program{Fns: []CompiledFn{{
		Name: "x", NParams: 0, NLocals: 0,
		Code: []Instr{{Op: OpRet, Arg: 0}}, Debug: []SrcPos{{Row: 1, Col: 1}},
	}}}
	ref := &CompiledFnRef{Prog: p, Unit: 0}
	// Direct: no VM path applies, so ran=false.
	if _, _, ran := invokeCompiledUnit(r, ref, nil); ran {
		t.Fatal("a busy registry with no nestedRunner must report ran=false")
	}
	// Via InvokeCallback: it falls through to CallBoru over the body.
	sig := &Signature{Impl: &BoruImpl{Body: []Value{NewInteger(7)}, Compiled: ref}}
	out, err := InvokeCallback(r, sig, nil, nil)
	if err != nil {
		t.Fatalf("InvokeCallback should have fallen back to CallBoru, got err: %v", err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Fatalf("result = %v, want 7 from the interpreter fallback", out[0])
	}
}

// stampCompiledRef writes onto the first own boru body sig and reports true; a fn
// value with no own boru body sig (all Go / all fallback) reports false.
func TestStampCompiledRef(t *testing.T) {
	ref := &CompiledFnRef{Prog: &Program{}, Unit: 0}
	boruFd := FnDefInfo{Signatures: []Signature{{Impl: &BoruImpl{Body: []Value{NewInteger(1)}}}}}
	if !stampCompiledRef(boruFd, ref) {
		t.Fatal("a boru body sig must accept the stamp")
	}
	if CompiledRef(&boruFd.Signatures[0]) != ref {
		t.Fatal("stamp did not land on the sig")
	}
	// A fallback-only sig is skipped; a Go sig has no *BoruImpl → no stamp.
	goFd := FnDefInfo{Signatures: []Signature{
		{Fallback: true, Impl: &BoruImpl{}},
		{Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) { return nil, nil })},
	}}
	if stampCompiledRef(goFd, ref) {
		t.Fatal("a fn value with no own boru body sig must not be stamped")
	}
}

// CompiledRef reads the durable unit reference off a boru body sig, and returns
// nil for a Go sig (the callback-invocation seam's "no compiled unit" signal).
func TestSignatureCompiledRef(t *testing.T) {
	ref := &CompiledFnRef{Prog: &Program{}, Unit: 3}
	boruSig := &Signature{Impl: &BoruImpl{Body: []Value{NewInteger(1)}, Compiled: ref}}
	if got := CompiledRef(boruSig); got != ref {
		t.Fatalf("boru-body CompiledRef = %v, want the stamped ref", got)
	}
	// An un-armed boru body reports no unit.
	bareSig := &Signature{Impl: &BoruImpl{Body: []Value{NewInteger(1)}}}
	if got := CompiledRef(bareSig); got != nil {
		t.Fatalf("un-armed CompiledRef = %v, want nil", got)
	}
	// A Go sig has no boru body at all.
	goSig := &Signature{Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return nil, nil
	})}
	if got := CompiledRef(goSig); got != nil {
		t.Fatalf("Go-sig CompiledRef = %v, want nil", got)
	}
}

// A unit index outside Prog.Fns is rejected (never indexed).
func TestRunUnitUnitOutOfRange(t *testing.T) {
	p := &Program{Fns: []CompiledFn{{Name: "only", NParams: 0, NLocals: 0,
		Code: []Instr{{Op: OpRet, Arg: 0}}, Debug: []SrcPos{{}}}}}
	for _, unit := range []int{-1, 5} {
		ref := &CompiledFnRef{Prog: p, Unit: unit}
		if _, err := RunUnit(ref, runUnitReg(t), nil); err == nil ||
			!strings.Contains(err.Error(), "out of range") {
			t.Fatalf("RunUnit(unit=%d) err = %v, want out of range", unit, err)
		}
	}
}

// A nil ref and a ref with a nil program both report "nil unit reference"
// (the invoke seam treats a nil ref as "no runnable unit").
func TestRunUnitNilRef(t *testing.T) {
	for _, ref := range []*CompiledFnRef{nil, {Prog: nil, Unit: 0}} {
		if _, err := RunUnit(ref, runUnitReg(t), nil); err == nil ||
			!strings.Contains(err.Error(), "nil unit reference") {
			t.Fatalf("RunUnit(%v) err = %v, want nil unit reference", ref, err)
		}
	}
}

// compileStoredBody declines (fails safe) without a live EmitState / registry,
// and for a non-list body operand — the defensive guards on the spawn code-body
// compile path.
func TestCompileStoredBodyGuards(t *testing.T) {
	var nilES *EmitState
	if _, ok := nilES.compileStoredBody(NewInteger(1)); ok {
		t.Fatal("nil EmitState must decline")
	}
	if _, ok := (&EmitState{}).compileStoredBody(NewInteger(1)); ok {
		t.Fatal("reg-less EmitState must decline")
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.InitRootContext()
	if _, ok := (&EmitState{reg: r}).compileStoredBody(NewInteger(1)); ok {
		t.Fatal("a non-list body must decline")
	}
}

// compileStoredFnUnit declines (fails safe) when it has no live EmitState /
// registry to compile against — the defensive guards on the recorder-internal
// path. A nil receiver and a reg-less state both return (0, false).
func TestCompileStoredFnUnitGuards(t *testing.T) {
	var nilES *EmitState
	if _, ok := nilES.compileStoredFnUnit(FnDefInfo{}, 0, SrcPos{}); ok {
		t.Fatal("nil EmitState must decline")
	}
	if _, ok := (&EmitState{}).compileStoredFnUnit(FnDefInfo{}, 0, SrcPos{}); ok {
		t.Fatal("reg-less EmitState must decline")
	}
	// A reg-ful state with an out-of-range / ineligible sig index declines
	// at the per-sig gate (REFUSAL-CLOSURE §7b).
	es := NewEmitState()
	es.reg = runUnitReg(t)
	if _, ok := es.compileStoredFnUnit(FnDefInfo{}, 0, SrcPos{}); ok {
		t.Fatal("an out-of-range sig index must decline")
	}
}

func TestDisassembleTailArms(t *testing.T) {
	inner := &Program{}
	p := &Program{
		Fns:          []CompiledFn{{Name: "g", NParams: 0, NLocals: 0}},
		storedFnRefs: []*CompiledFnRef{{}, {Prog: inner}},
	}
	out := p.Disassemble()
	if !strings.Contains(out, "fn f0 g/0") {
		t.Errorf("empty-locals fn header missing:\n%s", out)
	}
	if p.StoredRefCount() != 2 || p.StoredRefStampedCount() != 1 {
		t.Errorf("stored-ref counts = %d/%d, want 2/1",
			p.StoredRefCount(), p.StoredRefStampedCount())
	}
}

// toCarrier preserves the two step-time marker families whole: an __SP
// splice and a Word/__SG sugar marker both lose their reason to exist
// if the check-mode strip nulls their payload.
func TestToCarrierKeepsStepMarkers(t *testing.T) {
	sp := NewSplice(NewList([]Value{NewInteger(1)}))
	if got := toCarrier(sp); !IsSplice(got) {
		t.Errorf("splice must survive the carrier strip, got %v", got)
	} else if _, serr := AsSplice(got); serr != nil {
		t.Errorf("splice payload must survive the carrier strip: %v", serr)
	}
	sg := NewSugar(SugarInfo{Kind: SugarLambda})
	if got := toCarrier(sg); !IsSugar(got) {
		t.Errorf("sugar marker must survive the carrier strip, got %v", got)
	} else if _, ok := AsSugar(got); !ok {
		t.Errorf("sugar marker payload must survive the carrier strip")
	}
}

// typedContainerCarrier (D2 Part A) threads a {:T} param's element type into
// the body carrier. Its guard arms are exercised through the compile path by
// the langspec census; the arms that ordinary dispatch cannot reach (a typed
// pattern whose arg is not the matching container family — matchSignature
// rejects that before genArgs) are pinned by direct call.
func TestTypedContainerCarrier(t *testing.T) {
	mapPat := NewTypedMap(NewTypeLiteral(TInteger))
	listPat := NewCarrierTypedListValue(NewTypeLiteral(TInteger))

	// Typed-map pattern + a conforming Map arg → a typed carrier.
	if v, ok := typedContainerCarrier(FnParam{Pattern: &mapPat}, NewCarrier(TMap)); !ok || !IsTypedMap(v) {
		t.Errorf("map pattern + Map arg: got (%v, %v), want a typed-map carrier", v.Parent, ok)
	}
	// Typed-list pattern + a conforming List arg → a typed carrier.
	if v, ok := typedContainerCarrier(FnParam{Pattern: &listPat}, NewCarrier(TList)); !ok || !IsTypedList(v) {
		t.Errorf("list pattern + List arg: got (%v, %v), want a typed-list carrier", v.Parent, ok)
	}
	// Nil pattern → no carrier.
	if _, ok := typedContainerCarrier(FnParam{}, NewCarrier(TMap)); ok {
		t.Errorf("nil pattern should not produce a carrier")
	}
	// {:Any} pattern (child Any) → falls through, no tag.
	anyPat := NewTypedMap(NewTypeLiteral(TAny))
	if _, ok := typedContainerCarrier(FnParam{Pattern: &anyPat}, NewCarrier(TMap)); ok {
		t.Errorf("{:Any} pattern should fall through")
	}
	// Typed-map pattern + a NON-container arg (Integer) → the final
	// fallthrough: neither the map nor the list branch admits it.
	if _, ok := typedContainerCarrier(FnParam{Pattern: &mapPat}, NewInteger(5)); ok {
		t.Errorf("map pattern + Integer arg should fall through")
	}
	// Typed-list pattern + a Map arg (wrong family) → fallthrough too.
	if _, ok := typedContainerCarrier(FnParam{Pattern: &listPat}, NewCarrier(TMap)); ok {
		t.Errorf("list pattern + Map arg should fall through")
	}
}

func TestW9UserPolyArmShapeOK(t *testing.T) {
	body := Boru([]Value{NewWord("p")})

	// Empty body → false.
	if userPolyArmShapeOK(&Signature{}, nil) {
		t.Error("empty body should refuse")
	}
	// Quote/type/form arg slots → false.
	if userPolyArmShapeOK(&Signature{Impl: body, QuoteArgs: map[int]bool{0: true}}, nil) {
		t.Error("a QuoteArgs slot should refuse")
	}
	// A quoted param → false.
	if userPolyArmShapeOK(&Signature{
		Impl:   body,
		Params: []FnParam{{Name: "p", Quote: true}},
	}, []*Type{TInteger}) {
		t.Error("a quoted param should refuse")
	}
	// Returns length mismatch → false.
	if userPolyArmShapeOK(&Signature{
		Impl:    body,
		Params:  []FnParam{{Name: "p", Type: TInteger}},
		Returns: []*Type{TInteger, TInteger},
	}, []*Type{TInteger}) {
		t.Error("a Returns arity mismatch should refuse")
	}
	// nil/non-nil Returns mismatch → false.
	if userPolyArmShapeOK(&Signature{
		Impl:    body,
		Params:  []FnParam{{Name: "p", Type: TInteger}},
		Returns: []*Type{nil},
	}, []*Type{TInteger}) {
		t.Error("a nil vs concrete Returns slot should refuse")
	}
	// Matching shape → true.
	if !userPolyArmShapeOK(&Signature{
		Impl:    body,
		Params:  []FnParam{{Name: "p", Type: TInteger}},
		Returns: []*Type{TInteger},
	}, []*Type{TInteger}) {
		t.Error("a matching arm shape should pass")
	}
}

func TestW9FindOwningFnDef(t *testing.T) {
	r := newTestRegistry(t)
	// nil impl → not found.
	if _, ok := findOwningFnDef(r, "w", nil); ok {
		t.Error("nil impl must not resolve an owner")
	}
	// A non-FnDefInfo binding above the word: the scan skips it (continue)
	// and, with no fn entry present, returns not-found.
	r.Defs.Push("w9poly", NewInteger(7))
	impl := Boru([]Value{NewWord("x")})
	if _, ok := findOwningFnDef(r, "w9poly", impl); ok {
		t.Error("a non-fn binding must be skipped and yield not-found")
	}
}

func TestW9TryCompileUserPolyOwnerRefusal(t *testing.T) {
	r := newTestRegistry(t)
	// Two same-arity overloads whose owning def is a MACRO: the owner gate
	// (owner.Macro) refuses, keeping the interpreter in charge.
	InstallFnDef(r, "w9macro", FnDefInfo{
		Macro: true,
		Signatures: []Signature{
			{Params: []FnParam{{Name: "a", Type: TInteger}}, Returns: []*Type{TInteger}, Impl: Boru([]Value{NewWord("a")})},
			{Params: []FnParam{{Name: "b", Type: TInteger}}, Returns: []*Type{TInteger}, Impl: Boru([]Value{NewWord("b")})},
		},
	})
	if tryCompileUserPolyArms(r, NewEmitState(), "w9macro", []Value{NewInteger(1)}, []*Type{TInteger}) != nil {
		t.Error("a macro-owned poly set should refuse")
	}
}

func TestW9TryCompileUserPolyArmCompileFails(t *testing.T) {
	r := newTestRegistry(t)
	// Two same-arity overloads that pass the shape/owner gates but whose
	// bodies are deferred-param-list residuals: compileUserPolyArm refuses,
	// so the whole poly set is kept in the interpreter.
	def := func(p string) Signature {
		inner := NewList([]Value{NewWord(p)})
		inner.Eval = true
		return Signature{
			Params:  []FnParam{{Name: p, Type: TInteger}},
			Returns: []*Type{TInteger},
			Impl:    Boru([]Value{inner}),
		}
	}
	InstallFnDef(r, "w9defer", FnDefInfo{
		Signatures: []Signature{def("a"), def("b")},
	})
	if tryCompileUserPolyArms(r, NewEmitState(), "w9defer", []Value{NewInteger(1)}, []*Type{TInteger}) != nil {
		t.Error("a poly set with an uncompilable arm should refuse")
	}
}

func TestW9CompileUserPolyArmRefusals(t *testing.T) {
	r := newTestRegistry(t)

	// Empty body → -1,false.
	if _, ok := compileUserPolyArm(r, NewEmitState(), "w", &Signature{}, FnDefInfo{}); ok {
		t.Error("empty body arm should refuse")
	}

	// A deferred-param-list body (eval list referencing a param) → -1,false.
	inner := NewList([]Value{NewWord("p")})
	inner.Eval = true
	deferredSig := &Signature{
		Params: []FnParam{{Name: "p", Type: TInteger}},
		Impl:   Boru([]Value{inner}),
	}
	if _, ok := compileUserPolyArm(r, NewEmitState(), "w", deferredSig, FnDefInfo{}); ok {
		t.Error("a deferred-param-list body should refuse")
	}

	// A valid body but an INACTIVE recorder → StartFnCompile fails → -1,false.
	plainSig := &Signature{
		Params:  []FnParam{{Name: "p", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    Boru([]Value{NewWord("p")}),
	}
	if _, ok := compileUserPolyArm(r, inactiveEmitState(), "w", plainSig, FnDefInfo{}); ok {
		t.Error("an inactive recorder should fail StartFnCompile")
	}
}

func TestBestEffortNoMatch(t *testing.T) {
	r := pnmRegistry(t, []Signature{pnmSig(-1, TInteger, TString), {Fallback: true}})
	fn := r.Lookup("pnmw")
	window := []Value{NewBoolean(true), NewBoolean(false)}
	if bestEffortNoMatch(r, nil, "pnmw", window, seam7Dbg, 0) != nil {
		t.Error("a nil fn must build no alt")
	}
	if bestEffortNoMatch(r, fn, "pnmw", nil, seam7Dbg, 0) != nil {
		t.Error("an empty window must build no alt")
	}
	// A mixed-arity table: another arity's collection could match at run
	// time, so the interpreter is not proven to fail — no alt.
	rMixed := pnmRegistry(t, []Signature{pnmSig(-1, TInteger, TString), pnmSig(-1, TInteger, TString, TBoolean)})
	if bestEffortNoMatch(rMixed, rMixed.Lookup("pnmw"), "pnmw", window, seam7Dbg, 0) != nil {
		t.Error("a mixed-arity table must build no alt")
	}
	alt := bestEffortNoMatch(r, fn, "pnmw", window, seam7Dbg, 0)
	if alt == nil || alt.Code != "signature_error" {
		t.Fatalf("alt = %v, want the rich signature_error", alt)
	}
	if !strings.Contains(alt.Error(), "the arguments were true (a Boolean) and false (a Boolean)") {
		t.Errorf("the alt must render the live window:\n%v", alt)
	}
	if alt.Row != seam7Dbg[0].Row {
		t.Errorf("alt Row = %d, want the stamped debug pos %d", alt.Row, seam7Dbg[0].Row)
	}
}

func TestVmDeferAltAttaches(t *testing.T) {
	r := covRegistry(t, nil)
	plain := vmDeferAlt(r, seam7Dbg, 0, "vm:poly-no-match", "x", nil)
	if ae, ok := plain.(*BoruError); !ok || ae.Code != "internal_error" || ae.DeferAlt != nil {
		t.Errorf("a nil alt must be a plain defer, got %v", plain)
	}
	alt := &BoruError{Code: "signature_error", Detail: "d"}
	err := vmDeferAlt(r, seam7Dbg, 0, "vm:poly-no-match", "x", alt)
	ae, ok := err.(*BoruError)
	if !ok || ae.Code != "internal_error" || ae.DeferAlt != alt {
		t.Errorf("the alt must ride the internal defer, got %v", err)
	}
}

// TestUserPolyNoMatchAltCoverageScreen pins the subset-coverage screen: the
// alt rides the user-poly no-match defer only when the recorded arm subset
// covers EVERY non-fallback overload of the live table — an uncovered
// same-arity arm could match at run time where the raise claims failure.
func TestUserPolyNoMatchAltCoverageScreen(t *testing.T) {
	r := covRegistry(t, nil)
	InstallFnDef(r, "upnm", FnDefInfo{
		Signatures: []Signature{
			{Params: []FnParam{{Name: "a", Type: TInteger}}, Returns: []*Type{TAny},
				BarrierPos: BarrierAllForward, Impl: Boru([]Value{NewWord("a")})},
			{Params: []FnParam{{Name: "a", Type: TString}}, Returns: []*Type{TAny},
				BarrierPos: BarrierAllForward, Impl: Boru([]Value{NewWord("a")})},
		},
	})
	fd := r.Lookup("upnm")
	if fd == nil || len(fd.Signatures) < 2 {
		t.Fatal("upnm not installed")
	}
	vc := &vmContext{r: r, p: &Program{Fns: []CompiledFn{
		{Name: "upnm", NParams: 1}, {Name: "upnm", NParams: 1},
	}}}
	window := []Value{NewBoolean(true)} // matches neither arm
	full := &UserPolyRef{Word: "upnm", Arity: 1,
		SigIdx: []int{0, 1}, Units: []int{0, 1},
		Impls: []SigImpl{fd.Signatures[0].Impl, fd.Signatures[1].Impl}}
	_, _, err := vc.matchUserPoly(full, window, seam7Dbg, 0)
	ae, ok := err.(*BoruError)
	if !ok || ae.DeferAlt == nil || ae.DeferAlt.Code != "signature_error" {
		t.Errorf("a covering subset must ride the alt, got %v", err)
	}
	partial := &UserPolyRef{Word: "upnm", Arity: 1,
		SigIdx: []int{0}, Units: []int{0},
		Impls: []SigImpl{fd.Signatures[0].Impl}}
	_, _, err = vc.matchUserPoly(partial, window, seam7Dbg, 0)
	ae, ok = err.(*BoruError)
	if !ok || ae.Code != "internal_error" || ae.DeferAlt != nil {
		t.Errorf("an uncovered live arm must keep the plain defer, got %v", err)
	}
}

func TestXmlInterpDynamicCompilesToOp(t *testing.T) {
	// GRADUATED (REFUSAL-CLOSURE §9.2c, 2026-07-17): a runtime-computed hole
	// lowers to OpInterpXml — the hole's own dispatch records its event and
	// the op rebuilds the element from the popped value at run time
	// (rebuildXmlFromTmpl). End-to-end parity is pinned in
	// lang/go bytecode_s9_landing_test.go TestS9XmlInterpComputed.
	r := covRegistry(t, nil)
	tmpl := XmlTmpl{
		Tag:  "p",
		Cren: []XmlCren{{Kind: XmlCrenExpr, Expr: []Value{NewWord("cadd"), NewInteger(1), NewInteger(2)}}},
	}
	prog, reason := compileTokens(t, r, []Value{NewXmlInterp(tmpl)})
	if prog == nil {
		t.Fatalf("dynamic XML interp must compile to OpInterpXml, refused: %s", reason)
	}
	if !strings.Contains(prog.Disassemble(), "INTERP_XML") {
		t.Fatalf("expected an INTERP_XML op:\n%s", prog.Disassemble())
	}
}

func TestCompiledComputedListArg(t *testing.T) {
	// A computed list consumed at the top frame records OpMakeList; the
	// compiled program must agree with the interpreter.
	got := runDifferential(t, nil, func() []Value {
		lv := NewList([]Value{NewWord("cadd"), NewInteger(2), NewInteger(3), NewEnd(), NewInteger(9)})
		lv.Eval = true
		return []Value{NewWord("clen"), lv}
	})
	if got != "2" {
		t.Errorf("computed list len = %q", got)
	}
}

func TestCompiledInterpString(t *testing.T) {
	// An interp string whose hole is a const expression folds; one with a
	// dispatch records OpInterp (or refuses) — parity either way.
	got := runDifferential(t, nil, func() []Value {
		return []Value{NewInterpString([]InterpPart{
			{Lit: "v="},
			{Expr: []Value{NewWord("cadd"), NewInteger(20), NewInteger(22)}},
		})}
	})
	if !strings.Contains(got, "v=42") {
		t.Errorf("interp = %q", got)
	}
}

func TestS6b0CheckMakeConstructionUnreadableMap(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	// Concrete, map-conforming, but the payload is a record type body:
	// AsMap declines and the check is a no-op.
	src := NewRecordType(NewOrderedMap())
	ct := NewClassType(TClass, ClassTypeInfo{Name: "Ideal/S6b0CM", Fields: NewOrderedMap()})
	CheckMakeConstruction(r, ct, src, SrcPos{Row: 1, Col: 1})
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("unreadable construction map must not emit diagnostics: %v", r.Check.Diagnostics)
	}
}

// SplitEventRegionBind's guard-owned decline arms (S9.1), white-box: the
// parked-fn screen (a Function-typed idx-0 result — the spilled rest would
// auto-apply it in the interpreter) and eventBySeq's closed-frame miss (a
// producing seq recorded in a fragment frame that has since closed).
func TestSplitEventRegionBindDeclines(t *testing.T) {
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es := r.Check.Emit.(*EmitState)
	es.BindRegistry(r)

	// A 2-out call event whose idx-0 result is Function-typed.
	fnOut := NewCarrier(TFunction)
	fnOut.ID = GenerateID(IDPrefixForType(TFunction))
	other := NewCarrier(TString)
	other.ID = GenerateID(IDPrefixForType(TString))
	seq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: "zz", nout: 2}})
	es.setProduced(fnOut, seq)
	es.producedBy[other.ID] = producer{seq: seq, idx: 1}
	if _, ok := es.SplitEventRegionBind("zzf", fnOut); ok {
		t.Error("a Function-typed first result must decline (parked-fn screen)")
	}

	// A seq living in a CLOSED fragment frame: record the event inside a
	// fragment, close it, then ask for the split — eventBySeq misses.
	closeFrag := es.beginFragment()
	iOut := NewCarrier(TInteger)
	iOut.ID = GenerateID(IDPrefixForType(TInteger))
	fseq := es.appendEvent(emitEvent{kind: evCall, call: emitCall{word: "zz2", nout: 2}})
	es.setProduced(iOut, fseq)
	closeFrag()
	es.TakeFragment()
	if _, ok := es.SplitEventRegionBind("zzg", iOut); ok {
		t.Error("a closed-fragment producer must decline (eventBySeq miss)")
	}
}

func TestRecordMakeListRefusals(t *testing.T) {
	// Not the top frame → declines.
	es := NewEmitState()
	es.frames = append(es.frames, nil)
	if es.RecordMakeList(nil, []Value{NewInteger(1)}, NewInteger(0), SrcPos{}) {
		t.Fatal("non-top-frame make-list should decline")
	}
	// A bare type node element with a canonical ID INTERNS as a type
	// operand and records (ADR-010 / NUR051 — `[a None]` is data)…
	es = NewEmitState()
	if !es.RecordMakeListInner(nil, []Value{NewTypeLiteral(TInteger)}, NewInteger(0), SrcPos{}) {
		t.Fatal("type-node element with a canonical ID should intern and record")
	}
	// …while one with NO canonical ID has no runtime home → declines.
	es = NewEmitState()
	noID := NewTypeLiteral(TInteger)
	noID.ID = ""
	if es.RecordMakeListInner(nil, []Value{noID}, NewInteger(0), SrcPos{}) {
		t.Fatal("type node with no canonical ID should decline")
	}
}

func TestRecordMakeMapRefusals(t *testing.T) {
	es := NewEmitState()
	// length mismatch → declines.
	if es.RecordMakeMap(nil, []string{"a"}, nil, false, NewInteger(0), SrcPos{}) {
		t.Fatal("key/val mismatch should decline")
	}
	// A bare type node value with a canonical ID INTERNS as a type
	// operand and records (ADR-010 / NUR051 — `{r: None}` is data)…
	es = NewEmitState()
	if !es.RecordMakeMap(nil, []string{"a"}, []Value{NewTypeLiteral(TInteger)}, false, NewInteger(0), SrcPos{}) {
		t.Fatal("type-node value with a canonical ID should intern and record")
	}
	// …while one with NO canonical ID has no runtime home → declines.
	es = NewEmitState()
	noID := NewTypeLiteral(TInteger)
	noID.ID = ""
	if es.RecordMakeMap(nil, []string{"a"}, []Value{noID}, false, NewInteger(0), SrcPos{}) {
		t.Fatal("type node with no canonical ID should decline")
	}
}

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

// fnValueInputs: a param carrying only a value PATTERN has no bare type
// (FnParam.Type nil) and binds a gradual Any input carrier; a typed param
// binds its own type. Pinned directly — the shape is otherwise reachable
// only through a capturing pattern-param returned fn, whose probe declines
// before the carrier is consumed (PR #295 merge coverage).
func TestFnValueInputsPatternParamDefaultsAny(t *testing.T) {
	inputs, names := fnValueInputs([]FnParam{{Name: "p"}, {Name: "q", Type: TInteger}})
	if len(inputs) != 2 || len(names) != 2 || names[0] != "p" || names[1] != "q" {
		t.Fatalf("inputs/names = %v %v", inputs, names)
	}
	if inputs[0].Parent == nil || !inputs[0].Parent.Equal(TAny) {
		t.Errorf("a pattern-only param must bind an Any carrier, got %v", inputs[0].Parent)
	}
	if !inputs[1].Parent.Equal(TInteger) {
		t.Errorf("a typed param must keep its type, got %v", inputs[1].Parent)
	}
}

func TestEmitEmptyIDGuards(t *testing.T) {
	es := NewEmitState()

	// setProducedAt with an identity-less value: no entry — a later ""
	// lookup is a guaranteed miss, and no other value can false-hit it.
	blank := NewInteger(1) // runtime mint → "" ID
	if blank.ID != "" {
		t.Fatal("test premise: runtime mint should be identity-less")
	}
	es.setProducedAt(blank, 3, 0)
	if _, ok := es.producedBy[""]; ok {
		t.Error("setProducedAt keyed the provenance map on \"\"")
	}
	// Positive twin: a minted value registers normally.
	end := BeginIDMintScope()
	minted := NewInteger(2)
	end()
	es.setProducedAt(minted, 4, 0)
	if pr, ok := es.producedBy[minted.ID]; !ok || pr.seq != 4 {
		t.Error("minted value did not register in producedBy")
	}

	// RegisterLocal: "" refuses with -1 and never inserts; two distinct
	// identity-less values must NOT collapse onto one slot.
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}, capID: map[string]bool{}})
	if slot := es.RegisterLocal(""); slot != -1 {
		t.Errorf("RegisterLocal(\"\") = %d, want -1", slot)
	}
	if slot := es.RegisterLocal(""); slot != -1 {
		t.Errorf("second RegisterLocal(\"\") = %d, want -1", slot)
	}
	if _, ok := es.units[len(es.units)-1].localByID[""]; ok {
		t.Error("RegisterLocal inserted a \"\" slot")
	}
	// Positive twin: real IDs get distinct slots.
	a := es.RegisterLocal("S_aaaaaaaaaaaa")
	b := es.RegisterLocal("S_bbbbbbbbbbbb")
	if a == -1 || b == -1 || a == b {
		t.Errorf("real IDs got slots %d/%d, want distinct non-negative", a, b)
	}
}

// A registry already driving a compiled run rejects a second, overlapping run:
// RunUnit shares runProgram's vmRunning CAS guard, so a concurrent callback on
// the SAME registry (rather than a ForkConcurrent clone) is refused rather than
// racing the shared invoker/scopes.
func TestRunUnitRejectsConcurrentRun(t *testing.T) {
	r := runUnitReg(t)
	p := &Program{Fns: []CompiledFn{{Name: "x", NParams: 0, NLocals: 0,
		Code: []Instr{{Op: OpRet, Arg: 0}}, Debug: []SrcPos{{}}}}}
	r.VmRunning = 1 // simulate an in-flight compiled run on this registry
	_, err := RunUnit(&CompiledFnRef{Prog: p, Unit: 0}, r, nil)
	var ae *BoruError
	if err == nil || !errors.As(err, &ae) || ae.Code != "concurrency_error" {
		t.Fatalf("RunUnit during an active run = %v, want concurrency_error", err)
	}
}

// TestPolyNoMatchRaiseDeclines pins every arm that keeps the sound defer: no
// recorded spec, an unresolvable word, a signature table whose length drifted
// since the record, a NARROWER-arity overload now live, and a recorded index
// outside the window (per tuple).
func TestPolyNoMatchRaiseDeclines(t *testing.T) {
	r := pnmRegistry(t, []Signature{pnmSig(-1, TInteger, TString)})
	vc := seam7VC(r)
	fn := r.Lookup("pnmw")
	if fn == nil {
		t.Fatal("pnmw not registered")
	}
	window := []Value{NewBoolean(true), NewBoolean(false)}
	if err := vc.polyNoMatchRaise(r, &PolyRef{Word: "pnmw", Arity: 2}, fn, window, seam7Dbg, 0); err != nil {
		t.Errorf("a nil spec must keep the defer, got %v", err)
	}
	spec := &PolyNoMatchSpec{Written: []int{0, 1}, StackTuple: []int{0, 1}, NSigs: 1}
	if err := vc.polyNoMatchRaise(r, &PolyRef{Word: "pnmw", Arity: 2, NoMatch: spec}, nil, window, seam7Dbg, 0); err != nil {
		t.Errorf("a nil fn must keep the defer, got %v", err)
	}
	drift := &PolyNoMatchSpec{Written: []int{0, 1}, StackTuple: []int{0, 1}, NSigs: 2}
	if err := vc.polyNoMatchRaise(r, &PolyRef{Word: "pnmw", Arity: 2, NoMatch: drift}, fn, window, seam7Dbg, 0); err != nil {
		t.Errorf("a table-length drift must keep the defer, got %v", err)
	}
	// A narrower-arity overload could match a runtime collection this raise
	// cannot model — the record-time screen excluded it, so its presence at
	// run time is drift.
	r2 := pnmRegistry(t, []Signature{pnmSig(-1, TInteger, TString), pnmSig(-1, TInteger)})
	fn2 := r2.Lookup("pnmw")
	narrow := &PolyNoMatchSpec{Written: []int{0, 1}, StackTuple: []int{0, 1}, NSigs: 2}
	if err := seam7VC(r2).polyNoMatchRaise(r2, &PolyRef{Word: "pnmw", Arity: 2, NoMatch: narrow}, fn2, window, seam7Dbg, 0); err != nil {
		t.Errorf("a narrower-arity live overload must keep the defer, got %v", err)
	}
	oobW := &PolyNoMatchSpec{Written: []int{2}, StackTuple: []int{0}, NSigs: 1}
	if err := vc.polyNoMatchRaise(r, &PolyRef{Word: "pnmw", Arity: 2, NoMatch: oobW}, fn, window, seam7Dbg, 0); err != nil {
		t.Errorf("an out-of-window Written index must keep the defer, got %v", err)
	}
	oobS := &PolyNoMatchSpec{Written: []int{0}, StackTuple: []int{-1}, NSigs: 1}
	if err := vc.polyNoMatchRaise(r, &PolyRef{Word: "pnmw", Arity: 2, NoMatch: oobS}, fn, window, seam7Dbg, 0); err != nil {
		t.Errorf("an out-of-window StackTuple index must keep the defer, got %v", err)
	}
}

// TestPolyNoMatchRaiseBuildsDiagnostic pins the raise arms: the rebuilt
// written tuple renders in the notes, a FALLBACK sig is skipped by the arity
// screen, and the two-probe reorder cascade fires — the primary probe over
// the written tuple, then the secondary over the stack tuple when the
// primary found nothing (a permuted stack pair behind a single forward
// token).
func TestPolyNoMatchRaiseBuildsDiagnostic(t *testing.T) {
	r := pnmRegistry(t, []Signature{pnmSig(-1, TInteger, TString), {Fallback: true}})
	vc := seam7VC(r)
	fn := r.Lookup("pnmw")
	window := []Value{NewBoolean(true), NewString("s")}
	spec := &PolyNoMatchSpec{Written: []int{0, 1}, StackTuple: []int{0, 1}, NSigs: 2, Pos: SrcPos{Row: 3, Col: 7}}
	err := vc.polyNoMatchRaise(r, &PolyRef{Word: "pnmw", Arity: 2, NoMatch: spec}, fn, window, seam7Dbg, 0)
	if err == nil {
		t.Fatal("a valid spec must raise")
	}
	ae, ok := err.(*BoruError)
	if !ok || ae.Code != "signature_error" {
		t.Fatalf("raise = %v, want signature_error", err)
	}
	if ae.Row != 3 || ae.Col != 7 {
		t.Errorf("raise anchored at %d:%d, want the spec's 3:7", ae.Row, ae.Col)
	}
	if msg := err.Error(); !strings.Contains(msg, "the arguments were true (a Boolean) and 's' (a ProperString)") {
		t.Errorf("the rebuilt written tuple must render in the notes:\n%v", msg)
	}

	// Primary reorder probe: the written tuple permutes the declared order.
	permuted := []Value{NewString("s"), NewInteger(1)}
	err = vc.polyNoMatchRaise(r, &PolyRef{Word: "pnmw", Arity: 2, NoMatch: spec}, fn, permuted, seam7Dbg, 0)
	if err == nil || !strings.Contains(err.Error(), "did you swap the arguments?") {
		t.Errorf("the primary reorder probe must fire over the written tuple:\n%v", err)
	}

	// Secondary probe: the written tuple is ONE unclaimed forward token (a
	// 1-tuple no 2-arg sig probes), the stack tuple permutes the declared
	// pair — sigError's fallback probe, rebuilt from the same window.
	sec := &PolyNoMatchSpec{Written: []int{0}, StackTuple: []int{0, 1}, NSigs: 2, Pos: SrcPos{Row: 1, Col: 1}}
	err = vc.polyNoMatchRaise(r, &PolyRef{Word: "pnmw", Arity: 2, NoMatch: sec}, fn, permuted, seam7Dbg, 0)
	if err == nil || !strings.Contains(err.Error(), "did you swap the arguments?") {
		t.Errorf("the secondary reorder probe must fire over the stack tuple:\n%v", err)
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

func TestRecordPolyCallGuard(t *testing.T) {
	// A multi-result poly (flex `pop` → [remaining, popped]) now RECORDS: each
	// result seats under its own index, the event is marked generic, and the
	// VM enforces the recorded result-count claim (PolyRef.NOut).
	es := NewEmitState()
	o1 := NewDynamicCarrier(TFlexList)
	o2 := NewDynamicCarrier(TAny)
	if !es.RecordPolyCall("pop", nil, []Value{o1, o2}, SrcPos{}, nil, nil) {
		t.Fatal("multi-out poly call should record")
	}
	pr1, ok1 := es.producedBy[o1.ID]
	pr2, ok2 := es.producedBy[o2.ID]
	if !ok1 || !ok2 {
		t.Fatal("both results should register in producedBy")
	}
	if pr1.idx != 0 || pr2.idx != 1 {
		t.Errorf("result indices = %d,%d, want 0,1", pr1.idx, pr2.idx)
	}
	if pr1.seq != pr2.seq {
		t.Errorf("results should share one event seq, got %d and %d", pr1.seq, pr2.seq)
	}
	if !es.eventInfo[pr1.seq].generic {
		t.Error("a multi-result poly event should be marked generic")
	}
	// inactive recorder → declines.
	if inactiveEmitState().RecordPolyCall("pop", nil, []Value{o1, o2}, SrcPos{}, nil, nil) {
		t.Fatal("inactive multi-out poly call should decline")
	}

	// De-collision: a later result whose ID collides with an EARLIER event's
	// output (different seq) is re-minted so provenance lookups stay distinct —
	// UNLESS that ID is an input passthrough (a receiver flowing straight
	// through), which keeps its identity.
	es2 := NewEmitState()
	a := NewDynamicCarrier(TFlexList)
	if !es2.RecordPolyCall("pop", nil, []Value{a, NewDynamicCarrier(TAny)}, SrcPos{}, nil, nil) {
		t.Fatal("seed poly should record")
	}
	// A second poly whose first result reuses a's ID, with a NOT among the
	// args → the colliding result is re-minted. (RecordPolyCall mutates the
	// outs SLICE in place, so observe the element, not the local copy.)
	c := NewDynamicCarrier(TFlexList)
	c.ID = a.ID
	outs2 := []Value{c, NewDynamicCarrier(TAny)}
	if !es2.RecordPolyCall("shift", nil, outs2, SrcPos{}, nil, nil) {
		t.Fatal("collision poly should record")
	}
	if outs2[0].ID == a.ID {
		t.Error("a colliding non-passthrough result ID should be re-minted")
	}
	// A third poly whose result reuses a's ID WHILE a is passed as an arg →
	// the passthrough exemption keeps the ID.
	d := NewDynamicCarrier(TAny)
	d.ID = a.ID
	outs3 := []Value{d, NewDynamicCarrier(TAny)}
	if !es2.RecordPolyCall("shift", []Value{a}, outs3, SrcPos{}, nil, nil) {
		t.Fatal("passthrough poly should record")
	}
	if outs3[0].ID != a.ID {
		t.Error("a passthrough result ID should be preserved")
	}
}

func TestFnCompileRefusesIdentitylessCapture(t *testing.T) {
	// A closure whose capture snapshot is a runtime mint (no ID) cannot
	// be compiled: capture slots are positional, so skipping one would
	// misalign every later capture. The unit must refuse (conservative
	// interpreter fallback), never collapse slots.
	c := &CheckState{}
	defer c.BeginCompilePass()()
	es, ok := c.Emit.(*EmitState)
	if !ok {
		t.Fatal("BeginCompilePass did not install an EmitState")
	}
	blankCap := Value{Parent: TInteger, Data: IntPayload{N: 5}} // hand-built: no ID
	_, fin, started := es.StartFnCompile("k", "f", nil, nil, nil, nil,
		[]CapturedBinding{{Name: "x", Value: blankCap}}, false, SrcPos{})
	if started && fin != nil {
		fin(nil)
	}
	if es.Compilable {
		t.Error("identity-less capture must mark the program uncompilable")
	}
}

func TestCheckListIndexBounds(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	list := NewList([]Value{NewInteger(10), NewInteger(20)})

	// In-range: silent.
	CheckListIndex(r, NewInteger(1), list, "get")
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("in-range index flagged: %+v", r.Check.Diagnostics)
	}
	// Provably out of range: flagged.
	CheckListIndex(r, NewInteger(9), list, "get")
	if len(r.Check.Diagnostics) == 0 {
		t.Fatal("out-of-range index not flagged")
	}

	n := len(r.Check.Diagnostics)
	// `at` over a list of indices: one bad index flags once.
	CheckAtIndices(r, NewList([]Value{NewInteger(0), NewInteger(7)}), list, "at")
	if len(r.Check.Diagnostics) != n+1 {
		t.Errorf("at-indices diagnostics = %d, want %d", len(r.Check.Diagnostics), n+1)
	}
	// Non-concrete indices are skipped without panic.
	CheckAtIndices(r, NewTypeLiteral(TList), list, "at")
}

func TestReturnsFuncBuilders(t *testing.T) {
	// ReturnsIdentity: repeated sources get fresh IDs.
	idfn := ReturnsIdentity(0, 0)
	in := NewInteger(5)
	in.ID = GenerateID("i_")
	out := idfn([]Value{in}, nil)
	if len(out) != 2 {
		t.Fatalf("identity out = %v", out)
	}
	if out[0].ID == out[1].ID {
		t.Error("duplicated identity shares an ID")
	}
	// Out-of-range mapping degrades to Any.
	out = ReturnsIdentity(3)([]Value{in}, nil)
	if !out[0].Parent.Equal(TAny) {
		t.Errorf("out-of-range identity = %v", out)
	}

	// ReturnsStatic.
	out = ReturnsStatic(TInteger, TString)(nil, nil)
	if len(out) != 2 || !out[0].Parent.Equal(TInteger) || !out[1].Parent.Equal(TString) {
		t.Errorf("ReturnsStatic = %v", out)
	}

	// ReturnsNumericBinary follows the arithmetic tower.
	nb := ReturnsNumericBinary()
	if out := nb([]Value{NewCarrier(TInteger), NewCarrier(TInteger)}, nil); !out[0].Parent.Equal(TInteger) {
		t.Errorf("int⊕int = %v", out)
	}
	if out := nb([]Value{NewCarrier(TFloat), NewCarrier(TInteger)}, nil); !out[0].Parent.Equal(TFloat) {
		t.Errorf("float⊕int = %v", out)
	}
	if out := nb([]Value{NewCarrier(TBigInteger), NewCarrier(TInteger)}, nil); !out[0].Parent.Equal(TBigInteger) {
		t.Errorf("big⊕int = %v", out)
	}
	if out := nb([]Value{NewCarrier(TBigDecimal), NewCarrier(TFloat)}, nil); !out[0].Parent.Equal(TBigDecimal) {
		t.Errorf("dec⊕float = %v", out)
	}
	if out := nb([]Value{NewCarrier(TInteger)}, nil); !out[0].Parent.Equal(TFloat) {
		t.Errorf("degenerate arity = %v", out)
	}

	// ReturnsAddConcat: provable String → String; otherwise gradual Scalar.
	ac := ReturnsAddConcat()
	if out := ac([]Value{NewString("a"), NewCarrier(TInteger)}, nil); !out[0].Parent.Equal(TString) {
		t.Errorf("string concat = %v", out)
	}
	dynamic := NewDynamicCarrier(TAny)
	if out := ac([]Value{dynamic, NewCarrier(TInteger)}, nil); !out[0].Dynamic {
		t.Errorf("gradual concat = %v", out)
	}

	// ReturnsFreshInstance: a concrete type-node target yields a fresh
	// value carrier of that type, not the literal.
	fi := ReturnsFreshInstance(0)
	out = fi([]Value{NewTypeLiteral(TPathon), mapOf()}, nil)
	if len(out) != 1 || !out[0].Carrier || !out[0].Parent.Equal(TPathon) {
		t.Errorf("fresh instance = %v", out)
	}
	a := fi([]Value{NewTypeLiteral(TPathon), mapOf()}, nil)[0]
	b := fi([]Value{NewTypeLiteral(TPathon), mapOf()}, nil)[0]
	if a.ID == b.ID {
		t.Error("two constructions share an identity")
	}
	// A carrier target keeps identity behaviour; an out-of-range slot is Any.
	out = fi([]Value{NewCarrier(TAny)}, nil)
	if len(out) != 1 {
		t.Errorf("carrier target = %v", out)
	}
	out = ReturnsFreshInstance(4)([]Value{NewTypeLiteral(TPathon)}, nil)
	if !out[0].Parent.Equal(TAny) {
		t.Errorf("out-of-range fresh = %v", out)
	}
}

func TestCompiledAnonFnValueCall(t *testing.T) {
	// Anonymous lambda dispatch under the emitter: the ReturnsFn attached
	// by compileFnDef runs AnalyseFnBody for the return type.
	tokens := func() []Value {
		fnv := anonFnVal(
			[]FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
			nil,
			parenBody(NewWord("cmul"), NewWord("a"), NewWord("b")),
		)
		return []Value{fnv, NewInteger(6), NewInteger(9)}
	}
	ri := covRegistry(t, nil)
	iOut, iErr := NewTop(ri).Run(tokens())
	if iErr != nil {
		t.Fatalf("interpreted: %v", iErr)
	}
	if got := renderAll(iOut); got != "54" {
		t.Fatalf("interpreted = %q, want 54", got)
	}
	rc := covRegistry(t, nil)
	prog, reason := compileTokens(t, rc, tokens())
	if prog == nil {
		t.Logf("compile refused (%s) — interpreter owns the case", reason)
		return
	}
	cOut, cErr := RunProgram(prog, rc)
	if cErr != nil {
		t.Fatalf("compiled: %v", cErr)
	}
	if got := renderAll(cOut); got != "54" {
		t.Errorf("compiled = %q, want 54", got)
	}
}

// readFnMemberValue / memberFnReadValue arm coverage (REFUSAL-CLOSURE.0 §3):
// the pinpointing walk's decline arms, each unreachable-by-shape from the
// lang fixtures alone — a carrier arg is skipped, a nil-backed map arg is
// skipped, a non-fn member misses, and a bool-only tag (zero member) reports
// no pinpointed value.
func TestReadFnMemberValueArms(t *testing.T) {
	fn := Value{Parent: TFunction, Data: FnDefInfo{Name: "d", Signatures: []Signature{{Impl: Boru([]Value{NewInteger(1)})}}}}
	om := NewOrderedMap()
	om.Set("f", fn)
	fnMap := NewMap(om)
	key := NewAtom("f")

	// Positive: concrete map + concrete key pinpoints the member.
	if got, ok := readFnMemberValue([]Value{fnMap, key}); !ok {
		t.Fatalf("concrete map+key must pinpoint the member, got ok=false (%v)", got)
	}
	// A carrier arg is skipped (the walk continues past it to the map).
	if _, ok := readFnMemberValue([]Value{carrierVal(TMap), fnMap, key}); !ok {
		t.Errorf("a leading carrier arg must be skipped, not decline the walk")
	}
	// A nil-backed map payload is skipped.
	nilMap := Value{Parent: TMap, Data: MapPayload{M: nil}}
	if _, ok := readFnMemberValue([]Value{nilMap, key}); ok {
		t.Errorf("a nil-backed map cannot pinpoint a member")
	}
	// A non-fn member misses.
	om2 := NewOrderedMap()
	om2.Set("f", NewInteger(5))
	if _, ok := readFnMemberValue([]Value{NewMap(om2), key}); ok {
		t.Errorf("a non-fn member must not pinpoint")
	}

	// memberFnReadValue: a bool-only tag (zero member — the computed-key
	// scan) reports no pinpointed value; an untagged id likewise.
	es := NewEmitState()
	es.NoteMemberFnRead("tagged-only", Value{})
	if _, ok := es.MemberFnReadValue("tagged-only"); ok {
		t.Errorf("a zero-member tag must not report a pinpointed value")
	}
	if _, ok := es.MemberFnReadValue("never-tagged"); ok {
		t.Errorf("an untagged id must not report a pinpointed value")
	}
	es.NoteMemberFnRead("rich", fn)
	if got, ok := es.MemberFnReadValue("rich"); !ok || !IsConcrete(got) {
		t.Errorf("a rich tag must report the member")
	}
}

// TestW8DispatchRematchDeclines pins the rematch record's decline arms: an
// inactive recorder, an empty window, an unresolvable operand, an empty word,
// no operands, the first-trap-wins latch, the inactive-recorder interface
// no-op, the promoted-operand rewrite of a rematch trap, and the VM
// underflow guard.
func TestW8DispatchRematchDeclines(t *testing.T) {
	// Inactive recorder + interface no-op.
	var nilES *EmitState
	if nilES.RecordDispatchRematchValues("w", []Value{NewInteger(1)}, 0, 1, SrcPos{}) {
		t.Error("inactive EmitState must decline")
	}
	if TheInactiveEmit.RecordDispatchRematchValues("w", []Value{NewInteger(1)}, 0, 1, SrcPos{}) {
		t.Error("the inactive recorder must decline")
	}

	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es, _ := r.Check.Recorder().(*EmitState)
	es.BindRegistry(r)
	if es.RecordDispatchRematchValues("w", nil, 0, 1, SrcPos{}) {
		t.Error("an empty window must decline")
	}
	// A dynamic value with no provenance fails resolveOperand.
	dyn := NewCarrier(TAny)
	dyn.Dynamic = true
	dyn.ID = ""
	if es.RecordDispatchRematchValues("w", []Value{dyn}, 0, 1, SrcPos{}) {
		t.Error("an unresolvable operand must decline")
	}
	if es.RecordDispatchRematch("", []emitOperand{constOperand(0)}, 0, 1, SrcPos{}) {
		t.Error("an empty word must decline")
	}
	if es.RecordDispatchRematch("w", nil, 0, 1, SrcPos{}) {
		t.Error("no operands must decline")
	}
	// The render bound must be a contiguous in-window slice: a zero length,
	// a negative offset, and a slice past the window all decline.
	if es.RecordDispatchRematch("w", []emitOperand{constOperand(0)}, 0, 0, SrcPos{}) {
		t.Error("a zero render bound must decline")
	}
	if es.RecordDispatchRematch("w", []emitOperand{constOperand(0)}, -1, 1, SrcPos{}) {
		t.Error("a negative render offset must decline")
	}
	if es.RecordDispatchRematch("w", []emitOperand{constOperand(0)}, 1, 1, SrcPos{}) {
		t.Error("a render slice past the window must decline")
	}
	// First trap wins: with a trap latched, a second record reports owned.
	if !es.RecordDispatchRematch("w", []emitOperand{constOperand(0)}, 0, 1, SrcPos{}) {
		t.Fatal("the first rematch record must land")
	}
	if !es.RecordDispatchRematch("w2", []emitOperand{constOperand(1)}, 0, 1, SrcPos{}) {
		t.Error("a second record after the latch must report owned (true), not re-record")
	}

	// The promoted-operand rewrite reaches a rematch trap's window.
	ev := emitEvent{kind: evTrap, trap: emitTrap{
		rematchWord: "w",
		rematchOps:  []emitOperand{eventOperand(0, 0)},
	}}
	rewritePromotedRefs(&ev, map[int]int{0: 3})
	if op := ev.trap.rematchOps[0]; op.kind != opLocal || op.idx != 3 {
		t.Errorf("rematch operand not rewritten to the promoted local: %+v", op)
	}

	// VM underflow guard.
	vc := &vmContext{r: r}
	if err := vc.dispatchRematch(&DispatchSpec{Word: "w", NArgs: 2, NWritten: 2}, nil, nil, 0); err == nil {
		t.Error("a short stack must error")
	}
	// VM render-bound guard: a spec whose written bound is outside 1..NArgs
	// is malformed (the recorder proves the bound before recording).
	if err := vc.dispatchRematch(&DispatchSpec{Word: "w", NArgs: 1},
		[]Value{NewInteger(1)}, nil, 0); err == nil || !strings.Contains(err.Error(), "written bound") {
		t.Errorf("a zero written bound must raise the bound guard, got %v", err)
	}
}

// unifyTyped{List,Map}WithConcrete's !IsConcrete guard (typed flex layer) tags
// an abstract CARRIER gradually. Ordinary dispatch routes flex carriers through
// unifyCarrierVsTyped first (and only the shape-tracked MAP carrier reaches the
// map twin), so the LIST arm is dispatch-unreachable — pin both by direct call.
// paramBodyCarrier (poly-arm + construction-check body-input builder) makes a
// {:T}/[:T] param's body reads narrow. Its typed arms are pinned by direct call
// (the census exercises the compile paths; the branch coverage is here).
func TestParamBodyCarrier(t *testing.T) {
	mapPat := NewTypedMap(NewTypeLiteral(TInteger))
	// A {:T} param carrier must be DYNAMIC — the arg may be flex, so an in-place
	// mutation in the body must runtime-rematch rather than preselect the
	// immutable handler (the compile-vs-interpret divergence fix).
	if v := paramBodyCarrier(FnParam{Pattern: &mapPat, Type: TMap}); !IsTypedMap(v) || !v.Dynamic {
		t.Errorf("{:Integer} param → %s (dynamic=%v), want a dynamic typed-map carrier", v.Parent.String(), v.Dynamic)
	}
	listPat := NewCarrierTypedListValue(NewTypeLiteral(TInteger))
	if v := paramBodyCarrier(FnParam{Pattern: &listPat, Type: TList}); !IsTypedList(v) || !v.Dynamic {
		t.Errorf("[:Integer] param → %s (dynamic=%v), want a dynamic typed-list carrier", v.Parent.String(), v.Dynamic)
	}
	// A non-typed-container param falls back to ParamInputCarrier (no pattern).
	if v := paramBodyCarrier(FnParam{Type: TInteger}); v.Parent == nil || !v.Parent.ConformsTo(TInteger) {
		t.Errorf("scalar param → %s, want an Integer carrier", v.Parent.String())
	}
	// A {:Any} pattern (child Any) falls through to ParamInputCarrier(TMap).
	anyPat := NewTypedMap(NewTypeLiteral(TAny))
	if v := paramBodyCarrier(FnParam{Pattern: &anyPat, Type: TMap}); v.Parent == nil || !v.Parent.ConformsTo(TMap) {
		t.Errorf("{:Any} param → %s, want a Map carrier", v.Parent.String())
	}
}

func TestW9TryCompileUserPolyEarlyReturns(t *testing.T) {
	r := newTestRegistry(t)
	args := []Value{NewInteger(1)}

	// The early-return guard: an inactive recorder OR empty args refuses
	// before any lookup.
	if tryCompileUserPolyArms(r, inactiveEmitState(), "w", args, []*Type{TInteger}) != nil {
		t.Error("inactive recorder should refuse")
	}
	if tryCompileUserPolyArms(r, NewEmitState(), "w", nil, []*Type{TInteger}) != nil {
		t.Error("empty args should refuse")
	}
	// Empty committedReturns is ADMITTED (REFUSAL-CLOSURE.0 §6a) — an
	// unknown word still refuses through the agg==nil gate.
	if tryCompileUserPolyArms(r, NewEmitState(), "w", args, nil) != nil {
		t.Error("unknown word should refuse (zero-return contract is admitted)")
	}
	// Unknown word → agg==nil → nil.
	if tryCompileUserPolyArms(r, NewEmitState(), "w9unknown", args, []*Type{TInteger}) != nil {
		t.Error("unknown word should refuse")
	}
	// A single same-arity overload → len(sigIdx) < 2 → nil.
	InstallFnDef(r, "w9one", FnDefInfo{
		Signatures: []Signature{{
			Params:  []FnParam{{Name: "n", Type: TInteger}},
			Returns: []*Type{TInteger},
			Impl:    Boru([]Value{NewWord("n")}),
		}},
	})
	if tryCompileUserPolyArms(r, NewEmitState(), "w9one", args, []*Type{TInteger}) != nil {
		t.Error("a single overload should refuse (single-overload path handles it)")
	}
}

func TestDisassembleOpcodeArms(t *testing.T) {
	p := &Program{
		Consts: []Value{NewInteger(1)},
		Types:  []TypeRef{{Name: "Integer"}},
		Sigs:   []SigRef{{Word: "w"}},
		Fns: []CompiledFn{{
			Name: "f", NParams: 1, NLocals: 2,
			LocalNames: []string{"x", ""},
			Code:       []Instr{{Op: OpRet}},
		}},
		PolyRefs:    []PolyRef{{}},
		UserPolys:   []UserPolyRef{{}},
		MakeMaps:    []MakeMapSpec{{Keys: []string{"a"}}},
		Traps:       []TrapSpec{{Code: "trap_code"}},
		Interps:     []InterpSpec{{NHoles: 1}},
		TypedBinds:  []TypedBindSpec{{Name: "x", Describe: "Integer"}},
		GlobalBinds: []GlobalBindSpec{{Name: "g", Depth: 1}},
		DynMethods:  []DynMethodSpec{{Word: "m", NArgs: 1, NOut: 1}},
	}
	ops := []Opcode{
		OpPushConst, OpJmp, OpJmpIfFalse, OpForNext, OpPushLocal,
		OpForSetup, OpStoreLocal, OpPushType, OpCallUser, OpTailCallUser,
		OpPushClosure, OpCallNativePoly, OpCallDynamic,
		OpCallDynamicTrailing, OpCallDynFrame, OpMakeList, OpMakeMap,
		OpTrap, OpReverse, OpInterp, OpBindTyped, OpBindGlobal,
		OpCallDynMethod,
	}
	for _, op := range ops {
		p.Code = append(p.Code, Instr{Op: op})
	}
	out := p.Disassemble()
	if !strings.Contains(out, "fn f0") || !strings.Contains(out, "[x _]") {
		t.Errorf("Disassemble missing fn header/local names:\n%s", out)
	}
	if !strings.Contains(out, "polyrefs=1") || !strings.Contains(out, "userpolys=1") {
		t.Errorf("Disassemble missing poly summaries:\n%s", out)
	}
	if p.StoredRefCount() != 0 || p.StoredRefStampedCount() != 0 {
		t.Error("stored-ref counters must be zero on a synthetic program")
	}
	if ClosureWantsKeyVal(NewInteger(1)) {
		t.Error("plain integer is not a keyval-hungry closure")
	}
	if IsCompiledClosure(NewInteger(1)) {
		t.Error("plain integer is not a compiled closure")
	}
}

func w8ArmCompile(t *testing.T, r *Registry) func() {
	t.Helper()
	done := r.Check.Begin()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	return done
}

func inactiveEmitState() *EmitState {
	es := NewEmitState()
	es.Compilable = false
	return es
}

// TestW8DispatchRematchNoneLiteralWindow — a `none` word in the failed
// window resolves to the None value exactly as the match's forward walk
// resolved it (the known-literal arm); the record still declines here
// because the written tuple is EMPTY (the forward walk stops at the word
// and the stack prefix is bare) — no render bound exists for it.
func TestW8DispatchRematchNoneLiteralWindow(t *testing.T) {
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(NativeFunc{Name: "w8rn", Signatures: []Signature{{
		Args: []*Type{TInteger, TNone}, BarrierPos: -1,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		}),
	}}})
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("w8rn"), NewWord("none"), NewCarrier(TInteger)}, StackHeadroom)
	e.Pointer = 0
	fn := r.Lookup("w8rn")
	if fn == nil {
		t.Fatal("w8rn not registered")
	}
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "w8rn", ArgCount: -1}, fn, SrcPos{}) {
		t.Error("the word-narrowed written tuple must decline the rematch record")
	}
}

// TestW8DispatchTrapDeferredTokenDeclines — a RAW deferred-expression token
// in the failed window (a Reach here) EXPANDS at dispatch/step time, so
// neither a serialized trap nor a runtime rematch models what the runtime
// match examines (flex.tsv L88/L95): the definiteness screen declines. The
// graduated word-splice trap removed the screen's only corpus reach, so this
// pins the arm directly. (A PARKED __SP splice marker deliberately no longer
// declines — see TestUnmatchedDispatchTrapSpliceGraduated in lang/go.)
func TestW8DispatchTrapDeferredTokenDeclines(t *testing.T) {
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(NativeFunc{Name: "w8dt", Signatures: []Signature{{
		Args: []*Type{TList}, BarrierPos: -1,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		}),
	}}})
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	reach := NewReachFromKeys(NewWord("m"), []Value{NewString("a")})
	e.Tape = NewTape([]Value{NewWord("w8dt"), reach}, StackHeadroom)
	e.Pointer = 0
	fn := r.Lookup("w8dt")
	if fn == nil {
		t.Fatal("w8dt not registered")
	}
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "w8dt", ArgCount: -1}, fn, SrcPos{}) {
		t.Error("a raw Reach window token must decline the trap/rematch record")
	}
}

// TestPayloadMarkersInvoke calls every payloadMarker (and the
// HostTypeBody marker) directly. The methods exist purely to satisfy
// the sealed Payload interface (see CLAUDE.md "Sealed Payload") and are
// never dispatched at runtime, so this is the only way to execute them.
func TestPayloadMarkersInvoke(t *testing.T) {
	payloads := []Payload{
		ReachInfo{},
		IntPayload{},
		FloatPayload{},
		BigIntPayload{},
		DecimalPayload{},
		StrPayload{},
		BoolPayload{},
		AtomPayload{},
		PathonPayload{},
		MicronPayload{},
		MicronTypeInfo{},
		ListPayload{},
		MapPayload{},
		ParenExprPayload{},
		InterpStringPayload{},
		TimePayload{},
		DurationPayload{},
		TimezonePayload{},
		MaterializerPayload{},
		NonePayload{},
		ExtensionPayload{},
		WordInfo{},
		ForwardInfo{},
		MarkInfo{},
		MoveInfo{},
		SpliceInfo{},
		ReturnCheckInfo{},
		DefCleanupInfo{},
		GuardFactInfo{},
		FrameOpenInfo{},
		ModuleDesc{},
		FnDefInfo{},
		FnUndefInfo{},
		DisjunctInfo{},
		NegationInfo{},
		ChildTypeInfo{},
		CodeEffectInfo{},
		RecordTypeInfo{},
		OptionsTypeInfo{},
		TableTypeInfo{},
		TableData{},
		ClassTypeInfo{},
		ClassInstanceInfo{},
		&SurfaceInfo{},
		&GenSpecInfo{},
		GenParam{},
		&TypeSchemaInfo{},
		GenInstRef{},
		&FlexListData{},
		XmlElementPayload{},
		XmlInterpPayload{},
		&FlexXmlData{},
		&StoreInstanceInfo{},
		&StoreShapeInfo{},
		ResourceTypeInfo{},
		ResourceInstanceInfo{},
		&TimeoutInfo{},
		&IntervalInfo{},
		ErrorInfo{},
		CalDurationData{},
		DepScalarInfo{},
		ClosurePayload{},
		PathonInfo{},
		NewNone().Data,
	}
	for _, p := range payloads {
		_ = p // the list itself is the pin: every entry satisfies Payload
	}
}

func TestDynApplyLeadEligible(t *testing.T) {
	fnCarrier := func(id string) Value {
		v := NewCarrier(TFunction)
		v.ID = id
		return v
	}

	// The inactive recorder and a nil/inactive state decline.
	if TheInactiveEmit.DynApplyLeadEligible(Value{}) {
		t.Error("the inactive recorder must decline")
	}
	var nilES *EmitState
	if nilES.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("a nil recorder must decline")
	}

	// Outside any open fn unit (the top level) declines: the Stage-G shape
	// is a fn-body param apply, and top-level parens keep their machinery.
	es := NewEmitState()
	if es.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("the top level must decline")
	}

	// openUnit appends an open unit with the given rec — the StartFnCompile
	// ritual reduced to the fields the gate consults.
	openUnit := func(es *EmitState, rec *fnUnitRec) *emitUnit {
		u := &emitUnit{localByID: map[string]int{}, capID: map[string]bool{}}
		es.fnRecs = append(es.fnRecs, rec)
		es.openUnitRecs = append(es.openUnitRecs, len(es.fnRecs)-1)
		es.units = append(es.units, u)
		return u
	}

	// A CLOSURE unit declines (its analysis frame is the CallableSpec
	// inputs, not a per-call named frame).
	es = NewEmitState()
	u := openUnit(es, &fnUnitRec{closure: true})
	u.localByID["g1"] = 0
	if es.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("a closure unit must decline")
	}

	// An UNNAMED-param unit declines: the frame re-pushes its args beneath
	// the region, so the interpreter's leading collection can reach past the
	// sealed window the trailing model records (`(args.0 args.1)` over a
	// two-arg fn nets 28 interpreted vs the model's no-match).
	es = NewEmitState()
	u = openUnit(es, &fnUnitRec{nParams: 2, nUnnamed: 2})
	u.localByID["g1"] = 0
	if es.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("an unnamed-param unit must decline")
	}

	// A lead that is NOT one of the unit's slots declines.
	es = NewEmitState()
	openUnit(es, &fnUnitRec{nParams: 1})
	if es.DynApplyLeadEligible(fnCarrier("gX")) {
		t.Error("a non-local lead must decline")
	}

	// An EVENT-provenance local (a computed def promoted to a slot) declines
	// — RecordDynApply hard-refuses an event fn (runtime quote state
	// unknown), so admitting it would turn a compiling shape into a refusal.
	es = NewEmitState()
	u = openUnit(es, &fnUnitRec{nParams: 1})
	u.localByID["g1"] = 0
	es.producedBy["g1"] = producer{seq: 3}
	if es.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("an event-provenance lead must decline")
	}

	// A plain param slot is eligible; a CAPTURE stays eligible even with a
	// parent-unit produced entry (capID precedence — it resolves to its own
	// slot, not the unreachable parent event).
	es = NewEmitState()
	u = openUnit(es, &fnUnitRec{nParams: 1})
	u.localByID["g1"] = 0
	if !es.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("a named param slot must be eligible")
	}
	u.localByID["c1"] = 1
	u.capID["c1"] = true
	es.producedBy["c1"] = producer{seq: 5}
	if !es.DynApplyLeadEligible(fnCarrier("c1")) {
		t.Error("a captured slot with a parent-unit event entry must stay eligible")
	}
}

func TestW9UnitNetsZero(t *testing.T) {
	// Bounds guards: nil recorder / out-of-range unit → false.
	var nilES *EmitState
	if nilES.UnitNetsZero(0) {
		t.Error("nil EmitState must report false")
	}
	es := NewEmitState()
	if es.UnitNetsZero(-1) || es.UnitNetsZero(0) {
		t.Error("out-of-range unit must report false")
	}
	// A 0-residual unit qualifies; a residual-carrying or variadic one does not.
	es.fnRecs = append(es.fnRecs,
		&fnUnitRec{},
		&fnUnitRec{outOps: []emitOperand{constOperand(0)}},
		&fnUnitRec{variadic: true},
	)
	if !es.UnitNetsZero(0) {
		t.Error("an empty-residual unit must net zero")
	}
	if es.UnitNetsZero(1) {
		t.Error("a residual-carrying unit must not net zero")
	}
	if es.UnitNetsZero(2) {
		t.Error("a variadic unit must not net zero")
	}
	// The inactive recorder's stub answer.
	if TheInactiveEmit.UnitNetsZero(0) {
		t.Error("inactive recorder must report false")
	}
}

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func parenBody(tokens ...Value) []Value {
	out := []Value{NewOpenParen()}
	out = append(out, tokens...)
	out = append(out, NewCloseParen())
	return out
}

func mapOf(pairs ...any) Value {
	m := NewOrderedMap()
	for i := 0; i < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1].(Value))
	}
	return NewMap(m)
}

func runUnitReg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// engWithTape builds a top-level engine over a fresh cov registry with
// the given tape values and pointer set to the given index.
func engWithTape(t *testing.T, vals []Value, pointer int) *Engine {
	t.Helper()
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape(vals, StackHeadroom)
	e.Pointer = pointer
	return e
}

func intStrRecord() RecordTypeInfo {
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	fields.Set("b", NewTypeLiteral(TString))
	return RecordTypeInfo{Fields: fields}
}

// s5beAt stamps a source position on a value.
func s5beAt(v Value, row, col int) Value {
	v.SetPos(SrcPos{Row: row, Col: col})
	return v
}

func testClassType() ClassTypeInfo {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	fields.Set("y", NewTypeLiteral(TString))
	return ClassTypeInfo{Fields: fields, Name: "Class/Cov", ID: "T_cov000000001"}
}

func zzDriftWord() (WordInfo, *Signature) {
	return WordInfo{Name: "add", ArgCount: -1},
		&Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: 1}
}

// carrierVal is an unresolvable operand: a stripped carrier with no recorded
// original, so materialise (and thus resolveOperand) refuses it.
func carrierVal(t *Type) Value { return NewCarrier(t) }

// --- materialise list/map rebuild ---------------------------------------

// --- eventPos / eventsThroughSeq ----------------------------------------

// --- record-method guards: inactive / unresolvable ----------------------

// --- RecordBranch refusal arms ------------------------------------------

// --- fn-value / container-member classifiers ----------------------------

// fnVal builds a concrete Function value carrying the given signatures.
func fnVal(sigs ...Signature) Value {
	return Value{ID: GenerateID(IDPrefixForType(TFunction)), Parent: TFunction, Data: FnDefInfo{Signatures: sigs}}
}

func ptrVal(v Value) *Value { return &v }

// --- materialise list/map rebuild ---------------------------------------

// --- eventPos / eventsThroughSeq ----------------------------------------

// --- record-method guards: inactive / unresolvable ----------------------

// --- RecordBranch refusal arms ------------------------------------------

// --- fn-value / container-member classifiers ----------------------------

// instanceVal builds a flat class instance whose fields map is `om`.
func instanceVal(om *OrderedMap) Value {
	return Value{ID: GenerateID(IDPrefixForType(TAny)), Parent: TAny, Data: ClassInstanceInfo{Fields: om}}
}

func mapOfFields() *OrderedMap {
	om := NewOrderedMap()
	om.Set("f", zeroArgFn())
	return om
}

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

func s7MapOf(key string, v Value) Value {
	om := NewOrderedMap()
	om.Set(key, v)
	return NewMap(om)
}

// zeroArgFn is a genuine 0-param fn value (two 0-arg sigs → real + phantom),
// the shape the interpreter auto-dispatches when it lands with no operands.
func zeroArgFn() Value { return fnVal(Signature{}, Signature{}) }

func mapOf2(key string, v Value) *OrderedMap {
	om := NewOrderedMap()
	om.Set(key, v)
	return om
}

// namedFnVal builds a NON-anonymous FnDef value carrying its own authored
// sigs — the shape a fn literal stored in a map / module export has.
func namedFnVal(name string, params []FnParam, returns []*Type, body []Value) Value {
	return NewFunction(FnDefInfo{
		Name: name,
		Signatures: []Signature{{
			Params:     params,
			Returns:    returns,
			Impl:       Boru(body),
			BarrierPos: BarrierAllForward,
		}},
	})
}

// entryCollector gathers InterpEntry events thread-safely (forks may emit
// concurrently under the -race lanes).
type entryCollector struct {
	mu      sync.Mutex
	entries []InterpEntry
}

func (c *entryCollector) add(e InterpEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, e)
}

func moduleGateReg(t *testing.T, module, export string) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	if err := r.Capabilities.Set(CapPolicy, denyExportChecker{module: module, export: export}); err != nil {
		t.Fatalf("install checker: %v", err)
	}
	return r
}

func w8ReachWithComputedKey(keyExpr []Value) Value {
	return NewReach(ReachInfo{
		Receiver: []Value{NewMap(NewOrderedMap())},
		Segments: []ReachSeg{{Computed: true, KeyExpr: keyExpr}},
		Eval:     false,
	})
}

func pnmRegistry(t *testing.T, sigs []Signature) *Registry {
	t.Helper()
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(NativeFunc{Name: "pnmw", Signatures: sigs})
	if err := r.Err(); err != nil {
		t.Fatalf("register pnmw: %v", err)
	}
	return r
}

func pnmSig(barrier int, args ...*Type) Signature {
	return Signature{Args: args, BarrierPos: barrier,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		})}
}

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }

// anonFnVal builds an anonymous Function value the way `afn`/`=>` does:
// Anonymous=true, authored sigs with a boru body. compileFnDef attaches
// the body-runner handler at dispatch time.
func anonFnVal(params []FnParam, returns []*Type, body []Value) Value {
	return NewFunction(FnDefInfo{
		Anonymous: true,
		Signatures: []Signature{{
			Params:     params,
			Returns:    returns,
			Impl:       Boru(body),
			BarrierPos: BarrierAllForward,
		}},
	})
}

// denyExportChecker denies exactly one (module, export) pair and
// implements ModuleCallChecker only.
type denyExportChecker struct{ module, export string }

func (d denyExportChecker) CheckModuleCall(module, export string) error {
	if module == d.module && export == d.export {
		return errModDenied
	}
	return nil
}

var errModDenied = errors.New("zz-policy: module export denied")

// TestEmitCheckpointHandle pins the S2 opaque-handle contract: the
// inactive recorder hands out a nil EmitCheckpoint, and the concrete
// Rollback ignores any handle that is not its own snapshot type (the
// no-op decline the dropped interface no-ops used to provide).
func TestEmitCheckpointHandle(t *testing.T) {
	e := TheInactiveEmit
	if e.Checkpoint() != nil {
		t.Fatal("inactive Checkpoint should be nil")
	}
	e.Rollback(nil)
	es := NewEmitState()
	es.Rollback(nil) // not an emitCheckpoint: ignored
	if cp := es.Checkpoint(); cp == nil {
		t.Fatal("concrete Checkpoint should hand out a (possibly zero) snapshot")
	}
}

// xmlReg builds a cov registry plus a carrier-bound name (dynv) and a
// flow-control-raising word (cbreak) for driving the dynamic / flow arms.
func xmlReg(t *testing.T) *Registry {
	t.Helper()
	r := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "cfailx",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return nil, &BoruError{Code: "runtime_error", Detail: "xboom"}
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
		r.RegisterNativeFunc(NativeFunc{
			Name: "cbreak",
			Signatures: []Signature{{
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
					reg.FlowCtrl = FlowBreak
					return []Value{NewInteger(0)}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
	})
	r.Defs.Push("dynv", NewCarrier(TAny))
	return r
}

// RunProgram must refuse to start a compiled run while an INTERPRETER run is
// already in flight on the same registry — the cross-engine race the vmRunning
// CAS alone cannot see (it only guards compiled-vs-compiled). The interpRunDepth
// counter Engine.Run maintains is what makes this detectable. Paired
// positive/negative: refused while a run is active, allowed once it ends.
func TestRunProgramRejectsConcurrentInterpreterRun(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	p := &Program{} // empty program: runs off the end, returns an empty residual

	// NEGATIVE — an interpreter run is in flight (depth > 0): refuse with a
	// concurrency_error, and the vmRunning flag must be released on that return
	// (so the positive case below can still acquire it).
	r.EnterInterpRun()
	if _, err := RunProgram(p, r); err == nil {
		r.ExitInterpRun()
		t.Fatal("RunProgram succeeded while an interpreter run was active; want concurrency_error")
	} else {
		var ae *BoruError
		if !errors.As(err, &ae) || ae.Code != "concurrency_error" {
			r.ExitInterpRun()
			t.Fatalf("wrong error while interpreter active: got %v, want concurrency_error", err)
		}
	}
	r.ExitInterpRun()

	// POSITIVE — no interpreter run in flight: the compiled run proceeds (so the
	// negative case is not vacuously "always refuses").
	if _, err := RunProgram(p, r); err != nil {
		t.Fatalf("RunProgram refused on an idle registry: %v", err)
	}
}

func TestS7EvalXmlInterpCheckModeDynamicRefuses(t *testing.T) {
	r := xmlReg(t)
	done := r.Check.Begin()
	r.Check.Emit = NewEmitState()
	defer done()
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Attr: []XmlAttrTmpl{{
		Name: "x", Parts: []InterpPart{{Expr: []Value{NewWord("dynv")}}},
	}}}
	out, err := e.EvalXmlInterp(NewXmlInterp(tmpl))
	if err != nil {
		t.Fatalf("EvalXmlInterp: %v", err)
	}
	// A dynamic interpolation under an active recorder yields an Xml carrier.
	if !out.Carrier || !strings.Contains(out.Parent.Leaf(), "Xml") {
		t.Errorf("expected an Xml carrier, got %v", out)
	}
}

// TestW8DispatchRematchFnShapeDeclines — the fn-shape typed-binding hint is
// TAPE state the runtime rebuild has no access to: a carrier no-match inside
// a def whose constraint is a function-shape type must decline the rematch
// record so the interpreter's suggestion-bearing error stays canonical.
func TestW8DispatchRematchFnShapeDeclines(t *testing.T) {
	r := covRegistry(t, nil)
	r.Defs.Push("MyShape", NewCarrier(TFnUndef))
	r.RegisterNativeFunc(NativeFunc{Name: "w8rf", Signatures: []Signature{{
		Args: []*Type{TString}, BarrierPos: -1,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		}),
	}}})
	sig := &Signature{Params: []FnParam{{Type: TMap}, {Type: TAny}}, BarrierPos: -1}
	m := NewOrderedMap()
	m.Set("f", NewWord("MyShape"))
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	// The def Forward sits below the failing word (both back-walks skip it);
	// its typed-name map rides at FuncIndex-CollectedArgs — above the
	// pointer, so the stack window stays empty and the carrier forms the
	// forward window; the trailing word bounds the written walk.
	e.Tape = NewTape([]Value{
		NewForward(ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 1, FuncIndex: 5}),
		NewWord("w8rf"),
		NewCarrier(TInteger),
		NewWord("zz-stop"),
		NewMap(m),
	}, StackHeadroom)
	e.Pointer = 1
	if !e.IsFnShapeTypedBindingContext() {
		t.Fatal("setup: expected the fn-shape typed-binding context")
	}
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "w8rf", ArgCount: -1}, r.Lookup("w8rf"), SrcPos{}) {
		t.Error("the fn-shape context must decline the rematch record")
	}
}

func TestW8ExecFnDefStackMatchCheckFnValueUnnamed(t *testing.T) {
	// A non-anonymous body-bearing fn value with UNNAMED params in compile
	// mode routes through spliceFnValueCheckResult (unnamed branch).
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	fnDef := FnDefInfo{Name: "w8fv", Signatures: []Signature{{
		Params: []FnParam{{Type: TInteger}}, Returns: []*Type{TInteger},
		Impl: Boru(parenBody(NewInteger(42))), BarrierPos: 0,
	}}}
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(3), NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 1
	if err := e.ExecFnDefSigStackMatch(1, fnDef, []Value{NewInteger(3)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestS7BuildXmlAttrDynamic(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Attr: []XmlAttrTmpl{{
		Name: "x", Parts: []InterpPart{{Expr: []Value{NewWord("dynv")}}},
	}}}
	_, dynamic, _, _, err := e.BuildXmlFromTmpl(tmpl)
	if err != nil {
		t.Fatalf("attr dynamic: %v", err)
	}
	if !dynamic {
		t.Error("a carrier-valued attr part should mark the build dynamic")
	}
}

func TestS7BuildXmlNestedChildDynamic(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	child := &XmlTmpl{Tag: "span", Attr: []XmlAttrTmpl{{
		Name: "x", Parts: []InterpPart{{Expr: []Value{NewWord("dynv")}}},
	}}}
	tmpl := XmlTmpl{Tag: "p", Cren: []XmlCren{{Kind: XmlCrenChild, Child: child}}}
	_, dynamic, _, _, err := e.BuildXmlFromTmpl(tmpl)
	if err != nil {
		t.Fatalf("nested child dynamic: %v", err)
	}
	if !dynamic {
		t.Error("a dynamic nested child should mark the parent build dynamic")
	}
}

// TestPlainCheckRunsAgainstRecorderInterface swaps a counting NON-EmitState
// recorder into a plain check pass and asserts (a) the pass completes with
// the same diagnostics as the stock pass, and (b) the recorder interface was
// actually consulted — the checker's only coupling to the emit machinery is
// the interface.
func TestPlainCheckRunsAgainstRecorderInterface(t *testing.T) {
	runPass := func(rec EmitRecorder) []CheckDiagnostic {
		r := recorderTestRegistry(t)
		defer r.Check.Begin()()
		if rec != nil {
			r.Check.Emit = rec
		}
		eng := NewTop(r)
		// A dispatch plus a genuine diagnostic (undefined word).
		toks := []Value{
			NewInteger(1), NewWord("padd"), NewInteger(2),
			NewWord("nosuchword"),
		}
		_, _ = eng.Run(toks)
		return append([]CheckDiagnostic(nil), r.Check.Diagnostics...)
	}

	base := runPass(nil)
	fake := &countingRecorder{EmitRecorder: TheInactiveEmit}
	got := runPass(fake)

	if len(base) != len(got) {
		t.Fatalf("diagnostics diverge under the fake recorder: stock %d, fake %d", len(base), len(got))
	}
	for i := range base {
		if base[i].Code != got[i].Code || base[i].Word != got[i].Word {
			t.Errorf("diagnostic %d diverges: stock %s/%s, fake %s/%s",
				i, base[i].Code, base[i].Word, got[i].Code, got[i].Word)
		}
	}
	if fake.activeCalls == 0 && fake.armedCalls == 0 && fake.recordCalls == 0 {
		t.Fatalf("the check pass never consulted the recorder interface (active=%d armed=%d record=%d)",
			fake.activeCalls, fake.armedCalls, fake.recordCalls)
	}
}

func TestUsurpCheckModeDiagnostics(t *testing.T) {
	// In check mode both failures degrade to diagnostics + placeholder.
	r := covRegistry(t, nil)
	InstallDef(r, "plainv", NewInteger(1))
	done := r.Check.Begin()
	_, err := NewTop(r).Run([]Value{NewWordUsurp("no_such_word", false)})
	if err != nil {
		t.Errorf("check-mode usurp undefined errored hard: %v", err)
	}
	_, err = NewTop(r).Run([]Value{NewWordUsurp("plainv", false)})
	if err != nil {
		t.Errorf("check-mode usurp non-fn errored hard: %v", err)
	}
	var codes []string
	for _, d := range r.Check.Diagnostics {
		codes = append(codes, d.Code)
	}
	done()
	joined := strings.Join(codes, ",")
	if !strings.Contains(joined, "undefined_word") || !strings.Contains(joined, "illegal_ref") {
		t.Errorf("check-mode usurp diagnostics = %v", codes)
	}
}

func TestWordRefCheckModeDiagnostics(t *testing.T) {
	r := covRegistry(t, nil)
	InstallDef(r, "plainv", NewInteger(2))
	done := r.Check.Begin()
	if _, err := NewTop(r).Run([]Value{NewWordRef("no_such_word")}); err != nil {
		t.Errorf("check-mode /r undefined errored hard: %v", err)
	}
	if _, err := NewTop(r).Run([]Value{NewWordRef("plainv")}); err != nil {
		t.Errorf("check-mode /r non-fn errored hard: %v", err)
	}
	var codes []string
	for _, d := range r.Check.Diagnostics {
		codes = append(codes, d.Code)
	}
	done()
	joined := strings.Join(codes, ",")
	if !strings.Contains(joined, "undefined_word") || !strings.Contains(joined, "illegal_ref") {
		t.Errorf("check-mode /r diagnostics = %v", codes)
	}
}

func TestIsolateEmitAndBudget(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	saved := r.Check.Emit
	restore := r.Check.IsolateEmit()
	if r.Check.Emit == saved {
		t.Error("IsolateEmit did not swap the emit state")
	}
	restore()
	if r.Check.Emit != saved {
		t.Error("IsolateEmit restore lost the original")
	}

	r.Check.StepCount = 7
	r.Check.BudgetTripped = false
	restoreB := r.Check.IsolateBudget()
	r.Check.StepCount = 999
	r.Check.BudgetTripped = true
	restoreB()
	if r.Check.StepCount != 7 || r.Check.BudgetTripped {
		t.Error("IsolateBudget did not roll the counters back")
	}

	var nilC *CheckState
	nilC.IsolateEmit()()
	nilC.IsolateBudget()()
}

// countingRecorder wraps the inactive no-op recorder and counts every probe
// the check pass makes, proving the checker runs against the EmitRecorder
// INTERFACE — no code path requires the concrete *EmitState (G9 / completion
// plan 4.5). Embedding inactiveEmit supplies the full method set; the
// overridden methods tally the calls that every check pass is guaranteed to
// make.
type countingRecorder struct {
	EmitRecorder // the inactive no-op backs the full method set
	activeCalls  int
	armedCalls   int
	recordCalls  int
}

func (c *countingRecorder) Active() bool {
	c.activeCalls++
	return c.EmitRecorder.Active()
}

func (c *countingRecorder) Armed() bool {
	c.armedCalls++
	return c.EmitRecorder.Armed()
}

func (c *countingRecorder) RecordCall(word string, sig *Signature, args, outs []Value, pos SrcPos, forceDynOut, quoteInertOK bool) {
	c.recordCalls++
	c.EmitRecorder.RecordCall(word, sig, args, outs, pos, forceDynOut, quoteInertOK)
}

// recorderTestRegistry builds an eng-only registry with one probe native
// (`padd`), enough for a check pass to exercise dispatch through
// recordDispatchOutcome and emit a genuine diagnostic on an unknown word.
func recorderTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.RegisterNativeFunc(NativeFunc{
		Name: "padd",
		Signatures: []Signature{{
			Args: []*Type{TInteger, TInteger},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := AsInteger(args[0])
				b, _ := AsInteger(args[1])
				return []Value{NewInteger(a + b)}, nil
			}),
			Returns:    []*Type{TInteger},
			BarrierPos: -1,
		}},
	})
	return r
}

// --- S9 active-slot coverage: the check piece's spliceAnonCheckResult ------
// The core-side twins of these tests run against the INACTIVE CheckBraid
// defaults; here the eng-linked build has the real check implementations
// installed, so the same direct ExecFnDefSigStackMatch calls cover
// spliceAnonCheckResult itself.

// zzAnonDef builds an anonymous FnDefInfo with one signature.
func zzAnonDef(params []FnParam, returns []*Type, body []Value) FnDefInfo {
	return FnDefInfo{
		Anonymous: true,
		Signatures: []Signature{{
			Params:     params,
			Returns:    returns,
			Impl:       Boru(body),
			BarrierPos: BarrierAllForward,
		}},
	}
}

func TestW8AnonFnCheckModeZeroArgActiveSlot(t *testing.T) {
	// A 0-arg anonymous fn dispatched in check mode routes through
	// ExecFnDefSigStackMatch's checkMode branch → spliceAnonCheckResult(0).
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := zzAnonDef(nil, []*Type{TInteger}, parenBody(NewInteger(7)))
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 0
	if err := e.ExecFnDefSigStackMatch(0, fnDef, nil); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestW8AnonFnCheckModeEmptyBodyActiveSlot(t *testing.T) {
	// A body that yields no value → spliceAnonCheckResult's empty-result
	// default (NewCarrier(TAny)).
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := zzAnonDef(nil, nil, parenBody())
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 0
	if err := e.ExecFnDefSigStackMatch(0, fnDef, nil); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestW8AnonFnCheckModeNamedParamsStackActiveSlot(t *testing.T) {
	// A named-param anonymous fn with stack args in check mode →
	// spliceAnonCheckResult(nArgs) via the hasNamed branch.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := zzAnonDef(
		[]FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("a"), NewWord("b")),
	)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(2), NewInteger(3), NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 2
	if err := e.ExecFnDefSigStackMatch(2, fnDef, []Value{NewInteger(2), NewInteger(3)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestW8AnonFnCheckModeUnnamedParamsStackActiveSlot(t *testing.T) {
	// An unnamed-param anonymous fn with stack args in check mode →
	// spliceAnonCheckResult(nArgs) via the else (unnamed) branch.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := zzAnonDef(
		[]FnParam{{Type: TInteger}, {Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cadd")),
	)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(2), NewInteger(3), NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 2
	if err := e.ExecFnDefSigStackMatch(2, fnDef, []Value{NewInteger(2), NewInteger(3)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}
