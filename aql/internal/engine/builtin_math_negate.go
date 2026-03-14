package engine

func registerNegate(r *Registry) {
	// negate: [int] -> [int] (suffix precedence)
	r.Register("negate", Signature{
		Args: []Type{TInteger},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewInteger(-args[0].AsInteger())}, nil
		},
	})

	// negate: [decimal] -> [decimal]
	r.Register("negate", Signature{
		Args: []Type{TDecimal},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewDecimal(-args[0].AsDecimal())}, nil
		},
	})
}
