package eng

import "fmt"

// predicateUnifier is the kernel-installed Unifier for predicate types
// — types whose body is a `fn [[x:BaseType] [Boolean] [body]]`.
// When the LCA walk in dispatchUnifier reaches the predicate type's
// *Type, this Unifier runs the predicate body against the structural
// candidate and admits the result only when the predicate accepts.
//
// Why this exists: before Phase 4, `Unify(Pos-literal, integer-5)`
// fell through to the lattice subtype rule and returned 5 wrapped as
// Pos without ever checking the predicate. The Reparent-at-typed-bind
// path in lang's `def` handler patched the common case (`def x:Pos
// 5`), but every other Unify call site (signature matching, options
// fields, record fields, the `unify` word, `make` constraints) bypassed
// the check. With a Unifier on the predicate type's lattice node, every
// path through Unify consults it.
//
// Holds a *Registry because RunPredicate needs to invoke the body
// through CallAQL — a registry-rooted operation. One Unifier per
// (predicate type, registry); fresh registries get fresh Unifiers
// installed by InstallType at type-declaration time.
type predicateUnifier struct {
	prev       TypeBehavior // previous Behavior (delegate Match/Format/Equal)
	registry   *Registry
	constraint Value // the predicate fn body
	typeName   string
}

// Match runs the predicate against v. The result of `v.Is(Pos)` —
// reached via sigTypeMatches during signature dispatch, by the `is`
// word, and by anything else that asks "is v a Pos?" — must reflect
// the predicate's verdict, not just the lattice walk. Without this
// override, predicate types would only accept values previously
// reparented via typed-def; raw `f 5` for `def f fn [[x:Pos] …]`
// would degrade to a lattice subtype check that always fails because
// Integer (5's parent) is not a descendant of Pos.
//
// Two gates:
//  1. v's declared type must satisfy the predicate's input type.
//     `"hello".Is(Pos)` rejects at this gate — String is not Integer.
//  2. The predicate body returns truthy (RunPredicate's "matched").
//
// Bare type literals (no Data) and carriers (CheckMode abstract
// values) pass through: a type literal is "the type itself", not an
// inhabitant, and carriers are placeholder values whose concreteness
// is asserted at runtime by some other path.
func (p *predicateUnifier) Match(v Value, t *Type) bool {
	if !IsConcrete(v) {
		if p.prev != nil {
			return p.prev.Match(v, t)
		}
		return DefaultBehavior.Match(v, t)
	}
	if p.registry == nil {
		// No registry attached — fall back to lattice walk so behavior
		// is at least no worse than before predicateUnifier existed.
		if p.prev != nil {
			return p.prev.Match(v, t)
		}
		return DefaultBehavior.Match(v, t)
	}
	// Gate 1: input-type compatibility.
	if inputT := PredicateInputType(p.constraint); inputT != nil {
		if !v.Parent.Matches(inputT) {
			return false
		}
	}
	// Gate 2: run the predicate.
	_, matched, err := p.registry.RunPredicate(p.constraint, v)
	if err != nil {
		return false
	}
	return matched
}

func (p *predicateUnifier) Format(v Value) string {
	if p.prev != nil {
		return p.prev.Format(v)
	}
	return DefaultBehavior.Format(v)
}

func (p *predicateUnifier) Equal(a, b Value) bool {
	if p.prev != nil {
		return p.prev.Equal(a, b)
	}
	return DefaultBehavior.Equal(a, b)
}

// Compare opt-out: delegate to prev (a Comparer if previously
// installed). This lets `behave compare/q` on a predicate type still
// work — the chain is `[behave compare wrapper] → predicateUnifier →
// DefaultBehavior`.
func (p *predicateUnifier) Compare(a, b Value) (int, error) {
	if cmp, ok := p.prev.(Comparer); ok {
		return cmp.Compare(a, b)
	}
	return 0, ErrNoComparer
}

// Unify is the capability slot. Two-step:
//  1. Get a candidate from the structural narrowing rule
//     (unifySameOrSubtype). This handles type-literal-vs-concrete and
//     subtype-narrowing without re-entering the LCA walk.
//  2. Run the predicate against the candidate; admit if accepted.
func (p *predicateUnifier) Unify(a, b Value) (Value, *UnifyError) {
	if p.registry == nil {
		return Value{}, &UnifyError{
			Reason: fmt.Sprintf("predicate type %s has no registry attached", p.typeName),
		}
	}

	// Get a candidate. unifySameOrSubtype is the right primitive: it
	// handles the "type literal narrows to concrete" rule (Pos-literal
	// vs 5 → 5) and the subtype-narrowing rule (Pos-value vs Integer-
	// value → take the Pos value as narrower) without recursing back
	// through dispatchUnifier.
	candidate, err := unifySameOrSubtype(a, b)
	if err != nil {
		return Value{}, err
	}

	// Concrete-only check: the predicate is meaningful for values with
	// Data, not bare type literals. When the candidate is a type
	// literal (e.g. Unify(Pos-literal, Pos-literal)) just admit it —
	// the structural rule already established compatibility.
	if !IsConcrete(candidate) {
		return candidate, nil
	}

	_, matched, perr := p.registry.RunPredicate(p.constraint, candidate)
	if perr != nil {
		return Value{}, &UnifyError{
			Reason: fmt.Sprintf("predicate %s: %s", p.typeName, perr.Error()),
			A:      a,
			B:      b,
		}
	}
	if !matched {
		return Value{}, &UnifyError{
			Reason: fmt.Sprintf("value does not satisfy predicate %s", p.typeName),
			A:      a,
			B:      b,
		}
	}
	return candidate, nil
}

// installPredicateUnifier attaches a predicateUnifier to def, wrapping
// any existing Behavior. Called by InstallType when minting a predicate
// type so the constraint runs at every Unify call site.
func installPredicateUnifier(def *Type, constraint Value, r *Registry, name string) {
	def.Behavior = &predicateUnifier{
		prev:       def.Behavior,
		registry:   r,
		constraint: constraint,
		typeName:   name,
	}
}
