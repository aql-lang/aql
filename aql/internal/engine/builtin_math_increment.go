package engine

func registerIncrement(r *Registry) {
	// increment: [int] -> [int] — adds 1 to an integer
	r.Register("increment", Signature{
		Args: []Type{TInteger},
		Handler: func(args []Value) ([]Value, error) {
			return []Value{NewInteger(args[0].AsInteger() + 1)}, nil
		},
	})
}
