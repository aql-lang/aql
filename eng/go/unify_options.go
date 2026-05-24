package eng

// unifyOptionsFamily owns unification when at least one side is an
// Options type. Options fields can carry concrete defaults, type-
// literal constraints, or disjuncts — three sub-rules that compose
// per-field.
func unifyOptionsFamily(a Value, sa ValueShape, b Value, sb ValueShape) (Value, *UnifyError) {
	// Two Options → unify field schemas (order-independent).
	if sa == ShapeOptions && sb == ShapeOptions {
		aOT, _ := AsOptionsType(a)
		bOT, _ := AsOptionsType(b)
		return unifyOptionsPair(aOT, bOT)
	}

	// Canonicalize: opts on the left, concrete on the right.
	var opts OptionsTypeInfo
	var concrete Value
	if sa == ShapeOptions {
		opts, _ = AsOptionsType(a)
		concrete = b
	} else {
		opts, _ = AsOptionsType(b)
		concrete = a
	}

	// Bare Map type literal vs Options → preserve the Options schema.
	if concrete.Data == nil {
		return NewOptionsType(opts.Fields), nil
	}

	// Options only accepts plain concrete maps, never structural map
	// subtypes (Record / TypedMap / nested Options).
	if IsRecordType(concrete) || IsTypedMap(concrete) || IsOptionsType(concrete) {
		return Value{}, unifyFail("Options only unifies with a plain Map", a, b)
	}

	cMap, _ := AsMap(concrete)

	// Extra keys in concrete not in Options → fail.
	for _, key := range cMap.Keys() {
		if _, ok := opts.Fields.Get(key); !ok {
			return Value{}, unifyFail("unknown key "+key+" not in Options schema", a, b)
		}
	}

	result := NewOrderedMap()
	for _, key := range opts.Fields.Keys() {
		optVal, _ := opts.Fields.Get(key)
		cVal, present := cMap.Get(key)
		if !present {
			defVal, ok := optionsDefault(optVal)
			if !ok {
				return Value{}, &UnifyError{
					Reason: "missing required key " + key + " with no default",
					Path:   []string{"field:" + key},
				}
			}
			result.Set(key, defVal)
			continue
		}
		unified, err := unifyOptionsField(optVal, cVal)
		if err != nil {
			return Value{}, err.withPath("field:" + key)
		}
		result.Set(key, unified)
	}
	return NewMap(result), nil
}

// unifyOptionsPair unifies two options types by unifying their field
// schemas. Key order is not significant.
func unifyOptionsPair(a, b OptionsTypeInfo) (Value, *UnifyError) {
	result, err := unifyFieldBags(a.Fields, b.Fields, false)
	if err != nil {
		return Value{}, err
	}
	return NewOptionsType(result), nil
}

// optionsDefault determines the default value for an Options field when
// the key is absent from the concrete map.
//   - Concrete value → use as default
//   - None → use None
//   - Type literal (Data==nil) → fail (requires a value)
//   - Disjunct → None if present, else first concrete alternative, else fail
func optionsDefault(v Value) (Value, bool) {
	if IsDisjunct(v) {
		disj, _ := AsDisjunct(v)
		alts := disj.Alternatives
		for _, alt := range alts {
			if IsNoneShape(alt) {
				return NewTypeLiteral(TNone), true
			}
		}
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
	if v.Data != nil {
		return v, true
	}
	return Value{}, false
}

// unifyOptionsField applies Options unification rules for a single
// field when the key IS present in the concrete map.
//   - Concrete Options value: accept cVal if same parent type (cVal wins)
//   - Type literal: standard Unify (type narrowing)
//   - Disjunct: apply rules to each alternative
func unifyOptionsField(optVal, cVal Value) (Value, *UnifyError) {
	if IsDisjunct(optVal) {
		disj, _ := AsDisjunct(optVal)
		for _, alt := range disj.Alternatives {
			if unified, err := unifyOptionsField(alt, cVal); err == nil {
				return unified, nil
			}
		}
		return Value{}, unifyFail("no disjunct alternative matched", optVal, cVal)
	}
	if optVal.Data != nil {
		baseType := optionsBaseType(optVal)
		if cVal.Parent.Matches(baseType) {
			return cVal, nil
		}
		return Value{}, unifyFail("value does not match field's base type", optVal, cVal)
	}
	return unifyInner(optVal, cVal)
}

// optionsBaseType returns the base (non-literal) type for a concrete
// value. For example, integer 42 (Scalar/Number/Integer/42) returns
// TInteger.
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
