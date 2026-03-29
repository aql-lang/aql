package engine

func register2over(r *Registry) {
	r.RegisterStackOnly("2over", Signature{
		Args: []Type{TAny, TAny, TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[0], args[1], args[2], args[3], args[0], args[1]}, nil
		},
	})
}
