package engine

import "math"

func registerCeil(r *Registry) {
	r.Register("ceil", Signature{
		Args: []Type{TDecimal},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewInteger(int64(math.Ceil(args[0].AsDecimal())))}, nil
		},
	})
}
