package engine

import "math"

func registerHypot(r *Registry) {
	registerBinaryNumOp(r, "hypot", func(a, b float64) (float64, error) {
		return math.Hypot(a, b), nil
	})
	// Also register [int, int] overload for hypot so it works without explicit decimal args.
	r.Register("hypot", Signature{
		Args: []Type{TInteger, TInteger},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewDecimal(math.Hypot(float64(args[0].AsInteger()), float64(args[1].AsInteger())))}, nil
		},
	})
}
