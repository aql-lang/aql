package eng

// ResolveWordsDeep recursively resolves word values to their semantic form.
// For lists, each element is resolved; for maps, each value is resolved.
// Scalar words are resolved via ResolveWordValue.
//
// Lives here (not in unify.go) because the operation is value
// preprocessing — interpreting `true`/`false`/type names that the
// parser left as bare Words — and is independent of unification. Unify
// is the only current caller, but anything else that needs to canonicalize
// embedded words can use it without dragging in the unifier.
func ResolveWordsDeep(v Value) Value {
	if IsWord(v) {
		return ResolveWordValue(v)
	}
	if v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v) {
		lst, _ := AsList(v)
		elems := lst.Slice()
		resolved := make([]Value, len(elems))
		for i, e := range elems {
			resolved[i] = ResolveWordsDeep(e)
		}
		return NewList(resolved)
	}
	if v.Parent.Equal(TMap) && v.Data != nil && !IsTypedMap(v) && !IsRecordType(v) && !IsOptionsType(v) {
		m, _ := AsMap(v)
		result := NewOrderedMap()
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			result.Set(key, ResolveWordsDeep(val))
		}
		// Preserve the optional-key marker so unifiers and matchers
		// downstream see the parser's optionality through resolution.
		if om, ok := m.(*OrderedMap); ok && om.Meta != nil {
			if opts, ok := om.Meta["opt"].(map[string]bool); ok && len(opts) > 0 {
				result.Meta = map[string]any{"opt": opts}
			}
		}
		return NewMap(result)
	}
	return v
}
