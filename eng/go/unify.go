package eng

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
	a = ResolveWordsDeep(a)
	b = ResolveWordsDeep(b)

	// A type literal IS its lattice node after the type/value merge,
	// so its denoted type is the value itself — not its Parent, which
	// is now the supertype. Carriers keep Parent pointing at the type
	// they carry, so they take the plain Parent. A Data==nil value
	// with an empty ID is a manually-constructed `Value{Parent: T,
	// Data: nil}` (used in tests as a stand-in for a value of type T);
	// treat its Parent as the denoted type since &a has no lattice
	// identity to compare against.
	aType := a.Parent
	if a.Data == nil && !a.Carrier && a.ID != "" {
		aType = &a
	}
	bType := b.Parent
	if b.Data == nil && !b.Carrier && b.ID != "" {
		bType = &b
	}

	// Disjunct unification first: try each alternative, succeed on first match.
	// Must come before none/any checks so that disjuncts containing none work.
	if IsDisjunct(a) {
		disj, _ := AsDisjunct(a)
		return unifyDisjunct(disj, b)
	}
	if IsDisjunct(b) {
		disj, _ := AsDisjunct(b)
		return unifyDisjunct(disj, a)
	}

	// "never" is the bottom type — uninhabited, only unifies with
	// itself. Unify(Never, T) for T != Never always fails: there is
	// no value satisfying both types because Never has no values at
	// all. Checked before None and Any so that Never on either side
	// short-circuits before any other rule applies.
	aNever := aType.Equal(TNever)
	bNever := bType.Equal(TNever)
	if aNever || bNever {
		if aNever && bNever {
			return a, true
		}
		return Value{}, false
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
	// <"z"). Its Parent IS the base scalar; the DepScalarInfo payload
	// is what distinguishes it from an ordinary scalar value. Three
	// cases:
	//   1. DepScalar vs concrete scalar: succeeds iff the scalar's
	//      type matches the base AND the value satisfies the
	//      comparison. Returns the plain scalar (not the DepScalar)
	//      so downstream consumers see a normal value.
	//   2. DepScalar vs DepScalar over the same base: combine the
	//      constraints (intersection) — same-side bounds tighten,
	//      opposite-side bounds form an interval. Empty result
	//      (e.g. gt 10 vs lt 5) fails. Returns a fresh DepScalar.
	//   3. DepScalar vs DepScalar over different bases: fails
	//      (incompatible bases).
	if a.IsDepScalar() && b.IsDepScalar() {
		if !aType.Equal(bType) {
			return Value{}, false
		}
		aInfo, err := a.AsDepScalar()
		if err != nil {
			return Value{}, false
		}
		bInfo, err := b.AsDepScalar()
		if err != nil {
			return Value{}, false
		}
		combined, ok := combineDepScalars(aInfo, bInfo)
		if !ok {
			return Value{}, false
		}
		return NewValueRaw(aType, combined), true
	}
	if a.IsDepScalar() && !b.IsDepScalar() && b.Data != nil {
		if bType.Matches(aType) {
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
		if aType.Matches(bType) {
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

	// Function-signature unification. A FnUndef value carries one or
	// more (input, output) sig patterns and acts as a structural
	// function-type constraint; the other side must be a function
	// value (TFnDef or TFunction wrapping FnDefInfo) whose signatures
	// cover the FnUndef pattern. The first slice uses exact-match
	// rules — see the recommendation block in the commit message for
	// the variance/overload extensions planned for a follow-up.
	if aType.Equal(TFnUndef) && (bType.Equal(TFnDef) || bType.Equal(TFunction)) {
		if FnUndefMatchesFnDef(a, b) {
			return b, true
		}
		return Value{}, false
	}
	if bType.Equal(TFnUndef) && (aType.Equal(TFnDef) || aType.Equal(TFunction)) {
		if FnUndefMatchesFnDef(b, a) {
			return a, true
		}
		return Value{}, false
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
		if ValuesEqual(a, b) {
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
		if bIsList && !IsTableType(b) {
			return b, true
		}
		return Value{}, false
	}
	if bIsList && b.Data == nil {
		if aIsList && !IsTableType(a) {
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
	aTable := IsTableType(a)
	bTable := IsTableType(b)

	if aTable && bTable {
		// Both table types: unify their record schemas.
		aTT, _ := AsTableType(a)
		bTT, _ := AsTableType(b)
		unified, ok := unifyRecordTypes(aTT.Record, bTT.Record)
		if !ok {
			return Value{}, false
		}
		uRec, _ := AsRecordType(unified)
		return NewTableType(uRec), true
	}

	if aTable || bTable {
		// One is a table, the other is not — cannot unify.
		return Value{}, false
	}

	// Check for typed lists (child type constraints).
	aTyped := IsTypedList(a)
	bTyped := IsTypedList(b)

	if aTyped && bTyped {
		// Both typed lists: unify child types.
		aCT, _ := AsChildType(a)
		bCT, _ := AsChildType(b)
		unified, ok := Unify(aCT.Child, bCT.Child)
		if !ok {
			return Value{}, false
		}
		return NewTypedList(unified), true
	}

	if aTyped {
		// a is typed, b is concrete: each element must unify with the child type.
		aCT, _ := AsChildType(a)
		bLst, _ := AsList(b)
		return unifyTypedWithConcrete(aCT.Child, bLst.Slice())
	}

	if bTyped {
		// b is typed, a is concrete: each element must unify with the child type.
		bCT, _ := AsChildType(b)
		aLst, _ := AsList(a)
		return unifyTypedWithConcrete(bCT.Child, aLst.Slice())
	}

	// Both concrete lists: element-by-element unification.
	aLst, _ := AsList(a)
	aElems := aLst.Slice()
	bLst, _ := AsList(b)
	bElems := bLst.Slice()

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
		if bIsMap && !IsRecordType(b) {
			return b, true
		}
		return Value{}, false
	}
	if bIsMap && b.Data == nil {
		if aIsMap && !IsRecordType(a) {
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
	aRecord := IsRecordType(a)
	bRecord := IsRecordType(b)

	if aRecord && bRecord {
		// Both record types: unify field schemas with order enforcement.
		aRT, _ := AsRecordType(a)
		bRT, _ := AsRecordType(b)
		return unifyRecordTypes(aRT, bRT)
	}

	if aRecord || bRecord {
		// One is a record, the other is not — cannot unify.
		return Value{}, false
	}

	// Check for options types.
	aOptions := IsOptionsType(a)
	bOptions := IsOptionsType(b)
	if aOptions || bOptions {
		return unifyOptions(a, aOptions, b, bOptions)
	}

	// Check for typed maps (child type constraints).
	aTyped := IsTypedMap(a)
	bTyped := IsTypedMap(b)

	if aTyped && bTyped {
		// Both typed maps: unify child types.
		aCT, _ := AsChildType(a)
		bCT, _ := AsChildType(b)
		unified, ok := Unify(aCT.Child, bCT.Child)
		if !ok {
			return Value{}, false
		}
		return NewTypedMap(unified), true
	}

	if aTyped {
		// a is typed, b is concrete: each value must unify with the child type.
		aCT, _ := AsChildType(a)
		bMap, _ := AsMap(b)
		return unifyTypedMapWithConcrete(aCT.Child, bMap)
	}

	if bTyped {
		// b is typed, a is concrete: each value must unify with the child type.
		bCT, _ := AsChildType(b)
		aMap, _ := AsMap(a)
		return unifyTypedMapWithConcrete(bCT.Child, aMap)
	}

	// Both concrete maps: key-by-key unification.
	aMap, _ := AsMap(a)
	bMap, _ := AsMap(b)

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

// unifyFieldBags unifies two field-schema maps key-by-key. Both must
// hold the same number of fields and each field-type pair must unify.
// When orderStrict is true the keys must also appear in the same order
// (record-type semantics); when false key order is irrelevant
// (options-type semantics). Returns the merged schema and ok.
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
// schemas. Keys must match in the same order — field order is part of
// a record type's identity.
func unifyRecordTypes(a, b RecordTypeInfo) (Value, bool) {
	result, ok := unifyFieldBags(a.Fields, b.Fields, true)
	if !ok {
		return Value{}, false
	}
	return NewRecordType(result), true
}

// unifyOptions handles unification when at least one side is an options type.
func unifyOptions(a Value, aIsOptions bool, b Value, bIsOptions bool) (Value, bool) {
	if aIsOptions && bIsOptions {
		aOT, _ := AsOptionsType(a)
		bOT, _ := AsOptionsType(b)
		return unifyOptionsPair(aOT, bOT)
	}

	// Normalize: opts is the Options side, concrete is the other.
	var opts OptionsTypeInfo
	var concrete Value
	if aIsOptions {
		opts, _ = AsOptionsType(a)
		concrete = b
	} else {
		opts, _ = AsOptionsType(b)
		concrete = a
	}

	// Bare Map type literal unifies with Options.
	if concrete.Data == nil {
		return NewOptionsType(opts.Fields), true
	}

	// Other side must be a plain concrete map.
	if IsRecordType(concrete) || IsTypedMap(concrete) || IsOptionsType(concrete) {
		return Value{}, false
	}

	cMap, _ := AsMap(concrete)

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
	if IsDisjunct(v) {
		disj, _ := AsDisjunct(v)
		alts := disj.Alternatives
		// Check for None first — match either form (sentinel value
		// or NewTypeLiteral(TNone) where Parent is nil post the
		// degenerate-root setup).
		for _, alt := range alts {
			if IsNoneShape(alt) {
				return NewTypeLiteral(TNone), true
			}
		}
		// Check for a concrete alternative.
		for _, alt := range alts {
			if alt.Data != nil && !IsDisjunct(alt) {
				return alt, true
			}
		}
		return Value{}, false
	}

	if IsNoneShape(v) {
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
	if IsDisjunct(optVal) {
		disj, _ := AsDisjunct(optVal)
		for _, alt := range disj.Alternatives {
			if unified, ok := unifyOptionsField(alt, cVal); ok {
				return unified, true
			}
		}
		return Value{}, false
	}

	// Concrete default: accept cVal if compatible type.
	if optVal.Data != nil {
		baseType := optionsBaseType(optVal)
		if cVal.Parent.Matches(baseType) {
			return cVal, true
		}
		return Value{}, false
	}

	// Type literal: standard unification.
	return Unify(optVal, cVal)
}

// optionsBaseType returns the base (non-literal) type for a concrete value.
// For example, integer 42 (Scalar/Number/Integer/42) returns TInteger.
func optionsBaseType(v Value) *Type {
	switch {
	case v.Parent.Matches(TInteger):
		return TInteger
	case v.Parent.Matches(TDecimal):
		return TDecimal
	case v.Parent.Matches(TString):
		return TString
	case v.Parent.Matches(TBoolean):
		return TBoolean
	case v.Parent.Equal(TMap):
		return TMap
	case v.Parent.Equal(TList):
		return TList
	case v.Parent.Equal(TNone):
		return TNone
	default:
		return v.Parent
	}
}

// unifyOptionsPair unifies two options types by unifying their field
// schemas. Key order is not significant.
func unifyOptionsPair(a, b OptionsTypeInfo) (Value, bool) {
	result, ok := unifyFieldBags(a.Fields, b.Fields, false)
	if !ok {
		return Value{}, false
	}
	return NewOptionsType(result), true
}

// unifyDisjunct tries to unify a value against each alternative in a disjunct.
// Returns the first successful unification. For map alternatives, uses open
// (subset) matching where the candidate only needs to contain the alternative's
// key-value pairs.
func unifyDisjunct(disj DisjunctInfo, val Value) (Value, bool) {
	// "any" unifies with the whole disjunct, preserving it. Covers
	// two value shapes: the bare type literal NewTypeLiteral(TAny)
	// (Data=nil; the value IS the TAny lattice node, &val.Equal(TAny))
	// and the Any-typed carrier (Data=nil, Carrier=true, Parent=TAny).
	if val.Data == nil && (val.Parent.Equal(TAny) || (&val).Equal(TAny)) {
		return NewDisjunct(disj.Alternatives), true
	}

	for _, alt := range disj.Alternatives {
		// For concrete map alternatives against concrete map values,
		// use open (subset) matching.
		if alt.Parent.Equal(TMap) && val.Parent.Equal(TMap) &&
			!IsRecordType(alt) && !IsRecordType(val) &&
			!IsTypedMap(alt) && !IsTypedMap(val) &&
			!IsOptionsType(alt) && !IsOptionsType(val) {
			if alt.Data != nil && val.Data != nil {
				if OpenUnifyMap(alt, val) {
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
