package engine

import "fmt"

// registerArgs registers the "args" word which returns the current function's
// argument list from the args stack. Used with dot notation: args.0, args.1.
func registerArgs(r *Registry) {
	r.Register("args", Signature{
		Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			if len(r.argsStack) == 0 {
				return nil, fmt.Errorf("args: not inside a function")
			}
			return []Value{r.argsStack[len(r.argsStack)-1]}, nil
		},
	})
}
