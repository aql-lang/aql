package native

import (
	"fmt"
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

func RegisterStackCollect(r *engine.Registry) {
	r.RegisterNativeFunc(engine.NativeFunc{
		Name:              "stack",
		ForwardPrecedence: false,
		Signatures: []engine.NativeSig{{
			Args:      []engine.Type{engine.TInteger},
			FullStack: true,
			Handler: func(args []engine.Value, _ map[string]engine.Value, stack []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				_as0, _ := args[0].AsConcreteInteger()
				n := int(_as0)
				if n < 0 || n > len(stack) {
					return nil, fmt.Errorf("stack: count %d out of range (stack depth %d)", n, len(stack))
				}
				items := make([]engine.Value, n)
				copy(items, stack[len(stack)-n:])
				return append(stack, engine.NewList(items)), nil
			},
			// Check-mode FullStack: `stack N` wraps the top N
			// entries into a list. We don't know N statically, so
			// build a typed list whose element carrier is the
			// join of all preserved stack carriers, and leave the
			// preserved stack below it. Net effect: stack count
			// reduces by an unknown amount. Conservative model:
			// keep the whole stack, append the typed-list carrier.
			CheckFullStackFn: func(_ []engine.Value, stack []engine.Value) []engine.Value {
				var elem engine.Type = engine.TAny
				if len(stack) > 0 {
					elem = stack[0].VType
					for i := 1; i < len(stack); i++ {
						elem = engine.CommonAncestorType(elem, stack[i].VType)
					}
				}
				return append(append([]engine.Value(nil), stack...), engine.NewCarrierTypedList(elem))
			},
		}},
	})
}
