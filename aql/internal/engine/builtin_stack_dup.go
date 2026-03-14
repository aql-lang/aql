package engine

func registerDup(r *Registry) {
	r.RegisterPrefixOnly("dup", Signature{
		Args: []Type{TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[0], args[0]}, nil
		},
	})
}
