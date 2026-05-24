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
		m, _ := AsMap(concrete)
		return unifyTypedMapWithConcrete(ct.Child, m)
	}

	// Both concrete maps: key-by-key unification, with absent-on-one-
	// side keys defaulting against None.
	aMap, _ := AsMap(a)
	bMap, _ := AsMap(b)
	return unifyConcreteMaps(aMap, bMap)
}

func unifyConcreteMaps(aMap, bMap ReadMap) (Value, *UnifyError) {
	noneVal := NewTypeLiteral(TNone)
	result := NewOrderedMap()

	// Optional-key rule (universal): when a key is present on one side
	// and marked optional there but absent on the other side, omit it
	// from the result entirely. When the key is NOT marked optional,
	// fall back to the closed-map rule — unify the present value
	// against None, which succeeds only for None-valued / None-typed
	// payloads. This implements "? means None or absent" symmetrically.
	for _, key := range aMap.Keys() {
		aVal, _ := aMap.Get(key)
		bVal, ok := bMap.Get(key)
		if !ok {
			if OptionalKeyInMap(aMap, key) {
				continue
			}
			unified, err := unifyInner(aVal, noneVal)
			if err != nil {
				return Value{}, err.withPath("key:" + key)
			}
			result.Set(key, unified)
			if OptionalKeyInMap(aMap, key) {
				result.MarkOptionalKey(key)
			}
			continue
		}
		unified, err := unifyInner(aVal, bVal)
		if err != nil {
			return Value{}, err.withPath("key:" + key)
		}
		result.Set(key, unified)
		// Preserve optionality if EITHER side marked the key — chained
		// unify operations downstream then see the same shape.
		if OptionalKeyInMap(aMap, key) || OptionalKeyInMap(bMap, key) {
			result.MarkOptionalKey(key)
		}
	}
	for _, key := range bMap.Keys() {
		if _, ok := aMap.Get(key); ok {
			continue
		}
		if OptionalKeyInMap(bMap, key) {
			continue
		}
		bVal, _ := bMap.Get(key)
		unified, err := unifyInner(bVal, noneVal)
		if err != nil {
			return Value{}, err.withPath("key:" + key)
		}
		result.Set(key, unified)
	}
	return NewMap(result), nil
}

// unifyTypedMapWithConcrete unifies a child type constraint against
// each value of a concrete map. Every value must unify.
func unifyTypedMapWithConcrete(childType Value, m ReadMap) (Value, *UnifyError) {
	result := NewOrderedMap()
	for _, key := range m.Keys() {
		val, _ := m.Get(key)
		unified, err := unifyInner(childType, val)
		if err != nil {
			return Value{}, err.withPath("key:" + key)
		}
		result.Set(key, unified)
	}
	return NewMap(result), nil
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
