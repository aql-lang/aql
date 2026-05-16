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
// two operand VTypes — e.g. Integer-vs-Decimal walks up to Number, which
// owns the numeric ordering; Date-vs-Date stays on Date.
//
// Types without a Comparer in their lattice surface a clear
// "type X does not support compare" error. Two values rooted in
// disjoint branches (Integer-vs-String) also error: the lattice walk
// finds no shared ancestor below Any/Scalar that owns a Comparer.
//
// DepScalar values represent type-level constraints, not concrete
// scalars — they are rejected up front to avoid silently coercing
// zero values through AsNumber/AsString.
func CompareValues(a, b Value) (int, error) {
	if a.IsDepScalar() || b.IsDepScalar() {
		return 0, fmt.Errorf("cannot compare dependent-type constraint with %s", b.VType.String())
	}
	if a.VType == nil || b.VType == nil {
		return 0, fmt.Errorf("cannot compare values with nil type")
	}
	lca := lowestCommonAncestor(a.VType, b.VType)
	if lca == nil {
		return 0, fmt.Errorf("cannot compare %s and %s", a.VType.String(), b.VType.String())
	}
	for t := lca; t != nil; t = t.Parent {
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
	return 0, fmt.Errorf("cannot compare %s and %s", a.VType.String(), b.VType.String())
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

// ComparisonNatives is the consolidated set of comparison words (lt,
// gt, lte, gte, eq, neq, deq) plus the closed-interval DepScalar
// constructor `between`. Installed by the engine's top-level Register
// via a slice walk.
var ComparisonNatives = []NativeFunc{
	// lt: [any, any] -> [boolean] — less than
	// Swap: `a b lt` means a < b, so compare args[1] < args[0].
	// Also accepts `Integer lt N` to construct a DepInteger constraint.
	{
		Name:        "lt",
		ForwardArgs: true,
		Signatures: []NativeSig{
			makeDepScalarSig("lt", DepLT),
			{
				Args:    []*Type{TAny, TAny},
				Handler: ltHandler,
				Returns: []*Type{TBoolean},
			},
		},
	},

	// gt: [any, any] -> [boolean] — greater than
	{
		Name:        "gt",
		ForwardArgs: true,
		Signatures: []NativeSig{
			makeDepScalarSig("gt", DepGT),
			{
				Args:    []*Type{TAny, TAny},
				Handler: gtHandler,
				Returns: []*Type{TBoolean},
			},
		},
	},

	// lte: [any, any] -> [boolean] — less than or equal
	{
		Name:        "lte",
		ForwardArgs: true,
		Signatures: []NativeSig{
			makeDepScalarSig("lte", DepLTE),
			{
				Args:    []*Type{TAny, TAny},
				Handler: lteHandler,
				Returns: []*Type{TBoolean},
			},
		},
	},

	// gte: [any, any] -> [boolean] — greater than or equal
	{
		Name:        "gte",
		ForwardArgs: true,
		Signatures: []NativeSig{
			makeDepScalarSig("gte", DepGTE),
			{
				Args:    []*Type{TAny, TAny},
				Handler: gteHandler,
				Returns: []*Type{TBoolean},
			},
		},
	},

	// between: closed-interval DepScalar constructor — defined in
	// depscalar.go alongside the dependent-type machinery so adding
	// new bound shapes only touches one file.
	betweenNative,

	// eq: [any, any] -> [boolean] — exact equality (identity for non-scalars)
	{
		Name:        "eq",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: eqHandler,
			Returns: []*Type{TBoolean},
		}},
	},

	// neq: [any, any] -> [boolean] — not equal (negation of eq)
	{
		Name:        "neq",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: neqHandler,
			Returns: []*Type{TBoolean},
		}},
	},

	// deq: [any, any] -> [boolean] — deep equality (traverse non-scalars)
	{
		Name:        "deq",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: deqHandler,
			Returns: []*Type{TBoolean},
		}},
	},
}

func ltHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := CompareValues(args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("lt: %w", err)
	}
	return []Value{NewBoolean(cmp < 0)}, nil
}

func gtHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := CompareValues(args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("gt: %w", err)
	}
	return []Value{NewBoolean(cmp > 0)}, nil
}

func lteHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := CompareValues(args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("lte: %w", err)
	}
	return []Value{NewBoolean(cmp <= 0)}, nil
}

func gteHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := CompareValues(args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("gte: %w", err)
	}
	return []Value{NewBoolean(cmp >= 0)}, nil
}

func eqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(ExactEqual(args[0], args[1]))}, nil
}

func neqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(!ExactEqual(args[0], args[1]))}, nil
}

func deqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(DeepEqual(args[0], args[1]))}, nil
}
