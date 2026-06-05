package eng

import "testing"

// TestDynamicCarrierMatch pins the not-disjoint matching rule for the
// bounded dynamic(T) modality (design/dynamic-modality-report.0.md): a
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

// TestDynamicResultContagion pins gradual contagion: a result derived
// from a dynamic carrier is itself dynamic (so the modality flows
// downstream), with the sig's declared return as its bound — while a
// result from only strict args stays strict.
func TestDynamicResultContagion(t *testing.T) {
	r, _ := NewRegistry()
	sig := &Signature{Returns: []*Type{TInteger}}

	strict := carrierResults(r, "w", sig, []Value{NewCarrier(TString)}, SrcPos{})
	if len(strict) != 1 || strict[0].Dynamic {
		t.Errorf("strict args must yield a strict result, got Dynamic=%v", strict[0].Dynamic)
	}

	dyn := carrierResults(r, "w", sig, []Value{NewDynamicCarrier(TAny)}, SrcPos{})
	if len(dyn) != 1 || !dyn[0].Dynamic {
		t.Fatalf("a dynamic arg must make the result dynamic, got Dynamic=%v", dyn[0].Dynamic)
	}
	if !dyn[0].Parent.Equal(TInteger) {
		t.Errorf("contagion result bound = %s, want the declared return Integer", dyn[0].Parent)
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
		{NewDynamicCarrierValue(NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})), "dynamic(Integer|String)"},
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
