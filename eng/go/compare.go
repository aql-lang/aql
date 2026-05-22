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
// When the lattice walk finds no Comparer, the order falls to the
// unified lattice Rank: every type carries one Rank giving a total
// order over the whole lattice (see typetable.go::builtinDecls), so
// any two values of different types order by Rank alone. Two values
// of equal Rank — the identical type, or two user/external types that
// inherit one builtin's Rank — break to compareTypes (name/id), then
// to size, then to an element-wise structural comparison. Distinct
// values never collapse to 0.
//
// DepScalar values (type-level constraints) flow through this same
// path.
func CompareValues(a, b Value) (int, error) {
	if a.Parent == nil || b.Parent == nil {
		return 0, fmt.Errorf("cannot compare values with nil type")
	}
	for t := lowestCommonAncestor(a.Parent, b.Parent); t != nil; t = t.Parent {
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
	// No Comparer in the shared lattice — order by the type lattice.
	// compareTypes is a total order on *Type (Rank, then depth, name,
	// id), so any two values of distinct types resolve here.
	if c := compareTypes(a.Parent, b.Parent); c != 0 {
		return c, nil
	}
	// Identical type — order by value: size, then element-wise
	// structure, so distinct values never collapse to 0.
	if sa, sb := SizeOf(a), SizeOf(b); sa != sb {
		return cmpInt(sa, sb), nil
	}
	return compareStructural(a, b)
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
// For non-scalars (list, map): compares by identity (same container).
func ExactEqual(a, b Value) bool {
	// none == none
	if a.Parent.Equal(TNone) && b.Parent.Equal(TNone) {
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
		return a.Parent.Equal(b.Parent) && ValuesEqual(a, b)
	}

	// Types: structural comparison.
	if IsTypeBody(a) && IsTypeBody(b) {
		return a.Parent.Equal(b.Parent) && ValuesEqual(a, b)
	}

	// Scalars: compare by value.
	if a.Parent.Matches(TNumber) && b.Parent.Matches(TNumber) {
		_as9, _ := AsNumber(a)
		_as8, _ := AsNumber(b)
		return _as9 == _as8
	}
	if a.Parent.Matches(TString) && b.Parent.Matches(TString) {
		_as11, _ := AsString(a)
		_as10, _ := AsString(b)
		return _as11 == _as10
	}
	if a.Parent.Matches(TBoolean) && b.Parent.Matches(TBoolean) {
		_as13, _ := AsBoolean(a)
		_as12, _ := AsBoolean(b)
		return _as13 == _as12
	}
	if a.Parent.Equal(TAtom) && b.Parent.Equal(TAtom) {
		_as15, _ := AsAtom(a)
		_as14, _ := AsAtom(b)
		return _as15 == _as14
	}

	// Non-scalars: identity comparison — both values must refer to the
	// same underlying container.
	if a.Parent.Equal(TList) && b.Parent.Equal(TList) {
		return sameContainer(a.Data, b.Data)
	}
	if a.Parent.Equal(TMap) && b.Parent.Equal(TMap) {
		return sameContainer(a.Data, b.Data)
	}

	return false
}

// sameContainer reports whether two non-scalar payloads refer to the
// same underlying container — the identity test behind ExactEqual for
// lists and maps. A MapPayload identifies by its *OrderedMap pointer;
// a ListPayload by the backing array of its element slice, so a value
// dup'd from a list is identical to its source while two separate
// literals are not. Payloads with no aliasable identity (table data,
// materializers, …) are never equal here.
//
// It must not apply `==` to a Payload directly: ListPayload holds a
// slice and is therefore not a comparable type — a bare
// `a.Data == b.Data` panics at runtime.
func sameContainer(a, b Payload) bool {
	switch av := a.(type) {
	case MapPayload:
		bv, ok := b.(MapPayload)
		return ok && av.M == bv.M
	case ListPayload:
		bv, ok := b.(ListPayload)
		if !ok {
			return false
		}
		// An empty list has no backing array to alias by — treat all
		// empty lists as the single empty list.
		if len(av.Elems) == 0 || len(bv.Elems) == 0 {
			return len(av.Elems) == len(bv.Elems)
		}
		return &av.Elems[0] == &bv.Elems[0]
	default:
		return false
	}
}

// DeepEqual returns true if two values are deeply equal.
// Traverses lists and maps depth-first comparing all leaf values.
func DeepEqual(a, b Value) bool {
	// none
	if a.Parent.Equal(TNone) && b.Parent.Equal(TNone) {
		return true
	}

	// DepScalar pre-empts scalar dispatch — see ExactEqual for the
	// reasoning. Two DepScalars compare equal iff their type and
	// constraint payload match.
	if a.IsDepScalar() || b.IsDepScalar() {
		if !a.IsDepScalar() || !b.IsDepScalar() {
			return false
		}
		return a.Parent.Equal(b.Parent) && ValuesEqual(a, b)
	}

	// Scalars.
	if a.Parent.Matches(TNumber) && b.Parent.Matches(TNumber) {
		_as17, _ := AsNumber(a)
		_as16, _ := AsNumber(b)
		return _as17 == _as16
	}
	if a.Parent.Matches(TString) && b.Parent.Matches(TString) {
		_as19, _ := AsString(a)
		_as18, _ := AsString(b)
		return _as19 == _as18
	}
	if a.Parent.Matches(TBoolean) && b.Parent.Matches(TBoolean) {
		_as21, _ := AsBoolean(a)
		_as20, _ := AsBoolean(b)
		return _as21 == _as20
	}
	if a.Parent.Equal(TAtom) && b.Parent.Equal(TAtom) {
		_as23, _ := AsAtom(a)
		_as22, _ := AsAtom(b)
		return _as23 == _as22
	}

	// Lists: same length, each element deeply equal.
	if a.Parent.Equal(TList) && b.Parent.Equal(TList) {
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
	if a.Parent.Equal(TMap) && b.Parent.Equal(TMap) {
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

// CmpHandler implements `cmp` — a three-way comparison. `a b cmp`
// returns -1 when a sorts before b, 0 when they tie, and 1 when a
// sorts after b, using the same total order as lt / gt / sort. The
// CompareValues result is normalised to its sign, so a custom
// `behave compare` body that returns a nonzero magnitude other than
// ±1 still yields exactly -1 / 0 / 1.
func CmpHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := CompareValues(args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("cmp: %w", err)
	}
	switch {
	case cmp < 0:
		return []Value{NewInteger(-1)}, nil
	case cmp > 0:
		return []Value{NewInteger(1)}, nil
	default:
		return []Value{NewInteger(0)}, nil
	}
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
