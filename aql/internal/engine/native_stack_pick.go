package engine

import "fmt"

func RegisterPick(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "pick",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			Args:      []Type{TInteger},
			FullStack: true,
			Handler: func(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
				_as0, _ := args[0].AsConcreteInteger()
				n := int(_as0)
				if n < 0 || n >= len(stack) {
					return nil, fmt.Errorf("pick: index %d out of range (stack depth %d)", n, len(stack))
				}
				return append(stack, stack[len(stack)-1-n]), nil
			},
			// Check-mode FullStack: preserve the stack and append
			// a carrier whose type is the join of all existing
			// stack carrier types (we don't know statically which
			// index pick will hit, so widen).
			CheckFullStackFn: func(_ []Value, stack []Value) []Value {
				if len(stack) == 0 {
					return append(append([]Value(nil), stack...), NewCarrier(TAny))
				}
				t := stack[0].VType
				for i := 1; i < len(stack); i++ {
					t = CommonAncestorType(t, stack[i].VType)
					if t.Equal(TAny) {
						break
					}
				}
				return append(append([]Value(nil), stack...), NewCarrier(t))
			},
		}},
	})
}
