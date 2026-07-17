package eng

import "testing"

// Direct white-box arms of tryRecordDriftWindow (REFUSAL-CLOSURE §1) that the
// source-level fixtures cannot reach — the w8ArmCompile harness drives the
// gate with hand-built tapes exactly like the refuseForwardStackDrift twin.

func zzDriftWord() (WordInfo, *Signature) {
	return WordInfo{Name: "add", ArgCount: -1},
		&Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: 1}
}

func TestDriftWindowOutOfRangePosition(t *testing.T) {
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	e := NewTop(r)
	e.tape = NewTape([]Value{NewInteger(1)}, stackHeadroom)
	w, sig := zzDriftWord()
	if e.tryRecordDriftWindow(w, sig, []int{0, 999}) {
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
	e.tape = NewTape([]Value{NewInteger(5), dyn, NewInteger(9), NewWord("add"), NewInteger(1)}, stackHeadroom)
	e.pointer = 3
	w, sig := zzDriftWord()
	if e.tryRecordDriftWindow(w, sig, []int{1, 0}) {
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
	e.tape = NewTape([]Value{NewInteger(5), dyn, NewWord("add"), NewInteger(1)}, stackHeadroom)
	e.pointer = 2
	w, sig := zzDriftWord()
	if e.tryRecordDriftWindow(w, sig, []int{1, 0}) {
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
	e.tape = NewTape([]Value{orphan, dyn, NewWord("add"), NewInteger(1)}, stackHeadroom)
	e.pointer = 2
	w, sig := zzDriftWord()
	if e.tryRecordDriftWindow(w, sig, []int{1, 0}) {
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
	es.bindRegistry(r)
	orphan := NewCarrier(TList) // no event, no local, no const home
	if es.RecordSpliceDyn(orphan, SrcPos{}) {
		t.Error("an unresolvable payload must decline")
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
	es.bindRegistry(r)

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

// recordCallRefusal's computed-range-list classifier (white-box): a `for`
// whose range arg carries an OpMakeList producer refuses with the dedicated
// reason. Source shapes rarely reach this arm since the start/step widening
// (RecordLoop owns the const/local/event partition), but the classifier
// stays for the lowerable=false paths where the runtime-assembled range
// diverges under the CALL_NATIVE for-handler.
func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRecordCallRefusalComputedRangeList(t *testing.T) {
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es := r.Check.Emit.(*EmitState)
	es.bindRegistry(r)
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
