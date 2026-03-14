package engine

func register2swap(r *Registry) {
	r.RegisterPrefixOnly("2swap", Signature{
		Args: []Type{TAny, TAny, TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[2], args[3], args[0], args[1]}, nil
		},
	})
}
