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

func (numberCompareBehavior) Compare(a, b Value) (int, error) {
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
	as, _ := AsString(a)
	bs, _ := AsString(b)
	return strings.Compare(as, bs), nil
}

// booleanCompareBehavior orders booleans false < true.
type booleanCompareBehavior struct{ defaultBehavior }

func (booleanCompareBehavior) Compare(a, b Value) (int, error) {
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
	ap, aerr := AsPath(a)
	bp, berr := AsPath(b)
	if aerr != nil || berr != nil {
		// Not a Path pair after all — fall back to rendered order.
		return strings.Compare(b.String(), a.String())
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
