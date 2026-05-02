package engine

func RegisterNot(r *Registry) {
	// not coerces non-boolean args (same rules as `convert boolean`).
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewBoolean(!CoerceBoolean(args[0]))}, nil
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:              "not",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TBoolean}, Handler: handler, Returns: []Type{TBoolean}},
			{Args: []Type{TAny}, Handler: handler, Returns: []Type{TBoolean}},
		},
	})
}
