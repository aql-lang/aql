package eng

// patternsOk runs Signature.Patterns against the matched arg
// positions. `fwd` is the count of positions filled from forward
// tokens; positions [0..fwd) are forward args and [fwd..) are stack
// args.
//
// Forward vs stack handling:
//
//   - Scalar-literal patterns (Integer, Float, String, Boolean,
//     Atom — concrete `Data != nil` payloads) are checked on EVERY
//     position. This is the §1.1 entry point: a sig with
//     `Patterns[0] = NewInteger(0)` must reject any non-zero arg
//     regardless of which side it came from.
//   - Structural patterns (record/map shapes, `OpenUnifyMap`
//     candidates) are checked ONLY on stack-matched positions.
//     The legacy semantics — that handlers may further constrain
//     forward args inside the handler body — depends on this skip.
//     Tightening it would break callers like `create` whose 1-arg
//     `(Map) Patterns={kind:"api"}` sig was previously matched on
//     non-api maps when the handler then routed by stack contents.
func patternsOk(sig *Signature, positions []int, tape *Tape, fwd int, r *Registry) bool {
	for idx := 0; idx < sig.TotalArgs(); idx++ {
		pattern, ok := sigPattern(sig, idx)
		if !ok {
			continue
		}
		if idx >= len(positions) {
			continue
		}
		isForward := idx < fwd
		val := tape.At(positions[idx])
		// A forward position may still hold the unresolved Word token
		// for a def-bound value (the forward scan plans against
		// Defs.Top but does not rewrite the tape). Resolve it the same
		// way before unifying — otherwise a typed-list/map pattern
		// (`xs:[:Integer]`) rejects `f zs` while accepting the literal
		// `f [1 2]`, purely by spelling.
		if isForward && r != nil && IsWord(val) {
			if w, werr := AsWord(val); werr == nil {
				if top, ok := r.Defs.Top(w.Name); ok {
					val = top
				}
			}
		}
		if pattern.Parent.Equal(TMap) && val.Parent.Equal(TMap) &&
			pattern.Data != nil && val.Data != nil &&
			!IsOptionsType(pattern) &&
			!IsRecordType(val) && !IsTypedMap(val) && !IsOptionsType(val) {
			if isForward {
				// Legacy: structural map patterns only enforced on
				// stack positions. See doc comment.
				continue
			}
			if !OpenUnifyMap(pattern, val) {
				return false
			}
			continue
		}
		// Concrete scalar pattern? Always check.
		// *Type-literal / non-concrete pattern on a forward position?
		// Skip — handlers may further constrain inside the body.
		if isForward && !IsConcrete(pattern) {
			continue
		}
		if _, uOk := Unify(val, pattern); !uOk {
			return false
		}
	}
	return true
}

// OpenUnifyMap checks whether candidate contains at least the key-value pairs
// of pattern. Extra keys in candidate are allowed (open/subset matching).
//
// This is an asymmetric subset match, not a unifier — it returns only
// ok/!ok and never produces a unified value. Lives next to patternsOk
// because both are matching primitives used by signature dispatch.
//
// Missing-key rule: when a pattern key is absent on the candidate,
// synthesise an Absent value and unify the pattern's value against it.
// A pattern value containing `Absent` as a disjunct alternative (the
// `?:T` desugaring) accepts it; any other shape rejects it. This
// implements the "? means None or absent" rule via the type system —
// no out-of-band optional-key metadata required.
func OpenUnifyMap(pattern, candidate Value) bool {
	pMap, _ := AsMap(pattern)
	cMap, _ := AsMap(candidate)

	// Non-concrete map shapes (a typed map's ChildTypeInfo, record /
	// options bodies) have no OrderedMap to walk — AsMap returns nil
	// for them. Route those pairs through the full unifier, whose map
	// family owns typed-vs-concrete and record matching, rather than
	// panicking on pMap.Keys(). Callers' guards vary; this is the
	// single defensive boundary.
	if pMap == nil || cMap == nil {
		_, ok := Unify(pattern, candidate)
		return ok
	}

	absentVal := NewTypeLiteral(TAbsent)
	for _, key := range pMap.Keys() {
		pVal, _ := pMap.Get(key)
		cVal, ok := cMap.Get(key)
		if !ok {
			if _, uOk := Unify(pVal, absentVal); !uOk {
				return false
			}
			continue
		}
		if _, uOk := Unify(pVal, cVal); !uOk {
			return false
		}
	}
	return true
}
