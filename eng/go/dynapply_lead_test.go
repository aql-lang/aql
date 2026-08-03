package eng

// dynapply_lead_test.go pins EmitState.DynApplyLeadEligible — the Stage-G
// leading one-arg apply admission (stepCloseParen's paren-collapse; see
// checker-compiler-completeness-review.0.md §8.2(1)). White-box per the
// fold_fullstack pattern: every exclusion arm and both admission shapes.

import "testing"

func TestDynApplyLeadEligible(t *testing.T) {
	fnCarrier := func(id string) Value {
		v := NewCarrier(TFunction)
		v.ID = id
		return v
	}

	// The inactive recorder and a nil/inactive state decline.
	if (inactiveEmit{}).DynApplyLeadEligible(Value{}) {
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
