package eng

import "testing"

// TestInactiveDispatchBraid pins the compiler-less defaults behind the
// S3 slot table: each is the exact decline an unarmed recording pass
// produces. The live table is compiler-installed at init
// (installDispatchBraid); these bodies are the post-cut check-only
// configuration.
func TestInactiveDispatchBraid(t *testing.T) {
	inactiveDispatchRecordOutcome(nil, "", nil, nil, nil, SrcPos{}, nil)
	if v, ok := inactiveDispatchTryFoldScalarConst(nil, nil, nil); ok || IsConcrete(v) {
		t.Fatal("inactive scalar fold must decline")
	}
	if inactiveDispatchTryRecordPoly(nil, "", nil, nil, nil, SrcPos{}, false, nil, false, nil) {
		t.Fatal("inactive poly record must decline")
	}
	if inactiveDispatchCompileUserPolyArms(nil, TheInactiveEmit, "", nil, nil) != nil {
		t.Fatal("inactive poly-arm compile must decline")
	}
	if p, barred := inactiveDispatchPlanUserPoly(nil, TheInactiveEmit, "", nil, nil); p != nil || barred {
		t.Fatal("inactive poly plan must decline")
	}
}

// TestUserPolyPlanHandle pins the opaque-plan accessors: a nil-plan
// bake returns a NIL INTERFACE through the installer wrapper (the
// typed-nil trap), and a concrete plan's accessors expose the parallel
// slices the record site unpacks.
func TestUserPolyPlanHandle(t *testing.T) {
	if got := DispatchBraid.CompileUserPolyArms(nil, TheInactiveEmit, "", nil, nil); got != nil {
		t.Fatal("a declined bake must surface as a nil interface")
	}
	p := &userPolyPlan{sigIdx: []int{1}, units: []int{2}, impls: []SigImpl{nil}, sigs: []Signature{{}}}
	var h UserPolyPlan = p
	if len(h.SigIdx()) != 1 || len(h.Units()) != 1 || len(h.Impls()) != 1 || len(h.Sigs()) != 1 {
		t.Fatal("plan accessors must expose the parallel slices")
	}
	p.joined = []bool{true}
	p.outs = []Value{NewInteger(9)}
	out := []Value{NewInteger(1)}
	h.SubstituteJoinedOuts(out)
	if n, err := AsInteger(out[0]); err != nil || n != 9 {
		t.Fatal("SubstituteJoinedOuts must substitute the joined position")
	}
}
