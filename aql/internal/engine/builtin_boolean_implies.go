package engine

func registerImplies(r *Registry) {
	// With forward-first matching, args are reversed: args[0]=right, args[1]=left.
	// implies: !left || right = !args[1] || args[0]
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		left := args[1].AsBoolean()
		right := args[0].AsBoolean()
		return []Value{NewBoolean(!left || right)}, nil
	}
	r.Register("implies", Signature{
		Args:    []Type{TBoolean, TBoolean},
		Handler: handler,
	})
}
