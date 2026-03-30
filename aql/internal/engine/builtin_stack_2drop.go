package engine

func register2drop(r *Registry) {
	r.RegisterStackOnly("2drop", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return nil, nil
		},
	})
}
