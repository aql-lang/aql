package engine

import "math"

func registerMathConstants(r *Registry) {
	// Math constants (zero-arg, stack-only).
	// Named with "math-" prefix to avoid colliding with user-defined words.
	r.RegisterStackOnly("math-pi", Signature{
		Args: []Type{},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewDecimal(math.Pi)}, nil
		},
	})
	r.RegisterStackOnly("math-e", Signature{
		Args: []Type{},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewDecimal(math.E)}, nil
		},
	})
}
