package engine

import "math"

func registerTrunc(r *Registry) {
	r.Register("trunc", Signature{
		Args: []Type{TDecimal},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewInteger(int64(math.Trunc(args[0].AsDecimal())))}, nil
		},
	})
}
