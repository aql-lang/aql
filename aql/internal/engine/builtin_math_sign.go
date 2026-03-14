package engine

func registerSign(r *Registry) {
	// sign: [int] -> [int] returns -1, 0, or 1
	r.Register("sign", Signature{
		Args: []Type{TInteger},
		Handler: func(args []Value) ([]Value, error) {
			v := args[0].AsInteger()
			switch {
			case v < 0:
				return []Value{NewInteger(-1)}, nil
			case v > 0:
				return []Value{NewInteger(1)}, nil
			default:
				return []Value{NewInteger(0)}, nil
			}
		},
	})
	// sign: [decimal] -> [int]
	r.Register("sign", Signature{
		Args: []Type{TDecimal},
		Handler: func(args []Value) ([]Value, error) {
			v := args[0].AsDecimal()
			switch {
			case v < 0:
				return []Value{NewInteger(-1)}, nil
			case v > 0:
				return []Value{NewInteger(1)}, nil
			default:
				return []Value{NewInteger(0)}, nil
			}
		},
	})
}
