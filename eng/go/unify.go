package eng

// Unify is the kernel's structural unifier — the intersection of two
// values in the lattice. Returns the unified value and true on
// success, or (Value{}, false) on failure.
//
// Dispatch model (this file):
//
//  1. ResolveWordsDeep preprocesses both sides so bare words inside
//     list/map literals participate as their semantic values.
//  2. Shape() classifies each side into one ValueShape.
//  3. The top dispatcher routes to a family handler keyed by the
//     "ruling" shape. Special degenerate roots (Never, None, Any) and
//     the Disjunct fold are handled inline; everything else routes to
//     a per-family file (unify_list.go, unify_map.go, unify_disjunct.go,
//     unify_dep.go, unify_fnsig.go).
//
// Each family handler receives both sides plus their shapes, and is
// responsible for canonicalizing the asymmetric arms (type-literal vs
// concrete, typed vs untyped, etc.) so the per-pair logic appears
// exactly once instead of mirrored.
//
// The narrowing fall-through (same type → ValuesEqual, subtype → take
// the narrower) lives at the end of this file: unifySameOrSubtype.
func Unify(a, b Value) (Value, bool) {
	a = ResolveWordsDeep(a)
	b = ResolveWordsDeep(b)

	sa := Shape(a)
	sb := Shape(b)

	// Disjunct fold first — must come before degenerate-root checks so
	// disjuncts containing None work (e.g. `String or None`).
	if sa == ShapeDisjunct {
		disj, _ := AsDisjunct(a)
		return unifyDisjunct(disj, b)
	}
	if sb == ShapeDisjunct {
		disj, _ := AsDisjunct(b)
		return unifyDisjunct(disj, a)
	}

	// Never — bottom type, only unifies with itself.
	if sa == ShapeNever || sb == ShapeNever {
		if sa == sb {
			return a, true
		}
		return Value{}, false
	}

	// None — only unifies with itself.
	if sa == ShapeNone || sb == ShapeNone {
		if sa == sb {
			return a, true
		}
		return Value{}, false
	}

	// Any — yields the other (more specific) side.
	if sa == ShapeAny {
		return b, true
	}
	if sb == ShapeAny {
		return a, true
	}

	// Family handlers — any side in the family routes to that family's
	// owner, which canonicalizes argument order internally. A bare
	// type literal whose denoted type is List/Map also routes to the
	// corresponding family (e.g. `List unify [1,2]`).
	aListLit := sa == ShapeTypeLiteral && denotedType(a).Equal(TList)
	bListLit := sb == ShapeTypeLiteral && denotedType(b).Equal(TList)
	if IsListShape(sa) || IsListShape(sb) || aListLit || bListLit {
		return unifyListFamily(a, sa, b, sb)
	}
	aMapLit := sa == ShapeTypeLiteral && denotedType(a).Equal(TMap)
	bMapLit := sb == ShapeTypeLiteral && denotedType(b).Equal(TMap)
	if IsMapShape(sa) || IsMapShape(sb) || aMapLit || bMapLit {
		return unifyMapFamily(a, sa, b, sb)
	}
	if sa == ShapeDepScalar || sb == ShapeDepScalar {
		return unifyDepScalar(a, sa, b, sb)
	}
	if sa == ShapeFnUndef || sb == ShapeFnUndef {
		return unifyFnUndefShape(a, sa, b, sb)
	}

	// General narrowing: type-literal-vs-concrete, same-type literal
	// compare, subtype relation. Handled together because they're all
	// just "pick the narrower side if compatible".
	return unifySameOrSubtype(a, b)
}

// unifySameOrSubtype is the general scalar-narrowing fall-through. By
// the time we reach here both sides are non-list, non-map, non-disjunct,
// non-depscalar, non-fnundef — so it's just type-literal vs concrete
// or two values along the same scalar lattice chain.
func unifySameOrSubtype(a, b Value) (Value, bool) {
	aType := denotedType(a)
	bType := denotedType(b)

	// Type literal unifies with any concrete whose type matches.
	if a.Data == nil && b.Data != nil && bType.Matches(aType) {
		return b, true
	}
	if b.Data == nil && a.Data != nil && aType.Matches(bType) {
		return a, true
	}

	// Same type → compare literal values.
	if aType.Equal(bType) {
		if ValuesEqual(a, b) {
			return a, true
		}
		return Value{}, false
	}

	// Subtype relation → narrower side wins.
	if aType.IsSubtypeOf(bType) {
		return a, true
	}
	if bType.IsSubtypeOf(aType) {
		return b, true
	}

	return Value{}, false
}

// denotedType returns the lattice type the value denotes. For a bare
// type literal the value IS the lattice node; for a carrier or
// concrete value it is the Parent. A Data==nil value with an empty ID
// is a manually-constructed `Value{Parent: T}` (used in tests as a
// stand-in for a value of type T); treat its Parent as the denoted
// type since &v has no lattice identity to compare against.
func denotedType(v Value) *Type {
	if v.Data == nil && !v.Carrier && v.ID != "" {
		return &v
	}
	return v.Parent
}
