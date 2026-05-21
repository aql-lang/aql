package eng

import (
	"sort"
	"strings"
)

// compareTypes is a total order on *Type. CompareValues uses it as the
// post-size tiebreaker for a same-branch pair, and it is the order
// under which type literals sort. Two distinct types never compare
// equal. The keys, in priority order:
//
//  1. family rank — the declared complexity of the type's family
//     (List < Map, …). A subtype inherits its family's rank via the
//     parent-chain walk, so a `def Foo List` and a `def Bar List`
//     both rank as List.
//  2. depth — the shallower type first: the more general type before
//     a subtype that refines it (List before a `def Foo List`).
//  3. name — lexical. The nominal key that finally separates sibling
//     types — Foo and Bar — which share both family rank and depth.
//  4. id — the lattice identity string; the last-ditch floor for the
//     rare pair of distinct types that share a name (a shadowed def).
//
// Precondition: CompareValues reaches this only for a same-branch
// pair — rootBranchRank is checked first.
func compareTypes(a, b *Type) int {
	if a == b {
		return 0
	}
	if c := cmpInt(typeFamilyRank(a), typeFamilyRank(b)); c != 0 {
		return c
	}
	if c := cmpInt(typeDepth(a), typeDepth(b)); c != 0 {
		return c
	}
	if c := strings.Compare(a.Name, b.Name); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}

// typeFamilyRank ranks a type by the declared complexity of its
// family, least complex first — a List leads a Map. The ordering is
// editorial: extend the switch to rank further families. A type with
// no ranked ancestor ranks 0 and so falls through to compareTypes's
// depth and name keys.
func typeFamilyRank(t *Type) int {
	for ; t != nil; t = t.Parent {
		switch t {
		case TList:
			return 1
		case TMap:
			return 2
		}
	}
	return 0
}

// typeDepth is the length of t's parent chain — its distance from the
// lattice root. A subtype is one deeper than the type it refines.
func typeDepth(t *Type) int {
	d := 0
	for ; t != nil; t = t.Parent {
		d++
	}
	return d
}

// cmpInt returns -1, 0, or 1 for the ordering of two ints.
func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// compareStructural breaks a tie between two values that share a VType
// and a size. A List-like value compares element by element, a
// Map-like value compares its sorted keys and then the value at each
// key — both recursing through CompareValues. Any other value is an
// opaque type with no structure to descend, and falls to a last-resort
// comparison of the canonical (round-trippable) rendering — the one
// deliberate use of a rendered form as an ordering key beyond String,
// Word, and Atom.
func compareStructural(a, b Value) (int, error) {
	if IsConcrete(a) && IsConcrete(b) {
		if a.VType.Matches(TList) {
			return compareListElems(a, b)
		}
		if a.VType.Matches(TMap) {
			return compareMapEntries(a, b)
		}
	}
	return strings.Compare(CanonValue(a), CanonValue(b)), nil
}

// compareListElems orders two list-like values element by element.
func compareListElems(a, b Value) (int, error) {
	ae, aerr := AsMutableList(a)
	be, berr := AsMutableList(b)
	if aerr != nil || berr != nil {
		// Not a plain element list (a typed list, a table, …) — fall
		// back to the canonical rendering.
		return strings.Compare(CanonValue(a), CanonValue(b)), nil
	}
	if len(ae) != len(be) {
		return cmpInt(len(ae), len(be)), nil
	}
	for i := range ae {
		c, err := CompareValues(ae[i], be[i])
		if err != nil {
			return 0, err
		}
		if c != 0 {
			return c, nil
		}
	}
	return 0, nil
}

// compareMapEntries orders two map-like values by their sorted key
// lists, then by the value stored at each shared key.
func compareMapEntries(a, b Value) (int, error) {
	am, aerr := AsMutableMap(a)
	bm, berr := AsMutableMap(b)
	if aerr != nil || berr != nil {
		return strings.Compare(CanonValue(a), CanonValue(b)), nil
	}
	ak := append([]string(nil), am.Keys()...)
	bk := append([]string(nil), bm.Keys()...)
	if len(ak) != len(bk) {
		return cmpInt(len(ak), len(bk)), nil
	}
	sort.Strings(ak)
	sort.Strings(bk)
	for i := range ak {
		if c := strings.Compare(ak[i], bk[i]); c != 0 {
			return c, nil
		}
	}
	// Same key set — compare the value at each key, in key order.
	for _, k := range ak {
		av, _ := am.Get(k)
		bv, _ := bm.Get(k)
		c, err := CompareValues(av, bv)
		if err != nil {
			return 0, err
		}
		if c != 0 {
			return c, nil
		}
	}
	return 0, nil
}
