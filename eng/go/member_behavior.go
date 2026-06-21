package eng

// memberBehavior is a TypeBehavior built from a single Go membership
// predicate. It is the host-Go counterpart of the wiring the AQL
// `def`/`refine`/`tor` path installs automatically: a host that can say
// "does value v belong to this type?" gets every kernel touch-point for
// free, instead of hand-implementing (and keeping consistent) Match,
// Unify, Format and the canon format-delegate marker.
//
//   - Match — the predicate. Dispatch (sigTypeMatches), the `is` word's
//     fast path, options/record fields: all ask Match.
//   - Unify — the SAME predicate, admitting the concrete side when it
//     belongs. The `is` word, `case`, fn return checks and typed
//     containers ask Unify, not Match; deriving one from the other is
//     what keeps the two answers from drifting (the gap that made the
//     first hand-written host enum so fiddly).
//   - Format / FormatDelegate — render by the kernel default (the value's
//     lattice family), so a member renders as itself and canon keeps its
//     family form (e.g. an Atom subtype's `name/q`).
//   - Equal — kernel default.
type memberBehavior struct {
	member func(v Value) bool
}

// MemberBehavior builds a TypeBehavior whose inhabitants are exactly the
// concrete values satisfying member. Use it for host (Go) types that are
// defined by a membership rule — a closed set of values, a tag check, a
// range — so the type participates in `is`, signature dispatch, `case`,
// and return checks identically to an AQL-defined refinement, from one
// predicate. Pair it with TypeTable.MintMemberType (or pass it to
// MintTypeWithBehavior / RegisterExternalBuiltin) to attach it to a node.
func MemberBehavior(member func(v Value) bool) TypeBehavior {
	return memberBehavior{member: member}
}

// Match reports membership. A bare type literal or carrier is "the type
// itself" / an abstract placeholder, not an inhabitant, so it defers to
// the lattice walk; concrete values are put to the predicate.
func (b memberBehavior) Match(v Value, t *Type) bool {
	if !IsConcrete(v) {
		return DefaultBehavior.Match(v, t)
	}
	return b.member(v)
}

// Unify admits the concrete side that satisfies the predicate, in either
// argument order (dispatchUnifier starts at the more specific denoted
// type, so the membership test is reached for `value is Type` and the
// reverse). A concrete member wins. When there is a concrete operand and
// none satisfy the predicate, the result is a DEFINITIVE failure — not
// ErrNoUnifier — so the kernel's structural fallback cannot re-admit a
// non-member by lattice subtyping alone (the soundness hole a plain
// opt-out leaves). A purely type-level unification (no concrete operand,
// e.g. literal-vs-literal) defers to the structural rule, which doesn't
// need the predicate.
func (b memberBehavior) Unify(a, c Value) (Value, *UnifyError) {
	sawConcrete := false
	for _, v := range []Value{a, c} {
		if IsConcrete(v) {
			sawConcrete = true
			if b.member(v) {
				return v, nil
			}
		}
	}
	if sawConcrete {
		return Value{}, unifyFail("value is not a member of the type", a, c)
	}
	return unifySameOrSubtype(a, c)
}

func (b memberBehavior) Equal(a, c Value) bool { return DefaultBehavior.Equal(a, c) }
func (b memberBehavior) Format(v Value) string { return DefaultBehavior.Format(v) }

// FormatDelegate marks the behavior as delegating Format to the kernel
// default, so canon / Value.String render a member by its lattice family
// rather than routing through Format (which would drop family-specific
// source forms such as an Atom subtype's `/q` suffix).
func (b memberBehavior) FormatDelegate() {}

// MintMemberType mints `name` as a subtype of parent whose inhabitants
// are exactly the concrete values satisfying member, with the full
// Match/Unify/Format wiring (MemberBehavior) attached. It is the
// host-Go equivalent of an AQL refinement (`def Name (refine Parent …)`):
// one call yields a node that behaves like a first-class type across
// dispatch, `is`, `case` and return checks, and the returned *Type is
// ready to drop into native signatures and to tag values with.
//
// Like every minted type the node carries no FixedID and is absent from
// the builtin name index and the FixedID snapshot — it is owned by
// whoever minted it (a module sub-registry, a host plugin), not a kernel
// builtin.
func (tt *TypeTable) MintMemberType(name string, parent *Type, member func(v Value) bool) *Type {
	return tt.MintTypeWithBehavior(name, parent, MemberBehavior(member))
}
