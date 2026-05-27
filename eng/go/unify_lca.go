package eng

// ErrNoUnifier is the sentinel a Unifier returns when it holds a
// placeholder slot (e.g. a wrapped Behavior whose user-defined unifier
// body is empty). dispatchUnifier recognises it via pointer equality
// and continues the parent-chain walk, treating the Behavior as if it
// didn't satisfy the Unifier interface at all.
//
// Mirrors ErrNoComparer in compare.go — single-slot wrapper Behaviors
// carrying multiple capabilities (compare / canon / unify / …) need
// per-capability opt-out so missing slots don't terminate dispatch
// prematurely.
var ErrNoUnifier = &UnifyError{Reason: "no unifier in this Behavior"}

// dispatchUnifier walks the lattice looking for a Unifier capability
// to handle (a, b). Returns (value, err, true) if a Unifier owned the
// result, (zero, nil, false) if no Unifier applied and the caller
// should fall through to the kernel's structural dispatch.
//
// Walk strategy: start from the MORE SPECIFIC of the two denoted types
// (the one that's a subtype of the other), then walk parent-ward. This
// differs from Comparer's LCA walk because unification's "narrowing
// into a constrained type" case asks the constraint (the more specific
// type) to validate — e.g. `Unify(Pos-literal, integer-5)` must
// trigger Pos's predicate Unifier even though the LCA is Integer.
//
// When neither type is a subtype of the other, fall back to the LCA
// walk (same as Comparer). Predicate intersection across sibling
// refinement types is a known limitation — only the more specific
// of the two chains is walked; combining `Pos` and `Even` requires
// `(Pos tand Even)` rather than implicit Unify.
//
// Bare type literals participate via denotedType so unifying a refined
// type's type literal with a value still triggers the type's Unifier.
// Carriers expose their declared type via Parent — the same walk
// applies.
func dispatchUnifier(a, b Value) (Value, *UnifyError, bool) {
	aType := denotedType(a)
	bType := denotedType(b)
	if aType == nil || bType == nil {
		return Value{}, nil, false
	}

	var start *Type
	switch {
	case bType.IsSubtypeOf(aType):
		start = bType
	case aType.IsSubtypeOf(bType):
		start = aType
	default:
		start = lowestCommonAncestor(aType, bType)
	}

	for t := start; t != nil; t = t.Parent {
		if t.Behavior == nil {
			continue
		}
		u, ok := t.Behavior.(Unifier)
		if !ok {
			continue
		}
		v, err := u.Unify(a, b)
		if err == ErrNoUnifier {
			continue
		}
		return v, err, true
	}
	return Value{}, nil, false
}
