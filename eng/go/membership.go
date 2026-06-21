package eng

// Membership-defined types — the convergence point for the two ways the
// kernel installs a "type whose inhabitants are the values satisfying a
// rule": the AQL path (predicateUnifier, rule = an AQL fn body run via
// RunPredicate) and the host-Go path (memberBehavior, rule = a Go func).
// Both answer the same question — "does v inhabit this type?" — at the
// same kernel touch-points (Match for dispatch, Unify for `is`/`case`/
// return checks, Format/canon, Compare). Only the evaluator differs, so
// the Match and Unify CORES live here and both Behaviors route through
// them; the per-path structs hold just the evaluator.

// baseBehavior returns the Behavior a membership Behavior delegates
// Format/Equal/Compare to: the Behavior it wrapped, or DefaultBehavior
// when it wrapped none.
func baseBehavior(prev TypeBehavior) TypeBehavior {
	if prev != nil {
		return prev
	}
	return DefaultBehavior
}

// baseCompare delegates Compare to prev when prev is a Comparer, else
// opts out (ErrNoComparer) — so a membership Behavior never invents an
// ordering it lacks, yet a `behave compare/q` wrapper underneath still
// participates.
func baseCompare(prev TypeBehavior, a, b Value) (int, error) {
	if c, ok := baseBehavior(prev).(Comparer); ok {
		return c.Compare(a, b)
	}
	return 0, ErrNoComparer
}

// matchMembership is the shared Match contract. A non-inhabitant — a bare
// type literal ("the type itself") or a carrier (a CheckMode abstract
// value) — defers to the lattice walk; a concrete value is put to member.
func matchMembership(v Value, t *Type, prev TypeBehavior, member func(Value) bool) bool {
	if !IsConcrete(v) {
		return baseBehavior(prev).Match(v, t)
	}
	return member(v)
}

// unifyMembership is the shared Unify contract. It admits a concrete
// operand the oracle accepts (yielding the oracle's possibly-narrowed
// output); when a concrete operand is present and none are accepted it
// fails DEFINITIVELY — not ErrNoUnifier — so the kernel's structural
// fallback cannot re-admit a non-member by lattice subtyping alone. With
// no concrete operand (a purely type-level unification) it defers to the
// structural rule, which needs no predicate.
//
// admit returns (output, matched, err): output is the value to yield on a
// match — the candidate itself for a Go predicate, or RunPredicate's
// result for an AQL one.
func unifyMembership(a, c Value, typeName string, admit func(Value) (Value, bool, error)) (Value, *UnifyError) {
	sawConcrete := false
	for _, v := range []Value{a, c} {
		if !IsConcrete(v) {
			continue
		}
		sawConcrete = true
		out, matched, err := admit(v)
		if err != nil {
			return Value{}, &UnifyError{Reason: typeName + ": " + err.Error(), A: a, B: c}
		}
		if matched {
			return out, nil
		}
	}
	if sawConcrete {
		return Value{}, unifyFail("value is not a member of "+typeName, a, c)
	}
	return unifySameOrSubtype(a, c)
}
