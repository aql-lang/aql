package eng

// unifyMapFamily owns unification when either side is in the Map
// family (Map, TypedMap, Record, Options) or is a bare Map type
// literal.
//
// The Map family has more sub-shapes than List, but the same
// canonicalization principle applies: type literals normalize to the
// `lit` slot, exclusive shapes (Record, Options) only unify with their
// own kind, and typed-vs-concrete arms are collapsed by ordering.
func unifyMapFamily(a Value, sa ValueShape, b Value, sb ValueShape) (Value, bool) {
	// Bare Map type literal: unifies with any Map-family value except
	// a record (records are nominal).
	aLit := sa == ShapeTypeLiteral && denotedType(a).Equal(TMap)
	bLit := sb == ShapeTypeLiteral && denotedType(b).Equal(TMap)
	if aLit {
		if sb == ShapeRecord {
			return Value{}, false
		}
		if sb == ShapeOptions {
			info, _ := AsOptionsType(b)
			return NewOptionsType(info.Fields), true
		}
		if IsMapShape(sb) || bLit {
			return b, true
		}
		return Value{}, false
	}
	if bLit {
		if sa == ShapeRecord {
			return Value{}, false
		}
		if sa == ShapeOptions {
			info, _ := AsOptionsType(a)
			return NewOptionsType(info.Fields), true
		}
		if IsMapShape(sa) {
			return a, true
		}
		return Value{}, false
	}

	if !IsMapShape(sa) || !IsMapShape(sb) {
		return Value{}, false
	}

	// Record is exclusive — only unifies with another record. Field
	// order is part of a record's identity.
	if sa == ShapeRecord || sb == ShapeRecord {
		if sa != sb {
			return Value{}, false
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
		unified, ok := Unify(aCT.Child, bCT.Child)
		if !ok {
			return Value{}, false
		}
		return NewTypedMap(unified), true
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

func unifyConcreteMaps(aMap, bMap ReadMap) (Value, bool) {
	noneVal := NewTypeLiteral(TNone)
	result := NewOrderedMap()

	for _, key := range aMap.Keys() {
		aVal, _ := aMap.Get(key)
		bVal, ok := bMap.Get(key)
		if !ok {
			unified, uOk := Unify(aVal, noneVal)
			if !uOk {
				return Value{}, false
			}
			result.Set(key, unified)
			continue
		}
		unified, uOk := Unify(aVal, bVal)
		if !uOk {
			return Value{}, false
		}
		result.Set(key, unified)
	}
	for _, key := range bMap.Keys() {
		if _, ok := aMap.Get(key); ok {
			continue
		}
		bVal, _ := bMap.Get(key)
		unified, uOk := Unify(bVal, noneVal)
		if !uOk {
			return Value{}, false
		}
		result.Set(key, unified)
	}
	return NewMap(result), true
}

// unifyTypedMapWithConcrete unifies a child type constraint against
// each value of a concrete map. Every value must unify.
func unifyTypedMapWithConcrete(childType Value, m ReadMap) (Value, bool) {
	result := NewOrderedMap()
	for _, key := range m.Keys() {
		val, _ := m.Get(key)
		unified, ok := Unify(childType, val)
		if !ok {
			return Value{}, false
		}
		result.Set(key, unified)
	}
	return NewMap(result), true
}

// unifyFieldBags unifies two field-schema maps key-by-key. Both must
// hold the same number of fields and each field-type pair must unify.
// When orderStrict is true the keys must also appear in the same order
// (record-type semantics); when false key order is irrelevant
// (options-type semantics).
func unifyFieldBags(a, b *OrderedMap, orderStrict bool) (*OrderedMap, bool) {
	if a.Len() != b.Len() {
		return nil, false
	}
	aKeys := a.Keys()
	bKeys := b.Keys()
	result := NewOrderedMap()
	for i, key := range aKeys {
		if orderStrict && bKeys[i] != key {
			return nil, false
		}
		bVal, ok := b.Get(key)
		if !ok {
			return nil, false
		}
		aVal, _ := a.Get(key)
		unified, uOk := Unify(aVal, bVal)
		if !uOk {
			return nil, false
		}
		result.Set(key, unified)
	}
	return result, true
}

// unifyRecordTypes unifies two record types by unifying their field
// schemas. Keys must match in the same order.
func unifyRecordTypes(a, b RecordTypeInfo) (Value, bool) {
	result, ok := unifyFieldBags(a.Fields, b.Fields, true)
	if !ok {
		return Value{}, false
	}
	return NewRecordType(result), true
}
