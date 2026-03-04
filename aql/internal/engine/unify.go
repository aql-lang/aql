package engine

import "fmt"

// Unify attempts to unify two values. If the values can be unified (their types
// are compatible and can narrow), it returns the unified value and true.
// Otherwise it returns an error description and false.
//
// Unification rules for scalar types:
//   - "none" only unifies with "none", nothing else (not even "any")
//   - Equal types with equal data: return either value, true
//   - One type is a subtype of the other: return the narrower (more specific) value, true
//   - One type is "any": return the other (more specific) value, true
//   - Same leaf type but different literal values: fail (each literal is its own narrow type)
//   - Incompatible type hierarchies: fail
//
// Unification rules for complex types:
//   - Lists: unify element-by-element in order; lengths must match
//   - Maps: closed unification; key sets must be identical; each value pair must unify
func Unify(a, b Value) (Value, bool) {
	aType := a.VType
	bType := b.VType

	// "none" only unifies with "none".
	aNone := aType.Equal(TNone)
	bNone := bType.Equal(TNone)
	if aNone || bNone {
		if aNone && bNone {
			return a, true
		}
		return Value{}, false
	}

	// If either is "any", unify to the other (more specific) value.
	if aType.Equal(TAny) {
		return b, true
	}
	if bType.Equal(TAny) {
		return a, true
	}

	// List unification.
	aList := aType.Equal(TList)
	bList := bType.Equal(TList)
	if aList || bList {
		return unifyLists(a, aList, b, bList)
	}

	// Map unification.
	aMap := aType.Equal(TMap)
	bMap := bType.Equal(TMap)
	if aMap || bMap {
		return unifyMaps(a, aMap, b, bMap)
	}

	// If both types are exactly equal, compare literal values.
	if aType.Equal(bType) {
		if valuesEqual(a, b) {
			return a, true
		}
		// Same type, different literal values — cannot unify.
		return Value{}, false
	}

	// Check subtype relationships.
	// If a is a subtype of b, a is narrower → return a.
	if aType.IsSubtypeOf(bType) {
		return a, true
	}
	// If b is a subtype of a, b is narrower → return b.
	if bType.IsSubtypeOf(aType) {
		return b, true
	}

	// No compatible type relationship.
	return Value{}, false
}

// unifyLists handles unification when at least one side is a list.
func unifyLists(a Value, aIsList bool, b Value, bIsList bool) (Value, bool) {
	// Type literal "list" (Data==nil) unifies with any list type.
	if aIsList && a.Data == nil {
		if bIsList {
			return b, true
		}
		return Value{}, false
	}
	if bIsList && b.Data == nil {
		if aIsList {
			return a, true
		}
		return Value{}, false
	}

	// Both must be list types.
	if !aIsList || !bIsList {
		return Value{}, false
	}

	// Check for typed lists (child type constraints).
	aTyped := a.IsTypedList()
	bTyped := b.IsTypedList()

	if aTyped && bTyped {
		// Both typed lists: unify child types.
		aChild := a.AsChildType().Child
		bChild := b.AsChildType().Child
		unified, ok := Unify(aChild, bChild)
		if !ok {
			return Value{}, false
		}
		return NewTypedList(unified), true
	}

	if aTyped {
		// a is typed, b is concrete: each element must unify with the child type.
		return unifyTypedWithConcrete(a.AsChildType().Child, b.AsList())
	}

	if bTyped {
		// b is typed, a is concrete: each element must unify with the child type.
		return unifyTypedWithConcrete(b.AsChildType().Child, a.AsList())
	}

	// Both concrete lists: element-by-element unification.
	aElems := a.AsList()
	bElems := b.AsList()

	// Lengths must match.
	if len(aElems) != len(bElems) {
		return Value{}, false
	}

	// Unify element by element.
	result := make([]Value, len(aElems))
	for i := range aElems {
		unified, ok := Unify(aElems[i], bElems[i])
		if !ok {
			return Value{}, false
		}
		result[i] = unified
	}

	return NewList(result), true
}

// unifyTypedWithConcrete unifies a child type constraint against each element
// of a concrete list. Every element must unify with the child type.
func unifyTypedWithConcrete(childType Value, elems []Value) (Value, bool) {
	result := make([]Value, len(elems))
	for i, elem := range elems {
		unified, ok := Unify(childType, elem)
		if !ok {
			return Value{}, false
		}
		result[i] = unified
	}
	return NewList(result), true
}

// unifyMaps handles unification when at least one side is a map.
// Maps are closed: both must have exactly the same set of keys.
func unifyMaps(a Value, aIsMap bool, b Value, bIsMap bool) (Value, bool) {
	// Type literal "map" (Data==nil) unifies with any map type.
	if aIsMap && a.Data == nil {
		if bIsMap {
			return b, true
		}
		return Value{}, false
	}
	if bIsMap && b.Data == nil {
		if aIsMap {
			return a, true
		}
		return Value{}, false
	}

	// Both must be map types.
	if !aIsMap || !bIsMap {
		return Value{}, false
	}

	// Check for record types (field schema constraints).
	aRecord := a.IsRecordType()
	bRecord := b.IsRecordType()

	if aRecord && bRecord {
		// Both record types: unify field schemas.
		return unifyRecordTypes(a.AsRecordType(), b.AsRecordType())
	}

	if aRecord {
		if _, ok := b.Data.(*OrderedMap); ok {
			// a is record type, b is concrete map.
			return unifyRecordWithConcrete(a.AsRecordType(), b.AsMap())
		}
		return Value{}, false
	}

	if bRecord {
		if _, ok := a.Data.(*OrderedMap); ok {
			// b is record type, a is concrete map.
			return unifyRecordWithConcrete(b.AsRecordType(), a.AsMap())
		}
		return Value{}, false
	}

	// Check for typed maps (child type constraints).
	aTyped := a.IsTypedMap()
	bTyped := b.IsTypedMap()

	if aTyped && bTyped {
		// Both typed maps: unify child types.
		aChild := a.AsChildType().Child
		bChild := b.AsChildType().Child
		unified, ok := Unify(aChild, bChild)
		if !ok {
			return Value{}, false
		}
		return NewTypedMap(unified), true
	}

	if aTyped {
		// a is typed, b is concrete: each value must unify with the child type.
		return unifyTypedMapWithConcrete(a.AsChildType().Child, b.AsMap())
	}

	if bTyped {
		// b is typed, a is concrete: each value must unify with the child type.
		return unifyTypedMapWithConcrete(b.AsChildType().Child, a.AsMap())
	}

	// Both concrete maps: key-by-key unification.
	aMap := a.AsMap()
	bMap := b.AsMap()

	// Key sets must be identical (closed maps).
	if aMap.Len() != bMap.Len() {
		return Value{}, false
	}

	result := NewOrderedMap()

	// Walk keys of a in order, check each exists in b.
	for _, key := range aMap.Keys() {
		aVal, _ := aMap.Get(key)
		bVal, ok := bMap.Get(key)
		if !ok {
			// Key in a but not in b — fail.
			return Value{}, false
		}

		unified, uOk := Unify(aVal, bVal)
		if !uOk {
			return Value{}, false
		}
		result.Set(key, unified)
	}

	return NewMap(result), true
}

// unifyTypedMapWithConcrete unifies a child type constraint against each value
// of a concrete map. Every value must unify with the child type.
func unifyTypedMapWithConcrete(childType Value, m *OrderedMap) (Value, bool) {
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

// unifyRecordTypes unifies two record types by unifying their field schemas.
// Both must have exactly the same set of keys; each field type pair is unified.
func unifyRecordTypes(a, b RecordTypeInfo) (Value, bool) {
	aFields := a.Fields
	bFields := b.Fields

	if aFields.Len() != bFields.Len() {
		return Value{}, false
	}

	result := NewOrderedMap()
	for _, key := range aFields.Keys() {
		aVal, _ := aFields.Get(key)
		bVal, ok := bFields.Get(key)
		if !ok {
			return Value{}, false
		}
		unified, uOk := Unify(aVal, bVal)
		if !uOk {
			return Value{}, false
		}
		result.Set(key, unified)
	}

	return NewRecordType(result), true
}

// unifyRecordWithConcrete unifies a record type schema against a concrete map.
// The map must have exactly the same keys as the record, and each value must
// unify with the corresponding field type constraint.
func unifyRecordWithConcrete(record RecordTypeInfo, m *OrderedMap) (Value, bool) {
	fields := record.Fields

	if fields.Len() != m.Len() {
		return Value{}, false
	}

	result := NewOrderedMap()
	for _, key := range fields.Keys() {
		fieldType, _ := fields.Get(key)
		mapVal, ok := m.Get(key)
		if !ok {
			return Value{}, false
		}
		unified, uOk := Unify(fieldType, mapVal)
		if !uOk {
			return Value{}, false
		}
		result.Set(key, unified)
	}

	return NewMap(result), true
}

// valuesEqual compares the data payloads of two values with the same type.
func valuesEqual(a, b Value) bool {
	// Type literals (Data == nil) with equal types are always equal.
	if a.Data == nil && b.Data == nil {
		return true
	}
	// One is a type literal and the other is a concrete value — not equal.
	if a.Data == nil || b.Data == nil {
		return false
	}
	switch {
	case a.VType.Matches(TString):
		return a.AsString() == b.AsString()
	case a.VType.Matches(TInteger):
		return a.AsInteger() == b.AsInteger()
	case a.VType.Matches(TBoolean):
		return a.AsBoolean() == b.AsBoolean()
	case a.VType.Equal(TList):
		aCT, aOk := a.Data.(ChildTypeInfo)
		bCT, bOk := b.Data.(ChildTypeInfo)
		if aOk && bOk {
			return aCT.Child.VType.Equal(bCT.Child.VType) && valuesEqual(aCT.Child, bCT.Child)
		}
		if aOk != bOk {
			return false
		}
		return listsEqual(a.AsList(), b.AsList())
	case a.VType.Equal(TMap):
		aRT, aRec := a.Data.(RecordTypeInfo)
		bRT, bRec := b.Data.(RecordTypeInfo)
		if aRec && bRec {
			return mapsEqual(aRT.Fields, bRT.Fields)
		}
		if aRec != bRec {
			return false
		}
		aCT, aOk := a.Data.(ChildTypeInfo)
		bCT, bOk := b.Data.(ChildTypeInfo)
		if aOk && bOk {
			return aCT.Child.VType.Equal(bCT.Child.VType) && valuesEqual(aCT.Child, bCT.Child)
		}
		if aOk != bOk {
			return false
		}
		return mapsEqual(a.AsMap(), b.AsMap())
	default:
		return fmt.Sprintf("%v", a.Data) == fmt.Sprintf("%v", b.Data)
	}
}

// listsEqual compares two list payloads element by element.
func listsEqual(a, b []Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].VType.Equal(b[i].VType) || !valuesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// mapsEqual compares two map payloads by keys and values.
func mapsEqual(a, b *OrderedMap) bool {
	if a.Len() != b.Len() {
		return false
	}
	for _, k := range a.Keys() {
		aVal, _ := a.Get(k)
		bVal, ok := b.Get(k)
		if !ok {
			return false
		}
		if !aVal.VType.Equal(bVal.VType) || !valuesEqual(aVal, bVal) {
			return false
		}
	}
	return true
}

// registerUnify registers the "unify" word in the given registry.
func registerUnify(r *Registry) {
	unifyHandler := func(args []Value) ([]Value, error) {
		unified, ok := Unify(args[0], args[1])
		if ok {
			return []Value{unified, NewBoolean(true)}, nil
		}
		return []Value{NewString("~unify-fail"), NewBoolean(false)}, nil
	}

	// unify: [any, any] -> [any, boolean]
	r.Register("unify", Signature{
		Args:    []Type{TAny, TAny},
		Handler: unifyHandler,
	})
}
