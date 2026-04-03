package engine

import "fmt"

func registerPick(r *Registry) {
	r.RegisterStackOnly("pick", Signature{
		Args:      []Type{TInteger},
		FullStack: true,
		Handler: func(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
			_as0, _ := args[0].AsInteger()
			n := int(_as0)
			if n < 0 || n >= len(stack) {
				return nil, fmt.Errorf("pick: index %d out of range (stack depth %d)", n, len(stack))
			}
			return append(stack, stack[len(stack)-1-n]), nil
		},
	})
}
