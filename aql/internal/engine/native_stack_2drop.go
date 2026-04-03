package engine

func register2drop(r *Registry) {
	r.RegisterStackOnly("2drop", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		},
	})
}
