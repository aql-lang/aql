package engine

import "math"

func registerRound(r *Registry) {
	r.Register("round", Signature{
		Args: []Type{TDecimal},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewInteger(int64(math.Round(args[0].AsDecimal())))}, nil
		},
	})
}
