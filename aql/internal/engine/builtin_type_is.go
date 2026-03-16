package engine

func registerIs(r *Registry) {
	// is: [any, any] -> [boolean]
	// Returns true if a unifies with b and the result equals a.
	// This is a type/value check: "42 is Number" → true, "42 is String" → false.
	r.Register("is", Signature{
		Args:       []Type{TAny, TAny},
		Precedence: 1,
		Handler: func(args []Value) ([]Value, error) {
			a, b := args[0], args[1]
			unified, ok := Unify(a, b)
			if !ok {
				return []Value{NewBoolean(false)}, nil
			}
			// Check that the unified result equals a.
			if !unified.VType.Equal(a.VType) {
				return []Value{NewBoolean(false)}, nil
			}
			if !valuesEqual(unified, a) {
				return []Value{NewBoolean(false)}, nil
			}
			return []Value{NewBoolean(true)}, nil
		},
	})
}
