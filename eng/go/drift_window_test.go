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
