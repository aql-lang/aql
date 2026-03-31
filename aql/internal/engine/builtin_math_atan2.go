package engine

import "math"

func registerAtan2(r *Registry) {
	// With forward-first matching, args are reversed relative to natural order.
	// Swap operands so `a b atan2` = atan2(a,b) and `atan2 b a` = atan2(a,b).
	registerBinaryNumOp(r, "atan2", func(a, b float64) (float64, error) {
		return math.Atan2(b, a), nil
	})
	r.Register("atan2", Signature{
		Args: []Type{TInteger, TInteger},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewDecimal(math.Atan2(float64(args[1].AsInteger()), float64(args[0].AsInteger())))}, nil
		},
	})
}
