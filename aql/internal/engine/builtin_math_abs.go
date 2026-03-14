package engine

import "math"

func registerAbs(r *Registry) {
	// abs: [int] -> [int] (suffix precedence)
	r.Register("abs", Signature{
		Args: []Type{TInteger},
		Handler: func(args []Value) ([]Value, error) {
			v := args[0].AsInteger()
			if v < 0 {
				v = -v
			}
			return []Value{NewInteger(v)}, nil
		},
	})

	// abs: [decimal] -> [decimal]
	r.Register("abs", Signature{
		Args: []Type{TDecimal},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewDecimal(math.Abs(args[0].AsDecimal()))}, nil
		},
	})
}
