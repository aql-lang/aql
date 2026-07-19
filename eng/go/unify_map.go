package eng

// unifyMapFamily owns unification when either side is in the Map
// family (Map, TypedMap, Record, Options) or is a bare Map type
// literal.
//
// The Map family has more sub-shapes than List, but the same
// canonicalization principle applies: type literals normalize to the
// `lit` slot, exclusive shapes (Record, Options) only unify with their
// own kind, and typed-vs-concrete arms are collapsed by ordering.
func unifyMapFamily(a Value, sa ValueShape, b Value, sb ValueShape) (Value, *UnifyError) {
	// Bare FlexMap type literal: nominal-subtype rule — unifies only
	// with a concrete FlexMap (or another FlexMap literal). A plain
	// map is NOT a FlexMap; the supertype literal `Map` accepts flex
	// values below via the ordinary family rule.
	if res, handled, err := unifyFlexLiteral(a, sa, b, sb, TFlexMap, IsFlexMap, "FlexMap"); handled {
		return res, err
	}

	// A flex-family carrier (check-mode `(flex …)` residual with no shape
	// tracking) vs a typed map {:T}: gradual accept, element-tagged. (The
	// shape-tracked map carrier is ShapeMap and flows through the concrete arm
	// below; this covers the bare-carrier fallback.) See unifyCarrierVsTyped.
	if res, handled := unifyCarrierVsTyped(a, sa, b, sb, ShapeTypedMap, TFlexMap); handled {
		return res, nil
	}

	// Bare Map type literal: unifies with any Map-family value except
	// a record (records are nominal).
	aLit := sa == ShapeTypeLiteral && denotedType(a).Equal(TMap)
	bLit := sb == ShapeTypeLiteral && denotedType(b).Equal(TMap)
	if aLit {
		if sb == ShapeRecord {
			return Value{}, unifyFail("Map type literal does not unify with Record", a, b)
		}
		if sb == ShapeOptions {
			info, _ := AsOptionsType(b)
			return NewOptionsType(info.Fields), nil
		}
		if IsMapShape(sb) || bLit {
			return b, nil
		}
		return Value{}, unifyFail("Map type literal needs a map-family right-hand side", a, b)
	}
	if bLit {
		if sa == ShapeRecord {
			return Value{}, unifyFail("Map type literal does not unify with Record", a, b)
		}
		if sa == ShapeOptions {
			info, _ := AsOptionsType(a)
			return NewOptionsType(info.Fields), nil
		}
		if IsMapShape(sa) {
			return a, nil
		}
		return Value{}, unifyFail("Map type literal needs a map-family left-hand side", a, b)
	}

	if !IsMapShape(sa) || !IsMapShape(sb) {
		return Value{}, unifyFail("map family requires map-shaped values on both sides", a, b)
	}

	// Record is exclusive — only unifies with another record. Field
	// order is part of a record's identity.
	if sa == ShapeRecord || sb == ShapeRecord {
		if sa != sb {
			return Value{}, unifyFail("Record only unifies with Record", a, b)
		}
		aRT, _ := AsRecordType(a)
		bRT, _ := AsRecordType(b)
		return unifyRecordTypes(aRT, bRT)
	}

	// Options dispatches to its own handler — the field rules
	// (defaults, disjunct alternatives, concrete-vs-literal) are
	// substantial enough to keep separate.
	if sa == ShapeOptions || sb == ShapeOptions {
		return unifyOptionsFamily(a, sa, b, sb)
	}

	// Both typed maps → unify child types.
	if sa == ShapeTypedMap && sb == ShapeTypedMap {
		aCT, _ := AsChildType(a)
		bCT, _ := AsChildType(b)
		unified, err := unifyInner(aCT.Child, bCT.Child)
		if err != nil {
			return Value{}, err.withPath("child")
		}
		return NewTypedMap(unified), nil
	}

	// One side typed, other concrete: every value must unify with the
	// child type.
	if sa == ShapeTypedMap || sb == ShapeTypedMap {
		var typed, concrete Value
		if sa == ShapeTypedMap {
			typed, concrete = a, b
		} else {
			typed, concrete = b, a
		}
		ct, _ := AsChildType(typed)
		return unifyTypedMapWithConcrete(concrete, ct.Child)
	}

	// Both concrete maps: key-by-key unification, with absent-on-one-
	// side keys defaulting against None.
	aMap, _ := AsMap(a)
	bMap, _ := AsMap(b)
	return unifyConcreteMaps(aMap, bMap)
}

func unifyConcreteMaps(aMap, bMap ReadMap) (Value, *UnifyError) {
	absentVal := NewTypeLiteral(TAbsent)
	result := NewOrderedMap()

	// Missing-key rule: synthesise an Absent value and unify the
	// present-side value against it. A disjunct that contains Absent
	// (the `?:T` desugaring) accepts it via the disjunct fold; any
	// other shape rejects it — so non-optional missing keys fail
	// naturally without a marker check.
	//
	// Omission rule: a unified value of Absent does not occupy a slot
	// in the result map. Absent is the type of non-presence, so a key
	// whose value is Absent is by definition not present in the map.
	for _, key := range aMap.Keys() {
		aVal, _ := aMap.Get(key)
		bVal, ok := bMap.Get(key)
		if !ok {
			unified, err := unifyInner(aVal, absentVal)
			if err != nil {
				return Value{}, err.withPath("key:" + key)
			}
			if Shape(unified) == ShapeAbsent {
				continue
			}
			result.Set(key, unified) //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			continue
		}
		unified, err := unifyInner(aVal, bVal)
		if err != nil {
			return Value{}, err.withPath("key:" + key)
		}
		if Shape(unified) == ShapeAbsent {
			continue
		}
		result.Set(key, unified)
	}
	for _, key := range bMap.Keys() {
		if _, ok := aMap.Get(key); ok {
			continue
		}
		bVal, _ := bMap.Get(key)
		unified, err := unifyInner(bVal, absentVal)
		if err != nil {
			return Value{}, err.withPath("key:" + key)
		}
		if Shape(unified) == ShapeAbsent {
			continue
		}
		result.Set(key, unified) //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
	}
	return NewMap(result), nil
}

// unifyTypedMapWithConcrete unifies a child type constraint against each
// value of the concrete map side. Every value must unify. The result RETAINS
// childType as its element tag (Value.elem) so writes can be enforced and
// reads narrowed, and PRESERVES the concrete side's mutability: a FlexMap
// stays a FlexMap (flex only toggles mutability — it never strips the
// element-type contract), a plain Map stays a plain Map. A CARRIER (a
// check-mode abstract map, IsConcrete false — e.g. the residual of
// `(flex {…})`) has no readable entries: unify gradually to the carrier's own
// kind, tagged, rather than dereferencing nil (the flex-{:T} panic).
// See design/TYPED-CONTAINER-TAG-RETENTION.0.md.
func unifyTypedMapWithConcrete(concrete, childType Value) (Value, *UnifyError) {
	if !IsConcrete(concrete) {
		// A carrier (a check-mode abstract map with no readable entries) tags
		// gradually; its concrete elements are validated at runtime.
		out := concrete
		out.SetElemConstraint(childType)
		return out, nil
	}
	m, _ := AsMap(concrete) // concrete map-family (plain or flex) → readable
	res, err := unifyMapValues(m, func(string) Value { return childType })
	if err != nil {
		return Value{}, err
	}
	if IsFlexMap(concrete) {
		om, _ := AsMutableMap(res) // res is a fresh plain Map — reuse its OrderedMap
		res = NewFlexMap(om)
	}
	res.SetElemConstraint(childType)
	return res, nil
}

// unifyFieldBags unifies two field-schema maps key-by-key. Both must
// hold the same number of fields and each field-type pair must unify.
// When orderStrict is true the keys must also appear in the same order
// (record-type semantics); when false key order is irrelevant
// (options-type semantics).
func unifyFieldBags(a, b *OrderedMap, orderStrict bool) (*OrderedMap, *UnifyError) {
	if a.Len() != b.Len() {
		return nil, &UnifyError{Reason: "field-count mismatch"}
	}
	aKeys := a.Keys()
	bKeys := b.Keys()
	result := NewOrderedMap()
	for i, key := range aKeys {
		if orderStrict && bKeys[i] != key {
			return nil, &UnifyError{Reason: "field order mismatch at " + key}
		}
		bVal, ok := b.Get(key)
		if !ok {
			return nil, &UnifyError{Reason: "field " + key + " missing on right side"}
		}
		aVal, _ := a.Get(key)
		unified, err := unifyInner(aVal, bVal)
		if err != nil {
			return nil, err.withPath("field:" + key)
		}
		result.Set(key, unified)
	}
	return result, nil
}

// unifyRecordTypes unifies two record types by unifying their field
// schemas. Keys must match in the same order.
func unifyRecordTypes(a, b RecordTypeInfo) (Value, *UnifyError) {
	result, err := unifyFieldBags(a.Fields, b.Fields, true)
	if err != nil {
		return Value{}, err
	}
	return NewRecordType(result), nil
}
