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
	// Concrete flex nodes order by content exactly like their immutable
	// family (flexness is a mutability mode, not value identity), so a
	// deq-equal flex/plain pair also cmp-equals. Bare type literals are
	// NOT normalised — `FlexList cmp List` stays a Rank ordering.
	if IsConcrete(a) {
		aType = nodeFamily(aType)
	}
	if IsConcrete(b) {
		bType = nodeFamily(bType)
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

// OrderingReturnsFn builds the check-mode ReturnsFn for the family-
// restricted ordering words (lt / gt / lte / gte / cmp). When BOTH
// operands are statically determinate it runs the (pure) handler to
// detect a cross-family pair statically — Integer vs String can never be
// ordered, for any values — and emits an [aql/incomparable] error
// diagnostic, so the mismatch is caught at check time instead of slipping
// through as a runtime error. With a non-determinate operand the runtime
// family is unknown, so nothing is flagged. The result carrier (Boolean
// for the predicates, Integer for cmp) is returned either way so analysis
// continues.
func OrderingReturnsFn(handler Handler, result *Type) ReturnsFunc {
	return func(args []Value, r *Registry) []Value {
		if r != nil && len(args) == 2 && orderingDeterminate(args[0]) && orderingDeterminate(args[1]) {
			if _, err := handler(args, nil, nil, r); err != nil {
				var ae *AqlError
				if errors.As(err, &ae) && ae.Code == "incomparable" {
					pos := args[0].Pos
					r.Check.AddDiagnostic(CheckDiagnostic{
						Code:   "incomparable",
						Detail: ae.Detail,
						Row:    pos.Row,
						Col:    pos.Col,
					})
				}
			}
		}
		return []Value{NewCarrier(result)}
	}
}

// orderingDeterminate reports whether v's ordering family is fixed at
// check time, so the ordering handler may be run on it to detect a
// cross-family pair soundly. A concrete value qualifies (its family is
// its own). `none` also qualifies even though it is non-concrete
// (Data==nil): None is a SINGLETON type, so a non-dynamic none-shaped
// value is determinately `none` at runtime — running the handler on it
// yields exactly the runtime result (no false positive). A dynamic /
// gradual carrier never qualifies: its family is genuinely unknown.
func orderingDeterminate(v Value) bool {
	if v.Dynamic {
		return false
	}
	return IsConcrete(v) || IsNoneShape(v)
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
	// Depth-aligned walk: lift the deeper node to its sibling's depth, then
	// climb both in lockstep until the chains meet — O(d) instead of the old
	// O(d_a · d_b) chain-vs-chain scan. typeDepth reads the cached Type.Depth
	// (O(1) for registered types), so the alignment is O(1) too. Compared via
	// Type.Equal (pointer OR canonical ID): a type literal is a by-value copy
	// of its node, so ValueType() may hand us a copy whose address differs
	// from the canonical node's, while ID-less ad-hoc types still match by
	// pointer.
	if a == nil || b == nil {
		return nil
	}
	da, db := typeDepth(a), typeDepth(b)
	for da > db {
		a = a.Parent
		da--
	}
	for db > da {
		b = b.Parent
		db--
	}
	for a != nil && b != nil {
		if a.Equal(b) {
			return a
		}
		a = a.Parent
		b = b.Parent
	}
	return nil
}

// ExactEqual returns true if two values are exactly equal.
// For scalars (integer, string, boolean, atom, none): compares by value.
// For types: compares structurally via ValuesEqual.
// For non-scalars (list, map): compares by identity (same container).
// scalarSemanticEqual is the scalar-leaf equality dispatch shared by
// ExactEqual (eq) and DeepEqual (deq): a scalar is identified by
// content, not container identity, so the two words must agree on
// every scalar pair. handled=false means the pair is not a
// scalar-semantic pair and the caller's own dispatch continues.
//
// Two clauses:
//
//  1. Same-Parent scalar leaves with a custom Behavior (Bytes, the
//     Time family, plugin scalars) compare by Behavior.Equal. The
//     same-Parent gate keeps this to matching leaf types, and
//     ConformsTo(TScalar) keeps reference-backed non-scalars
//     (List/Map/Object/Tensor/…) on the callers' identity /
//     structural paths. The dispatch is the one ValuesEqual already
//     uses.
//
//  2. Cross-node, NESTED scalar subtype: a `refine`-newtype value
//     (def B (refine Bytes); def x:B …) carries the minted node, not
//     the base, so the same-Parent gate misses `x eq <plain Bytes>`.
//     When one operand's type is an ancestor of the other's — i.e.
//     their lowest common ancestor IS one of the two parents — they
//     share a base scalar leaf, so compare by that base's Comparer
//     (a newtype's wrapper Behavior delegates Compare to the base).
//     eq/deq then agree with cmp on a tagged subtype, as the ordering
//     LCA walk in CompareValues already does. The NESTED guard is
//     load-bearing: it admits newtype-vs-base but EXCLUDES sibling
//     leaves (Date vs DateTime — LCA Time, neither parent — which
//     timeCompareBehavior would otherwise rank as chronologically
//     equal), keeping those on the callers' identity paths,
//     unchanged.
//
// Callers must have dispatched the by-value scalar families
// (Number/String/Boolean/Atom) and DepScalar payloads before calling,
// so those never reach here.
func scalarSemanticEqual(a, b Value) (equal, handled bool) {
	if a.Parent == b.Parent && a.Parent != nil && a.Parent.ConformsTo(TScalar) &&
		a.Parent.Behavior != nil && a.Parent.Behavior != DefaultBehavior {
		return a.Parent.Behavior.Equal(a, b), true
	}
	if a.Parent != b.Parent {
		if lca := lowestCommonAncestor(a.Parent, b.Parent); lca != nil &&
			!lca.Equal(TScalar) && lca.ConformsTo(TScalar) &&
			(lca.Equal(a.Parent) || lca.Equal(b.Parent)) {
			if cmp, ok := lca.Behavior.(Comparer); ok {
				if res, e := cmp.Compare(a, b); e == nil {
					return res == 0, true
				}
			}
		}
	}
	return false, false
}

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
		// An arbitrary-precision operand is compared exactly via apd
		// (cross-leaf magnitude: 1 == 0d1 == 1.0). A NaN/non-finite Float
		// fails toDecimalExact, so it is never equal — preserving nan≠nan.
		if numIsBig(a) || numIsBig(b) {
			ar, aok := toRatExact(a)
			br, bok := toRatExact(b)
			return aok && bok && ar.Cmp(br) == 0
		}
		// Two int64-backed Integers compare EXACTLY as int64; the float64
		// projection below rounds magnitudes above 2^53 and would report
		// distinct large integers as equal. A Float on either side keeps
		// the float comparison (cross-leaf magnitude: 1 == 1.0).
		if a.Parent.ConformsTo(TInteger) && b.Parent.ConformsTo(TInteger) {
			ai, _ := AsInteger(a)
			bi, _ := AsInteger(b)
			return ai == bi
		}
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
	if a.Parent.ConformsTo(TAtom) && b.Parent.ConformsTo(TAtom) {
		_as15, _ := AsAtom(a)
		_as14, _ := AsAtom(b)
		return _as15 == _as14
	}

	// Other scalar value-types (Bytes, the Time family, …) compare by
	// their type's semantic equality — see scalarSemanticEqual (shared
	// with DeepEqual so eq and deq agree on every scalar leaf).
	if equal, handled := scalarSemanticEqual(a, b); handled {
		return equal
	}

	// Non-scalars: identity comparison — both values must refer to the
	// same underlying container. Flexness is part of identity here:
	// nodeFamily-normalised comparison would still fail on container
	// identity (a flex copy is a fresh container), but normalising the
	// family keeps the dispatch uniform for flex-vs-flex pairs.
	if nodeFamily(a.Parent).Equal(TList) && nodeFamily(b.Parent).Equal(TList) {
		return a.Parent.Equal(b.Parent) && sameContainer(a.Data, b.Data)
	}
	if nodeFamily(a.Parent).Equal(TMap) && nodeFamily(b.Parent).Equal(TMap) {
		return a.Parent.Equal(b.Parent) && sameContainer(a.Data, b.Data)
	}
	// XML elements: identity like Map/List — `eq` is container identity
	// (a flex copy / a fresh literal is not eq to its source), while `deq`
	// (DeepEqual) is structural. See design/XML-LITERAL.0.md §5.
	if IsXmlValue(a) && IsXmlValue(b) {
		return a.Parent.Equal(b.Parent) && sameContainer(a.Data, b.Data)
	}
	// Flat instances (class + Resource/Entity) identify by their
	// underlying field map — the same aliasing rule as Map: two bindings
	// to one instance are eq, two structurally-equal instances are not
	// (that's deq). A shared Fields pointer is the identity key; a
	// class/resource cross-pair can never share one, so it stays unequal.
	if af, _, aok := flatInstanceParts(a); aok {
		bf, _, bok := flatInstanceParts(b)
		return bok && af != nil && af == bf
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
	case *FlexListData:
		// Pointer identity — simpler and stronger than the
		// backing-array probe: the store pointer IS the container.
		bv, ok := b.(*FlexListData)
		return ok && av == bv
	case *FlexXmlData:
		// Pointer identity — the FlexXml store pointer IS the container.
		bv, ok := b.(*FlexXmlData)
		return ok && av == bv
	case XmlElementPayload:
		// An immutable Node/Xml has no pointer-backed store; it aliases
		// by its shared attribute map and children slice (a value dup'd
		// from one binding stays eq to its source; two fresh literals do
		// not). Empty children alias as the single empty children.
		bv, ok := b.(XmlElementPayload)
		if !ok || av.Tag != bv.Tag || av.Attr != bv.Attr {
			return false
		}
		if len(av.Cren) == 0 || len(bv.Cren) == 0 {
			return len(av.Cren) == len(bv.Cren)
		}
		return &av.Cren[0] == &bv.Cren[0]
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
		if numIsBig(a) || numIsBig(b) {
			ar, aok := toRatExact(a)
			br, bok := toRatExact(b)
			return aok && bok && ar.Cmp(br) == 0
		}
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
	if a.Parent.ConformsTo(TAtom) && b.Parent.ConformsTo(TAtom) {
		_as23, _ := AsAtom(a)
		_as22, _ := AsAtom(b)
		return _as23 == _as22
	}

	// Other scalar value-types (Bytes, the Time family, …) compare by
	// their type's semantic equality, exactly as eq does — deq must
	// never disagree with eq on a scalar leaf (there is no identity /
	// structural distinction to draw for an immutable scalar). Without
	// this clause every scalar outside the four by-value families fell
	// through to the final `return false`, so two content-equal Bytes
	// were eq-equal but deq-unequal — including nested inside a List.
	if equal, handled := scalarSemanticEqual(a, b); handled {
		return equal
	}

	// Lists: same length, each element deeply equal. Flex nodes are
	// normalised to their family — a FlexList deep-equals a plain List
	// of the same content.
	if nodeFamily(a.Parent).Equal(TList) && nodeFamily(b.Parent).Equal(TList) {
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
	if nodeFamily(a.Parent).Equal(TMap) && nodeFamily(b.Parent).Equal(TMap) {
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

	// XML elements (immutable Node/Xml or mutable FlexXml): structural
	// equality over tag / attributes / children, so a `parse xml` result
	// and the equivalent embedded literal — and a flex copy and its source
	// — compare deeply equal (attribute order is not significant). See
	// core_xml.go::xmlElementsEqual and design/XML-LITERAL.0.md §5.6.
	if IsXmlValue(a) && IsXmlValue(b) {
		return xmlElementsEqual(a, b)
	}

	// Flat instances (class + Resource/Entity): structural equality
	// requires the SAME (exact) type — a Point3 is never deq-equal to a
	// Point even with equal visible fields — and field-wise deep
	// equality. The key set is the union of schema fields and own
	// fields; both instance kinds store a flat Fields map, so a lookup
	// is a plain map hit. See design/CLASS-OBJECT.10.md.
	if IsFlatInstance(a) && IsFlatInstance(b) {
		if !a.Parent.Equal(b.Parent) {
			return false
		}
		aFields, aSchema, aok := flatInstanceParts(a)
		bFields, bSchema, bok := flatInstanceParts(b)
		if !aok || !bok || aFields == nil || bFields == nil {
			return false
		}
		seen := map[string]bool{}
		var keys []string
		addKeys := func(ks []string) {
			for _, k := range ks {
				if !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
		}
		addKeys(aSchema)
		_ = bSchema // a.Parent == b.Parent, so their schemas coincide
		addKeys(aFields.Keys())
		addKeys(bFields.Keys())
		for _, k := range keys {
			av, aok := aFields.Get(k)
			bv, bok := bFields.Get(k)
			if aok != bok {
				return false
			}
			if aok && !DeepEqual(av, bv) {
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
