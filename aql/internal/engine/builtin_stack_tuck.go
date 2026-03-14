package engine

func registerTuck(r *Registry) {
	r.RegisterPrefixOnly("tuck", Signature{
		Args: []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{args[1], args[0], args[1]}, nil
		},
	})
}
