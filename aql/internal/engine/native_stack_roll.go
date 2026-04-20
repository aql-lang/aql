package engine

import "fmt"

func registerRoll(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "roll",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args:      []Type{TInteger},
			FullStack: true,
			Handler: func(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
				_as0, _ := args[0].AsInteger()
				n := int(_as0)
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
			// Check-mode FullStack: roll moves one stack element to
			// the top without changing the total count. We don't
			// know statically which element; return a copy of the
			// stack whose new top is the join of existing elements.
			CheckFullStackFn: func(_ []Value, stack []Value) []Value {
				if len(stack) == 0 {
					return nil
				}
				// Result: stack minus one element + picked-top.
				// Conservative: keep all stack entries as-is and
				// mark the last as the joined carrier.
				out := append([]Value(nil), stack...)
				t := stack[0].VType
				for i := 1; i < len(stack); i++ {
					t = commonAncestorType(t, stack[i].VType)
				}
				out[len(out)-1] = NewCarrier(t)
				return out
			},
		}},
	})
}
