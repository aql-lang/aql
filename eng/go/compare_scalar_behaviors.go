package eng

import "strings"

// Comparer implementations for the kernel scalar types and Word. Each embeds
// defaultBehavior so Match/Format/Equal stay at the kernel default;
// the only addition is the Compare method, which makes the type
// orderable for `lt`/`gt`/`lte`/`gte`/`sort`. Descendants of a
// scalar (e.g. Integer < Number, EmptyString < String) inherit the
// Comparer via the lattice walk in CompareValues — no per-subtype
// registration needed.
//
// The Scalar root itself carries scalarCompareBehavior — the
// cross-branch comparator. It orders values from different branches
// (e.g. Integer-vs-String) by a fixed branch precedence and is
// reached only when the LCA walk finds no branch-level Comparer.

// numberCompareBehavior compares any pair of values rooted under
// Scalar/Number (Integer, Decimal, or their dep variants) via the
// canonical float promotion in AsNumber.
type numberCompareBehavior struct{ defaultBehavior }

// formatDelegate marks every scalar Comparer Behavior in this file
// as "Format is the kernel default". Value.String walks the parent
// chain looking for a *real* Format override; tagged behaviours are
// skipped so the kernel renderer runs unchanged. See
// formatDelegatesToDefault in value.go for the dispatch logic.
func (numberCompareBehavior) formatDelegate()  {}
func (stringCompareBehavior) formatDelegate()  {}
func (booleanCompareBehavior) formatDelegate() {}
func (atomCompareBehavior) formatDelegate()    {}
func (scalarCompareBehavior) formatDelegate()  {}
func (wordCompareBehavior) formatDelegate()    {}

// litVsConcreteOrder applies the type-literal-first rule at the top
// of every family Comparer: when exactly one side is a bare type
// literal (Data==nil, !Carrier) and the other carries a concrete
// payload, the type literal sorts first.
//
// This places every type literal strictly BELOW every concrete value
// in the same family — including the family's zero-valued inhabitant
// (`Integer < 0`, `String < ''`, `Boolean < false`). Without this
// rule the family Comparer would read the type literal's nil payload
// as a zero value and tie with `0` / `''` / `false`, putting two
// distinct lattice nodes in the same equivalence class and
// violating the strict-total-order property.
//
// Returns (sign, true) when exactly one side is a literal. Returns
// (0, false) when both are literals or both concrete — the caller
// then picks the appropriate branch (litVsLitOrder for both-
// literal, the family's value compare for both-concrete).
func litVsConcreteOrder(a, b Value) (int, bool) {
	aLit := a.Data == nil && !a.Carrier
	bLit := b.Data == nil && !b.Carrier
	if aLit && !bLit {
		return -1, true
	}
	if !aLit && bLit {
		return 1, true
	}
	return 0, false
}

// litVsLitOrder orders two bare type literals by lattice node
// position via compareTypes (Rank → depth → name → ID). Used by the
// family Comparers when both inputs are type literals — in that
// case the value-payload compare would tie (both read as zero) so
// we fall back to lattice identity.
//
// The two values are by-value copies of their lattice nodes, so &a
// and &b ARE the nodes for compareTypes' purposes.
func litVsLitOrder(a, b Value) int {
	return compareTypes(&a, &b)
}

func (numberCompareBehavior) Compare(a, b Value) (int, error) {
	// DepScalar values share a numeric Parent (Integer, Decimal,
	// Number) with concrete scalars but carry a DepScalarInfo payload
	// rather than a number — they have no numeric ordering. Signal
	// "I don't apply" so CompareValues falls through to the lattice
	// + structural compare (which tie-breaks on canonical form).
	if a.IsDepScalar() || b.IsDepScalar() {
		return 0, ErrNoComparer
	}
	if c, ok := litVsConcreteOrder(a, b); ok {
		return c, nil
	}
	if a.Data == nil && b.Data == nil {
		return litVsLitOrder(a, b), nil
	}
	af, _ := AsNumber(a)
	bf, _ := AsNumber(b)
	switch {
	case af < bf:
		return -1, nil
	case af > bf:
		return 1, nil
	default:
		return 0, nil
	}
}

// stringCompareBehavior orders strings lexicographically (UTF-8 byte
// order via strings.Compare).
type stringCompareBehavior struct{ defaultBehavior }

func (stringCompareBehavior) Compare(a, b Value) (int, error) {
	if a.IsDepScalar() || b.IsDepScalar() {
		return 0, ErrNoComparer
	}
	if c, ok := litVsConcreteOrder(a, b); ok {
		return c, nil
	}
	if a.Data == nil && b.Data == nil {
		return litVsLitOrder(a, b), nil
	}
	as, _ := AsString(a)
	bs, _ := AsString(b)
	return strings.Compare(as, bs), nil
}

// booleanCompareBehavior orders booleans false < true.
type booleanCompareBehavior struct{ defaultBehavior }

func (booleanCompareBehavior) Compare(a, b Value) (int, error) {
	if a.IsDepScalar() || b.IsDepScalar() {
		return 0, ErrNoComparer
	}
	if c, ok := litVsConcreteOrder(a, b); ok {
		return c, nil
	}
	if a.Data == nil && b.Data == nil {
		return litVsLitOrder(a, b), nil
	}
	ab, _ := AsBoolean(a)
	bb, _ := AsBoolean(b)
	switch {
	case ab == bb:
		return 0, nil
	case !ab:
		return -1, nil
	default:
		return 1, nil
	}
}

// atomCompareBehavior orders atoms lexicographically by name.
type atomCompareBehavior struct{ defaultBehavior }

func (atomCompareBehavior) Compare(a, b Value) (int, error) {
	if a.IsDepScalar() || b.IsDepScalar() {
		return 0, ErrNoComparer
	}
	if c, ok := litVsConcreteOrder(a, b); ok {
		return c, nil
	}
	if a.Data == nil && b.Data == nil {
		return litVsLitOrder(a, b), nil
	}
	as, _ := AsAtom(a)
	bs, _ := AsAtom(b)
	return strings.Compare(as, bs), nil
}

// wordCompareBehavior orders Word values lexicographically by their
// rendered form. Word — like String and Atom — is a name-like type
// whose deliberate comparison basis is the text itself; this is one
// of the few places Value.String is used as an ordering key.
type wordCompareBehavior struct{ defaultBehavior }

func (wordCompareBehavior) Compare(a, b Value) (int, error) {
	if c, ok := litVsConcreteOrder(a, b); ok {
		return c, nil
	}
	if a.Data == nil && b.Data == nil {
		return litVsLitOrder(a, b), nil
	}
	return strings.Compare(a.String(), b.String()), nil
}

// scalarCompareBehavior is the Comparer on the abstract Scalar root.
// It gives cross-family scalar pairs (e.g. Integer-vs-String) a defined
// order by their unified lattice Rank — the same key CompareValues'
// fallback uses, so the Scalar root needs no private branch ladder and,
// since the bare Scalar root literal carries a real Rank, it never has
// to bail.
//
// CompareValues reaches it only for cross-family pairs: the LCA walk
// stops at a branch root's own Comparer (Number/String/Boolean/Atom)
// for any same-family pair. The one same-Rank pair that does reach here
// is Path-vs-Path — Path has no Comparer of its own — so two paths fall
// through to Scalar, where comparePaths orders them by segment count
// (shortest first), then segment by segment in reverse lexical order,
// then relative before absolute.
type scalarCompareBehavior struct{ defaultBehavior }

func (scalarCompareBehavior) Compare(a, b Value) (int, error) {
	// Cross-family scalar pairs (e.g. Boolean-vs-Integer) and the
	// Path-vs-Path case both land here. For cross-family pairs the
	// Rank discriminator must own the result — applying the type-
	// literal-first rule here would override Rank (e.g. `true cmp
	// Integer` should stay -1 via Rank, not flip to +1 because
	// Integer is a literal). The Path-vs-Path-literal case is
	// settled by comparePaths' own litVsConcrete check.
	if c := compareTypes(ValueType(a), ValueType(b)); c != 0 {
		return c, nil
	}
	// Same scalar type — Path-vs-Path orders by segment count, then
	// segment by segment, then absolute before relative.
	return comparePaths(a, b), nil
}

// comparePaths orders two Path values by three keys in turn: shorter
// paths (fewer segments) sort first, then segment by segment in
// reverse lexical order, then a relative path before an absolute one.
// scalarCompareBehavior routes the Path-vs-Path case here.
func comparePaths(a, b Value) int {
	// DepScalar pair: order by canonical form (forward lex on the
	// rendered "(base op bound)" string). They have no numeric
	// ordering of their own — their Comparers signal ErrNoComparer
	// so we land here — and the spec's "tie-breaks on canonical
	// form" rule wants forward lex, not the Path-reverse rule.
	if a.IsDepScalar() && b.IsDepScalar() {
		return strings.Compare(a.String(), b.String())
	}
	// Type-literal-first rule for the Path family: the bare `Path`
	// type literal sorts strictly below every concrete path value.
	// Done here (not in scalarCompareBehavior) so the rule applies
	// only inside the Path family — cross-family scalar pairs route
	// through scalarCompareBehavior's Rank-only path.
	if c, ok := litVsConcreteOrder(a, b); ok {
		return c
	}
	ap, aerr := AsPath(a)
	bp, berr := AsPath(b)
	if aerr != nil || berr != nil {
		// Two type literals (both Data==nil) or two non-Path
		// concretes — fall back to lattice identity then to
		// rendered form.
		if a.Data == nil && b.Data == nil {
			return litVsLitOrder(a, b)
		}
		return strings.Compare(a.String(), b.String())
	}
	switch {
	case len(ap.Parts) < len(bp.Parts):
		return -1
	case len(ap.Parts) > len(bp.Parts):
		return 1
	}
	// Equal segment count — compare segment by segment in reverse
	// lexical order. Comparing the parts directly, rather than the
	// "/"-joined render, keeps the separator byte from skewing the
	// order.
	for i := range ap.Parts {
		if c := strings.Compare(bp.Parts[i], ap.Parts[i]); c != 0 {
			return c
		}
	}
	// Same size and segments — a relative path sorts before an
	// absolute one.
	switch {
	case !ap.Abs && bp.Abs:
		return -1
	case ap.Abs && !bp.Abs:
		return 1
	default:
		return 0
	}
}

// init attaches the scalar Comparers to their owning kernel types.
// The Builtin TypeTable has been populated by the typetable.go init
// at this point (same package, lexicographic file order isn't
// guaranteed but Builtin is a package-level var initialised via the
// init() in typetable.go which Go runs before this file's init
// unless there's a dependency — see below).
//
// Run order: package vars are initialised before any init() runs,
// so Builtin is constructed (and TNumber/TString/etc. are non-nil)
// before this init fires. We assign Behaviors directly on the *Type
// values; CompareValues consults Behavior at call time.
func init() {
	TNumber.Behavior = numberCompareBehavior{}
	TString.Behavior = stringCompareBehavior{}
	TBoolean.Behavior = booleanCompareBehavior{}
	TAtom.Behavior = atomCompareBehavior{}
	TScalar.Behavior = scalarCompareBehavior{}
	TWord.Behavior = wordCompareBehavior{}
}
