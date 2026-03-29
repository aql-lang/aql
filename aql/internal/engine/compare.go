package engine

import "fmt"

// compareValues returns -1, 0, or 1 for natural ordering of two values.
// Comparison rules:
//   - Integers: numeric order
//   - Strings: lexicographic order
//   - Booleans: false < true
//   - Atoms: lexicographic order on atom name
//   - Cross-type: ordered by type name (atom < boolean < number < string)
//   - Lists, maps, and other types: not orderable, returns error
func compareValues(a, b Value) (int, error) {
	// Numeric comparisons: both operands are some form of Number.
	if a.VType.Matches(TNumber) && b.VType.Matches(TNumber) {
		af, bf := a.AsNumber(), b.AsNumber()
		if af < bf {
			return -1, nil
		}
		if af > bf {
			return 1, nil
		}
		return 0, nil
	}

	if a.VType.Matches(TString) && b.VType.Matches(TString) {
		as, bs := a.AsString(), b.AsString()
		if as < bs {
			return -1, nil
		}
		if as > bs {
			return 1, nil
		}
		return 0, nil
	}

	if a.VType.Matches(TBoolean) && b.VType.Matches(TBoolean) {
		ab, bb := a.AsBoolean(), b.AsBoolean()
		if ab == bb {
			return 0, nil
		}
		if !ab {
			return -1, nil // false < true
		}
		return 1, nil
	}

	if a.VType.Equal(TAtom) && b.VType.Equal(TAtom) {
		as, bs := a.AsAtom(), b.AsAtom()
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

// exactEqual returns true if two values are exactly equal.
// For scalars (integer, string, boolean, atom, none): compares by value.
// For types: compares structurally via valuesEqual.
// For non-scalars (list, map): compares by identity (same pointer).
func exactEqual(a, b Value) bool {
	// none == none
	if a.VType.Equal(TNone) && b.VType.Equal(TNone) {
		return true
	}

	// Types: structural comparison.
	if isTypeValue(a) && isTypeValue(b) {
		return a.VType.Equal(b.VType) && valuesEqual(a, b)
	}

	// Scalars: compare by value.
	if a.VType.Matches(TNumber) && b.VType.Matches(TNumber) {
		return a.AsNumber() == b.AsNumber()
	}
	if a.VType.Matches(TString) && b.VType.Matches(TString) {
		return a.AsString() == b.AsString()
	}
	if a.VType.Matches(TBoolean) && b.VType.Matches(TBoolean) {
		return a.AsBoolean() == b.AsBoolean()
	}
	if a.VType.Equal(TAtom) && b.VType.Equal(TAtom) {
		return a.AsAtom() == b.AsAtom()
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

// deepEqual returns true if two values are deeply equal.
// Traverses lists and maps depth-first comparing all leaf values.
func deepEqual(a, b Value) bool {
	// none
	if a.VType.Equal(TNone) && b.VType.Equal(TNone) {
		return true
	}

	// Scalars.
	if a.VType.Matches(TNumber) && b.VType.Matches(TNumber) {
		return a.AsNumber() == b.AsNumber()
	}
	if a.VType.Matches(TString) && b.VType.Matches(TString) {
		return a.AsString() == b.AsString()
	}
	if a.VType.Matches(TBoolean) && b.VType.Matches(TBoolean) {
		return a.AsBoolean() == b.AsBoolean()
	}
	if a.VType.Equal(TAtom) && b.VType.Equal(TAtom) {
		return a.AsAtom() == b.AsAtom()
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
			if !deepEqual(aElems[i], bElems[i]) {
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
			if !deepEqual(aVal, bVal) {
				return false
			}
		}
		return true
	}

	// Different types or unsupported — not equal.
	return false
}

func registerComparison(r *Registry) {
	// lt: [any, any] -> [boolean] — less than
	r.Register("lt", Signature{
		Args:       []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			cmp, err := compareValues(args[0], args[1])
			if err != nil {
				return nil, fmt.Errorf("lt: %w", err)
			}
			return []Value{NewBoolean(cmp < 0)}, nil
		},
	})

	// gt: [any, any] -> [boolean] — greater than
	r.Register("gt", Signature{
		Args:       []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			cmp, err := compareValues(args[0], args[1])
			if err != nil {
				return nil, fmt.Errorf("gt: %w", err)
			}
			return []Value{NewBoolean(cmp > 0)}, nil
		},
	})

	// lte: [any, any] -> [boolean] — less than or equal
	r.Register("lte", Signature{
		Args:       []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			cmp, err := compareValues(args[0], args[1])
			if err != nil {
				return nil, fmt.Errorf("lte: %w", err)
			}
			return []Value{NewBoolean(cmp <= 0)}, nil
		},
	})

	// gte: [any, any] -> [boolean] — greater than or equal
	r.Register("gte", Signature{
		Args:       []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			cmp, err := compareValues(args[0], args[1])
			if err != nil {
				return nil, fmt.Errorf("gte: %w", err)
			}
			return []Value{NewBoolean(cmp >= 0)}, nil
		},
	})

	// eq: [any, any] -> [boolean] — exact equality (identity for non-scalars)
	r.Register("eq", Signature{
		Args:       []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewBoolean(exactEqual(args[0], args[1]))}, nil
		},
	})

	// neq: [any, any] -> [boolean] — not equal (negation of eq)
	r.Register("neq", Signature{
		Args:       []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewBoolean(!exactEqual(args[0], args[1]))}, nil
		},
	})

	// deq: [any, any] -> [boolean] — deep equality (traverse non-scalars)
	r.Register("deq", Signature{
		Args:       []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewBoolean(deepEqual(args[0], args[1]))}, nil
		},
	})
}
