package eng

import "testing"

// A gradual (dynamic) carrier matches a slot unless its bound is PROVABLY
// disjoint from the slot. The `tand` disjointness probe wrongly reports two
// container-family carriers as disjoint when one conforms to the other
// (tand(List, Node) = Never even though List ⊑ Node), so sigTypeMatches now
// checks conformance in both directions first. Without it, a value pulled from
// a fn whose declared return narrowed to a dynamic List/Map carrier failed to
// match a Node-typed `get`/`set` receiver — the trie client's node-walker
// cascade (`(nd kid-items)` / `build-row`-result → `get`).
func TestDynamicCarrierMatchesNodeFamily(t *testing.T) {
	dyn := func(b *Type) Value { v := NewCarrier(b); v.Dynamic = true; return v }

	// Positive: a dynamic container bound matches the broader Node slot
	// (X ⊑ t), and a dynamic Node matches a narrower List/Map slot (t ⊑ X,
	// gradual optimism), and Any matches everything.
	match := []struct {
		name string
		v    Value
		slot *Type
	}{
		{"dynamic(List) vs Node", dyn(TList), TNode},
		{"dynamic(Map) vs Node", dyn(TMap), TNode},
		{"dynamic(Node) vs List", dyn(TNode), TList},
		{"dynamic(Node) vs Map", dyn(TNode), TMap},
		{"dynamic(List) vs List", dyn(TList), TList},
		{"dynamic(Integer) vs Number", dyn(TInteger), TNumber},
		{"dynamic(Number) vs Integer", dyn(TNumber), TInteger},
		{"dynamic(Any) vs Node", dyn(TAny), TNode},
	}
	for _, c := range match {
		if !SigTypeMatches(c.v, c.slot) {
			t.Errorf("%s: expected match, got false", c.name)
		}
	}

	// Negative: provably-disjoint bounds still do NOT match — the gradual
	// looseness must not erase real type distinctions.
	noMatch := []struct {
		name string
		v    Value
		slot *Type
	}{
		{"dynamic(Integer) vs String", dyn(TInteger), TString},
		{"dynamic(String) vs Integer", dyn(TString), TInteger},
		{"dynamic(List) vs String", dyn(TList), TString},
	}
	for _, c := range noMatch {
		if SigTypeMatches(c.v, c.slot) {
			t.Errorf("%s: expected NO match (provably disjoint), got true", c.name)
		}
	}
}
