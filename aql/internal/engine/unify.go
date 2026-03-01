package engine

import "fmt"

// Unify attempts to unify two values. If the values can be unified (their types
// are compatible and can narrow), it returns the unified value and true.
// Otherwise it returns an error description and false.
//
// Unification rules for scalar types:
//   - "none" only unifies with "none", nothing else (not even "any")
//   - Equal types with equal data: return either value, true
//   - One type is a subtype of the other: return the narrower (more specific) value, true
//   - One type is "any": return the other (more specific) value, true
//   - Same leaf type but different literal values: fail (each literal is its own narrow type)
//   - Incompatible type hierarchies: fail
//
// Unification rules for complex types:
//   - Lists: unify element-by-element in order; lengths must match
//   - Maps: closed unification; key sets must be identical; each value pair must unify
func Unify(a, b Value) (Value, bool) {
	aType := a.VType
	bType := b.VType

	// "none" only unifies with "none".
	aNone := aType.Equal(TNone)
	bNone := bType.Equal(TNone)
	if aNone || bNone {
		if aNone && bNone {
			return a, true
		}
		return Value{}, false
	}

	// If either is "any", unify to the other (more specific) value.
	if aType.Equal(TAny) {
		return b, true
	}
	if bType.Equal(TAny) {
		return a, true
	}

	// List unification.
	aList := aType.Equal(TList)
	bList := bType.Equal(TList)
	if aList || bList {
		return unifyLists(a, aList, b, bList)
	}

	// Map unification.
	aMap := aType.Equal(TMap)
	bMap := bType.Equal(TMap)
	if aMap || bMap {
		return unifyMaps(a, aMap, b, bMap)
	}

	// If both types are exactly equal, compare literal values.
	if aType.Equal(bType) {
		if valuesEqual(a, b) {
			return a, true
		}
		// Same type, different literal values — cannot unify.
		return Value{}, false
	}

	// Check subtype relationships.
	// If a is a subtype of b, a is narrower → return a.
	if aType.IsSubtypeOf(bType) {
		return a, true
	}
	// If b is a subtype of a, b is narrower → return b.
	if bType.IsSubtypeOf(aType) {
		return b, true
	}

	// No compatible type relationship.
	return Value{}, false
}

// unifyLists handles unification when at least one side is a list.
func unifyLists(a Value, aIsList bool, b Value, bIsList bool) (Value, bool) {
	// Type literal "list" (Data==nil) unifies with any concrete list.
	if aIsList && a.Data == nil {
		if bIsList {
			return b, true
		}
		return Value{}, false
	}
	if bIsList && b.Data == nil {
		if aIsList {
			return a, true
		}
		return Value{}, false
	}

	// Both must be concrete lists.
	if !aIsList || !bIsList {
		return Value{}, false
	}

	aElems := a.AsList()
	bElems := b.AsList()

	// Lengths must match.
	if len(aElems) != len(bElems) {
		return Value{}, false
	}

	// Unify element by element.
	result := make([]Value, len(aElems))
	for i := range aElems {
		unified, ok := Unify(aElems[i], bElems[i])
		if !ok {
			return Value{}, false
		}
		result[i] = unified
	}

	return NewList(result), true
}

// unifyMaps handles unification when at least one side is a map.
// Maps are closed: both must have exactly the same set of keys.
func unifyMaps(a Value, aIsMap bool, b Value, bIsMap bool) (Value, bool) {
	// Type literal "map" (Data==nil) unifies with any concrete map.
	if aIsMap && a.Data == nil {
		if bIsMap {
			return b, true
		}
		return Value{}, false
	}
	if bIsMap && b.Data == nil {
		if aIsMap {
			return a, true
		}
		return Value{}, false
	}

	// Both must be concrete maps.
	if !aIsMap || !bIsMap {
		return Value{}, false
	}

	aMap := a.AsMap()
	bMap := b.AsMap()

	// Key sets must be identical (closed maps).
	if aMap.Len() != bMap.Len() {
		return Value{}, false
	}

	result := NewOrderedMap()

	// Walk keys of a in order, check each exists in b.
	for _, key := range aMap.Keys() {
		aVal, _ := aMap.Get(key)
		bVal, ok := bMap.Get(key)
		if !ok {
			// Key in a but not in b — fail.
			return Value{}, false
		}

		unified, uOk := Unify(aVal, bVal)
		if !uOk {
			return Value{}, false
		}
		result.Set(key, unified)
	}

	return NewMap(result), true
}

// valuesEqual compares the data payloads of two values with the same type.
func valuesEqual(a, b Value) bool {
	// Type literals (Data == nil) with equal types are always equal.
	if a.Data == nil && b.Data == nil {
		return true
	}
	// One is a type literal and the other is a concrete value — not equal.
	if a.Data == nil || b.Data == nil {
		return false
	}
	switch {
	case a.VType.Matches(TString):
		return a.AsString() == b.AsString()
	case a.VType.Matches(TInteger):
		return a.AsInteger() == b.AsInteger()
	case a.VType.Matches(TBoolean):
		return a.AsBoolean() == b.AsBoolean()
	case a.VType.Equal(TList):
		return listsEqual(a.AsList(), b.AsList())
	case a.VType.Equal(TMap):
		return mapsEqual(a.AsMap(), b.AsMap())
	default:
		return fmt.Sprintf("%v", a.Data) == fmt.Sprintf("%v", b.Data)
	}
}

// listsEqual compares two list payloads element by element.
func listsEqual(a, b []Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].VType.Equal(b[i].VType) || !valuesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// mapsEqual compares two map payloads by keys and values.
func mapsEqual(a, b *OrderedMap) bool {
	if a.Len() != b.Len() {
		return false
	}
	for _, k := range a.Keys() {
		aVal, _ := a.Get(k)
		bVal, ok := b.Get(k)
		if !ok {
			return false
		}
		if !aVal.VType.Equal(bVal.VType) || !valuesEqual(aVal, bVal) {
			return false
		}
	}
	return true
}

// registerUnify registers the "unify" word in the given registry.
func registerUnify(r *Registry) {
	unifyHandler := func(args []Value) ([]Value, error) {
		unified, ok := Unify(args[0], args[1])
		if ok {
			return []Value{unified, NewBoolean(true)}, nil
		}
		return []Value{NewString("~unify-fail"), NewBoolean(false)}, nil
	}

	// unify: [any, any] -> [any, boolean]    (prefix)
	//        [any | any] -> [any, boolean]    (infix)
	//        [| any, any] -> [any, boolean]   (suffix)
	r.Register("unify",
		Signature{
			Prefix:  []Type{TAny, TAny},
			Handler: unifyHandler,
		},
		Signature{
			Prefix:  []Type{TAny},
			Suffix:  []Type{TAny},
			Handler: unifyHandler,
		},
		Signature{
			Suffix:  []Type{TAny, TAny},
			Handler: unifyHandler,
		},
	)
}
