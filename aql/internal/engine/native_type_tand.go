package engine

func RegisterTand(r *Registry) {
	// tand combines two values by conjunction (intersection).
	//
	// Algebra:
	//   - Never tand T = T tand Never = Never (annihilator)
	//   - tand distributes over tor: (A tor B) tand C reduces to
	//     (A tand C) tor (B tand C); same on the right. Both sides
	//     being disjuncts produces the cross product. Distribution
	//     keeps tand and tor in a sound algebraic relationship —
	//     intersections of unions reduce to unions of intersections,
	//     so callers never have to factor by hand.
	//   - For two concrete maps, fields with the same key are
	//     unified pairwise (incompatible field values → Never).
	//   - Otherwise tand falls back to Unify; failure → Never.
	//
	// args[0] = nearest (top/forward), args[1] = farther (stack).
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{tandValues(args[1], args[0])}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "tand",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:       []Type{TAny, TAny},
				BarrierPos: 1,
				Handler:    handler,
				Returns:    []Type{TAny},
			},
		},
	})
}

// tandValues computes the type-level intersection of a and b.
// Distributes over disjuncts, propagates Never (annihilator), merges
// concrete maps field-wise, and falls back to Unify for everything
// else. Always returns a value — disjoint inputs collapse to Never.
//
// Distribution rule: (A tor B) tand C = (A tand C) tor (B tand C).
// When both sides are disjuncts, the cross product is computed and
// each pair recursively reduced. Never-valued cross-product entries
// are filtered (Never is the identity for tor), and structurally
// identical alternatives are deduped.
func tandValues(a, b Value) Value {
	// Never annihilates: T tand Never = Never tand T = Never.
	if a.VType.Equal(TNever) || b.VType.Equal(TNever) {
		return NewTypeLiteral(TNever)
	}

	// Distribute over disjuncts on either side. FlattenDisjunctAlts
	// returns the alternatives for a disjunct or [v] for anything
	// else, so the cross-product loop below handles the
	// scalar/scalar, disjunct/scalar, scalar/disjunct, and
	// disjunct/disjunct cases uniformly.
	if a.IsDisjunct() || b.IsDisjunct() {
		aAlts := FlattenDisjunctAlts(a)
		bAlts := FlattenDisjunctAlts(b)
		var result []Value
		for _, ax := range aAlts {
			for _, bx := range bAlts {
				result = append(result, tandValues(ax, bx))
			}
		}
		// Run the disjunct simplifier (Never filter, dedup,
		// subsumption) so the cross-product output is canonicalised
		// the same way `tor` would canonicalise an explicit union.
		simplified := simplifyDisjunctAlts(result)
		switch len(simplified) {
		case 0:
			return NewTypeLiteral(TNever)
		case 1:
			return simplified[0]
		default:
			return NewDisjunct(simplified)
		}
	}

	// Concrete map field-wise merge.
	if isPlainConcreteMap(a) && isPlainConcreteMap(b) {
		merged, ok := mergeMaps(a.AsMap(), b.AsMap())
		if !ok {
			return NewTypeLiteral(TNever)
		}
		return NewMap(merged)
	}

	// Fallback: standard unification.
	unified, ok := Unify(a, b)
	if !ok {
		return NewTypeLiteral(TNever)
	}
	return unified
}

// isPlainConcreteMap reports whether v is a non-typed, non-record,
// non-options concrete map (Data is *OrderedMap).
func isPlainConcreteMap(v Value) bool {
	if !v.VType.Equal(TMap) || v.Data == nil {
		return false
	}
	if v.IsRecordType() || v.IsOptionsType() || v.IsTypedMap() {
		return false
	}
	return v.AsMap() != nil
}

// mergeMaps walks keys of a then b in order, intersecting values for
// keys present in both. Keys present in only one side are kept as-is.
// Returns ok=false when any overlapping key has incompatible values —
// the caller propagates that as Never (the empty intersection).
//
// Field-level uses tandValues so distribution applies inside fields:
// {a:(Int tor Str)} tand {a:(Str tor Int)} produces {a:(Int tor Str)}
// rather than a single-alt projection (which is what plain Unify
// would do via "first matching alt" semantics).
func mergeMaps(a, b ReadMap) (*OrderedMap, bool) {
	result := NewOrderedMap()
	for _, key := range a.Keys() {
		aVal, _ := a.Get(key)
		if bVal, present := b.Get(key); present {
			combined := tandValues(aVal, bVal)
			if combined.VType.Equal(TNever) {
				return nil, false
			}
			result.Set(key, combined)
			continue
		}
		result.Set(key, aVal)
	}
	for _, key := range b.Keys() {
		if _, ok := a.Get(key); ok {
			continue
		}
		bVal, _ := b.Get(key)
		result.Set(key, bVal)
	}
	return result, true
}
