package engine

import "math"

func registerMathConstants(r *Registry) {
	// Math constants (zero-arg, prefix-only).
	// Named with "math-" prefix to avoid colliding with user-defined words.
	r.RegisterPrefixOnly("math-pi", Signature{
		Args: []Type{},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewDecimal(math.Pi)}, nil
		},
	})
	r.RegisterPrefixOnly("math-e", Signature{
		Args: []Type{},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewDecimal(math.E)}, nil
		},
	})
}
