package engine

func registerRot(r *Registry) {
	r.RegisterPrefixOnly("rot", Signature{
		Args: []Type{TAny, TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[1], args[2], args[0]}, nil
		},
	})
}
