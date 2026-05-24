package eng

// Unify is the kernel's structural unifier — the intersection of two
// values in the lattice. Returns the unified value and true on
// success, or (Value{}, false) on failure.
//
// For callers that need a structured failure (which field, which
// element, what reason), use UnifyExplain.
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
	v, err := UnifyExplain(a, b)
	return v, err == nil
}

// UnifyExplain is the structured-error counterpart to Unify. Returns
// (unified, nil) on success or (Value{}, *UnifyError) describing the
// failure path on mismatch. Use this when the caller needs to report
// which field/element failed (record field unification, options
// matching, `make` constraint checking, the lang-level `unify` word).
func UnifyExplain(a, b Value) (Value, *UnifyError) {
	a = ResolveWordsDeep(a)
	b = ResolveWordsDeep(b)
	return unifyInner(a, b)
}

// UnifyR is Unify with a Registry to enable predicate-FnDef
// constraint evaluation. When one side is a predicate FnDef (or a
// Disjunct/ChildType-typed-collection containing one) and a Registry
// is provided, RunPredicate handles the matching. Without a Registry,
// behaves identically to Unify.
//
// Use this from sites that already have a Registry in hand and may
// receive predicate-typed constraints — record-field check, options-
// field check, the lang-level `unify` word.
func UnifyR(a, b Value, r *Registry) (Value, bool) {
	v, err := UnifyExplainR(a, b, r)
	return v, err == nil
}

// UnifyExplainR — see UnifyR. Returns structured failure.
//
// Pushes r onto the unifyRegistryStack so recursive calls inside the
// per-family handlers (list element, map field, disjunct alternative)
// can pick it up via currentUnifyRegistry without each handler taking
// an explicit r parameter. The kernel is single-threaded per Engine,
// so the package-level stack is safe.
func UnifyExplainR(a, b Value, r *Registry) (Value, *UnifyError) {
	a = ResolveWordsDeep(a)
	b = ResolveWordsDeep(b)
	if r != nil {
		pushUnifyRegistry(r)
		defer popUnifyRegistry()
	}
	return unifyInnerR(a, b, r)
}

// unifyRegistryStack holds the Registry chain for in-flight
// UnifyExplainR calls. Family handlers (unifyConcreteMaps,
// unifyTypedListWithConcrete, unifyDisjunct, etc.) consult the top
// of the stack via currentUnifyRegistry when they encounter a
// predicate-fn constraint embedded in a structural type.
var unifyRegistryStack []*Registry

func pushUnifyRegistry(r *Registry) {
	unifyRegistryStack = append(unifyRegistryStack, r)
}

func popUnifyRegistry() {
	if n := len(unifyRegistryStack); n > 0 {
		unifyRegistryStack = unifyRegistryStack[:n-1]
	}
}

// currentUnifyRegistry returns the Registry of the in-flight
// UnifyExplainR call, or nil if no Registry-aware call is in flight.
func currentUnifyRegistry() *Registry {
	if n := len(unifyRegistryStack); n > 0 {
		return unifyRegistryStack[n-1]
	}
	return nil
}

// unifyInnerR — Registry-threaded dispatch. Pre-pass handles
// predicate-FnDef constraints (and disjunct alternatives that contain
// them) by routing through RunPredicate; everything else falls
// through to the standard kernel dispatch.
func unifyInnerR(a, b Value, r *Registry) (Value, *UnifyError) {
	return unifyInner(a, b)
}

// isPredicateFnValue reports whether v is a function value whose
// first signature has a single typed parameter — the shape a
// predicate type has.
func isPredicateFnValue(v Value) bool {
	if v.Parent == nil {
		return false
	}
	if !v.Parent.Equal(TFnDef) && !v.Parent.Equal(TFunction) {
		return false
	}
	info, ok := v.Data.(FnDefInfo)
	if !ok || len(info.Sigs) == 0 {
		return false
	}
	return len(info.Sigs[0].Params) == 1
}

// resolvePredicateRef returns the predicate fn body when v references
// a predicate type via name AND the type's Behavior is the
// predicateUnifier installed by InstallType. The Behavior check is
// what distinguishes a predicate TYPE from an ordinary 1-arg fn
// value — without it, every 1-arg fn would look like a predicate and
// hijack standard unification (e.g. FnUndef variance checks).
//
// Accepts three reference shapes:
//   - Bare type literal of a named predicate type (Pos's *Type).
//   - Word naming a predicate-typed def (`Pos` in `[:Pos]`).
//   - Atom naming a predicate-typed def (`Pos` after quote/resolve).
func resolvePredicateRef(v Value, r *Registry) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	var name string
	switch {
	case IsWord(v):
		w, _ := AsWord(v)
		name = w.Name
	case v.Parent != nil && v.Parent.Matches(TAtom) && v.Data != nil:
		w, _ := AsAtom(v)
		name = w
	case v.Data == nil && !v.Carrier && v.ID != "" && v.Name != "":
		name = v.Name
	case isPredicateFnValue(v):
		// Direct FnDef body — try the FnDef's Name field. Predicate
		// types installed via `def Pos fn […]` carry Name="Pos" on
		// their FnDef payload after InstallType wires the binding.
		if info, ok := v.Data.(FnDefInfo); ok {
			name = info.Name
		}
		if name == "" {
			// Anonymous fn — try reverse lookup: walk the def table
			// for a typed binding whose body equals v. This is what
			// catches record-field constraints stored as the FnDef
			// value (not the *Type literal).
			for _, n := range r.Defs.Names() {
				body, ok := r.TopTypeBody(n)
				if !ok {
					continue
				}
				if body.Equal(&v) {
					name = n
					break
				}
			}
		}
	}
	if name == "" {
		return Value{}, false
	}
	def := r.LookupTypeName(name)
	if def == nil {
		return Value{}, false
	}
	if _, ok := def.Behavior.(*predicateUnifier); !ok {
		return Value{}, false
	}
	body, ok := r.TopTypeBody(name)
	if !ok {
		return Value{}, false
	}
	return body, true
}

// disjunctHasPredicate reports whether any alternative in the
// disjunct is a predicate fn value.
func disjunctHasPredicate(disj DisjunctInfo) bool {
	for _, alt := range disj.Alternatives {
		if isPredicateFnValue(alt) {
			return true
		}
	}
	return false
}

// runPredicateUnify evaluates a predicate fn constraint against a
// candidate value via RunPredicate and folds the result into the
// UnifyExplain shape.
func runPredicateUnify(r *Registry, pred, val Value) (Value, *UnifyError) {
	out, matched, err := r.RunPredicate(pred, val)
	if err != nil {
		return Value{}, &UnifyError{Reason: err.Error(), A: pred, B: val}
	}
	if !matched {
		return Value{}, unifyFail("value does not satisfy predicate", pred, val)
	}
	return out, nil
}

// unifyDisjunctR is the Registry-aware disjunct walk. Tries each
// alternative via unifyInnerR so predicate-fn alternatives are
// evaluated correctly.
func unifyDisjunctR(disj DisjunctInfo, val Value, r *Registry) (Value, *UnifyError) {
	if val.Data == nil && (val.Parent.Equal(TAny) || (&val).Equal(TAny)) {
		return NewDisjunct(disj.Alternatives), nil
	}
	for _, alt := range disj.Alternatives {
		if unified, err := unifyInnerR(alt, val, r); err == nil {
			return unified, nil
		}
	}
	return Value{}, unifyFail("no disjunct alternative matched", NewDisjunct(disj.Alternatives), val)
}

// unifyInner is the post-resolution dispatcher. All recursive calls
// inside the family handlers use this entry so ResolveWordsDeep runs
// exactly once per top-level call.
//
// Pre-pass: if a Registry is active on the unifyRegistryStack and
// either operand is a predicate-fn constraint (or a disjunct
// containing one), route through RunPredicate so structural contexts
// — typed-list child, typed-map child, record field, disjunct
// alternative — honor predicate types without each handler needing
// to know about Registry.
func unifyInner(a, b Value) (Value, *UnifyError) {
	if r := currentUnifyRegistry(); r != nil {
		if pred, ok := resolvePredicateRef(a, r); ok && b.Data != nil {
			return runPredicateUnify(r, pred, b)
		}
		if pred, ok := resolvePredicateRef(b, r); ok && a.Data != nil {
			return runPredicateUnify(r, pred, a)
		}
		if IsDisjunct(a) {
			disj, _ := AsDisjunct(a)
			if disjunctHasPredicate(disj) {
				return unifyDisjunctR(disj, b, r)
			}
		}
		if IsDisjunct(b) {
			disj, _ := AsDisjunct(b)
			if disjunctHasPredicate(disj) {
				return unifyDisjunctR(disj, a, r)
			}
		}
	}
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
			return a, nil
		}
		return Value{}, unifyFail("never only unifies with never", a, b)
	}

	// None — only unifies with itself.
	if sa == ShapeNone || sb == ShapeNone {
		if sa == sb {
			return a, nil
		}
		return Value{}, unifyFail("none only unifies with none", a, b)
	}

	// Absent — only unifies with itself. Mirrors None's rule. Absent
	// is kernel-internal: it appears as a synthesized fill value when
	// the map unifier encounters a missing key. A disjunct containing
	// Absent (the `?:T` desugaring) accepts it via the Disjunct fold
	// above; any other shape rejects it.
	if sa == ShapeAbsent || sb == ShapeAbsent {
		if sa == sb {
			return a, nil
		}
		return Value{}, unifyFail("absent only unifies with absent", a, b)
	}

	// Any — yields the other (more specific) side.
	if sa == ShapeAny {
		return b, nil
	}
	if sb == ShapeAny {
		return a, nil
	}

	// Behavior-driven dispatch: walk the LCA of the two operand types
	// looking for a Unifier capability. The first non-opt-out Unifier
	// owns the result — same pattern CompareValues uses for Comparer.
	// Predicate types and refine-with-clause types auto-install
	// Unifiers (see core_type.go::InstallType) so narrowing into a
	// constrained type checks the constraint; external plugin types
	// and `behave unify/q` user installs also flow through here.
	if v, err, hit := dispatchUnifier(a, b); hit {
		return v, err
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
func unifySameOrSubtype(a, b Value) (Value, *UnifyError) {
	aType := denotedType(a)
	bType := denotedType(b)

	// Type literal unifies with any concrete whose type matches.
	if a.Data == nil && b.Data != nil && bType.Matches(aType) {
		return b, nil
	}
	if b.Data == nil && a.Data != nil && aType.Matches(bType) {
		return a, nil
	}

	// Same type → compare literal values.
	if aType.Equal(bType) {
		if ValuesEqual(a, b) {
			return a, nil
		}
		return Value{}, unifyFail("same type, different literal values", a, b)
	}

	// Subtype relation → narrower side wins.
	if aType.IsSubtypeOf(bType) {
		return a, nil
	}
	if bType.IsSubtypeOf(aType) {
		return b, nil
	}

	return Value{}, unifyFail("incompatible types", a, b)
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
