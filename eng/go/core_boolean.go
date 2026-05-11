package eng

// This file owns the boolean / logical-connective core words
// (not, and, or) and the type-level connective core words
// (tor, tand). They are part of the aqleng language proper —
// any consumer that wants the standard truthy/falsy semantics
// or the disjunct-builder ops uses these implementations rather
// than rolling their own.
//
// not / and / or follow Lisp/Python short-circuit semantics:
//
//   not v        — boolean negation, always returns Boolean
//   a and b      — returns args[0] (=a) if args[1] (=b) is falsy,
//                  else args[1]; non-Boolean inputs go through
//                  CoerceBoolean for the truthiness test but
//                  the returned value preserves its original type
//   a or b       — returns args[1] (=b) if it is truthy,
//                  else args[0] (=a); same coercion rule
//
// tor / tand operate on Type-shaped values:
//
//   T tor U      — disjunct union; flattens nested disjuncts,
//                  removes Never (the disjunct identity), and
//                  dedupes structurally identical alternatives.
//                  Single-alt result returned bare; empty result
//                  collapses to Never.
//   T tand U     — type intersection, distributing over disjuncts.
//                  Concrete maps merge field-wise. Anything that
//                  fails to unify collapses to Never.
//
// These five words are bytewise-identical ports of the production
// lang/engine/native_boolean.go and native_type.go
// registrations (with their dependencies — tandValues + helpers —
// also moved into aqleng; see fn_params.go's pattern for how the
// move played out).

// registerCoreBoolean installs `not`, `and`, `or` on r.
func registerCoreBoolean(r *Registry) {
	registerCoreOr(r)
	registerCoreAnd(r)
	registerCoreNot(r)
}

// registerCoreOr — `or` with TBoolean and TAny overloads.
func registerCoreOr(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "or",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TBoolean, TBoolean}, BarrierPos: 1, Handler: orHandler, Returns: []Type{TBoolean}},
			{Args: []Type{TAny, TAny}, BarrierPos: 1, Handler: orHandler, Returns: []Type{TAny}},
		},
	})
}

// registerCoreAnd — `and` with TBoolean and TAny overloads.
func registerCoreAnd(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "and",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TBoolean, TBoolean}, Handler: andHandler, Returns: []Type{TBoolean}},
			{Args: []Type{TAny, TAny}, Handler: andHandler, Returns: []Type{TAny}},
		},
	})
}

// registerCoreNot — `not` with TBoolean and TAny overloads.
func registerCoreNot(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "not",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TBoolean}, Handler: notHandler, Returns: []Type{TBoolean}},
			{Args: []Type{TAny}, Handler: notHandler, Returns: []Type{TBoolean}},
		},
	})
}

// registerCoreTypeOps installs `tor`, `tand`.
func registerCoreTypeOps(r *Registry) {
	registerCoreTor(r)
	registerCoreTand(r)
}

// registerCoreTor — `tor` (type-level disjunct union).
func registerCoreTor(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "tor",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TAny, TAny},
			BarrierPos: 1,
			Handler:    torHandler,
			ReturnsFn:  torReturnsFn,
		}},
	})
}

// registerCoreTand — `tand` (type-level intersection).
func registerCoreTand(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "tand",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TAny, TAny},
			BarrierPos: 1,
			Handler:    tandHandler,
			Returns:    []Type{TAny},
		}},
	})
}

// orHandler returns args[1] when truthy, else args[0]. Short-circuit.
func orHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if CoerceBoolean(args[1]) {
		return []Value{args[1]}, nil
	}
	return []Value{args[0]}, nil
}

// andHandler returns args[1] when falsy, else args[0]. Short-circuit.
func andHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !CoerceBoolean(args[1]) {
		return []Value{args[1]}, nil
	}
	return []Value{args[0]}, nil
}

// notHandler always returns a Boolean.
func notHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(!CoerceBoolean(args[0]))}, nil
}

// torHandler is the type-level disjunct-builder. Flattens any
// disjunct alternatives in either input, simplifies (Never is the
// identity, structural duplicates are removed), and returns:
//   - Never if no alternatives remain
//   - the lone alternative if only one remains
//   - a fresh disjunct otherwise
func torHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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

// torReturnsFn is the carrier-mode counterpart: joins the two input
// carriers into one carrier whose type is their disjunct.
func torReturnsFn(args []Value, _ *Registry) []Value {
	if len(args) != 2 {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{JoinCarriers(args[1], args[0])}
}

// tandHandler delegates to TandValues for the actual intersection
// computation. Args are passed in [args[1], args[0]] order so the
// natural infix reading (`A tand B → tandValues(A, B)`) matches the
// b-op-a handler convention — see mirror.tsv.
func tandHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{TandValues(args[1], args[0])}, nil
}

// TandValues computes the type-level intersection of a and b.
// Distributes over disjuncts, propagates Never (annihilator), merges
// concrete maps field-wise, and falls back to Unify for everything
// else. Always returns a value — disjoint inputs collapse to Never.
//
// Distribution rule: (A tor B) tand C = (A tand C) tor (B tand C).
// When both sides are disjuncts, the cross product is computed and
// each pair recursively reduced. Never-valued cross-product entries
// are filtered (Never is the identity for tor), and structurally
// identical alternatives are deduped.
//
// Exported because aql's `tall` (typed-all reduction) folds over a
// list of values via this same intersection. Future higher-order
// type combinators may want it too.
func TandValues(a, b Value) Value {
	if a.VType.Equal(TNever) || b.VType.Equal(TNever) {
		return NewTypeLiteral(TNever)
	}

	if a.IsDisjunct() || b.IsDisjunct() {
		aAlts := FlattenDisjunctAlts(a)
		bAlts := FlattenDisjunctAlts(b)
		var result []Value
		for _, ax := range aAlts {
			for _, bx := range bAlts {
				result = append(result, TandValues(ax, bx))
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
// keys present in both via TandValues. Keys present in only one side
// are kept as-is. Returns ok=false when any overlapping key has
// incompatible values — the caller propagates that as Never (the
// empty intersection).
func mergeMaps(a, b ReadMap) (*OrderedMap, bool) {
	result := NewOrderedMap()
	for _, key := range a.Keys() {
		aVal, _ := a.Get(key)
		if bVal, present := b.Get(key); present {
			combined := TandValues(aVal, bVal)
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
