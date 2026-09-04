package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The freeze discipline's table is keyed by name and valued by WHAT the unit
// froze. These pin NoteFrozenRead's contract directly — the end-to-end rows
// live in lang/go/frozen_module_read_test.go, which cannot see the arms below
// because a program that reaches them always refuses for the same reason.

// openUnit puts the state inside an open unit, which is the only state in
// which NoteFrozenRead records at all.
func openUnit(es *EmitState, storedRef bool) {
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "u", storedRefUnit: storedRef})
	es.openUnitRecs = append(es.openUnitRecs, len(es.fnRecs)-1)
}

func TestNoteFrozenReadRecordsTheBakeKind(t *testing.T) {
	for _, c := range []struct {
		bake core.FrozenBake
		want string
	}{
		{core.FrozenBakeValue, "value"},
		{core.FrozenBakeType, "type"},
		{core.FrozenBakeCall, "call target"},
	} {
		es := NewEmitState()
		openUnit(es, false)
		es.NoteFrozenRead("n", c.bake)
		got, ok := es.frozenReads["n"]
		if !ok {
			t.Fatalf("%v: an in-unit module read must record", c.bake)
		}
		if got.String() != c.want {
			t.Errorf("%v: refusal would name %q, want %q", c.bake, got.String(), c.want)
		}
	}
}

// The invalid zero is DROPPED, not stored. A note that reached no classifier
// would otherwise refuse a program while naming an artifact nothing froze —
// and "No Zero-Value Overload (CRITICAL)" forbids letting the zero stand in
// for a real member.
func TestNoteFrozenReadDropsTheInvalidZero(t *testing.T) {
	es := NewEmitState()
	openUnit(es, false)
	es.NoteFrozenRead("n", core.FrozenBakeNone)
	if _, ok := es.frozenReads["n"]; ok {
		t.Error("an unclassified note must be dropped, not recorded as a default bake")
	}
}

// FIRST BAKE WINS, so the refusal's text cannot depend on analysis order.
func TestNoteFrozenReadKeepsTheFirstBake(t *testing.T) {
	es := NewEmitState()
	openUnit(es, false)
	es.NoteFrozenRead("n", core.FrozenBakeCall)
	es.NoteFrozenRead("n", core.FrozenBakeValue)
	if got := es.frozenReads["n"]; got != core.FrozenBakeCall {
		t.Errorf("second note overwrote the first: got %v", got)
	}
}

// The negatives, each of which must record NOTHING: no open unit (top level,
// where analysis order is program order), an empty name, and a STORED-REF
// unit, whose rebind handling is NotifyNameRebound's own depHit arm and whose
// entries here would double-report one rebind.
func TestNoteFrozenReadNegatives(t *testing.T) {
	top := NewEmitState()
	top.NoteFrozenRead("n", core.FrozenBakeValue)
	if len(top.frozenReads) != 0 {
		t.Error("a TOP-LEVEL read must not record: the bake is the read the interpreter makes")
	}

	empty := NewEmitState()
	openUnit(empty, false)
	empty.NoteFrozenRead("", core.FrozenBakeValue)
	if len(empty.frozenReads) != 0 {
		t.Error("an empty name must not record")
	}

	stored := NewEmitState()
	openUnit(stored, true)
	stored.NoteFrozenRead("n", core.FrozenBakeValue)
	if len(stored.frozenReads) != 0 {
		t.Error("a stored-ref unit's read must stay out of the table")
	}
}

// The latch fires only with NO unit open, and its reason names the bake.
func TestNotifyNameReboundNamesTheBake(t *testing.T) {
	es := NewEmitState()
	openUnit(es, false)
	es.NoteFrozenRead("g", core.FrozenBakeCall)
	// Still inside the unit: a body-local def shadows independently.
	es.NotifyNameRebound("g")
	if !es.Compilable {
		t.Fatalf("a rebind with a unit still open must not refuse; got %q", es.Reason)
	}
	es.openUnitRecs = es.openUnitRecs[:0]
	es.NotifyNameRebound("g")
	want := "module binding g rebound after a fn unit baked its call target"
	if es.Compilable || es.Reason != want {
		t.Errorf("want refusal %q, got compilable=%v reason=%q", want, es.Compilable, es.Reason)
	}
}

// An unfrozen name is untouched by a rebind — the latch is keyed on an actual
// bake, not on "reads a module ref".
func TestNotifyNameReboundIgnoresUnfrozenNames(t *testing.T) {
	es := NewEmitState()
	es.NotifyNameRebound("never-read")
	if !es.Compilable {
		t.Errorf("a rebind of a name no unit baked must not refuse; got %q", es.Reason)
	}
}
