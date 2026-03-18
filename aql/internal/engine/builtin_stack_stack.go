package engine

import "fmt"

func registerStackCollect(r *Registry) {
	r.RegisterPrefixOnly("stack", Signature{
		Args: []Type{TInteger},
		FullStackHandler: func(args []Value, stack []Value) ([]Value, error) {
			n := int(args[0].AsInteger())
			if n < 0 || n > len(stack) {
				return nil, fmt.Errorf("stack: count %d out of range (stack depth %d)", n, len(stack))
			}
			startIdx := len(stack) - n
			items := make([]Value, n)
			copy(items, stack[startIdx:])
			result := make([]Value, startIdx+1)
			copy(result, stack[:startIdx])
			result[startIdx] = NewList(items)
			return result, nil
		},
	})
}
