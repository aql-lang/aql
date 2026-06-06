package eng

import (
	"errors"
	"fmt"
	"math"
)

// ErrNoComparer is returned by Comparer.Compare implementations that
// hold a placeholder slot (e.g. a wrapped Behavior whose user-defined
// comparator body is empty). CompareValues recognises it and continues
// the parent-chain walk, treating the Behavior as if it didn't satisfy
// the Comparer interface at all. This lets a single Behavior wrapper
// carry multiple optional capabilities (compare / canon / …) where
// only some are installed without prematurely terminating dispatch.
var ErrNoComparer = errors.New("eng: no comparer in this Behavior")

// CompareValues returns -1, 0, or 1 for natural ordering of two values.
//
// Dispatch is type-driven: the compare logic for a pair of values lives
// on the type's Behavior (Comparer capability), not in a switch ladder
// here. The dispatch routes through the lowest common ancestor of the
// two operand VTypes — e.g. Integer-vs-Float walks up to Number,
// which owns the numeric ordering; Integer-vs-String walks up to
// Scalar, which owns the cross-branch ordering; Date-vs-Date stays
// on Date.
//
// When the lattice walk finds no Comparer, the order falls to the
// unified lattice Rank: every type carries one Rank giving a total
// order over the whole lattice (see typetable.go::builtinDecls), so
// any two values of different types order by Rank alone. Two values
// of equal Rank — the identical type, or two user/external types that
// inherit one builtin's Rank — break to compareTypes (name/id), then
// to size, then to an element-wise structural comparison. Distinct
// values never collapse to 0.
//
// DepScalar values (type-level constraints) flow through this same
// path.
func CompareValues(a, b Value) (int, error) {
	n, _, err := compareValuesClassified(a, b)
	return n, err
}

// compareValuesClassified is CompareValues plus a report of HOW the
// order was decided. viaFamily is true when the result came from a
// same-family Comparer (a Comparer on a lattice node other than the
// cross-family catch-all TScalar) OR the two operands are the exact same
// type. It is false when the pair only orders via the cross-family
// scalar catch-all (scalarCompareBehavior@TScalar) or the lattice Rank
// fallback — i.e. values of unrelated families.
//
// The restricted ordering words (cmp / lt / lte / gt / gte) accept a
// pair only when viaFamily is true; tcmp and every internal caller use
// CompareValues, which keeps the full total order over all values.
func compareValuesClassified(a, b Value) (n int, viaFamily bool, err error) {
	// A bare type literal IS its lattice node; its type-for-comparison
	// is itself, not its Parent (now the supertype).
	aType := ValueType(a)
	bType := ValueType(b)
	if aType == nil || bType == nil {
		return 0, false, fmt.Errorf("cannot compare values with nil type")
	}
	sameType := aType.Equal(bType)
	for t := lowestCommonAncestor(aType, bType); t != nil; t = t.Parent {
		cmp, ok := t.Behavior.(Comparer)
		if !ok {
			continue
		}
		res, e := cmp.Compare(a, b)
		if errors.Is(e, ErrNoComparer) {
			// Wrapper Behavior signalled "I satisfy Comparer
			// structurally but have no body installed" (DepScalar
			// opt-out; the Time comparer declining instant-vs-duration;
			// …) — keep walking the parent chain.
			continue
		}
		// Resolved by a Comparer. It counts as a same-family resolution
		// unless it is the cross-family catch-all on TScalar applied to
		// two different types (Integer-vs-String, Boolean-vs-Number).
		return res, sameType || !t.Equal(TScalar), e
	}
	// No Comparer in the shared lattice — order by the type lattice.
	// compareTypes is a total order on *Type (Rank, then depth, name,
	// id), so any two values of distinct types resolve here. Reached
	// only for cross-branch pairs, so this is never a family resolution.
	if c := compareTypes(aType, bType); c != 0 {
		return c, false, nil
	}
	// Identical type — order by value: size, then element-wise
	// structure, so distinct values never collapse to 0.
	if sa, sb := SizeOf(a), SizeOf(b); sa != sb {
		return cmpInt(sa, sb), sameType, nil
	}
	res, e := compareStructural(a, b)
	return res, sameType, e
}

// OrderComparable reports whether a and b may be compared by the
// family-restricted ordering words (cmp / lt / lte / gt / gte): true
// when they are the same type or share a same-family Comparer, false
// when they only order via the cross-type total order (use tcmp).
func OrderComparable(a, b Value) bool {
	_, viaFamily, err := compareValuesClassified(a, b)
	return err == nil && viaFamily
}

// orderedCompare runs the family-restricted comparison behind the
// ordering words. It returns the -1/0/1 result, or an [aql/incomparable]
// error when the pair only orders via the cross-type total order — the
// caller should reach for tcmp instead.
func orderedCompare(op string, a, b Value) (int, error) {
	n, viaFamily, err := compareValuesClassified(a, b)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	if !viaFamily {
		return 0, incomparableError(op, a, b)
	}
	return n, nil
}

// incomparableError is the [aql/incomparable] error the restricted
// ordering words raise for cross-family operands.
func incomparableError(op string, a, b Value) error {
	return &AqlError{
		Code: "incomparable",
		Detail: fmt.Sprintf("%s: cannot order %s and %s",
			op, ValueType(a).String(), ValueType(b).String()),
		Hint: "different types with no shared ordering; use tcmp for a cross-type total order",
	}
}

// lowestCommonAncestor returns the closest type that is an ancestor
// of both a and b on the parent chain. Returns nil only if a and b
// share no common ancestor (the type tables guarantee a single root,
// so in practice this returns at worst the root type).
func lowestCommonAncestor(a, b *Type) *Type {
	// Compared via Type.Equal (pointer OR canonical ID): a type
	// literal is a by-value copy of its node, so ValueType() may hand
	// us a copy whose address differs from the canonical node's,
	// while ID-less ad-hoc types still match by pointer.
	var aChain []*Type
	for t := a; t != nil; t = t.Parent {
		aChain = append(aChain, t)
	}
	for t := b; t != nil; t = t.Parent {
		for _, at := range aChain {
			if at.Equal(t) {
				return t
			}
		}
	}
	return nil
}

// ExactEqual returns true if two values are exactly equal.
// For scalars (integer, string, boolean, atom, none): compares by value.
// For types: compares structurally via ValuesEqual.
// For non-scalars (list, map): compares by identity (same container).
func ExactEqual(a, b Value) bool {
	// none == none
	if ValueType(a).Equal(TNone) && ValueType(b).Equal(TNone) {
		return true
	}

	// DepScalar pre-empts the ConformsTo(TNumber)/ConformsTo(TString)/...
	// dispatch below: the lattice override would otherwise route
	// DepInteger payloads into AsNumber and silently compare zero
	// values. Two DepScalars are equal iff their constraint shapes
	// match (delegated through ValuesEqual).
	if a.IsDepScalar() || b.IsDepScalar() {
		if !a.IsDepScalar() || !b.IsDepScalar() {
			return false
		}
		return a.Parent.Equal(b.Parent) && ValuesEqual(a, b)
	}

	// Types: structural comparison.
	if IsTypeBody(a) && IsTypeBody(b) {
		return a.Parent.Equal(b.Parent) && ValuesEqual(a, b)
	}

	// Scalars: compare by value.
	if a.Parent.ConformsTo(TNumber) && b.Parent.ConformsTo(TNumber) {
		_as9, _ := AsNumber(a)
		_as8, _ := AsNumber(b)
		return _as9 == _as8
	}
	if a.Parent.ConformsTo(TString) && b.Parent.ConformsTo(TString) {
		_as11, _ := AsString(a)
		_as10, _ := AsString(b)
		return _as11 == _as10
	}
	if a.Parent.ConformsTo(TBoolean) && b.Parent.ConformsTo(TBoolean) {
		_as13, _ := AsBoolean(a)
		_as12, _ := AsBoolean(b)
		return _as13 == _as12
	}
	if a.Parent.Equal(TAtom) && b.Parent.Equal(TAtom) {
		_as15, _ := AsAtom(a)
		_as14, _ := AsAtom(b)
		return _as15 == _as14
	}

	// Non-scalars: identity comparison — both values must refer to the
	// same underlying container.
	if a.Parent.Equal(TList) && b.Parent.Equal(TList) {
		return sameContainer(a.Data, b.Data)
	}
	if a.Parent.Equal(TMap) && b.Parent.Equal(TMap) {
		return sameContainer(a.Data, b.Data)
	}

	return false
}

// sameContainer reports whether two non-scalar payloads refer to the
// same underlying container — the identity test behind ExactEqual for
// lists and maps. A MapPayload identifies by its *OrderedMap pointer;
// a ListPayload by the backing array of its element slice, so a value
// dup'd from a list is identical to its source while two separate
// literals are not. Payloads with no aliasable identity (table data,
// materializers, …) are never equal here.
//
// It must not apply `==` to a Payload directly: ListPayload holds a
// slice and is therefore not a comparable type — a bare
// `a.Data == b.Data` panics at runtime.
func sameContainer(a, b Payload) bool {
	switch av := a.(type) {
	case MapPayload:
		bv, ok := b.(MapPayload)
		return ok && av.M == bv.M
	case ListPayload:
		bv, ok := b.(ListPayload)
		if !ok {
			return false
		}
		// An empty list has no backing array to alias by — treat all
		// empty lists as the single empty list.
		if len(av.Elems) == 0 || len(bv.Elems) == 0 {
			return len(av.Elems) == len(bv.Elems)
		}
		return &av.Elems[0] == &bv.Elems[0]
	default:
		return false
	}
}

// DeepEqual returns true if two values are deeply equal.
// Traverses lists and maps depth-first comparing all leaf values.
func DeepEqual(a, b Value) bool {
	// none
	if ValueType(a).Equal(TNone) && ValueType(b).Equal(TNone) {
		return true
	}

	// DepScalar pre-empts scalar dispatch — see ExactEqual for the
	// reasoning. Two DepScalars compare equal iff their type and
	// constraint payload match.
	if a.IsDepScalar() || b.IsDepScalar() {
		if !a.IsDepScalar() || !b.IsDepScalar() {
			return false
		}
		return a.Parent.Equal(b.Parent) && ValuesEqual(a, b)
	}

	// Scalars.
	if a.Parent.ConformsTo(TNumber) && b.Parent.ConformsTo(TNumber) {
		_as17, _ := AsNumber(a)
		_as16, _ := AsNumber(b)
		return _as17 == _as16
	}
	if a.Parent.ConformsTo(TString) && b.Parent.ConformsTo(TString) {
		_as19, _ := AsString(a)
		_as18, _ := AsString(b)
		return _as19 == _as18
	}
	if a.Parent.ConformsTo(TBoolean) && b.Parent.ConformsTo(TBoolean) {
		_as21, _ := AsBoolean(a)
		_as20, _ := AsBoolean(b)
		return _as21 == _as20
	}
	if a.Parent.Equal(TAtom) && b.Parent.Equal(TAtom) {
		_as23, _ := AsAtom(a)
		_as22, _ := AsAtom(b)
		return _as23 == _as22
	}

	// Lists: same length, each element deeply equal.
	if a.Parent.Equal(TList) && b.Parent.Equal(TList) {
		aElems, aErr := AsMutableList(a)
		bElems, bErr := AsMutableList(b)
		if aErr != nil || bErr != nil {
			// Typed lists, table types, etc. — compare structurally via String().
			return a.String() == b.String()
		}
		if len(aElems) != len(bElems) {
			return false
		}
		for i := range aElems {
			if !DeepEqual(aElems[i], bElems[i]) {
				return false
			}
		}
		return true
	}

	// Maps: same keys, each value deeply equal.
	if a.Parent.Equal(TMap) && b.Parent.Equal(TMap) {
		aMap, aErr := AsMutableMap(a)
		bMap, bErr := AsMutableMap(b)
		if aErr != nil || bErr != nil {
			// Record types, typed maps — compare structurally via String().
			return a.String() == b.String()
		}
		if aMap.Len() != bMap.Len() {
			return false
		}
		for _, key := range aMap.Keys() {
			aVal, _ := aMap.Get(key)
			bVal, bHas := bMap.Get(key)
			if !bHas {
				return false
			}
			if !DeepEqual(aVal, bVal) {
				return false
			}
		}
		return true
	}

	// Different types or unsupported — not equal.
	return false
}

// The comparison-word registrations (lt / gt / lte / gte / eq /
// neq / deq / between) live in lang/go/engine/native_compare.go. The
// handlers and MakeDepScalarSig helper are exported eng primitives.

// The ordering words lt / gt / lte / gte / cmp are family-restricted:
// they compare only same-type values, or values a shared same-family
// Comparer can handle (Integer-vs-Float, two Dates, …). A cross-family
// pair (Integer-vs-String, List-vs-Map) raises [aql/incomparable] and
// directs the caller to tcmp, which keeps the full cross-type total
// order. The guard lives in orderedCompare; tcmp (TcmpHandler) bypasses
// it.

// numericUnordered reports whether a relational comparison of a and b is
// IEEE-754 *unordered*: both are concrete numbers and at least one is
// NaN. The ordering words lt / lte / gt / gte all yield false in that
// case (NaN is neither less, equal, nor greater). Cross-family pairs
// (e.g. Float-vs-String) are NOT unordered here — they fall through to
// the family guard, which raises [aql/incomparable]. The total-order
// words cmp / tcmp / sort deliberately bypass this and give NaN a defined
// slot (see numberCompareBehavior.Compare).
func numericUnordered(a, b Value) bool {
	return isConcreteNumber(a) && isConcreteNumber(b) && (isNaNValue(a) || isNaNValue(b))
}

func isConcreteNumber(v Value) bool {
	return !v.IsDepScalar() && IsConcrete(v) && ValueType(v).ConformsTo(TNumber)
}

func isNaNValue(v Value) bool {
	if !isConcreteNumber(v) {
		return false
	}
	f, err := AsNumber(v)
	return err == nil && math.IsNaN(f)
}

func LtHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if numericUnordered(args[0], args[1]) {
		return []Value{NewBoolean(false)}, nil
	}
	cmp, err := orderedCompare("lt", args[1], args[0])
	if err != nil {
		return nil, err
	}
	return []Value{NewBoolean(cmp < 0)}, nil
}

func GtHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if numericUnordered(args[0], args[1]) {
		return []Value{NewBoolean(false)}, nil
	}
	cmp, err := orderedCompare("gt", args[1], args[0])
	if err != nil {
		return nil, err
	}
	return []Value{NewBoolean(cmp > 0)}, nil
}

func LteHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if numericUnordered(args[0], args[1]) {
		return []Value{NewBoolean(false)}, nil
	}
	cmp, err := orderedCompare("lte", args[1], args[0])
	if err != nil {
		return nil, err
	}
	return []Value{NewBoolean(cmp <= 0)}, nil
}

func GteHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if numericUnordered(args[0], args[1]) {
		return []Value{NewBoolean(false)}, nil
	}
	cmp, err := orderedCompare("gte", args[1], args[0])
	if err != nil {
		return nil, err
	}
	return []Value{NewBoolean(cmp >= 0)}, nil
}

// CmpHandler implements `cmp` — a three-way comparison restricted to
// same-family operands. `a b cmp` returns -1 when a sorts before b, 0
// when they tie, and 1 when a sorts after b, using the same family
// ordering as lt / gt. Cross-family operands raise [aql/incomparable];
// use tcmp for a cross-type total order. The result is normalised to
// its sign, so a custom `behave compare` body that returns a nonzero
// magnitude other than ±1 still yields exactly -1 / 0 / 1.
func CmpHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := orderedCompare("cmp", args[1], args[0])
	if err != nil {
		return nil, err
	}
	return []Value{NewInteger(int64(signOf(cmp)))}, nil
}

// TcmpHandler implements `tcmp` — the total-order three-way comparison.
// Unlike cmp, it compares ANY two values via the unified lattice order
// (the same order sort and the collection words use), returning -1 / 0
// / 1. Use it when you deliberately want cross-type ordering that cmp
// refuses (e.g. `1 tcmp "a"`).
func TcmpHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := CompareValues(args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("tcmp: %w", err)
	}
	return []Value{NewInteger(int64(signOf(cmp)))}, nil
}

// signOf normalises an arbitrary comparison magnitude to -1 / 0 / 1.
func signOf(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

func EqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(ExactEqual(args[0], args[1]))}, nil
}

func NeqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(!ExactEqual(args[0], args[1]))}, nil
}

func DeqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(DeepEqual(args[0], args[1]))}, nil
}
