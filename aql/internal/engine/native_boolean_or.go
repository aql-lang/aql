package engine

func registerOr(r *Registry) {
	// Boolean or: needs BarrierPos to match the disjunction signature's
	// BarrierPos bonus in scoring. TBoolean specificity still wins over TAny.
	boolHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		_as1, _ := args[0].AsBoolean()
		_as0, _ := args[1].AsBoolean()
		return []Value{NewBoolean(_as1 || _as0)}, nil
	}
	r.Register("or", Signature{
		Args:       []Type{TBoolean, TBoolean},
		BarrierPos: 1,
		Handler:    boolHandler,
	})

	// or for non-boolean values: creates a disjunct (union type).
	// BarrierPos=1 prevents greedy forward consumption of chained `or` words.
	// args[0] = nearest (top/forward), args[1] = farther (stack).
	r.Register("or", Signature{
		Args:       []Type{TAny, TAny},
		BarrierPos: 1,
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			var alts []Value
			// Flatten left side (farther/stack) first to maintain source order.
			if args[1].IsDisjunct() {
				_as2, _ := args[1].AsDisjunct()
				alts = append(alts, _as2.Alternatives...)
			} else {
				alts = append(alts, args[1])
			}
			// Flatten right side (nearest/forward).
			if args[0].IsDisjunct() {
				_as3, _ := args[0].AsDisjunct()
				alts = append(alts, _as3.Alternatives...)
			} else {
				alts = append(alts, args[0])
			}
			return []Value{NewDisjunct(alts)}, nil
		},
	})
}
