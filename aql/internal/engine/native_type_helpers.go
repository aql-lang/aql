package engine

// This file contains pure helper functions used by the type natives
// (the typeNatives slice in native_type.go). Helpers that have a tight
// coupling to one handler live alongside that handler; this file
// hosts the cross-cutting builders used by `inspect`, `tor`, `tand`,
// `tall`, etc.

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
	if a.VType.Equal(TNever) || b.VType.Equal(TNever) {
		return NewTypeLiteral(TNever)
	}

	if a.IsDisjunct() || b.IsDisjunct() {
		aAlts := FlattenDisjunctAlts(a)
		bAlts := FlattenDisjunctAlts(b)
		var result []Value
		for _, ax := range aAlts {
			for _, bx := range bAlts {
				result = append(result, tandValues(ax, bx))
			}
		}
		simplified := SimplifyDisjunctAlts(result)
		switch len(simplified) {
		case 0:
			return NewTypeLiteral(TNever)
		case 1:
			return simplified[0]
		default:
			return NewDisjunct(simplified)
		}
	}

	if isPlainConcreteMap(a) && isPlainConcreteMap(b) {
		merged, ok := mergeMaps(a.AsMap(), b.AsMap())
		if !ok {
			return NewTypeLiteral(TNever)
		}
		return NewMap(merged)
	}

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

// buildInspection constructs a word_inspection map for the named word.
func buildInspection(r *Registry, name string) Value {
	result := NewOrderedMap()
	result.Set("name", NewString(name))

	fn := r.Lookup(name)
	if fn == nil {
		if r.HasDef(name) {
			result.Set("kind", NewAtom("defined"))
			result.Set("signatures", NewList(nil))
			return NewValueRaw(TInspect, result)
		}
		result.Set("kind", NewAtom("unknown"))
		result.Set("signatures", NewList(nil))
		return NewValueRaw(TInspect, result)
	}

	if len(fn.Sigs) > 0 {
		result.Set("kind", NewAtom("defined"))
	} else {
		result.Set("kind", NewAtom("native"))
	}

	result.Set("forward_precedence", NewBoolean(fn.ForwardPrecedence))

	var sigMaps []Value
	for _, sig := range fn.Signatures {
		sm := NewOrderedMap()

		var argVals []Value
		for _, argType := range sig.Args {
			argVals = append(argVals, NewString(argType.String()))
		}
		if argVals == nil {
			argVals = []Value{}
		}
		sm.Set("args", NewList(argVals))

		sigMaps = append(sigMaps, NewMap(sm))
	}
	if sigMaps == nil {
		sigMaps = []Value{}
	}
	result.Set("signatures", NewList(sigMaps))

	return NewValueRaw(TInspect, result)
}

// buildTypeInspection constructs a type_inspection map for a type value.
func buildTypeInspection(name string, tv Value) Value {
	result := NewOrderedMap()

	if name != "" {
		result.Set("name", NewString(name))
	}

	result.Set("type", NewString(tv.VType.String()))

	switch {
	case tv.IsRecordType():
		result.Set("kind", NewAtom("record"))
		rt, _ := tv.AsRecordType()
		fields := NewOrderedMap()
		for _, k := range rt.Fields.Keys() {
			v, _ := rt.Fields.Get(k)
			fields.Set(k, NewString(v.VType.String()))
		}
		result.Set("fields", NewMap(fields))

	case tv.IsTableType():
		result.Set("kind", NewAtom("table"))
		tt, _ := tv.AsTableType()
		fields := NewOrderedMap()
		for _, k := range tt.Record.Fields.Keys() {
			v, _ := tt.Record.Fields.Get(k)
			fields.Set(k, NewString(v.VType.String()))
		}
		result.Set("fields", NewMap(fields))

	case tv.IsDisjunct():
		result.Set("kind", NewAtom("disjunct"))
		di, _ := tv.AsDisjunct()
		alts := make([]Value, len(di.Alternatives))
		for i, alt := range di.Alternatives {
			alts[i] = NewString(alt.VType.String())
		}
		result.Set("alternatives", NewList(alts))

	case tv.IsTypedList():
		result.Set("kind", NewAtom("typed_list"))
		_as1, _ := tv.AsChildType()
		child := _as1.Child
		result.Set("child", NewString(child.VType.String()))

	case tv.IsTypedMap():
		result.Set("kind", NewAtom("typed_map"))
		_as2, _ := tv.AsChildType()
		child := _as2.Child
		result.Set("child", NewString(child.VType.String()))

	case tv.VType.Equal(TFnUndef):
		result.Set("kind", NewAtom("function_shape"))
		uInfo, _ := tv.Data.(FnUndefInfo)
		sigs := make([]Value, 0, len(uInfo.Sigs))
		for _, spec := range uInfo.Sigs {
			sig := NewOrderedMap()
			params := make([]Value, len(spec.Params))
			for i, p := range spec.Params {
				params[i] = NewString(p.Type.String())
			}
			sig.Set("params", NewList(params))
			rets := make([]Value, len(spec.Returns))
			for i, r := range spec.Returns {
				rets[i] = NewString(r.String())
			}
			sig.Set("returns", NewList(rets))
			sigs = append(sigs, NewMap(sig))
		}
		result.Set("signatures", NewList(sigs))

	case tv.IsDepScalar():
		result.Set("kind", NewAtom("dependent_scalar"))
		info, _ := tv.AsDepScalar()
		leaf := DependentLeafFromType(tv.VType)
		result.Set("leaf", NewString(leaf))
		if info.Lo != nil {
			lo := NewOrderedMap()
			lo.Set("kind", NewString(BoundToKind(info.Lo, true).String()))
			lo.Set("value", info.Lo.Value)
			result.Set("lo", NewMap(lo))
		}
		if info.Hi != nil {
			hi := NewOrderedMap()
			hi.Set("kind", NewString(BoundToKind(info.Hi, false).String()))
			hi.Set("value", info.Hi.Value)
			result.Set("hi", NewMap(hi))
		}

	default:
		result.Set("kind", NewAtom("literal"))
	}

	return NewValueRaw(TInspect, result)
}
