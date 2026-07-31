package eng

import "testing"

// --- disjunctCombosTakeSig (the partitioned user-fn recording gate) --------

func ljImplSig(impl SigImpl, args ...*Type) Signature {
	return Signature{Args: args, BarrierPos: -1, Impl: impl}
}

func TestDisjunctCombosTakeSig(t *testing.T) {
	r := covRegistry(t, nil)
	implAny := &BORUImpl{Body: []Value{NewInteger(1)}}
	implInt := &BORUImpl{Body: []Value{NewInteger(2)}}
	implStr := &BORUImpl{Body: []Value{NewInteger(3)}}
	r.RegisterNativeFunc(NativeFunc{Name: "ljone", Signatures: []Signature{ljImplSig(implAny, TAny)}})
	r.RegisterNativeFunc(NativeFunc{Name: "ljtwo", Signatures: []Signature{
		ljImplSig(implInt, TInteger), ljImplSig(implStr, TString),
	}})
	if err := r.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	d := strictDisjunct(NewTypeLiteral(TInteger), NewTypeLiteral(TString))
	anySig := &Signature{Args: []*Type{TAny}, BarrierPos: -1, Impl: implAny}

	if disjunctCombosTakeSig(r, "lj-no-such-word", []Value{d}, anySig) {
		t.Error("an unresolvable word must decline")
	}
	// 5 two-alternative disjunct args = 32 combos > the partition cap.
	wide := make([]Value, 5)
	for i := range wide {
		wide[i] = strictDisjunct(NewTypeLiteral(TInteger), NewTypeLiteral(TString))
	}
	if disjunctCombosTakeSig(r, "ljone", wide, anySig) {
		t.Error("a combo enumeration over the cap must decline")
	}
	// The String alternative first-matches the SIBLING String arm, not the
	// committed Integer one — one baked CALL_USER would miscompile it.
	intSig := &Signature{Args: []*Type{TInteger}, BarrierPos: -1, Impl: implInt}
	if disjunctCombosTakeSig(r, "ljtwo", []Value{d}, intSig) {
		t.Error("a combo taking a sibling overload must decline")
	}
	// Every combo lands on the sole Any arm — one recorded call is faithful.
	if !disjunctCombosTakeSig(r, "ljone", []Value{d}, anySig) {
		t.Error("uniform combos over the committed sig must pass")
	}
}

// TestCarrierResultsPartitionCountMismatchKeepsRecorded pins carrierResults'
// partitioned-user-fn tail: when the partition-joined row count differs from
// the recorded dispatch's result count, the RECORDED results are returned
// verbatim (sound, just wider) instead of re-IDing a mismatched tuple. The
// crafted ReturnsFn answers TWO carriers for the whole-disjunct args and ONE
// per concrete alternative combo.
func TestCarrierResultsPartitionCountMismatchKeepsRecorded(t *testing.T) {
	r := covRegistry(t, nil)
	impl := &BORUImpl{Body: []Value{NewInteger(1)}, FnFrame: &FnFrameMeta{}}
	r.RegisterNativeFunc(NativeFunc{Name: "ljmis", Signatures: []Signature{{
		Args: []*Type{TAny}, BarrierPos: -1, Impl: impl,
		ReturnsFn: func(args []Value, _ *Registry) []Value {
			if len(args) == 1 && IsDisjunct(args[0]) {
				return []Value{NewCarrier(TInteger), NewCarrier(TString)}
			}
			return []Value{NewCarrier(TInteger)}
		},
	}}})
	if err := r.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	done := w8ArmCompile(t, r)
	defer done()
	fn := r.Lookup("ljmis")
	if fn == nil || len(fn.Signatures) == 0 {
		t.Fatal("ljmis not registered")
	}
	d := strictDisjunct(NewTypeLiteral(TInteger), NewTypeLiteral(TString))
	out := carrierResults(r, "ljmis", &fn.Signatures[0], []Value{d}, SrcPos{}, nil, false)
	if len(out) != 2 {
		t.Fatalf("the count mismatch must keep the RECORDED results (2), got %d: %v", len(out), out)
	}
}
