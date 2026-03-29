package engine

func registerSwap(r *Registry) {
	r.RegisterStackOnly("swap", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[1], args[0]}, nil
		},
	})
}
