package eng

import "testing"

// TestSetUnitDeclGuard covers the out-of-range no-op in SetUnitDecl, the
// mirror of TestSetUnitParamTypesGuard: a unit index outside es.fnRecs is
// silently ignored (the named-fn / user-poly compile paths only ever pass a
// unit returned by StartFnCompile).
func TestSetUnitDeclGuard(t *testing.T) {
	NewEmitState().SetUnitDecl(-1, DeclSite{}) // out-of-range → no-op
}

// TestRecordTrapErrNil covers RecordTrapErr's nil-error guard: a nil AqlError
// records nothing and reports "not owned here". Callers always pass a
// freshly-built interpreter error, so this is the defensive floor.
func TestRecordTrapErrNil(t *testing.T) {
	if NewEmitState().RecordTrapErr(nil, SrcPos{}) {
		t.Fatal("RecordTrapErr(nil) should refuse")
	}
}

// TestRecordTrapFirstWins covers the "a trap is already recorded" arm of
// RecordTrap: the first top-level trap wins (execution can reach only one),
// and a second RecordTrap reports ownership without overwriting it.
func TestRecordTrapFirstWins(t *testing.T) {
	es := NewEmitState() // fresh state is active with one frame and one unit
	if !es.RecordTrap("first", "d1", "w1", "h1", SrcPos{}) {
		t.Fatal("first RecordTrap should be owned here")
	}
	firstAt := es.trapAt
	if firstAt == 0 {
		t.Fatal("first RecordTrap should have recorded a trap event")
	}
	if !es.RecordTrap("second", "d2", "w2", "h2", SrcPos{}) {
		t.Fatal("second RecordTrap should still report ownership")
	}
	if es.trapAt != firstAt {
		t.Fatalf("second RecordTrap overwrote the trap: trapAt %d != %d", es.trapAt, firstAt)
	}
}
