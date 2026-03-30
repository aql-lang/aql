package engine

func registerNip(r *Registry) {
	r.RegisterStackOnly("nip", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[1]}, nil
		},
	})
}
