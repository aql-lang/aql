package engine

// registerPopArgs registers the internal "__pop-args" word used to clean up
// the args stack after a fn-defined function body finishes executing.
func registerPopArgs(r *Registry) {
	r.Register("__pop-args", Signature{
		Handler: func(_ []Value) ([]Value, error) {
			if len(r.argsStack) > 0 {
				r.argsStack = r.argsStack[:len(r.argsStack)-1]
			}
			return nil, nil
		},
	})
}
