package engine

func registerDepth(r *Registry) {
	r.RegisterStackOnly("depth", Signature{
		FullStack: true,
		Handler: func(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
			return append(stack, NewInteger(int64(len(stack)))), nil
		},
	})
}
