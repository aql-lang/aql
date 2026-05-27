package eng

// unifyDisjunct tries to unify a value against each alternative in a
// disjunct. Returns the first successful unification. For map
// alternatives, uses open (subset) matching where the candidate only
// needs to contain the alternative's key-value pairs.
//
// Asymmetric by design: disj is always the disjunct side, val is the
// other side. The top dispatcher in unify.go handles the swap.
func unifyDisjunct(disj DisjunctInfo, val Value) (Value, *UnifyError) {
	// "any" unifies with the whole disjunct, preserving it. Covers
	// two value shapes: the bare type literal NewTypeLiteral(TAny)
	// (Data=nil; the value IS the TAny lattice node) and the Any-
	// typed carrier (Data=nil, Carrier=true, Parent=TAny).
	if val.Data == nil && (val.Parent.Equal(TAny) || (&val).Equal(TAny)) {
		return NewDisjunct(disj.Alternatives), nil
	}

	for _, alt := range disj.Alternatives {
		// Concrete map alternative against a concrete map value uses
		// open (subset) matching — the disjunct alternative acts as a
		// pattern, not a full schema.
		if alt.Parent.Equal(TMap) && val.Parent.Equal(TMap) &&
			!IsRecordType(alt) && !IsRecordType(val) &&
			!IsTypedMap(alt) && !IsTypedMap(val) &&
			!IsOptionsType(alt) && !IsOptionsType(val) {
			if alt.Data != nil && val.Data != nil {
				if OpenUnifyMap(alt, val) {
					return val, nil
				}
				continue
			}
		}
		if unified, err := unifyInner(alt, val); err == nil {
			return unified, nil
		}
	}
	return Value{}, unifyFail("no disjunct alternative matched", NewDisjunct(disj.Alternatives), val)
}
