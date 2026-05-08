package engine

func RegisterTor(r *Registry) {
	// tor builds a disjunct (union type) from two values.
	// BarrierPos=1 prevents greedy forward consumption of chained `tor` words.
	// args[0] = nearest (top/forward), args[1] = farther (stack).
	//
	// Algebra:
	//   - Never is the identity element: T tor Never = T (filtered).
	//   - Idempotence: T tor T = T (structurally identical alternatives
	//     are deduped at construction).
	//   - Subsumption: when one alternative is a subtype of another
	//     (Integer tor Number → Number), the subtype drops out. Concrete
	//     values absorbed by a covering type literal also drop
	//     (5 tor Integer → Integer). Concrete values are NOT subsumed
	//     by other concrete values of the same type — `1 tor 2` keeps
	//     both, since each carries information the other doesn't.
	//   - Singleton/empty disjuncts collapse: 0 alts → Never, 1 alt
	//     → bare value (no wrapper).
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		// Flatten both sides into a single alternative slice;
		// FlattenDisjunctAlts returns the existing alternatives for
		// a disjunct or [v] for any other value. Source order is
		// preserved by walking left (farther/stack) then right.
		alts := append(FlattenDisjunctAlts(args[1]), FlattenDisjunctAlts(args[0])...)
		simplified := SimplifyDisjunctAlts(alts)
		if len(simplified) == 0 {
			return []Value{NewTypeLiteral(TNever)}, nil
		}
		if len(simplified) == 1 {
			return []Value{simplified[0]}, nil
		}
		return []Value{NewDisjunct(simplified)}, nil
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
				ReturnsFn: func(args []Value, r *Registry) []Value {
					if len(args) != 2 {
						return []Value{NewCarrier(TAny)}
					}
					return []Value{JoinCarriers(args[1], args[0])}
				},
			},
		},
	})
}

// SimplifyDisjunctAlts: re-exported from aqleng via aliases.go
