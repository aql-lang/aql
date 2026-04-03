package engine

// registerPopArgs registers the internal "__pa" word used to clean up
// the args stack after a fn-defined function body finishes executing.
func registerPopArgs(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "__pa",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if len(r.argsStack) > 0 {
					r.argsStack = r.argsStack[:len(r.argsStack)-1]
				}
				return nil, nil
			},
		}},
	})
}
