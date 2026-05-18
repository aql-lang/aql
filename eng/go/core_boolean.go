package eng

// This file owns the algorithms behind the type-level connective
// words (tor / tand) and the helper TandValues which is reused by
// lang's typed-all reduction. The matching word registrations now
// live in lang/go/engine/native_boolean.go (not/and/or) and
// lang/go/engine/native_type.go (tor/tand) — eng exposes only the
// algorithm primitives.
//
// tor / tand operate on *Type-shaped values:
//
//   T tor U      — disjunct union; flattens nested disjuncts,
//                  removes Never (the disjunct identity), and
//                  dedupes structurally identical alternatives.
//                  Single-alt result returned bare; empty result
//                  collapses to Never.
//   T tand U     — type intersection, distributing over disjuncts.
//                  Concrete maps merge field-wise. Anything that
//                  fails to unify collapses to Never.

// TorHandler is the type-level disjunct-builder handler. Flattens any
// disjunct alternatives in either input, simplifies (Never is the
// identity, structural duplicates are removed), and returns:
//   - Never if no alternatives remain
//   - the lone alternative if only one remains
//   - a fresh disjunct otherwise
//
// Exported so lang's tor registration (lang/go/engine/native_type.go)
// can wire dispatch into it without forking the algorithm.
func TorHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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

// TorReturnsFn is the carrier-mode counterpart: joins the two input
// carriers into one carrier whose type is their disjunct.
func TorReturnsFn(args []Value, _ *Registry) []Value {
	if len(args) != 2 {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{JoinCarriers(args[1], args[0])}
}

// TandHandler delegates to TandValues for the actual intersection
// computation. Args are passed in [args[1], args[0]] order so the
// natural infix reading (`A tand B → tandValues(A, B)`) matches the
// b-op-a handler convention — see mirror.tsv.
func TandHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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

	if IsDisjunct(a) || IsDisjunct(b) {
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
		ma, _ := AsMap(a)
		mb, _ := AsMap(b)
		merged, ok := mergeMaps(ma, mb)
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
	if IsRecordType(v) || IsOptionsType(v) || IsTypedMap(v) {
		return false
	}
	m, err := AsMap(v)
	return err == nil && m != nil
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
