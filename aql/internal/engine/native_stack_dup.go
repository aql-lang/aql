package engine

func registerDup(r *Registry) {
	r.RegisterStackOnly("dup", Signature{
		Args: []Type{TAny},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{args[0], args[0]}, nil
		},
	})
}
