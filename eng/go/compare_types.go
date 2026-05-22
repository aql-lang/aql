package eng

import (
	"sort"
	"strings"
)

// compareTypes is a total order on *Type — the tiebreaker CompareValues
// applies once two values share a unified Rank, and the order under
// which type literals sort. Two distinct types never compare equal.
// The keys, in priority order:
//
//  1. Rank — the unified lattice rank (typetable.go::builtinDecls). A
//     builtin child always ranks above its parent; user and external
//     types inherit the parent's Rank, so a builtin and all the user
//     types descending from it share one Rank and fall to key 2.
//  2. depth — the shallower type first: the more general type before a
//     subtype that refines it (List before a `def Foo refine List`).
//  3. name — lexical. Separates sibling types that share Rank and
//     depth — Foo and Bar, both `def … refine List`.
//  4. id — the lattice identity string; the last-ditch floor for the
//     rare pair of distinct types that share a name (a shadowed def).
func compareTypes(a, b *Type) int {
	if a == b {
		return 0
	}
	if c := cmpInt(rankOf(a), rankOf(b)); c != 0 {
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

// rankOf returns t's unified lattice Rank. Builtins, MintType, and
// RegisterExternalBuiltin all set Rank at creation, so this normally
// returns t.Rank directly; the parent-chain walk is a fallback for a
// *Type assembled without one (chiefly in tests).
func rankOf(t *Type) int {
	for ; t != nil; t = t.Parent {
		if t.Rank != 0 {
			return t.Rank
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

// compareStructural breaks a tie between two values that share a Parent
// and a size. A List-like value compares element by element, a
// Map-like value compares its sorted keys and then the value at each
// key — both recursing through CompareValues. Any other value is an
// opaque type with no structure to descend, and falls to a last-resort
// comparison of the canonical (round-trippable) rendering — the one
// deliberate use of a rendered form as an ordering key beyond String,
// Word, and Atom.
func compareStructural(a, b Value) (int, error) {
	if IsConcrete(a) && IsConcrete(b) {
		if a.Parent.Matches(TList) {
			return compareListElems(a, b)
		}
		if a.Parent.Matches(TMap) {
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
