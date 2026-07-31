package eng

import "testing"

// TestSetUnitDeclGuard covers the out-of-range no-op in SetUnitDecl, the
// mirror of TestSetUnitParamTypesGuard: a unit index outside es.fnRecs is
// silently ignored (the named-fn / user-poly compile paths only ever pass a
// unit returned by StartFnCompile).
func TestSetUnitDeclGuard(t *testing.T) {
	NewEmitState().SetUnitDecl(-1, DeclSite{}) // out-of-range → no-op
}

// TestSetUnitReturnPatternsGuard is the RET-side twin of the two guards
// above: SetUnitReturnPatterns carries FnSig.ReturnPatterns onto the compiled
// unit so the VM's RET enforces a declared union / structural return, and it
// takes the same out-of-range no-op as its param-side and decl-site
// neighbours. Both real callers pass a unit StartFnCompile returned and have
// already screened `unit < 0`, so the guard is reachable only here.
func TestSetUnitReturnPatternsGuard(t *testing.T) {
	NewEmitState().SetUnitReturnPatterns(-1, nil) // out-of-range → no-op
}

// TestRecordTrapErrNil covers RecordTrapErr's nil-error guard: a nil BoruError
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

// TestRecordTrapErrFirstWins is the RecordTrapErr mirror of
// TestRecordTrapFirstWins: once a top-level trap is recorded, a second
// RecordTrapErr reports ownership (the trapAt-already-set arm) without
// overwriting the first trap event. Two statically-definite unmatched
// dispatches in one compiled unit reach this at the integration level; the
// white-box call pins the guard directly.
func TestRecordTrapErrFirstWins(t *testing.T) {
	es := NewEmitState() // fresh state is active with one frame and one unit
	if !es.RecordTrapErr(&BoruError{Code: "first", Detail: "d1"}, SrcPos{}) {
		t.Fatal("first RecordTrapErr should be owned here")
	}
	firstAt := es.trapAt
	if firstAt == 0 {
		t.Fatal("first RecordTrapErr should have recorded a trap event")
	}
	if !es.RecordTrapErr(&BoruError{Code: "second", Detail: "d2"}, SrcPos{}) {
		t.Fatal("second RecordTrapErr should still report ownership")
	}
	if es.trapAt != firstAt {
		t.Fatalf("second RecordTrapErr overwrote the trap: trapAt %d != %d", es.trapAt, firstAt)
	}
}
