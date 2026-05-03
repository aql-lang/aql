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

	// Distribute over disjuncts on either side.
	aDisj := a.IsDisjunct()
	bDisj := b.IsDisjunct()
	if aDisj || bDisj {
		var aAlts, bAlts []Value
		if aDisj {
			ad, _ := a.AsDisjunct()
			aAlts = ad.Alternatives
		} else {
			aAlts = []Value{a}
		}
		if bDisj {
			bd, _ := b.AsDisjunct()
			bAlts = bd.Alternatives
		} else {
			bAlts = []Value{b}
		}
		var result []Value
		for _, ax := range aAlts {
			for _, bx := range bAlts {
				r := tandValues(ax, bx)
				if r.VType.Equal(TNever) {
					continue
				}
				dup := false
				for _, prev := range result {
					// valuesEqual returns true for any two type
					// literals (Data==nil); existing callers always
					// pre-check VType. Mirror that here so disjoint
					// type literals (Integer vs String) survive
					// dedup.
					if prev.VType.Equal(r.VType) && valuesEqual(prev, r) {
						dup = true
						break
					}
				}
				if !dup {
					result = append(result, r)
				}
			}
		}
		switch len(result) {
		case 0:
			return NewTypeLiteral(TNever)
		case 1:
			return result[0]
		default:
			return NewDisjunct(result)
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

// mergeMaps walks keys of a then b in order, unifying values for keys
// present in both. Keys present in only one side are kept as-is.
// Returns ok=false when any overlapping key has incompatible values —
// the caller propagates that as Never (the empty intersection).
func mergeMaps(a, b ReadMap) (*OrderedMap, bool) {
	result := NewOrderedMap()
	for _, key := range a.Keys() {
		aVal, _ := a.Get(key)
		if bVal, present := b.Get(key); present {
			unified, ok := Unify(aVal, bVal)
			if !ok {
				return nil, false
			}
			result.Set(key, unified)
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
