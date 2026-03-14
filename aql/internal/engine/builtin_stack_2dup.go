package engine

func register2dup(r *Registry) {
	r.RegisterPrefixOnly("2dup", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[0], args[1], args[0], args[1]}, nil
		},
	})
}
