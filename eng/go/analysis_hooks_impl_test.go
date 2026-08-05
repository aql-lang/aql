package eng

import "testing"

// TestInactiveAnalysisImpl pins the check-less defaults behind the S1
// implementation table: each is the exact no-op an inactive analysis
// pass produces. The live table is check-installed at init; these
// bodies are the post-cut core-only configuration.
func TestInactiveAnalysisImpl(t *testing.T) {
	inactiveFnConstructionPass(nil, "", FnDefInfo{})
	if inactiveReturnsFn(nil, "", FnSig{}, FnDefInfo{}) != nil {
		t.Fatal("inactive returnsFn must be nil")
	}
	in := []Value{NewInteger(1)}
	if got := inactiveStripToCarriers(in); len(got) != 1 || !ValuesEqual(got[0], in[0]) {
		t.Fatal("inactive stripToCarriers must pass through")
	}
	if got := inactiveZeroOutResiduals(nil, in); len(got) != 1 {
		t.Fatal("inactive zeroOutResiduals must pass through")
	}
	if inactiveCarrierResults(nil, "", nil, nil, SrcPos{}, nil, false) != nil {
		t.Fatal("inactive carrierResults must be nil")
	}
	if inactiveMixedConform(Value{}, TInteger) {
		t.Fatal("inactive mixedConform must be false")
	}
	if inactiveValueCarriesCarrier(NewCarrier(TInteger)) {
		t.Fatal("inactive valueCarriesCarrier must be false")
	}
	if inactiveAtUncaughtTopLevel(nil) {
		t.Fatal("inactive atUncaughtTopLevel must be false")
	}
	inactiveAddUnique(nil, CheckDiagnostic{})
	c := NewCarrier(TInteger)
	if got := inactiveJoinCarriers(Value{}, c); !ValuesEqual(got, c) {
		t.Fatal("inactive join must keep the last write")
	}
}
