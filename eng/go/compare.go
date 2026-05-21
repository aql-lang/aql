package eng

import (
	"errors"
	"fmt"
)

// ErrNoComparer is returned by Comparer.Compare implementations that
// hold a placeholder slot (e.g. a wrapped Behavior whose user-defined
// comparator body is empty). CompareValues recognises it and continues
// the parent-chain walk, treating the Behavior as if it didn't satisfy
// the Comparer interface at all. This lets a single Behavior wrapper
// carry multiple optional capabilities (compare / canon / …) where
// only some are installed without prematurely terminating dispatch.
var ErrNoComparer = errors.New("eng: no comparer in this Behavior")

// CompareValues returns -1, 0, or 1 for natural ordering of two values.
//
// Dispatch is type-driven: the compare logic for a pair of values lives
// on the type's Behavior (Comparer capability), not in a switch ladder
// here. The dispatch routes through the lowest common ancestor of the
// two operand VTypes — e.g. Integer-vs-Decimal walks up to Number,
// which owns the numeric ordering; Integer-vs-String walks up to
// Scalar, which owns the cross-branch ordering; Date-vs-Date stays
// on Date.
//
// When the lattice walk finds no Comparer, the order is still a strict
// total order. Cross-branch pairs order by the top-level branch
// precedence
//
//	Never < Any < None < Word < Type < Scalar < Node < Ideal
//
// A same-branch pair falls back to size (smaller SizeOf first, so a
// shorter List leads a longer one); equal sizes then break to the
// type order (compareTypes — family rank, depth, name, id), and two
// values of the identical type break to an element-wise structural
// comparison. Distinct values never collapse to 0.
//
// DepScalar values (type-level constraints) flow through this same
// path.
func CompareValues(a, b Value) (int, error) {
	if a.VType == nil || b.VType == nil {
		return 0, fmt.Errorf("cannot compare values with nil type")
	}
	for t := lowestCommonAncestor(a.VType, b.VType); t != nil; t = t.Parent {
		cmp, ok := t.Behavior.(Comparer)
		if !ok {
			continue
		}
		n, err := cmp.Compare(a, b)
		if errors.Is(err, ErrNoComparer) {
			// Wrapper Behavior signalled "I satisfy Comparer
			// structurally but have no body installed" — keep
			// walking the parent chain.
			continue
		}
		return n, err
	}
	// No Comparer in the shared lattice. Cross-branch pairs order by
	// the top-level branch precedence.
	if ra, rb := rootBranchRank(a), rootBranchRank(b); ra != rb {
		if ra < rb {
			return -1, nil
		}
		return 1, nil
	}
	// Same branch, no Comparer — order by size (smaller SizeOf first,
	// so a shorter List leads a longer one). Equal sizes break to the
	// type order, then — for two values of the very same type — to an
	// element-wise structural comparison, so distinct values never
	// collapse to 0.
	if sa, sb := SizeOf(a), SizeOf(b); sa != sb {
		if sa < sb {
			return -1, nil
		}
		return 1, nil
	}
	if c := compareTypes(a.VType, b.VType); c != 0 {
		return c, nil
	}
	return compareStructural(a, b)
}

// rootBranchRank returns v's top-level branch precedence:
//
//	Never < Any < None < Word < Type < Scalar < Node < Ideal
//
// The values are spaced by 1_000_000, matching Type.Rank.
func rootBranchRank(v Value) int {
	root := v.VType
	for root.Parent != nil {
		root = root.Parent
	}
	switch root {
	case TNever:
		return 0
	case TAny:
		return 1_000_000
	case TNone:
		return 2_000_000
	case TWord:
		return 3_000_000
	case TType:
		return 4_000_000
	case TScalar:
		return 5_000_000
	case TNode:
		return 6_000_000
	case TIdeal:
		return 7_000_000
	default:
		return 8_000_000
	}
}

// lowestCommonAncestor returns the closest type that is an ancestor
// of both a and b on the parent chain. Returns nil only if a and b
// share no common ancestor (the type tables guarantee a single root,
// so in practice this returns at worst the root type).
func lowestCommonAncestor(a, b *Type) *Type {
	seen := make(map[*Type]bool)
	for t := a; t != nil; t = t.Parent {
		seen[t] = true
	}
	for t := b; t != nil; t = t.Parent {
		if seen[t] {
			return t
		}
	}
	return nil
}

// ExactEqual returns true if two values are exactly equal.
// For scalars (integer, string, boolean, atom, none): compares by value.
// For types: compares structurally via ValuesEqual.
// For non-scalars (list, map): compares by identity (same pointer).
func ExactEqual(a, b Value) bool {
	// none == none
	if a.VType.Equal(TNone) && b.VType.Equal(TNone) {
		return true
	}

	// DepScalar pre-empts the Matches(TNumber)/Matches(TString)/...
	// dispatch below: the lattice override would otherwise route
	// DepInteger payloads into AsNumber and silently compare zero
	// values. Two DepScalars are equal iff their constraint shapes
	// match (delegated through ValuesEqual).
	if a.IsDepScalar() || b.IsDepScalar() {
		if !a.IsDepScalar() || !b.IsDepScalar() {
			return false
		}
		return a.VType.Equal(b.VType) && ValuesEqual(a, b)
	}

	// Types: structural comparison.
	if IsTypeBody(a) && IsTypeBody(b) {
		return a.VType.Equal(b.VType) && ValuesEqual(a, b)
	}

	// Scalars: compare by value.
	if a.VType.Matches(TNumber) && b.VType.Matches(TNumber) {
		_as9, _ := AsNumber(a)
		_as8, _ := AsNumber(b)
		return _as9 == _as8
	}
	if a.VType.Matches(TString) && b.VType.Matches(TString) {
		_as11, _ := AsString(a)
		_as10, _ := AsString(b)
		return _as11 == _as10
	}
	if a.VType.Matches(TBoolean) && b.VType.Matches(TBoolean) {
		_as13, _ := AsBoolean(a)
		_as12, _ := AsBoolean(b)
		return _as13 == _as12
	}
	if a.VType.Equal(TAtom) && b.VType.Equal(TAtom) {
		_as15, _ := AsAtom(a)
		_as14, _ := AsAtom(b)
		return _as15 == _as14
	}

	// Non-scalars: identity comparison (same pointer).
	if a.VType.Equal(TList) && b.VType.Equal(TList) {
		return a.Data == b.Data
	}
	if a.VType.Equal(TMap) && b.VType.Equal(TMap) {
		return a.Data == b.Data
	}

	return false
}

// DeepEqual returns true if two values are deeply equal.
// Traverses lists and maps depth-first comparing all leaf values.
func DeepEqual(a, b Value) bool {
	// none
	if a.VType.Equal(TNone) && b.VType.Equal(TNone) {
		return true
	}

	// DepScalar pre-empts scalar dispatch — see ExactEqual for the
	// reasoning. Two DepScalars compare equal iff their type and
	// constraint payload match.
	if a.IsDepScalar() || b.IsDepScalar() {
		if !a.IsDepScalar() || !b.IsDepScalar() {
			return false
		}
		return a.VType.Equal(b.VType) && ValuesEqual(a, b)
	}

	// Scalars.
	if a.VType.Matches(TNumber) && b.VType.Matches(TNumber) {
		_as17, _ := AsNumber(a)
		_as16, _ := AsNumber(b)
		return _as17 == _as16
	}
	if a.VType.Matches(TString) && b.VType.Matches(TString) {
		_as19, _ := AsString(a)
		_as18, _ := AsString(b)
		return _as19 == _as18
	}
	if a.VType.Matches(TBoolean) && b.VType.Matches(TBoolean) {
		_as21, _ := AsBoolean(a)
		_as20, _ := AsBoolean(b)
		return _as21 == _as20
	}
	if a.VType.Equal(TAtom) && b.VType.Equal(TAtom) {
		_as23, _ := AsAtom(a)
		_as22, _ := AsAtom(b)
		return _as23 == _as22
	}

	// Lists: same length, each element deeply equal.
	if a.VType.Equal(TList) && b.VType.Equal(TList) {
		aElems, aErr := AsMutableList(a)
		bElems, bErr := AsMutableList(b)
		if aErr != nil || bErr != nil {
			// Typed lists, table types, etc. — compare structurally via String().
			return a.String() == b.String()
		}
		if len(aElems) != len(bElems) {
			return false
		}
		for i := range aElems {
			if !DeepEqual(aElems[i], bElems[i]) {
				return false
			}
		}
		return true
	}

	// Maps: same keys, each value deeply equal.
	if a.VType.Equal(TMap) && b.VType.Equal(TMap) {
		aMap, aErr := AsMutableMap(a)
		bMap, bErr := AsMutableMap(b)
		if aErr != nil || bErr != nil {
			// Record types, typed maps — compare structurally via String().
			return a.String() == b.String()
		}
		if aMap.Len() != bMap.Len() {
			return false
		}
		for _, key := range aMap.Keys() {
			aVal, _ := aMap.Get(key)
			bVal, bHas := bMap.Get(key)
			if !bHas {
				return false
			}
			if !DeepEqual(aVal, bVal) {
				return false
			}
		}
		return true
	}

	// Different types or unsupported — not equal.
	return false
}

// The comparison-word registrations (lt / gt / lte / gte / eq /
// neq / deq / between) live in lang/go/engine/native_compare.go. The
// handlers and MakeDepScalarSig helper are exported eng primitives.

func LtHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := CompareValues(args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("lt: %w", err)
	}
	return []Value{NewBoolean(cmp < 0)}, nil
}

func GtHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := CompareValues(args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("gt: %w", err)
	}
	return []Value{NewBoolean(cmp > 0)}, nil
}

func LteHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := CompareValues(args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("lte: %w", err)
	}
	return []Value{NewBoolean(cmp <= 0)}, nil
}

func GteHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := CompareValues(args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("gte: %w", err)
	}
	return []Value{NewBoolean(cmp >= 0)}, nil
}

func EqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(ExactEqual(args[0], args[1]))}, nil
}

func NeqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(!ExactEqual(args[0], args[1]))}, nil
}

func DeqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(DeepEqual(args[0], args[1]))}, nil
}
