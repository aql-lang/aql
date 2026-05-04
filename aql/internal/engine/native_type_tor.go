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
		simplified := simplifyDisjunctAlts(alts)
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

// simplifyDisjunctAlts filters Never, dedupes structurally identical
// alternatives, and applies subsumption: a strict subtype drops in
// favour of its supertype, and a concrete value drops if some other
// alternative is a covering type literal. Two concrete values of the
// same type are both kept — each one is a distinct piece of
// information that the type literal couldn't replace.
func simplifyDisjunctAlts(alts []Value) []Value {
	// First pass: drop Never.
	live := make([]Value, 0, len(alts))
	for _, alt := range alts {
		if alt.VType.Equal(TNever) {
			continue
		}
		live = append(live, alt)
	}
	// Second pass: keep an alt only if no other live alt subsumes or
	// duplicates it. "Earlier-wins" for duplicates so source order is
	// preserved among survivors.
	out := make([]Value, 0, len(live))
outer:
	for i, cand := range live {
		// Drop if structurally equal to an earlier kept alt.
		for j := 0; j < i; j++ {
			if live[j].VType.Equal(cand.VType) && valuesEqual(live[j], cand) {
				continue outer
			}
		}
		// Drop if subsumed by some other alt:
		//   - cand is a type literal whose VType is a strict subtype
		//     of another's (Integer subsumed by Number).
		//   - cand is a concrete value covered by another type literal
		//     (5 subsumed by Integer).
		// Strict subtype only: equal types are handled by dedup above.
		for j, other := range live {
			if i == j {
				continue
			}
			if cand.VType.Equal(other.VType) {
				continue
			}
			if !cand.VType.Matches(other.VType) {
				continue
			}
			// cand's type is a strict subtype of other's.
			if cand.Data == nil && other.Data == nil {
				continue outer
			}
			if cand.Data != nil && other.Data == nil {
				continue outer
			}
		}
		out = append(out, cand)
	}
	return out
}
