package engine

func registerDrop(r *Registry) {
	r.RegisterStackOnly("drop", Signature{
		Args: []Type{TAny},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		},
	})
}
