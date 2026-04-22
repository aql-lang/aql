package engine

import "fmt"

// RegisterArgs registers the "args" word which returns the current function's
// argument list from the args stack. Used with dot notation: args.0, args.1.
func RegisterArgs(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "args",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				if len(r.ArgsStack) == 0 {
					return nil, fmt.Errorf("args: not inside a function")
				}
				return []Value{r.ArgsStack[len(r.ArgsStack)-1]}, nil
			},
			Returns: []Type{TList},
		}},
	})
}
