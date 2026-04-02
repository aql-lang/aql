package engine

import "fmt"

func registerRoll(r *Registry) {
	r.RegisterStackOnly("roll", Signature{
		Args:      []Type{TInteger},
		FullStack: true,
		Handler: func(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
			n := int(args[0].AsInteger())
			if n < 0 || n >= len(stack) {
				return nil, fmt.Errorf("roll: index %d out of range (stack depth %d)", n, len(stack))
			}
			idx := len(stack) - 1 - n
			result := make([]Value, 0, len(stack))
			result = append(result, stack[:idx]...)
			result = append(result, stack[idx+1:]...)
			result = append(result, stack[idx])
			return result, nil
		},
	})
}
