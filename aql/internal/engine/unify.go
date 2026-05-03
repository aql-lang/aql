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
	// Resolve words to their semantic values (true/false → boolean,
	// type names → type literal, other words → atom) so that
	// unresolved words inside list literals participate correctly.
	a = resolveWordsDeep(a)
	b = resolveWordsDeep(b)

	aType := a.VType
	bType := b.VType

	// Disjunct unification first: try each alternative, succeed on first match.
	// Must come before none/any checks so that disjuncts containing none work.
	if a.IsDisjunct() {
		_as0, _ := a.AsDisjunct()
		return unifyDisjunct(_as0, b)
	}
	if b.IsDisjunct() {
		_as1, _ := b.AsDisjunct()
		return unifyDisjunct(_as1, a)
	}

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

	// Metatype matching: when both are type literals and one has a metatype
	// VType, check if the other's computed metatype matches.
	if a.Data == nil && b.Data == nil {
		aIsMeta := IsMetaType(aType)
		bIsMeta := IsMetaType(bType)
		if bIsMeta && !aIsMeta {
			if MetatypeFor(aType).Matches(bType) {
				return a, true
			}
		}
		if aIsMeta && !bIsMeta {
			if MetatypeFor(bType).Matches(aType) {
				return b, true
			}
		}
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

	// Dependent-scalar unification. A DepScalar carries a comparison
	// constraint over a base scalar type (e.g. Integer ≥10, String
	// <"z"); it unifies with a concrete value iff the value's type
	// matches the base AND the value satisfies the comparison. Returns
	// the plain scalar (not the DepScalar) so downstream consumers see
	// a normal value. IsDepScalar guards both sides — if both are
	// dependents the standard path-prefix matcher takes over.
	if a.IsDepScalar() && !b.IsDepScalar() && b.Data != nil {
		if base, ok := dependentLeafBaseType(dependentLeafFromType(aType)); ok && bType.Matches(base) {
			info, err := a.AsDepScalar()
			if err != nil {
				return Value{}, false
			}
			if depScalarCheck(info, b) {
				return b, true
			}
			return Value{}, false
		}
	}
	if b.IsDepScalar() && !a.IsDepScalar() && a.Data != nil {
		if base, ok := dependentLeafBaseType(dependentLeafFromType(bType)); ok && aType.Matches(base) {
			info, err := b.AsDepScalar()
			if err != nil {
				return Value{}, false
			}
			if depScalarCheck(info, a) {
				return a, true
			}
			return Value{}, false
		}
	}

	// Type literal unification: a type literal (Data==nil) unifies with
	// any concrete value whose type matches. Return the concrete value.
	if a.Data == nil && b.Data != nil && bType.Matches(aType) {
		return b, true
	}
	if b.Data == nil && a.Data != nil && aType.Matches(bType) {
		return a, true
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
	// Type literal "list" (Data==nil) unifies with any list type, but not tables.
	if aIsList && a.Data == nil {
		if bIsList && !b.IsTableType() {
			return b, true
		}
		return Value{}, false
	}
	if bIsList && b.Data == nil {
		if aIsList && !a.IsTableType() {
			return a, true
		}
		return Value{}, false
	}

	// Both must be list types.
	if !aIsList || !bIsList {
		return Value{}, false
	}

	// Check for table types (record-constrained lists).
	// Tables only unify with other tables, never with plain lists.
	aTable := a.IsTableType()
	bTable := b.IsTableType()

	if aTable && bTable {
		// Both table types: unify their record schemas.
		_as3, _ := a.AsTableType()
		_as2, _ := b.AsTableType()
		unified, ok := unifyRecordTypes(_as3.Record, _as2.Record)
		if !ok {
			return Value{}, false
		}
		_as4, _ := unified.AsRecordType()
		return NewTableType(_as4), true
	}

	if aTable || bTable {
		// One is a table, the other is not — cannot unify.
		return Value{}, false
	}

	// Check for typed lists (child type constraints).
	aTyped := a.IsTypedList()
	bTyped := b.IsTypedList()

	if aTyped && bTyped {
		// Both typed lists: unify child types.
		_as5, _ := a.AsChildType()
		aChild := _as5.Child
		_as6, _ := b.AsChildType()
		bChild := _as6.Child
		unified, ok := Unify(aChild, bChild)
		if !ok {
			return Value{}, false
		}
		return NewTypedList(unified), true
	}

	if aTyped {
		// a is typed, b is concrete: each element must unify with the child type.
		_as7, _ := a.AsChildType()
		return unifyTypedWithConcrete(_as7.Child, b.AsList().Slice())
	}

	if bTyped {
		// b is typed, a is concrete: each element must unify with the child type.
		_as8, _ := b.AsChildType()
		return unifyTypedWithConcrete(_as8.Child, a.AsList().Slice())
	}

	// Both concrete lists: element-by-element unification.
	aElems := a.AsList().Slice()
	bElems := b.AsList().Slice()

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
	// Type literal "map" (Data==nil) unifies with any map type, but not records.
	if aIsMap && a.Data == nil {
		if bIsMap && !b.IsRecordType() {
			return b, true
		}
		return Value{}, false
	}
	if bIsMap && b.Data == nil {
		if aIsMap && !a.IsRecordType() {
			return a, true
		}
		return Value{}, false
	}

	// Both must be map types.
	if !aIsMap || !bIsMap {
		return Value{}, false
	}

	// Check for record types (field schema constraints).
	// Records only unify with other records, never with maps or lists.
	aRecord := a.IsRecordType()
	bRecord := b.IsRecordType()

	if aRecord && bRecord {
		// Both record types: unify field schemas with order enforcement.
		_as10, _ := a.AsRecordType()
		_as9, _ := b.AsRecordType()
		return unifyRecordTypes(_as10, _as9)
	}

	if aRecord || bRecord {
		// One is a record, the other is not — cannot unify.
		return Value{}, false
	}

	// Check for options types.
	aOptions := a.IsOptionsType()
	bOptions := b.IsOptionsType()
	if aOptions || bOptions {
		return unifyOptions(a, aOptions, b, bOptions)
	}

	// Check for typed maps (child type constraints).
	aTyped := a.IsTypedMap()
	bTyped := b.IsTypedMap()

	if aTyped && bTyped {
		// Both typed maps: unify child types.
		_as11, _ := a.AsChildType()
		aChild := _as11.Child
		_as12, _ := b.AsChildType()
		bChild := _as12.Child
		unified, ok := Unify(aChild, bChild)
		if !ok {
			return Value{}, false
		}
		return NewTypedMap(unified), true
	}

	if aTyped {
		// a is typed, b is concrete: each value must unify with the child type.
		_as13, _ := a.AsChildType()
		return unifyTypedMapWithConcrete(_as13.Child, b.AsMap())
	}

	if bTyped {
		// b is typed, a is concrete: each value must unify with the child type.
		_as14, _ := b.AsChildType()
		return unifyTypedMapWithConcrete(_as14.Child, a.AsMap())
	}

	// Both concrete maps: key-by-key unification.
	aMap := a.AsMap()
	bMap := b.AsMap()

	noneVal := NewTypeLiteral(TNone)
	result := NewOrderedMap()

	// Walk keys of a in order; missing keys in b unify against None.
	for _, key := range aMap.Keys() {
		aVal, _ := aMap.Get(key)
		bVal, ok := bMap.Get(key)
		if !ok {
			// Key in a but not in b — try unifying a's value with None.
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

	// Keys in b but not in a — try unifying b's value with None.
	for _, key := range bMap.Keys() {
		if _, ok := aMap.Get(key); ok {
			continue // already handled above
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

// unifyTypedMapWithConcrete unifies a child type constraint against each value
// of a concrete map. Every value must unify with the child type.
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

// unifyRecordTypes unifies two record types by unifying their field schemas.
// Both must have exactly the same keys in exactly the same order; each field
// type pair is unified. Field order is significant.
func unifyRecordTypes(a, b RecordTypeInfo) (Value, bool) {
	aFields := a.Fields
	bFields := b.Fields

	if aFields.Len() != bFields.Len() {
		return Value{}, false
	}

	aKeys := aFields.Keys()
	bKeys := bFields.Keys()

	result := NewOrderedMap()
	for i, key := range aKeys {
		// Field order must match.
		if bKeys[i] != key {
			return Value{}, false
		}
		aVal, _ := aFields.Get(key)
		bVal, _ := bFields.Get(key)
		unified, uOk := Unify(aVal, bVal)
		if !uOk {
			return Value{}, false
		}
		result.Set(key, unified)
	}

	return NewRecordType(result), true
}

// unifyOptions handles unification when at least one side is an options type.
func unifyOptions(a Value, aIsOptions bool, b Value, bIsOptions bool) (Value, bool) {
	if aIsOptions && bIsOptions {
		_as16, _ := a.AsOptionsType()
		_as15, _ := b.AsOptionsType()
		return unifyOptionsPair(_as16, _as15)
	}

	// Normalize: opts is the Options side, concrete is the other.
	var opts OptionsTypeInfo
	var concrete Value
	if aIsOptions {
		opts, _ = a.AsOptionsType()
		concrete = b
	} else {
		opts, _ = b.AsOptionsType()
		concrete = a
	}

	// Bare Map type literal unifies with Options.
	if concrete.Data == nil {
		return NewOptionsType(opts.Fields), true
	}

	// Other side must be a plain concrete map.
	if concrete.IsRecordType() || concrete.IsTypedMap() || concrete.IsOptionsType() {
		return Value{}, false
	}

	cMap := concrete.AsMap()

	// Extra keys in concrete not in Options → fail.
	for _, key := range cMap.Keys() {
		if _, ok := opts.Fields.Get(key); !ok {
			return Value{}, false
		}
	}

	result := NewOrderedMap()

	for _, key := range opts.Fields.Keys() {
		optVal, _ := opts.Fields.Get(key)
		cVal, present := cMap.Get(key)

		if !present {
			// Key absent: use default or fail.
			defVal, ok := optionsDefault(optVal)
			if !ok {
				return Value{}, false
			}
			result.Set(key, defVal)
		} else {
			// Key present: apply Options field rules.
			unified, ok := unifyOptionsField(optVal, cVal)
			if !ok {
				return Value{}, false
			}
			result.Set(key, unified)
		}
	}

	return NewMap(result), true
}

// optionsDefault determines the default value for an Options field when
// the key is absent from the concrete map.
// - Concrete value → use as default
// - None → use None
// - Type literal (Data==nil) → fail (requires a value)
// - Disjunct → None if present, else first concrete alternative, else fail
func optionsDefault(v Value) (Value, bool) {
	if v.IsDisjunct() {
		_as17, _ := v.AsDisjunct()
		alts := _as17.Alternatives
		// Check for None first.
		for _, alt := range alts {
			if alt.VType.Equal(TNone) {
				return NewTypeLiteral(TNone), true
			}
		}
		// Check for a concrete alternative.
		for _, alt := range alts {
			if alt.Data != nil && !alt.IsDisjunct() {
				return alt, true
			}
		}
		return Value{}, false
	}

	if v.VType.Equal(TNone) {
		return v, true
	}

	// Concrete value (Data != nil) → use as default.
	if v.Data != nil {
		return v, true
	}

	// Type literal (Data == nil) → no default available.
	return Value{}, false
}

// unifyOptionsField applies Options unification rules for a single field
// when the key IS present in the concrete map.
// - Concrete Options value: accept cVal if same parent type (cVal wins)
// - Type literal: standard Unify (type narrowing)
// - Disjunct: apply rules to each term
func unifyOptionsField(optVal, cVal Value) (Value, bool) {
	if optVal.IsDisjunct() {
		_as18, _ := optVal.AsDisjunct()
		for _, alt := range _as18.Alternatives {
			if unified, ok := unifyOptionsField(alt, cVal); ok {
				return unified, true
			}
		}
		return Value{}, false
	}

	// Concrete default: accept cVal if compatible type.
	if optVal.Data != nil {
		baseType := optionsBaseType(optVal)
		if cVal.VType.Matches(baseType) {
			return cVal, true
		}
		return Value{}, false
	}

	// Type literal: standard unification.
	return Unify(optVal, cVal)
}

// optionsBaseType returns the base (non-literal) type for a concrete value.
// For example, integer 42 (Scalar/Number/Integer/42) returns TInteger.
func optionsBaseType(v Value) Type {
	switch {
	case v.VType.Matches(TInteger):
		return TInteger
	case v.VType.Matches(TDecimal):
		return TDecimal
	case v.VType.Matches(TString):
		return TString
	case v.VType.Matches(TBoolean):
		return TBoolean
	case v.VType.Equal(TMap):
		return TMap
	case v.VType.Equal(TList):
		return TList
	case v.VType.Equal(TNone):
		return TNone
	default:
		return v.VType
	}
}

// unifyOptionsPair unifies two options types by unifying their field schemas.
func unifyOptionsPair(a, b OptionsTypeInfo) (Value, bool) {
	if a.Fields.Len() != b.Fields.Len() {
		return Value{}, false
	}
	result := NewOrderedMap()
	for _, key := range a.Fields.Keys() {
		aVal, _ := a.Fields.Get(key)
		bVal, ok := b.Fields.Get(key)
		if !ok {
			return Value{}, false
		}
		unified, uOk := Unify(aVal, bVal)
		if !uOk {
			return Value{}, false
		}
		result.Set(key, unified)
	}
	return NewOptionsType(result), true
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
		_as20, _ := a.AsString()
		_as19, _ := b.AsString()
		return _as20 == _as19
	case a.VType.Matches(TInteger):
		_as22, _ := a.AsInteger()
		_as21, _ := b.AsInteger()
		return _as22 == _as21
	case a.VType.Matches(TBoolean):
		_as24, _ := a.AsBoolean()
		_as23, _ := b.AsBoolean()
		return _as24 == _as23
	case a.VType.Equal(TList):
		aTT, aTbl := a.Data.(TableTypeInfo)
		bTT, bTbl := b.Data.(TableTypeInfo)
		if aTbl && bTbl {
			return mapsEqual(aTT.Record.Fields, bTT.Record.Fields)
		}
		if aTbl != bTbl {
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
		return listsEqual(a.AsList().Slice(), b.AsList().Slice())
	case a.VType.Equal(TMap):
		aRT, aRec := a.Data.(RecordTypeInfo)
		bRT, bRec := b.Data.(RecordTypeInfo)
		if aRec && bRec {
			return mapsEqual(aRT.Fields, bRT.Fields)
		}
		if aRec != bRec {
			return false
		}
		aOT, aOpt := a.Data.(OptionsTypeInfo)
		bOT, bOpt := b.Data.(OptionsTypeInfo)
		if aOpt && bOpt {
			return mapsEqual(aOT.Fields, bOT.Fields)
		}
		if aOpt != bOpt {
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
func mapsEqual(a, b ReadMap) bool {
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

// unifyDisjunct tries to unify a value against each alternative in a disjunct.
// Returns the first successful unification. For map alternatives, uses open
// (subset) matching where the candidate only needs to contain the alternative's
// key-value pairs.
func unifyDisjunct(disj DisjunctInfo, val Value) (Value, bool) {
	// "any" unifies with the whole disjunct, preserving it.
	if val.VType.Equal(TAny) {
		return NewDisjunct(disj.Alternatives), true
	}

	for _, alt := range disj.Alternatives {
		// For concrete map alternatives against concrete map values,
		// use open (subset) matching.
		if alt.VType.Equal(TMap) && val.VType.Equal(TMap) &&
			!alt.IsRecordType() && !val.IsRecordType() &&
			!alt.IsTypedMap() && !val.IsTypedMap() &&
			!alt.IsOptionsType() && !val.IsOptionsType() {
			if alt.Data != nil && val.Data != nil {
				if openUnifyMap(alt, val) {
					return val, true
				}
				continue
			}
		}

		// Standard unification for all other cases.
		unified, ok := Unify(alt, val)
		if ok {
			return unified, true
		}
	}
	return Value{}, false
}

// openUnifyMap checks whether candidate contains at least the key-value pairs
// of pattern. Extra keys in candidate are allowed (open/subset matching).
func openUnifyMap(pattern, candidate Value) bool {
	pMap := pattern.AsMap()
	cMap := candidate.AsMap()

	for _, key := range pMap.Keys() {
		pVal, _ := pMap.Get(key)
		cVal, ok := cMap.Get(key)
		if !ok {
			return false
		}
		if _, uOk := Unify(pVal, cVal); !uOk {
			return false
		}
	}
	return true
}

// resolveWordsDeep recursively resolves word values to their semantic form.
// For lists, each element is resolved; for maps, each value is resolved.
// Scalar words are resolved via resolveWordValue.
func resolveWordsDeep(v Value) Value {
	if v.IsWord() {
		return resolveWordValue(v)
	}
	if v.VType.Equal(TList) && v.Data != nil && !v.IsTypedList() && !v.IsTableType() {
		elems := v.AsList().Slice()
		resolved := make([]Value, len(elems))
		for i, e := range elems {
			resolved[i] = resolveWordsDeep(e)
		}
		return NewList(resolved)
	}
	if v.VType.Equal(TMap) && v.Data != nil && !v.IsTypedMap() && !v.IsRecordType() && !v.IsOptionsType() {
		m := v.AsMap()
		result := NewOrderedMap()
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			result.Set(key, resolveWordsDeep(val))
		}
		return NewMap(result)
	}
	return v
}

// RegisterUnify registers the "unify" word in the given registry.
func RegisterUnify(r *Registry) {
	unifyHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		unified, ok := Unify(args[0], args[1])
		if ok {
			return []Value{unified, NewBoolean(true)}, nil
		}
		return []Value{NewString("~unify-fail"), NewBoolean(false)}, nil
	}

	// unify: [any, any] -> [any, boolean]
	r.RegisterNativeFunc(NativeFunc{
		Name:              "unify",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{TAny, TAny},
			Handler: unifyHandler,
			Returns: []Type{TAny, TBoolean},
		}},
	})
}
