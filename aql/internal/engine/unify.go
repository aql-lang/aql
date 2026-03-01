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

	// If both types are exactly equal, compare literal values.
	if aType.Equal(bType) {
		if valuesEqual(a, b) {
			return a, true
		}
		// Same type, different literal values — cannot unify.
		return Value{}, false
	}

	// If either is "any", unify to the other (more specific) value.
	if aType.Equal(TAny) {
		return b, true
	}
	if bType.Equal(TAny) {
		return a, true
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
	default:
		return fmt.Sprintf("%v", a.Data) == fmt.Sprintf("%v", b.Data)
	}
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
