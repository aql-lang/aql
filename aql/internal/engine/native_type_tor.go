package engine

func RegisterTor(r *Registry) {
	// tor builds a disjunct (union type) from two values.
	// BarrierPos=1 prevents greedy forward consumption of chained `tor` words.
	// args[0] = nearest (top/forward), args[1] = farther (stack).
	//
	// Never is the identity element for tor: T tor Never = T (the
	// uninhabited alternative contributes no values to the union, so
	// it drops out). When all alternatives are Never the result is
	// Never; when one alternative remains it is returned bare rather
	// than wrapped in a singleton disjunct.
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		var alts []Value
		// Flatten left side (farther/stack) first to maintain source order.
		if args[1].IsDisjunct() {
			d, _ := args[1].AsDisjunct()
			alts = append(alts, d.Alternatives...)
		} else {
			alts = append(alts, args[1])
		}
		// Flatten right side (nearest/forward).
		if args[0].IsDisjunct() {
			d, _ := args[0].AsDisjunct()
			alts = append(alts, d.Alternatives...)
		} else {
			alts = append(alts, args[0])
		}
		// Filter Never alternatives — they carry no values.
		filtered := alts[:0]
		for _, alt := range alts {
			if !alt.VType.Equal(TNever) {
				filtered = append(filtered, alt)
			}
		}
		if len(filtered) == 0 {
			return []Value{NewTypeLiteral(TNever)}, nil
		}
		if len(filtered) == 1 {
			return []Value{filtered[0]}, nil
		}
		return []Value{NewDisjunct(filtered)}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "tor",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:       []Type{TAny, TAny},
				BarrierPos: 1,
				Handler:    handler,
				// Builds a disjunction carrier that flattens incoming
				// disjuncts, subsumes subtypes, and applies
				// CarrierDisjunctCap widening.
				ReturnsFn: func(args []Value) []Value {
					if len(args) != 2 {
						return []Value{NewCarrier(TAny)}
					}
					return []Value{JoinCarriers(args[1], args[0])}
				},
			},
		},
	})
}
