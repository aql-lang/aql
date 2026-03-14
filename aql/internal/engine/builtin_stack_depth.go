package engine

func registerDepth(r *Registry) {
	r.RegisterPrefixOnly("depth", Signature{
		FullStackHandler: func(args []Value, stack []Value) ([]Value, error) {
			return append(stack, NewInteger(int64(len(stack)))), nil
		},
	})
}
