package engine

func registerDrop(r *Registry) {
	r.RegisterStackOnly("drop", Signature{
		Args: []Type{TAny},
		Handler: func(args []Value) ([]Value, error) {
			return nil, nil
		},
	})
}
