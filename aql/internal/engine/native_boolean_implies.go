package engine

func registerImplies(r *Registry) {
	// Signature [Boolean, Boolean]: args[0] = nearest to word (top/forward),
	// args[1] = farther (deeper/later). `a b implies` → args=[b,a] → !a||b.
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
