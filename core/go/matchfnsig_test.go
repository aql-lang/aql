package core

import "testing"

// TestMatchFnSig walks every arm of the matcher that moved down from basic/go
// (NUR107): the VM's dynamic apply needs it to tell "not a function" from "a
// function no overload of which admits these arguments", so each way a
// signature can fail to match is a branch the two lanes' agreement rests on.
func TestMatchFnSig(t *testing.T) {
	fnOf := func(sigs ...FnSig) Value {
		return NewFunction(FnDefInfo{Anonymous: true, Signatures: sigs})
	}
	one := FnSig{Params: []FnParam{{Name: "n", Type: TInteger}}, Returns: []*Type{TInteger}}
	two := FnSig{
		Params:  []FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		Returns: []*Type{TInteger},
	}

	// A value with no FnDefInfo payload has no own signatures to consult. The
	// caller must read this nil as "no opinion", not as "no match" — which is
	// why the VM's guard checks OwnSigs() length before trusting it.
	if MatchFnSig(NewInteger(5), []Value{NewInteger(1)}) != nil {
		t.Error("a non-fn payload must answer nil")
	}

	// Arity: the matcher skips a signature whose param count differs, so a
	// 1-arg callee under a 2-value window has nothing to match.
	if MatchFnSig(fnOf(one), []Value{NewInteger(1), NewInteger(2)}) != nil {
		t.Error("arity mismatch must not match")
	}
	if MatchFnSig(fnOf(two), []Value{NewInteger(1)}) != nil {
		t.Error("a 2-arg sig must not match a 1-value window")
	}

	// Type: the arg must conform to the declared param type. This is the arm
	// NUR107's headline shape takes — a String-typed param under an Integer.
	if MatchFnSig(fnOf(one), []Value{NewString("s")}) != nil {
		t.Error("a type mismatch must not match")
	}
	if MatchFnSig(fnOf(one), []Value{NewInteger(1)}) == nil {
		t.Error("a conforming arg must match")
	}

	// Overload selection walks in declaration order and returns the FIRST
	// admitting signature.
	got := MatchFnSig(fnOf(two, one), []Value{NewInteger(1)})
	if got == nil || len(got.Params) != 1 {
		t.Errorf("want the 1-arg overload, got %v", got)
	}

	// A VALUE PATTERN narrows past the type: same type, wrong value, no match.
	pat := NewInteger(7)
	valPat := FnSig{Params: []FnParam{{Name: "n", Type: TInteger, Pattern: &pat}}, Returns: []*Type{TInteger}}
	if MatchFnSig(fnOf(valPat), []Value{NewInteger(1)}) != nil {
		t.Error("a value pattern must reject a non-equal argument")
	}
	if MatchFnSig(fnOf(valPat), []Value{NewInteger(7)}) == nil {
		t.Error("a value pattern must accept its own value")
	}

	// A CARRIER pattern is a type-position placeholder, not a value test, so
	// the pattern arm is skipped entirely and the type check alone decides.
	carrier := NewCarrier(TInteger)
	carrierPat := FnSig{Params: []FnParam{{Name: "n", Type: TInteger, Pattern: &carrier}}, Returns: []*Type{TInteger}}
	if MatchFnSig(fnOf(carrierPat), []Value{NewInteger(3)}) == nil {
		t.Error("a carrier pattern must not narrow — the type check decides")
	}

	// A MAP pattern takes the OpenUnifyMap arm: the argument must carry the
	// pattern's keys, and may carry more.
	pm := NewOrderedMap()
	pm.Set("a", NewInteger(1))
	mapPat := NewValueRaw(TMap, MapPayload{M: pm})
	mapSig := FnSig{Params: []FnParam{{Name: "m", Type: TMap, Pattern: &mapPat}}, Returns: []*Type{TInteger}}

	hit := NewOrderedMap()
	hit.Set("a", NewInteger(1))
	hit.Set("b", NewInteger(2))
	if MatchFnSig(fnOf(mapSig), []Value{NewValueRaw(TMap, MapPayload{M: hit})}) == nil {
		t.Error("a map pattern must accept a superset argument")
	}
	miss := NewOrderedMap()
	miss.Set("a", NewInteger(9))
	if MatchFnSig(fnOf(mapSig), []Value{NewValueRaw(TMap, MapPayload{M: miss})}) != nil {
		t.Error("a map pattern must reject a differing value")
	}
}
