package native

import "testing"

// PathSubtype is the lexical, path-prefix-only sibling of Matches.
// These tests pin its semantics — no `Any` matches-everything, no
// Dep<Leaf> → base bolt-on, no metatype rules. The simpler check is
// what callers want when they're about to do `v.AsX()` and a
// `DepX` payload would be a silent miscompile.

func TestPathSubtype_Identity(t *testing.T) {
	if !TInteger.PathSubtype(TInteger) {
		t.Errorf("Integer.PathSubtype(Integer) = false, want true")
	}
}

func TestPathSubtype_StrictChild(t *testing.T) {
	// Integer is at Integer; Number is at Number.
	// Integer's parts strictly extend Number's, so it's a path
	// subtype.
	if !TInteger.PathSubtype(TNumber) {
		t.Errorf("Integer.PathSubtype(Number) = false, want true")
	}
}

func TestPathSubtype_ParentNotChild(t *testing.T) {
	// Number is NOT a path subtype of Integer — Number's parts are
	// shorter, so the prefix-of-pattern check fails.
	if TNumber.PathSubtype(TInteger) {
		t.Errorf("Number.PathSubtype(Integer) = true, want false")
	}
}

func TestPathSubtype_DisjointTypes(t *testing.T) {
	if TString.PathSubtype(TInteger) {
		t.Errorf("String.PathSubtype(Integer) = true, want false")
	}
}

// PathSubtype does NOT special-case `Any`. Use `Matches` for the
// lattice-aware "everything is Any" rule.
func TestPathSubtype_AnyIsNotMagical(t *testing.T) {
	if TInteger.PathSubtype(TAny) {
		t.Errorf("Integer.PathSubtype(Any) = true (PathSubtype must NOT special-case Any)")
	}
	if !TInteger.Matches(TAny) {
		t.Errorf("Integer.Matches(Any) = false (Matches MUST special-case Any)")
	}
}

// PathSubtype does NOT bolt the Dep<Leaf>→base relation onto the
// lattice — that's what the override in Matches is for. A DepInteger
// is at Type/Dependent/DepInteger, which doesn't share any prefix
// with Integer.
func TestPathSubtype_DepIntegerIsNotInteger(t *testing.T) {
	dep := NewDepScalar(DepGT, NewInteger(10))
	if dep.Parent.PathSubtype(TInteger) {
		t.Errorf("DepInteger.PathSubtype(Integer) = true (PathSubtype must NOT follow the Dep bolt-on)")
	}
	// Sanity: Matches DOES say yes through the bolt-on.
	if !dep.Parent.Matches(TInteger) {
		t.Errorf("DepInteger.Matches(Integer) = false (Matches MUST follow the Dep bolt-on)")
	}
}

// PathSubtype agrees with Matches on the cases neither special-rule
// touches — strict path-prefix relationships at the bottom of the
// lattice.
func TestPathSubtype_AgreesWithMatchesOnPrefixCases(t *testing.T) {
	cases := []struct {
		name            string
		t               *Type
		pattern         *Type
		wantPathSubtype bool
		wantMatches     bool
	}{
		{"Integer⊆Number", TInteger, TNumber, true, true},
		{"Number⊆Scalar", TNumber, TScalar, true, true},
		{"String⊆Scalar", TString, TScalar, true, true},
		{"Integer⊄String", TInteger, TString, false, false},
		{"Number⊄Integer", TNumber, TInteger, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.t.PathSubtype(tc.pattern); got != tc.wantPathSubtype {
				t.Errorf("PathSubtype: got %v, want %v", got, tc.wantPathSubtype)
			}
			if got := tc.t.Matches(tc.pattern); got != tc.wantMatches {
				t.Errorf("Matches: got %v, want %v", got, tc.wantMatches)
			}
		})
	}
}
