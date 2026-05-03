package engine

func RegisterTand(r *Registry) {
	// tand combines two values by conjunction (intersection). For two
	// concrete maps it merges keys (unifying overlapping values). For
	// other shapes it falls back to Unify.
	//
	// When the two sides are disjoint (Unify fails), the intersection
	// is empty and tand reduces to Never (the bottom type). This makes
	// tand total — every input pair has a defined result rather than
	// erroring. Map merging at incompatible keys propagates Never the
	// same way: a single Never field annihilates the whole record.
	//
	// args[0] = nearest (top/forward), args[1] = farther (stack).
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a := args[1]
		b := args[0]

		// Never annihilates: T tand Never = Never tand T = Never.
		if a.VType.Equal(TNever) || b.VType.Equal(TNever) {
			return []Value{NewTypeLiteral(TNever)}, nil
		}

		if isPlainConcreteMap(a) && isPlainConcreteMap(b) {
			merged, ok := mergeMaps(a.AsMap(), b.AsMap())
			if !ok {
				return []Value{NewTypeLiteral(TNever)}, nil
			}
			return []Value{NewMap(merged)}, nil
		}

		unified, ok := Unify(a, b)
		if !ok {
			return []Value{NewTypeLiteral(TNever)}, nil
		}
		return []Value{unified}, nil
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
