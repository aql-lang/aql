package eng

import "strings"

// Comparer implementations for the kernel scalar types. Each embeds
// defaultBehavior so Match/Format/Equal stay at the kernel default;
// the only addition is the Compare method, which makes the type
// orderable for `lt`/`gt`/`lte`/`gte`/`sort`. Descendants of a
// scalar (e.g. Integer < Number, EmptyString < String) inherit the
// Comparer via the lattice walk in CompareValues — no per-subtype
// registration needed.

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
}
