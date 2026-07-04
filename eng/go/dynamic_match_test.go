package eng

import "testing"

// TestDynamicCarrierMatch pins the not-disjoint matching rule for the
// bounded dynamic(T) modality (design/dynamic-modality-report.10.md): a
// dynamic carrier matches a signature slot unless its bound is PROVABLY
// disjoint from the slot, the optimistic dual of strict ConformsTo. The
// contrast block proves the modality is what flips the behaviour — a
// non-dynamic Carry<Any> still fails an Integer slot.
func TestDynamicCarrierMatch(t *testing.T) {
	dynDisjunct := NewDynamicCarrierValue(
		NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)}),
	)

	cases := []struct {
		name string
		v    Value
		slot *Type
		want bool
	}{
		// dynamic(Any) — classic gradual any: compatible with every slot.
		{"dyn(Any) vs Integer", NewDynamicCarrier(TAny), TInteger, true},
		{"dyn(Any) vs String", NewDynamicCarrier(TAny), TString, true},
		{"dyn(Any) vs Atom", NewDynamicCarrier(TAny), TAtom, true},
		{"dyn(Any) vs List", NewDynamicCarrier(TAny), TList, true},
		{"dyn(Any) vs Any", NewDynamicCarrier(TAny), TAny, true},

		// dynamic(Integer) — matches overlapping slots, fails disjoint ones.
		{"dyn(Integer) vs Integer", NewDynamicCarrier(TInteger), TInteger, true},
		{"dyn(Integer) vs Number", NewDynamicCarrier(TInteger), TNumber, true},
		{"dyn(Integer) vs Scalar", NewDynamicCarrier(TInteger), TScalar, true},
		{"dyn(Integer) vs Any", NewDynamicCarrier(TInteger), TAny, true},
		{"dyn(Integer) vs String", NewDynamicCarrier(TInteger), TString, false},
		{"dyn(Integer) vs Atom", NewDynamicCarrier(TInteger), TAtom, false},
		{"dyn(Integer) vs Boolean", NewDynamicCarrier(TInteger), TBoolean, false},
		{"dyn(Integer) vs List", NewDynamicCarrier(TInteger), TList, false},

		// dynamic(Number) overlaps Integer (a dynamic Number could be one).
		{"dyn(Number) vs Integer", NewDynamicCarrier(TNumber), TInteger, true},

		// dynamic(Integer tor String) — union bound, member-wise overlap.
		{"dyn(Int|Str) vs Number", dynDisjunct, TNumber, true},
		{"dyn(Int|Str) vs String", dynDisjunct, TString, true},
		{"dyn(Int|Str) vs Integer", dynDisjunct, TInteger, true},
		{"dyn(Int|Str) vs Atom", dynDisjunct, TAtom, false},
		{"dyn(Int|Str) vs List", dynDisjunct, TList, false},

		// Contrast: a NON-dynamic carrier keeps strict ConformsTo. This is
		// the whole point — strict Carry<Any> fails Integer, the footgun
		// dynamic exists to fix; and Carry<Integer> still fails String.
		{"strict Carry<Any> vs Integer", NewCarrier(TAny), TInteger, false},
		{"strict Carry<Integer> vs Integer", NewCarrier(TInteger), TInteger, true},
		{"strict Carry<Integer> vs Number", NewCarrier(TInteger), TNumber, true},
		{"strict Carry<Integer> vs String", NewCarrier(TInteger), TString, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sigTypeMatches(tc.v, tc.slot); got != tc.want {
				t.Errorf("sigTypeMatches = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDynamicResultContagion pins gradual contagion and its refinement: a
// result derived from a dynamic carrier is dynamic EXCEPT when its declared
// return is a CONCRETE, single-reachable CONTAINER (List/Map) — that type is
// fixed by the word's contract regardless of which runtime value flowed in,
// so a downstream `each`/`fold`/accessor can commit on it (the cross-module
// element-typing flip). A concrete SCALAR return (Integer) still rides
// dynamic (keeping it strict gains no consumer and can trip the forward/stack
// split detector), as does a genuinely input-dependent Any return. Strict
// args always yield a strict result.
func TestDynamicResultContagion(t *testing.T) {
	r, _ := NewRegistry()
	listSig := &Signature{Returns: []*Type{TList}}

	strict := carrierResults(r, "w", listSig, []Value{NewCarrier(TString)}, SrcPos{}, nil, false)
	if len(strict) != 1 || strict[0].Dynamic {
		t.Errorf("strict args must yield a strict result, got Dynamic=%v", strict[0].Dynamic)
	}

	// A dynamic input + a CONCRETE, single-reachable CONTAINER return: the
	// result type is statically known (List), so it stays STRICT — a
	// downstream `each` over such a declared-List return can now commit
	// instead of refusing "dynamic input at each".
	cont := carrierResults(r, "w", listSig, []Value{NewDynamicCarrier(TAny)}, SrcPos{}, nil, false)
	if len(cont) != 1 || cont[0].Dynamic {
		t.Fatalf("a concrete container return stays strict over a dynamic input, got Dynamic=%v", cont[0].Dynamic)
	}
	if !cont[0].Parent.Equal(TList) {
		t.Errorf("result bound = %s, want the declared return List", cont[0].Parent)
	}

	// A dynamic input + a concrete SCALAR return stays dynamic (contagion is
	// scoped to containers; a strict scalar gains nothing and can trip the
	// forward/stack split detector).
	intSig := &Signature{Returns: []*Type{TInteger}}
	sc := carrierResults(r, "w", intSig, []Value{NewDynamicCarrier(TAny)}, SrcPos{}, nil, false)
	if len(sc) != 1 || !sc[0].Dynamic {
		t.Fatalf("a dynamic input + scalar return must stay dynamic, got Dynamic=%v", sc[0].Dynamic)
	}

	// A dynamic input + a declared ANY return IS input-dependent (the type
	// is genuinely unknown), so the modality flows downstream as dynamic.
	anySig := &Signature{Returns: []*Type{TAny}}
	dyn := carrierResults(r, "w", anySig, []Value{NewDynamicCarrier(TAny)}, SrcPos{}, nil, false)
	if len(dyn) != 1 || !dyn[0].Dynamic {
		t.Fatalf("a dynamic input + Any return must stay dynamic, got Dynamic=%v", dyn[0].Dynamic)
	}
}

// TestDynamicFirstMatchPartition pins the first-match partition for the
// dispatch RESULT: when a dynamic bound reaches several overloads with
// divergent returns, the result is the UNION of those returns (sound),
// not just the first match's (too narrow). No production word has
// return-divergent overloads spanned by a dynamic bound, so this is
// verified with a synthetic word.
func TestDynamicFirstMatchPartition(t *testing.T) {
	r, _ := NewRegistry()
	r.RegisterNativeFunc(NativeFunc{
		Name: "wdiv",
		Signatures: []Signature{
			{Args: []*Type{TInteger}, Returns: []*Type{TBoolean}},
			{Args: []*Type{TString}, Returns: []*Type{TAtom}},
		},
	})
	intStr := NewDynamicCarrierValue(NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)}))

	// dynamic(Integer|String) reaches BOTH overloads → union {Boolean, Atom}.
	if rets := dynamicReachableReturns(r, "wdiv", []Value{intStr}); len(rets) != 2 {
		t.Fatalf("expected 2 reachable returns for dynamic(Integer|String), got %v", rets)
	}

	// The dispatch result is dynamic(Atom tor Boolean) — a union whose terms
	// print in canonical tcmp order (disjuncts are sorted at construction).
	sig := &Signature{Args: []*Type{TInteger}, Returns: []*Type{TBoolean}}
	out := carrierResults(r, "wdiv", sig, []Value{intStr}, SrcPos{}, nil, false)
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("expected a single dynamic result, got %v", out)
	}
	if got := out[0].String(); got != "dynamic(Atom tor Boolean)" {
		t.Errorf("partition result = %q, want dynamic(Atom tor Boolean)", got)
	}

	// A dynamic bound reaching only ONE overload → no refinement (the
	// matched return is already correct).
	if got := dynamicReachableReturns(r, "wdiv", []Value{NewDynamicCarrier(TInteger)}); got != nil {
		t.Errorf("dynamic(Integer) reaches one overload; expected no union, got %v", got)
	}
}

// TestDynamicCarrierString pins the trace rendering: a dynamic carrier
// renders as dynamic(<bound>) so the modality is legible, while a strict
// carrier is unchanged.
func TestDynamicCarrierString(t *testing.T) {
	cases := []struct {
		v    Value
		want string
	}{
		{NewDynamicCarrier(TInteger), "dynamic(Integer)"},
		{NewDynamicCarrier(TAny), "dynamic(Any)"},
		{NewDynamicCarrierValue(NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})), "dynamic(Integer tor String)"},
		{NewCarrier(TInteger), "Integer"}, // strict carrier unchanged
	}
	for _, tc := range cases {
		if got := tc.v.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

// TestDynamicCarrierConstructors pins the invariants of the dynamic
// constructors: Dynamic implies Carrier, the bound is preserved, and
// toCarrier never strips a dynamic carrier (which would null its bound).
func TestDynamicCarrierConstructors(t *testing.T) {
	d := NewDynamicCarrier(TInteger)
	if !d.Dynamic || !d.Carrier {
		t.Fatalf("NewDynamicCarrier: Dynamic=%v Carrier=%v, want both true", d.Dynamic, d.Carrier)
	}
	if !d.Parent.Equal(TInteger) {
		t.Errorf("NewDynamicCarrier bound = %s, want Integer", d.Parent)
	}

	// A disjunct promoted to dynamic keeps its alternatives.
	disj := NewDynamicCarrierValue(
		NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)}),
	)
	if !disj.Dynamic || !IsDisjunct(disj) {
		t.Errorf("NewDynamicCarrierValue: Dynamic=%v IsDisjunct=%v, want both true", disj.Dynamic, IsDisjunct(disj))
	}

	// toCarrier must return a dynamic carrier unchanged — same bound,
	// flag intact.
	got := toCarrier(d)
	if !got.Dynamic || !got.Parent.Equal(TInteger) {
		t.Errorf("toCarrier(dynamic) = Dynamic %v / %s, want dynamic Integer", got.Dynamic, got.Parent)
	}
}
