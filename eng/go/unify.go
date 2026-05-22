package eng

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
	a = ResolveWordsDeep(a)
	b = ResolveWordsDeep(b)

	// A type literal IS its lattice node after the type/value merge,
	// so its denoted type is the value itself — not its Parent, which
	// is now the supertype. Carriers keep Parent pointing at the type
	// they carry, so they take the plain Parent.
	aType := a.Parent
	if a.Data == nil && !a.Carrier {
		aType = &a
	}
	bType := b.Parent
	if b.Data == nil && !b.Carrier {
		bType = &b
	}

	// Disjunct unification first: try each alternative, succeed on first match.
	// Must come before none/any checks so that disjuncts containing none work.
	if IsDisjunct(a) {
		_as0, _ := AsDisjunct(a)
		return unifyDisjunct(_as0, b)
	}
	if IsDisjunct(b) {
		_as1, _ := AsDisjunct(b)
		return unifyDisjunct(_as1, a)
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
		out := NewValueRaw(aType, combined)
		return out, true
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
		_as3, _ := AsTableType(a)
		_as2, _ := AsTableType(b)
		unified, ok := unifyRecordTypes(_as3.Record, _as2.Record)
		if !ok {
			return Value{}, false
		}
		_as4, _ := AsRecordType(unified)
		return NewTableType(_as4), true
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
		_as5, _ := AsChildType(a)
		aChild := _as5.Child
		_as6, _ := AsChildType(b)
		bChild := _as6.Child
		unified, ok := Unify(aChild, bChild)
		if !ok {
			return Value{}, false
		}
		return NewTypedList(unified), true
	}

	if aTyped {
		// a is typed, b is concrete: each element must unify with the child type.
		_as7, _ := AsChildType(a)
		_bl, _ := AsList(b)
		return unifyTypedWithConcrete(_as7.Child, _bl.Slice())
	}

	if bTyped {
		// b is typed, a is concrete: each element must unify with the child type.
		_as8, _ := AsChildType(b)
		_al, _ := AsList(a)
		return unifyTypedWithConcrete(_as8.Child, _al.Slice())
	}

	// Both concrete lists: element-by-element unification.
	_aLst, _ := AsList(a)
	aElems := _aLst.Slice()
	_bLst, _ := AsList(b)
	bElems := _bLst.Slice()

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
		_as10, _ := AsRecordType(a)
		_as9, _ := AsRecordType(b)
		return unifyRecordTypes(_as10, _as9)
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
		_as11, _ := AsChildType(a)
		aChild := _as11.Child
		_as12, _ := AsChildType(b)
		bChild := _as12.Child
		unified, ok := Unify(aChild, bChild)
		if !ok {
			return Value{}, false
		}
		return NewTypedMap(unified), true
	}

	if aTyped {
		// a is typed, b is concrete: each value must unify with the child type.
		_as13, _ := AsChildType(a)
		_bMap, _ := AsMap(b)
		return unifyTypedMapWithConcrete(_as13.Child, _bMap)
	}

	if bTyped {
		// b is typed, a is concrete: each value must unify with the child type.
		_as14, _ := AsChildType(b)
		_aMap, _ := AsMap(a)
		return unifyTypedMapWithConcrete(_as14.Child, _aMap)
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
		_as16, _ := AsOptionsType(a)
		_as15, _ := AsOptionsType(b)
		return unifyOptionsPair(_as16, _as15)
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
		_as17, _ := AsDisjunct(v)
		alts := _as17.Alternatives
		// Check for None first.
		for _, alt := range alts {
			if alt.Parent.Equal(TNone) {
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

	if v.Parent.Equal(TNone) {
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
		_as18, _ := AsDisjunct(optVal)
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

// ValuesEqual compares the data payloads of two values with the same type.
//
// Routes through Behavior.Equal for the same-Parent case so types
// with normalisation semantics (CalDuration, DepScalar in a future
// step, and plugin types) can supply their own equality. The
// cross-Parent case falls through to the default switch since
// equality across types is a matching-strategy concern, not a
// per-type concern.
func ValuesEqual(a, b Value) bool {
	// Pluggable equality: when both sides share a Parent with a
	// non-default Behavior, delegate. Type literals (Data==nil) are
	// excluded — bare type equality is a lattice-identity check, not
	// a per-type semantic compare.
	if a.Data != nil && b.Data != nil &&
		a.Parent != nil && a.Parent == b.Parent &&
		a.Parent.Behavior != nil && a.Parent.Behavior != DefaultBehavior {
		return a.Parent.Behavior.Equal(a, b)
	}
	return valuesEqualDefault(a, b)
}

// valuesEqualDefault is the kernel's default equality path,
// bypassing the Behavior dispatch in ValuesEqual. Used by
// DefaultBehavior.Equal and by Behavior implementations that
// override Format but want fall-through equality without
// triggering infinite re-entry.
func valuesEqualDefault(a, b Value) bool {
	// Two Data==nil values: carriers are abstract (conservatively
	// equal); two type literals are equal iff they are the same
	// lattice identity.
	if a.Data == nil && b.Data == nil {
		if a.Carrier || b.Carrier {
			return true
		}
		return a.Equal(&b)
	}
	// One is a type literal and the other is a concrete value — not equal.
	if a.Data == nil || b.Data == nil {
		return false
	}
	// Dependent scalar: route to payload comparison BEFORE the
	// Matches(TString)/Matches(TInteger)/... dispatch below. The
	// lattice override makes DepString.Matches(TString)=true, so
	// without this branch a DepScalar would fall into AsString and
	// silently compare zero-value payloads.
	if a.IsDepScalar() || b.IsDepScalar() {
		if !a.IsDepScalar() || !b.IsDepScalar() {
			return false
		}
		ai, err := a.AsDepScalar()
		if err != nil {
			return false
		}
		bi, err := b.AsDepScalar()
		if err != nil {
			return false
		}
		return depScalarsEqual(ai, bi)
	}
	switch {
	case a.Parent.Matches(TString):
		_as20, _ := AsString(a)
		_as19, _ := AsString(b)
		return _as20 == _as19
	case a.Parent.Matches(TInteger):
		_as22, _ := AsInteger(a)
		_as21, _ := AsInteger(b)
		return _as22 == _as21
	case a.Parent.Matches(TBoolean):
		_as24, _ := AsBoolean(a)
		_as23, _ := AsBoolean(b)
		return _as24 == _as23
	case a.Parent.Equal(TList):
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
			return aCT.Child.Parent.Equal(bCT.Child.Parent) && ValuesEqual(aCT.Child, bCT.Child)
		}
		if aOk != bOk {
			return false
		}
		_aLst, _ := AsList(a)
		_bLst, _ := AsList(b)
		return listsEqual(_aLst.Slice(), _bLst.Slice())
	case a.Parent.Equal(TMap):
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
			return aCT.Child.Parent.Equal(bCT.Child.Parent) && ValuesEqual(aCT.Child, bCT.Child)
		}
		if aOk != bOk {
			return false
		}
		_aMap, _ := AsMap(a)
		_bMap, _ := AsMap(b)
		return mapsEqual(_aMap, _bMap)
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
		if !a[i].Parent.Equal(b[i].Parent) || !ValuesEqual(a[i], b[i]) {
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
		if !aVal.Parent.Equal(bVal.Parent) || !ValuesEqual(aVal, bVal) {
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
	if val.Parent.Equal(TAny) {
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

// OpenUnifyMap checks whether candidate contains at least the key-value pairs
// of pattern. Extra keys in candidate are allowed (open/subset matching).
func OpenUnifyMap(pattern, candidate Value) bool {
	pMap, _ := AsMap(pattern)
	cMap, _ := AsMap(candidate)

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

// ResolveWordsDeep recursively resolves word values to their semantic form.
// For lists, each element is resolved; for maps, each value is resolved.
// Scalar words are resolved via ResolveWordValue.
func ResolveWordsDeep(v Value) Value {
	if IsWord(v) {
		return ResolveWordValue(v)
	}
	if v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v) {
		_lst, _ := AsList(v)
		elems := _lst.Slice()
		resolved := make([]Value, len(elems))
		for i, e := range elems {
			resolved[i] = ResolveWordsDeep(e)
		}
		return NewList(resolved)
	}
	if v.Parent.Equal(TMap) && v.Data != nil && !IsTypedMap(v) && !IsRecordType(v) && !IsOptionsType(v) {
		m, _ := AsMap(v)
		result := NewOrderedMap()
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			result.Set(key, ResolveWordsDeep(val))
		}
		return NewMap(result)
	}
	return v
}

// The `unify` word registration lives in
// lang/go/engine/native_unify.go. UnifyHandler below is the exported
// algorithm primitive lang's registration wires dispatch into.

func UnifyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	unified, ok := Unify(args[0], args[1])
	if ok {
		return []Value{unified, NewBoolean(true)}, nil
	}
	return []Value{NewString("~unify-fail"), NewBoolean(false)}, nil
}
