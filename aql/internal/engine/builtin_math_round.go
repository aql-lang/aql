package engine

import "math"

func registerRound(r *Registry) {
	r.Register("round", Signature{
		Args: []Type{TDecimal},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewInteger(int64(math.Round(args[0].AsDecimal())))}, nil
		},
	})
}
