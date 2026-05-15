package eng

// patternsOk runs Signature.Patterns against the matched arg
// positions. `fwd` is the count of positions filled from forward
// tokens; positions [0..fwd) are forward args and [fwd..) are stack
// args.
//
// Forward vs stack handling:
//
//   - Scalar-literal patterns (Integer, Decimal, String, Boolean,
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
func patternsOk(sig *Signature, positions []int, stack []Value, fwd int) bool {
	if sig.Patterns == nil {
		return true
	}
	for idx, pattern := range sig.Patterns {
		if idx >= len(positions) {
			continue
		}
		isForward := idx < fwd
		val := stack[positions[idx]]
		if pattern.VType.Equal(TMap) && val.VType.Equal(TMap) &&
			pattern.Data != nil && val.Data != nil &&
			!pattern.IsOptionsType() &&
			!val.IsRecordType() && !val.IsTypedMap() && !val.IsOptionsType() {
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
		if isForward && pattern.Data == nil {
			continue
		}
		if _, uOk := Unify(val, pattern); !uOk {
			return false
		}
	}
	return true
}
