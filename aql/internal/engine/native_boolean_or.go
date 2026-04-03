package engine

func registerOr(r *Registry) {
	// Boolean or: needs BarrierPos to match the disjunction signature's
	// BarrierPos bonus in scoring. TBoolean specificity still wins over TAny.
	boolHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewBoolean(args[0].AsBoolean() || args[1].AsBoolean())}, nil
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
				alts = append(alts, args[1].AsDisjunct().Alternatives...)
			} else {
				alts = append(alts, args[1])
			}
			// Flatten right side (nearest/forward).
			if args[0].IsDisjunct() {
				alts = append(alts, args[0].AsDisjunct().Alternatives...)
			} else {
				alts = append(alts, args[0])
			}
			return []Value{NewDisjunct(alts)}, nil
		},
	})
}
