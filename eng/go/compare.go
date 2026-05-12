package eng

import "fmt"

// CompareValues returns -1, 0, or 1 for natural ordering of two values.
// Comparison rules:
//   - Integers: numeric order
//   - Strings: lexicographic order
//   - Booleans: false < true
//   - Atoms: lexicographic order on atom name
//   - Cross-type: ordered by type name (atom < boolean < number < string)
//   - Lists, maps, and other types: not orderable, returns error
func CompareValues(a, b Value) (int, error) {
	// DepScalar values represent type-level constraints, not concrete
	// scalars. Ordering them as if they were scalar values is a
	// category error — refuse rather than silently coerce zero
	// values through the Matches(TNumber)/AsNumber() path.
	if a.IsDepScalar() || b.IsDepScalar() {
		return 0, fmt.Errorf("cannot compare dependent-type constraint with %s", b.VType.String())
	}
	// Numeric comparisons: both operands are some form of Number.
	if a.VType.Matches(TNumber) && b.VType.Matches(TNumber) {
		_as1, _ := a.AsNumber()
		_as0, _ := b.AsNumber()
		af, bf := _as1, _as0
		if af < bf {
			return -1, nil
		}
		if af > bf {
			return 1, nil
		}
		return 0, nil
	}

	if a.VType.Matches(TString) && b.VType.Matches(TString) {
		_as3, _ := a.AsString()
		_as2, _ := b.AsString()
		as, bs := _as3, _as2
		if as < bs {
			return -1, nil
		}
		if as > bs {
			return 1, nil
		}
		return 0, nil
	}

	if a.VType.Matches(TBoolean) && b.VType.Matches(TBoolean) {
		_as5, _ := a.AsBoolean()
		_as4, _ := b.AsBoolean()
		ab, bb := _as5, _as4
		if ab == bb {
			return 0, nil
		}
		if !ab {
			return -1, nil // false < true
		}
		return 1, nil
	}

	if a.VType.Equal(TAtom) && b.VType.Equal(TAtom) {
		_as7, _ := a.AsAtom()
		_as6, _ := b.AsAtom()
		as, bs := _as7, _as6
		if as < bs {
			return -1, nil
		}
		if as > bs {
			return 1, nil
		}
		return 0, nil
	}

	return 0, fmt.Errorf("cannot compare %s and %s", a.VType.String(), b.VType.String())
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
		_as9, _ := a.AsNumber()
		_as8, _ := b.AsNumber()
		return _as9 == _as8
	}
	if a.VType.Matches(TString) && b.VType.Matches(TString) {
		_as11, _ := a.AsString()
		_as10, _ := b.AsString()
		return _as11 == _as10
	}
	if a.VType.Matches(TBoolean) && b.VType.Matches(TBoolean) {
		_as13, _ := a.AsBoolean()
		_as12, _ := b.AsBoolean()
		return _as13 == _as12
	}
	if a.VType.Equal(TAtom) && b.VType.Equal(TAtom) {
		_as15, _ := a.AsAtom()
		_as14, _ := b.AsAtom()
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
		_as17, _ := a.AsNumber()
		_as16, _ := b.AsNumber()
		return _as17 == _as16
	}
	if a.VType.Matches(TString) && b.VType.Matches(TString) {
		_as19, _ := a.AsString()
		_as18, _ := b.AsString()
		return _as19 == _as18
	}
	if a.VType.Matches(TBoolean) && b.VType.Matches(TBoolean) {
		_as21, _ := a.AsBoolean()
		_as20, _ := b.AsBoolean()
		return _as21 == _as20
	}
	if a.VType.Equal(TAtom) && b.VType.Equal(TAtom) {
		_as23, _ := a.AsAtom()
		_as22, _ := b.AsAtom()
		return _as23 == _as22
	}

	// Lists: same length, each element deeply equal.
	if a.VType.Equal(TList) && b.VType.Equal(TList) {
		aElems, aOk := a.Data.([]Value)
		bElems, bOk := b.Data.([]Value)
		if !aOk || !bOk {
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
		aMap, aOk := a.Data.(*OrderedMap)
		bMap, bOk := b.Data.(*OrderedMap)
		if !aOk || !bOk {
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
