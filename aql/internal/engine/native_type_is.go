package engine

func registerIs(r *Registry) {
	// is: [any, any] -> [boolean]
	// Returns true if a unifies with b and the result equals a.
	// This is a type/value check: "42 is Number" → true, "42 is String" → false.
	// args[0] = nearest (top/forward), args[1] = farther. `a is Type` → args=[Type,a].
	r.RegisterNativeFunc(NativeFunc{
		Name:              "is",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TAny, TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, b := args[1], args[0]
				// Metatype early-return: when pattern (b) is a metatype and
				// value (a) is a type literal, directly check metatype matching.
				// (a=value, b=pattern due to the swap above.)
				if b.Data == nil && IsMetaType(b.VType) && a.Data == nil {
					aMeta := MetatypeFor(a.VType)
					return []Value{NewBoolean(aMeta.Matches(b.VType))}, nil
				}
				unified, ok := Unify(a, b)
				if !ok {
					return []Value{NewBoolean(false)}, nil
				}
				// Compare against the resolved form of a so that words
				// (true/false, atoms) inside lists are treated as their
				// semantic values.
				resolved := resolveWordsDeep(a)
				if !unified.VType.Equal(resolved.VType) {
					return []Value{NewBoolean(false)}, nil
				}
				if !valuesEqual(unified, resolved) {
					return []Value{NewBoolean(false)}, nil
				}
				return []Value{NewBoolean(true)}, nil
			},
		}},
	})
}
