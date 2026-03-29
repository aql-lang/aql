package engine

func registerOr(r *Registry) {
	registerBinaryBoolOp(r, "or", func(a, b bool) bool { return a || b })

	// or for non-boolean values: creates a disjunct (union type).
	// The boolean signature (specificity 202) ties with this (202), but the
	// boolean signature is registered first, so it wins for boolean args.
	// Swap: `a or b` means left=a, right=b, so left=args[1], right=args[0].
	r.Register("or", Signature{
		Args:       []Type{TAny, TAny},
		Handler: func(args []Value) ([]Value, error) {
			var alts []Value
			// Flatten left side if already a disjunct.
			if args[1].IsDisjunct() {
				alts = append(alts, args[1].AsDisjunct().Alternatives...)
			} else {
				alts = append(alts, args[1])
			}
			// Flatten right side if already a disjunct.
			if args[0].IsDisjunct() {
				alts = append(alts, args[0].AsDisjunct().Alternatives...)
			} else {
				alts = append(alts, args[0])
			}
			return []Value{NewDisjunct(alts)}, nil
		},
	})
}
