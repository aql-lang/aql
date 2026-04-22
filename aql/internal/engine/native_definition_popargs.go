package engine

// RegisterPopArgs registers the internal "__pa" word used to clean up
// the args stack after a fn-defined function body finishes executing.
func RegisterPopArgs(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "__pa",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if len(r.ArgsStack) > 0 {
					r.ArgsStack = r.ArgsStack[:len(r.ArgsStack)-1]
				}
				return nil, nil
			},
			Returns: []Type{},
		}},
	})
}
