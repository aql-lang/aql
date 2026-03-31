package engine

import "math"

func registerFloor(r *Registry) {
	r.Register("floor", Signature{
		Args: []Type{TDecimal},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewInteger(int64(math.Floor(args[0].AsDecimal())))}, nil
		},
	})
}
