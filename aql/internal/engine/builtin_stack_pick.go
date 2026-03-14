package engine

import "fmt"

func registerPick(r *Registry) {
	r.RegisterPrefixOnly("pick", Signature{
		Args: []Type{TInteger},
		FullStackHandler: func(args []Value, stack []Value) ([]Value, error) {
			n := int(args[0].AsInteger())
			if n < 0 || n >= len(stack) {
				return nil, fmt.Errorf("pick: index %d out of range (stack depth %d)", n, len(stack))
			}
			return append(stack, stack[len(stack)-1-n]), nil
		},
	})
}
