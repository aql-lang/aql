package engine

import (
	"testing"
)

func TestFlexibleMatchPositional(t *testing.T) {
	// Positional match should always be preferred.
	vals := []Value{NewAtom("x"), NewList(nil)}
	ordered, ok := flexibleMatch(vals, []Type{TAtom, TList})
	if !ok {
		t.Fatal("expected positional match")
	}
	if ordered[0].AsAtom() != "x" {
		t.Errorf("expected atom x at [0], got %v", ordered[0])
	}
}

func TestFlexibleMatch2ArgSwap(t *testing.T) {
	// Values in reverse order should be reordered by permMatch.
	vals := []Value{NewList(nil), NewAtom("x")}
	ordered, ok := flexibleMatch(vals, []Type{TAtom, TList})
	if !ok {
		t.Fatal("expected permutation match")
	}
	if !ordered[0].VType.Equal(TAtom) {
		t.Errorf("expected atom at [0], got %s", ordered[0].VType)
	}
	if !ordered[1].VType.Equal(TList) {
		t.Errorf("expected list at [1], got %s", ordered[1].VType)
	}
}

func TestFlexibleMatch3Args(t *testing.T) {
	// 3-arg permutation: [list, integer, atom] should match [atom, list, integer].
	vals := []Value{NewList(nil), NewInteger(42), NewAtom("z")}
	types := []Type{TAtom, TList, TInteger}
	ordered, ok := flexibleMatch(vals, types)
	if !ok {
		t.Fatal("expected 3-arg permutation match")
	}
	if !ordered[0].VType.Equal(TAtom) {
		t.Errorf("[0] expected atom, got %s", ordered[0].VType)
	}
	if !ordered[1].VType.Equal(TList) {
		t.Errorf("[1] expected list, got %s", ordered[1].VType)
	}
	if !ordered[2].VType.Matches(TInteger) {
		t.Errorf("[2] expected integer, got %s", ordered[2].VType)
	}
}

func TestFlexibleMatchNoMatch(t *testing.T) {
	// No valid permutation exists.
	vals := []Value{NewAtom("a"), NewAtom("b")}
	_, ok := flexibleMatch(vals, []Type{TAtom, TList})
	if ok {
		t.Fatal("expected no match for incompatible types")
	}
}

func TestFlexibleMatchPrefersLeastDisplacement(t *testing.T) {
	// When multiple permutations match, prefer fewest displacements.
	// [atom, atom, list] with types [atom, atom, list] — positional wins (0 displacements).
	vals := []Value{NewAtom("a"), NewAtom("b"), NewList(nil)}
	types := []Type{TAtom, TAtom, TList}
	ordered, ok := flexibleMatch(vals, types)
	if !ok {
		t.Fatal("expected match")
	}
	// Positional match: atoms should stay in original order.
	if ordered[0].AsAtom() != "a" {
		t.Errorf("[0] expected atom a, got %s", ordered[0].AsAtom())
	}
	if ordered[1].AsAtom() != "b" {
		t.Errorf("[1] expected atom b, got %s", ordered[1].AsAtom())
	}
}

func TestPermMatch4Args(t *testing.T) {
	// 4-arg permutation test.
	vals := []Value{NewInteger(1), NewAtom("x"), NewList(nil), NewBoolean(true)}
	types := []Type{TBoolean, TInteger, TAtom, TList}
	ordered, ok := permMatch(vals, types)
	if !ok {
		t.Fatal("expected 4-arg permutation match")
	}
	if !ordered[0].VType.Matches(TBoolean) {
		t.Errorf("[0] expected boolean, got %s", ordered[0].VType)
	}
	if !ordered[1].VType.Matches(TInteger) {
		t.Errorf("[1] expected integer, got %s", ordered[1].VType)
	}
	if !ordered[2].VType.Equal(TAtom) {
		t.Errorf("[2] expected atom, got %s", ordered[2].VType)
	}
	if !ordered[3].VType.Equal(TList) {
		t.Errorf("[3] expected list, got %s", ordered[3].VType)
	}
}
