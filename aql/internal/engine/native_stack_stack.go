package engine

import "fmt"

func registerStackCollect(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "stack",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args:      []Type{TInteger},
			FullStack: true,
			Handler: func(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
				_as0, _ := args[0].AsInteger()
				n := int(_as0)
				if n < 0 || n > len(stack) {
					return nil, fmt.Errorf("stack: count %d out of range (stack depth %d)", n, len(stack))
				}
				items := make([]Value, n)
				copy(items, stack[len(stack)-n:])
				return append(stack, NewList(items)), nil
			},
			// Wraps top N stack values into a list.
			Returns: []Type{TList},
		}},
	})
}
