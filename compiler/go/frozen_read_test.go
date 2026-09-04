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

// rebindReachesModuleScope is NUR117's fix, and its three regimes are driven
// directly because each needs a suspension state the end-to-end rows cannot
// set on demand. The equality it tests — `suspended == keepModuleDepth +
// multiRunModuleDepth` — is what makes the fix ADDITIVE: any suspension that
// is not a leaking module-scope body run breaks it, so the latch can only gain
// the `do` / multi-run refusals it is for.
func TestRebindReachesModuleScope(t *testing.T) {
	for _, c := range []struct {
		name                         string
		openUnits, susp, keep, multi int
		want                         bool
	}{
		{"top level, nothing open", 0, 0, 0, 0, true},
		{"a do body at module scope", 0, 1, 1, 0, true},
		{"an each body at module scope", 0, 1, 0, 1, true},
		{"a do inside a do", 0, 2, 2, 0, true},
		{"an each nested inside a do", 0, 2, 1, 0, false},
		{"a fn body (suspended, neither counter)", 0, 1, 0, 0, false},
		{"a do inside a FN body (publication gate declined)", 0, 2, 1, 0, false},
		{"a unit open", 1, 0, 0, 0, false},
		{"a unit open under a do body", 1, 1, 1, 0, false},
	} {
		es := NewEmitState()
		for i := 0; i < c.openUnits; i++ {
			openUnit(es, false)
		}
		es.suspended = c.susp
		es.keepModuleDepth = c.keep
		es.multiRunModuleDepth = c.multi
		if got := es.rebindReachesModuleScope(); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	var nilES *EmitState
	if nilES.rebindReachesModuleScope() {
		t.Error("a nil recorder reaches no scope")
	}
}

// The latch must fire while the recorder is SUSPENDED by a do-body run — the
// whole of NUR117. Before the fix NotifyNameRebound returned at its Active()
// gate and never consulted the table.
func TestNotifyNameReboundFiresUnderADoBody(t *testing.T) {
	es := NewEmitState()
	openUnit(es, false)
	es.NoteFrozenRead("k", core.FrozenBakeValue)
	es.openUnitRecs = es.openUnitRecs[:0]

	// Suspended by a module-scope keep-defs run: the do body's defs leak.
	es.suspended, es.keepModuleDepth = 1, 1
	es.NotifyNameRebound("k")
	want := "module binding k rebound after a fn unit baked its value"
	if es.Compilable || es.Reason != want {
		t.Errorf("a rebind inside a do body must refuse; got compilable=%v reason=%q", es.Compilable, es.Reason)
	}
}

// ...and must NOT fire when the suspension is a fn body, whose defs are
// frame-locals. This is the guard's original case and the row that fails if
// the counters are ever incremented unconditionally.
func TestNotifyNameReboundStaysQuietUnderAFnBody(t *testing.T) {
	es := NewEmitState()
	openUnit(es, false)
	es.NoteFrozenRead("k", core.FrozenBakeValue)
	es.openUnitRecs = es.openUnitRecs[:0]

	es.suspended = 1 // suspended, but by neither leaking-body counter
	es.NotifyNameRebound("k")
	if !es.Compilable {
		t.Errorf("a fn-body-local rebind must not refuse; got %q", es.Reason)
	}
}
