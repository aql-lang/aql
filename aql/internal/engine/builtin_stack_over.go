package engine

func registerOver(r *Registry) {
	r.RegisterPrefixOnly("over", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[0], args[1], args[0]}, nil
		},
	})
}
