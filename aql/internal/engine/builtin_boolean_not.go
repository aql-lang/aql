package engine

func registerNot(r *Registry) {
	r.Register("not", Signature{
		Args: []Type{TBoolean},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewBoolean(!args[0].AsBoolean())}, nil
		},
	})
}
