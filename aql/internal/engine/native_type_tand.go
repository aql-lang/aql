package engine

import "fmt"

func RegisterTand(r *Registry) {
	// tand combines two values by conjunction. For two concrete maps it
	// merges keys (unifying overlapping values). For other shapes it
	// falls back to Unify.
	//
	// args[0] = nearest (top/forward), args[1] = farther (stack).
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a := args[1]
		b := args[0]

		if isPlainConcreteMap(a) && isPlainConcreteMap(b) {
			merged, err := mergeMaps(a.AsMap(), b.AsMap())
			if err != nil {
				return nil, err
			}
			return []Value{NewMap(merged)}, nil
		}

		unified, ok := Unify(a, b)
		if !ok {
			return nil, fmt.Errorf("tand: cannot unify values")
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
func mergeMaps(a, b ReadMap) (*OrderedMap, error) {
	result := NewOrderedMap()
	for _, key := range a.Keys() {
		aVal, _ := a.Get(key)
		if bVal, present := b.Get(key); present {
			unified, ok := Unify(aVal, bVal)
			if !ok {
				return nil, fmt.Errorf("tand: cannot unify values for key %q", key)
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
	return result, nil
}
