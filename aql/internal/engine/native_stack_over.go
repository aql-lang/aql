package engine

func registerOver(r *Registry) {
	r.RegisterStackOnly("over", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{args[0], args[1], args[0]}, nil
		},
	})
}
