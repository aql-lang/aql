package engine

import "math"

func registerAtan2(r *Registry) {
	registerBinaryNumOp(r, "atan2", func(a, b float64) (float64, error) {
		return math.Atan2(a, b), nil
	})
	// Also register [int, int] overload for atan2 so it works without explicit decimal args.
	r.Register("atan2", Signature{
		Args:       []Type{TInteger, TInteger},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewDecimal(math.Atan2(float64(args[0].AsInteger()), float64(args[1].AsInteger())))}, nil
		},
	})
}
