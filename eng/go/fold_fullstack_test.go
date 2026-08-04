package eng

// fold_fullstack_test.go pins EmitState.FoldFullStack — the static fold that
// graduated the full-stack words (depth/pick/roll) for provably-exact stacks
// (checker-compiler-completeness-review.0.md §8.2(2)). White-box per the
// emit_dynapply_fnunit pattern: every gate arm and every word's fold,
// positive and negative.

import "testing"

func ffsConst(id string, n int64) Value {
	v := NewInteger(n)
	v.ID = id
	return v
}

func TestFoldFullStackGates(t *testing.T) {
	one := NewInteger(1)

	// Inactive / suspended / nested-frame / nested-unit / open-mark states
	// all decline.
	var nilES *EmitState
	if _, ok := nilES.FoldFullStack("depth", nil, nil); ok {
		t.Error("a nil recorder must decline")
	}
	es := NewEmitState()
	es.Compilable = false
	if _, ok := es.FoldFullStack("depth", nil, nil); ok {
		t.Error("an uncompilable state must decline")
	}
	es = NewEmitState()
	es.suspended = 1
	if _, ok := es.FoldFullStack("depth", nil, nil); ok {
		t.Error("a suspended state must decline")
	}
	es = NewEmitState()
	es.frames = append(es.frames, nil)
	if _, ok := es.FoldFullStack("depth", nil, nil); ok {
		t.Error("a nested frame must decline")
	}
	es = NewEmitState()
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}})
	if _, ok := es.FoldFullStack("depth", nil, nil); ok {
		t.Error("a nested unit must decline")
	}
	es = NewEmitState()
	es.markWindowSeq = 3
	if _, ok := es.FoldFullStack("depth", nil, nil); ok {
		t.Error("an open mark window must decline")
	}

	// An identity-less stack entry declines; an unknown-provenance carrier
	// declines; a variadic producer declines.
	es = NewEmitState()
	if _, ok := es.FoldFullStack("depth", nil, []Value{one}); ok {
		t.Error("an ID-less entry must decline")
	}
	es = NewEmitState()
	dyn := NewCarrier(TAny)
	dyn.ID = "u1"
	if _, ok := es.FoldFullStack("depth", nil, []Value{dyn}); ok {
		t.Error("an unknown-provenance carrier must decline")
	}
	es = NewEmitState()
	vres := NewCarrier(TInteger)
	vres.ID = "v1"
	es.producedBy["v1"] = producer{seq: 7}
	es.eventInfo[7] = eventFlags{variadicResult: true}
	if _, ok := es.FoldFullStack("depth", nil, []Value{vres}); ok {
		t.Error("a variadic producer must decline — its runtime count is not its model count")
	}

	// An unknown full-stack word declines.
	es = NewEmitState()
	if _, ok := es.FoldFullStack("mystery", nil, []Value{ffsConst("c1", 1)}); ok {
		t.Error("an unknown word must decline")
	}
}

func TestFoldFullStackWords(t *testing.T) {
	stack := []Value{ffsConst("c1", 10), ffsConst("c2", 20)}

	// depth: a fresh CONCRETE count with the stack preserved. A frame-LOCAL
	// entry (a loop iterator's slot) is a known operand home and counts.
	es := NewEmitState()
	es.units[0].localByID["L1"] = 0
	local := NewCarrier(TInteger)
	local.ID = "L1"
	out, ok := es.FoldFullStack("depth", nil, append([]Value{local}, stack...))
	if !ok || len(out) != 4 {
		t.Fatalf("depth fold = %d values (ok=%v), want 4", len(out), ok)
	}
	if n, err := AsInteger(out[3]); err != nil || n != 3 || out[3].ID == "" {
		t.Errorf("depth top = %v, want concrete 3 with an ID", out[3])
	}

	// pick: a copy of the picked entry (same ID); an EVENT target is
	// promoted to a value-def local for the double reference.
	es = NewEmitState()
	ev := NewCarrier(TInteger)
	ev.ID = "e1"
	es.producedBy["e1"] = producer{seq: 4}
	es.eventInfo[4] = eventFlags{}
	st := []Value{ev, ffsConst("c3", 30)}
	out, ok = es.FoldFullStack("pick", []Value{NewInteger(1)}, st)
	if !ok || len(out) != 3 || out[2].ID != "e1" {
		t.Fatalf("pick fold = %v (ok=%v), want the e1 copy on top", out, ok)
	}
	if !es.eventInfo[4].valueDef {
		t.Error("an event-produced pick target must be promoted to a value-def local")
	}

	// pick declines: a non-concrete n, and an out-of-range n (the runtime
	// raises there — the fallback keeps the raise).
	es = NewEmitState()
	carrierN := NewCarrier(TInteger)
	carrierN.ID = "n1"
	if _, ok := es.FoldFullStack("pick", []Value{carrierN}, stack); ok {
		t.Error("a non-concrete n must decline")
	}
	if _, ok := es.FoldFullStack("pick", []Value{NewInteger(9)}, stack); ok {
		t.Error("an out-of-range n must decline")
	}
	if _, ok := es.FoldFullStack("pick", nil, stack); ok {
		t.Error("a missing n must decline")
	}

	// roll: the true permutation, every ID preserved.
	es = NewEmitState()
	st3 := []Value{ffsConst("c1", 1), ffsConst("c2", 2), ffsConst("c3", 3)}
	out, ok = es.FoldFullStack("roll", []Value{NewInteger(1)}, st3)
	if !ok || len(out) != 3 || out[0].ID != "c1" || out[1].ID != "c3" || out[2].ID != "c2" {
		t.Fatalf("roll fold = %v (ok=%v), want [c1 c3 c2]", out, ok)
	}
}
