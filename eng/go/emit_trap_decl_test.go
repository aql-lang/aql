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
