package eng

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached branches in fnsig.go — the pattern-compatibility arms of
// FnSigSatisfiesSpec, its return-arity guard, and the FnUndefMatchesFnDef
// payload / empty-constraint guards. Pure calls with crafted specs. Per
// design/TEST-SEAMS.10.md.

import "testing"

func TestW8FnSigSatisfiesSpecPatternAbsentOnCandidate(t *testing.T) {
	pat := NewTypeLiteral(TInteger)
	spec := FnSigSpec{Params: []FnParam{{Type: TInteger, Pattern: &pat}}}
	sig := FnSig{Params: []FnParam{{Type: TInteger}}} // sg.Pattern == nil
	if !FnSigSatisfiesSpec(sig, spec) {
		t.Fatal("a candidate without a pattern satisfies a spec that has one")
	}
}

func TestW8FnSigSatisfiesSpecPatternUnifyFail(t *testing.T) {
	spat := NewTypeLiteral(TInteger)
	gpat := NewTypeLiteral(TString)
	spec := FnSigSpec{Params: []FnParam{{Type: TAny, Pattern: &spat}}}
	sig := FnSig{Params: []FnParam{{Type: TAny, Pattern: &gpat}}}
	if FnSigSatisfiesSpec(sig, spec) {
		t.Fatal("incompatible patterns must not satisfy")
	}
}

func TestW8FnSigSatisfiesSpecReturnArity(t *testing.T) {
	spec := FnSigSpec{Returns: []*Type{TInteger}}
	sig := FnSig{} // zero returns vs one expected
	if FnSigSatisfiesSpec(sig, spec) {
		t.Fatal("return-arity mismatch must not satisfy")
	}
}

func TestW8FnUndefMatchesFnDefGuards(t *testing.T) {
	// undef value is not a FnUndefInfo.
	if FnUndefMatchesFnDef(NewInteger(1), NewValueRaw(TFnDef, FnDefInfo{})) {
		t.Fatal("non-FnUndef constraint value must not match")
	}
	// fn value is not a FnDefInfo.
	undef := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{Returns: []*Type{TInteger}}}})
	if FnUndefMatchesFnDef(undef, NewInteger(1)) {
		t.Fatal("non-FnDef candidate value must not match")
	}
	// Empty constraint trivially matches any function.
	emptyUndef := NewFnUndef(FnUndefInfo{})
	if !FnUndefMatchesFnDef(emptyUndef, NewValueRaw(TFnDef, FnDefInfo{})) {
		t.Fatal("an empty FnUndef constraint matches any function")
	}
}
