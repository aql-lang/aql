package engine

func RegisterDepth(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "depth",
		ForwardPrecedence: false,
		Signatures: []NativeSig{{
			FullStack: true,
			Handler: func(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
				return append(stack, NewInteger(int64(len(stack)))), nil
			},
			// Check-mode FullStack: preserve the carrier stack and
			// append one Integer (carrier depth is unknown
			// statically, but the runtime value type is fixed).
			CheckFullStackFn: func(_ []Value, stack []Value) []Value {
				return append(append([]Value(nil), stack...), NewCarrier(TInteger))
			},
		}},
	})
}
