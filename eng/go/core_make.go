package eng

import (
	"fmt"
	"strconv"
	"strings"
)

// registerCoreMake installs `make TARGET data` — the universal
// constructor for typed values. Multiple overloads cover the major
// type-construction shapes:
//
//	make ScalarType data            cast / parse a scalar
//	make ScalarType {opts} data     scalar with options (Path abs flag)
//	make ObjectType data            instantiate a named object
//	make Object data Object         instantiate with prototype
//	make Array [list]               build an Array
//	make Type Type {opts}           three-arg shape with arbitrary options
//	make Type Any                   two-arg fallback
//
// Mirrors the production lang `make` (formerly in
// lang/engine/native_type_make_helpers.go); the handlers are
// ported verbatim. Lang re-exports the helpers via aliases so any
// callers that reach into the package-private surface keep working.
func registerCoreMake(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name:              "make",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TScalarType, TMap, TAny}, Handler: makeScalarOptsHandler, ReturnsFn: ReturnsIdentity(0)},
			{Args: []Type{TObjectType, TMap}, Handler: makeObjHandler, ReturnsFn: ReturnsIdentity(0)},
			{Args: []Type{TArray, TList}, Handler: makeArrayHandler, Returns: []Type{TArray}},
			{Args: []Type{TScalarType, TAny}, Handler: makeScalarHandler, ReturnsFn: ReturnsIdentity(0)},
			{Args: []Type{TObject, TAny, TObject}, Handler: makeWithPrototype, Returns: []Type{TObject}},
			{Args: []Type{TAny, TAny, TMap}, Handler: makeWithOpts, Returns: []Type{TAny}},
			{Args: []Type{TAny, TAny}, Handler: makeHandler, Returns: []Type{TAny}},
		},
	})
}

// isTypeLike returns true if v looks like a type target for make
// (type literal with nil Data, record type, options type, table
// type, or object type).
func isTypeLike(v Value) bool {
	if v.Data == nil {
		return true
	}
	return v.IsRecordType() || v.IsOptionsType() || v.IsTableType() || v.IsObjectType()
}

// makeRecord creates a record instance from a source value and
// options. Used by both 2-arg and 3-arg make sigs targeting
// RecordTypeInfo.
func makeRecord(recType RecordTypeInfo, srcVal Value, useBase bool) ([]Value, error) {
	fieldKeys := recType.Fields.Keys()
	result := NewOrderedMap()

	fillFromMap := func(provided *OrderedMap) error {
		for _, key := range provided.Keys() {
			if _, ok := recType.Fields.Get(key); !ok {
				return fmt.Errorf("make: unknown field %q", key)
			}
		}
		for _, key := range fieldKeys {
			constraint, _ := recType.Fields.Get(key)
			val, ok := provided.Get(key)
			if !ok {
				if useBase {
					bv, err := BaseValueForConstraint(constraint)
					if err != nil {
						return fmt.Errorf("make: field %q: %w", key, err)
					}
					result.Set(key, bv)
					continue
				}
				noneVal := NewTypeLiteral(TNone)
				if _, unifOK := Unify(constraint, noneVal); unifOK {
					result.Set(key, noneVal)
					continue
				}
				return fmt.Errorf("make: missing field %q", key)
			}
			converted, err := MakeFieldValue(val, constraint)
			if err != nil {
				return fmt.Errorf("make: field %q: %w", key, err)
			}
			result.Set(key, converted)
		}
		return nil
	}

	if srcVal.VType.Equal(TMap) {
		provided, ok := srcVal.Data.(*OrderedMap)
		if !ok {
			return nil, fmt.Errorf("make: expected concrete map, got %s", srcVal.String())
		}
		if err := fillFromMap(provided); err != nil {
			return nil, err
		}
		return []Value{NewMap(result)}, nil
	}

	if !srcVal.VType.Equal(TList) {
		return nil, fmt.Errorf("make: record values must be a list or map, got %s", srcVal.String())
	}
	if srcVal.Data == nil {
		return nil, fmt.Errorf("make: record values must be a concrete list, got type literal")
	}
	elems := srcVal.AsList()

	isNamed := elems.Len() > 0 && elems.Get(0).VType.Equal(TMap)
	if isNamed {
		if _, ok := elems.Get(0).Data.(*OrderedMap); !ok {
			isNamed = false
		}
	}

	if isNamed {
		provided := NewOrderedMap()
		for _, elem := range elems.Slice() {
			if !elem.VType.Equal(TMap) {
				return nil, fmt.Errorf("make: mixed named and positional fields")
			}
			m, ok := elem.Data.(*OrderedMap)
			if !ok {
				return nil, fmt.Errorf("make: expected concrete map pair, got %s", elem.String())
			}
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				provided.Set(key, val)
			}
		}
		if err := fillFromMap(provided); err != nil {
			return nil, err
		}
	} else {
		if elems.Len() != len(fieldKeys) {
			return nil, fmt.Errorf("make: expected %d values, got %d",
				len(fieldKeys), elems.Len())
		}
		for i, key := range fieldKeys {
			constraint, _ := recType.Fields.Get(key)
			converted, err := MakeFieldValue(elems.Get(i), constraint)
			if err != nil {
				return nil, fmt.Errorf("make: field %q: %w", key, err)
			}
			result.Set(key, converted)
		}
	}

	return []Value{NewMap(result)}, nil
}

// parseMakeOptions extracts make options from an options map.
func parseMakeOptions(opts Value) (useBase bool, err error) {
	if !opts.VType.Equal(TMap) {
		return false, fmt.Errorf("make: options must be a map, got %s", opts.String())
	}
	m, ok := opts.Data.(*OrderedMap)
	if !ok {
		return false, fmt.Errorf("make: expected concrete options map")
	}
	if v, ok := m.Get("base"); ok {
		v = ResolveWordValue(v)
		if b, bOk := v.Data.(bool); bOk && b {
			useBase = true
		}
	}
	return useBase, nil
}

// buildBasePrototype creates a prototype instance with base values
// for a type that has no explicit prototype. If the type has a
// parent, it recursively builds prototypes up the chain.
func buildBasePrototype(objType ObjectTypeInfo) (*ObjectInstanceInfo, error) {
	var proto *ObjectInstanceInfo
	if objType.Parent != nil {
		var err error
		proto, err = buildBasePrototype(*objType.Parent)
		if err != nil {
			return nil, err
		}
	}

	fields := NewOrderedMap()
	for _, key := range objType.Fields.Keys() {
		constraint, _ := objType.Fields.Get(key)
		if constraint.Data != nil {
			fields.Set(key, constraint)
		} else {
			bv, err := BaseValueForConstraint(constraint)
			if err != nil {
				return nil, fmt.Errorf("make: field %q: %w", key, err)
			}
			fields.Set(key, bv)
		}
	}

	return &ObjectInstanceInfo{
		TypeRef:   &objType,
		Fields:    fields,
		Prototype: proto,
	}, nil
}

// makeObject creates an object instance from an ObjectTypeInfo, a
// map source, and an optional prototype instance.
func makeObject(objType ObjectTypeInfo, srcVal Value, prototype *ObjectInstanceInfo) ([]Value, error) {
	if !srcVal.VType.Equal(TMap) {
		return nil, fmt.Errorf("make: object values must be a map, got %s", srcVal.String())
	}
	provided, ok := srcVal.Data.(*OrderedMap)
	if !ok {
		return nil, fmt.Errorf("make: expected concrete map, got %s", srcVal.String())
	}

	if prototype == nil && objType.Parent != nil {
		var err error
		prototype, err = buildBasePrototype(*objType.Parent)
		if err != nil {
			return nil, err
		}
	}

	if prototype != nil && objType.Parent != nil {
		if prototype.TypeRef.ID != objType.Parent.ID {
			return nil, fmt.Errorf("make: prototype type %s does not match parent type %s",
				prototype.TypeRef.Name, objType.Parent.Name)
		}
	}

	allFields := objType.AllFields()

	for _, key := range provided.Keys() {
		if _, ok := allFields.Get(key); !ok {
			return nil, fmt.Errorf("make: unknown field %q for object type %s", key, objType.Name)
		}
	}

	ownFields := objType.Fields
	result := NewOrderedMap()

	for _, key := range ownFields.Keys() {
		constraint, _ := ownFields.Get(key)
		val, hasVal := provided.Get(key)

		if !hasVal {
			if constraint.Data != nil {
				result.Set(key, constraint)
				continue
			}
			return nil, fmt.Errorf("make: missing field %q for object type %s", key, objType.Name)
		}

		val = ResolveWordValue(val)

		if constraint.Data == nil {
			if val.VType.Matches(constraint.VType) {
				result.Set(key, val)
			} else {
				converted, err := MakeConvert(val, constraint.VType)
				if err != nil {
					return nil, fmt.Errorf("make: field %q: %w", key, err)
				}
				result.Set(key, converted)
			}
			continue
		}

		if val.VType.Matches(constraint.VType) {
			result.Set(key, val)
		} else {
			converted, err := MakeConvert(val, constraint.VType)
			if err != nil {
				return nil, fmt.Errorf("make: field %q: %w", key, err)
			}
			result.Set(key, converted)
		}
	}

	if prototype != nil {
		for _, key := range provided.Keys() {
			if _, ownOk := ownFields.Get(key); !ownOk {
				val, _ := provided.Get(key)
				val = ResolveWordValue(val)
				setPrototypeField(prototype, key, val)
			}
		}
	}

	return []Value{NewObjectInstance(ObjectInstanceInfo{
		TypeRef:   &objType,
		Fields:    result,
		Prototype: prototype,
	})}, nil
}

// makePath creates a Path value from a source (list or string).
func makePath(srcVal Value, abs bool) ([]Value, error) {
	switch {
	case srcVal.VType.Matches(TList) && srcVal.Data != nil:
		elems := srcVal.AsList()
		parts := make([]string, elems.Len())
		for i := 0; i < elems.Len(); i++ {
			parts[i] = ValToString(elems.Get(i))
		}
		return []Value{NewPath(parts, abs)}, nil
	case srcVal.VType.Matches(TString) && srcVal.Data != nil:
		s, _ := srcVal.AsString()
		if len(s) > 0 && s[0] == '/' {
			abs = true
			s = s[1:]
		}
		var parts []string
		if s != "" {
			parts = strings.Split(s, "/")
		}
		return []Value{NewPath(parts, abs)}, nil
	default:
		return nil, fmt.Errorf("make: Path source must be a list or string, got %s", srcVal.String())
	}
}

// makeHandler is the position-agnostic 2-arg make dispatcher.
func makeHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	if !isTypeLike(targetVal) && isTypeLike(srcVal) {
		targetVal, srcVal = srcVal, targetVal
	}

	targetVal = ResolveTypeLiteralDef(targetVal, reg)

	if targetVal.IsObjectType() {
		objType, _ := targetVal.AsObjectType()
		return makeObject(objType, srcVal, nil)
	}

	if targetVal.Data == nil && targetVal.VType.Equal(TPath) {
		return makePath(srcVal, false)
	}

	if targetVal.VType.Equal(TOptions) && targetVal.Data == nil {
		if !srcVal.VType.Equal(TMap) || srcVal.Data == nil {
			return nil, fmt.Errorf("make: Options requires a concrete map")
		}
		src, ok := srcVal.Data.(*OrderedMap)
		if !ok {
			return nil, fmt.Errorf("make: Options requires a concrete map")
		}
		return []Value{NewOptionsType(src)}, nil
	}

	if targetVal.IsRecordType() {
		recType, _ := targetVal.AsRecordType()
		return makeRecord(recType, srcVal, false)
	}

	if targetVal.IsTableType() {
		tableType, _ := targetVal.AsTableType()
		recType := tableType.Record

		if !srcVal.VType.Equal(TList) {
			return nil, fmt.Errorf("make: table values must be a list of row lists, got %s", srcVal.String())
		}
		if srcVal.Data == nil {
			return nil, fmt.Errorf("make: table values must be a concrete list, got type literal")
		}
		rows := srcVal.AsList()
		fieldKeys := recType.Fields.Keys()
		resultRows := make([]Value, 0, rows.Len())

		for rowIdx, rowVal := range rows.Slice() {
			if !rowVal.VType.Equal(TList) {
				return nil, fmt.Errorf("make: table row %d must be a list, got %s", rowIdx, rowVal.String())
			}
			if rowVal.Data == nil {
				return nil, fmt.Errorf("make: table row %d must be a concrete list, got type literal", rowIdx)
			}
			rowElems := rowVal.AsList()

			isNamed := rowElems.Len() > 0 && rowElems.Get(0).VType.Equal(TMap)
			if isNamed {
				if _, ok := rowElems.Get(0).Data.(*OrderedMap); !ok {
					isNamed = false
				}
			}

			result := NewOrderedMap()
			if isNamed {
				provided := NewOrderedMap()
				for _, elem := range rowElems.Slice() {
					if !elem.VType.Equal(TMap) {
						return nil, fmt.Errorf("make: table row %d: mixed named and positional fields", rowIdx)
					}
					m, ok := elem.Data.(*OrderedMap)
					if !ok {
						return nil, fmt.Errorf("make: table row %d: expected concrete map pair, got %s", rowIdx, elem.String())
					}
					for _, key := range m.Keys() {
						val, _ := m.Get(key)
						provided.Set(key, val)
					}
				}
				for _, key := range fieldKeys {
					val, ok := provided.Get(key)
					if !ok {
						return nil, fmt.Errorf("make: table row %d: missing field %q", rowIdx, key)
					}
					constraint, _ := recType.Fields.Get(key)
					converted, err := MakeFieldValue(val, constraint)
					if err != nil {
						return nil, fmt.Errorf("make: table row %d: field %q: %w", rowIdx, key, err)
					}
					result.Set(key, converted)
				}
				for _, key := range provided.Keys() {
					if _, ok := recType.Fields.Get(key); !ok {
						return nil, fmt.Errorf("make: table row %d: unknown field %q", rowIdx, key)
					}
				}
			} else {
				if rowElems.Len() != len(fieldKeys) {
					return nil, fmt.Errorf("make: table row %d: expected %d values, got %d",
						rowIdx, len(fieldKeys), rowElems.Len())
				}
				for i, key := range fieldKeys {
					constraint, _ := recType.Fields.Get(key)
					converted, err := MakeFieldValue(rowElems.Get(i), constraint)
					if err != nil {
						return nil, fmt.Errorf("make: table row %d: field %q: %w", rowIdx, key, err)
					}
					result.Set(key, converted)
				}
			}

			resultRows = append(resultRows, NewMap(result))
		}

		return []Value{NewList(resultRows)}, nil
	}

	if targetVal.Data != nil {
		return nil, fmt.Errorf("make: first argument must be a type literal or record type, got %s", targetVal.String())
	}

	targetType := targetVal.VType
	if srcVal.VType.Matches(targetType) {
		return []Value{srcVal}, nil
	}

	result, err := MakeConvert(srcVal, targetType)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// makeWithPrototype is the 3-arg make-with-prototype dispatcher.
func makeWithPrototype(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	resolved := make([]Value, len(args))
	for i, a := range args {
		resolved[i] = ResolveTypeLiteralDef(a, reg)
	}
	var targetVal, srcVal, protoVal Value
	for _, a := range resolved {
		switch {
		case a.IsObjectType() && targetVal.VType.Equal(Type{}):
			targetVal = a
		case a.IsObjectInstance():
			protoVal = a
		default:
			srcVal = a
		}
	}

	if !targetVal.IsObjectType() {
		return nil, fmt.Errorf("make: prototype can only be used with object types, got %s", targetVal.String())
	}
	if !protoVal.IsObjectInstance() {
		return nil, fmt.Errorf("make: prototype must be an object instance, got %s", protoVal.String())
	}

	objType, _ := targetVal.AsObjectType()
	protoInfo, _ := protoVal.AsObjectInstance()
	return makeObject(objType, srcVal, &protoInfo)
}

// makeWithOpts is the 3-arg make-with-options dispatcher.
func makeWithOpts(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	var targetVal, srcVal, optsVal Value
	for _, a := range args {
		resolved := ResolveTypeLiteralDef(a, reg)
		switch {
		case isTypeLike(resolved) && targetVal.VType.Equal(Type{}):
			targetVal = resolved
		default:
			if srcVal.VType.Equal(Type{}) {
				srcVal = a
			} else {
				optsVal = a
			}
		}
	}
	if optsVal.VType.Equal(TList) && srcVal.VType.Equal(TMap) && srcVal.Data != nil {
		srcVal, optsVal = optsVal, srcVal
	}

	useBase, err := parseMakeOptions(optsVal)
	if err != nil {
		return nil, err
	}

	if targetVal.IsObjectType() {
		objType, _ := targetVal.AsObjectType()
		return makeObject(objType, srcVal, nil)
	}

	if targetVal.IsRecordType() {
		recType, _ := targetVal.AsRecordType()
		return makeRecord(recType, srcVal, useBase)
	}

	if targetVal.Data == nil && targetVal.VType.Equal(TPath) {
		abs := false
		if optsMap := optsVal.AsMap(); optsMap != nil {
			if v, ok := optsMap.Get("abs"); ok && v.VType.Matches(TBoolean) {
				abs, _ = v.AsBoolean()
			}
		}
		return makePath(srcVal, abs)
	}

	return makeHandler([]Value{srcVal, targetVal}, nil, nil, nil)
}

// makeScalarHandler converts a scalar value to a target scalar type.
func makeScalarHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	if targetVal.Data != nil {
		return nil, fmt.Errorf("make: expected a type literal, got %s", targetVal.String())
	}
	targetType := targetVal.VType
	if targetType.Equal(TPath) {
		return makePath(srcVal, false)
	}
	if srcVal.VType.Matches(targetType) {
		return []Value{srcVal}, nil
	}
	result, err := MakeConvert(srcVal, targetType)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// makeObjHandler is the 2-arg [ObjectType, Map] make handler.
func makeObjHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	targetVal = ResolveTypeLiteralDef(targetVal, reg)
	if targetVal.IsObjectType() {
		objType, _ := targetVal.AsObjectType()
		return makeObject(objType, srcVal, nil)
	}
	return nil, fmt.Errorf("make: expected object type, got %s", targetVal.String())
}

// makeArrayHandler is the 2-arg [Array, List] make handler.
func makeArrayHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	srcVal := args[1]
	if !srcVal.VType.Equal(TList) || srcVal.Data == nil {
		return nil, fmt.Errorf("make: Array source must be a concrete list, got %s", srcVal.String())
	}
	return []Value{NewArray(srcVal.AsList().Slice())}, nil
}

// makeScalarOptsHandler is the 3-arg [ScalarType, Map, Any] make handler.
func makeScalarOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	targetVal, optsVal, srcVal := args[0], args[1], args[2]
	if targetVal.Data == nil && targetVal.VType.Equal(TPath) {
		abs := false
		if optsMap := optsVal.AsMap(); optsMap != nil {
			if v, ok := optsMap.Get("abs"); ok && v.VType.Matches(TBoolean) {
				abs, _ = v.AsBoolean()
			}
		}
		return makePath(srcVal, abs)
	}
	return makeScalarHandler([]Value{targetVal, srcVal}, nil, nil, nil)
}

// MakeConvert converts a source value to a target scalar type.
// Exported so production lang and downstream tooling can reuse the
// same scalar-coercion logic that backs `make`.
func MakeConvert(src Value, targetType Type) (Value, error) {
	switch {
	case targetType.Matches(TString):
		return NewString(ValToString(src)), nil

	case targetType.Matches(TDecimal):
		text := ValToString(src)
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return Value{}, fmt.Errorf("make: cannot convert %q to decimal", text)
		}
		return NewDecimal(f), nil

	case targetType.Matches(TNumber) || targetType.Matches(TInteger):
		text := ValToString(src)
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			f, ferr := strconv.ParseFloat(text, 64)
			if ferr != nil {
				return Value{}, fmt.Errorf("make: cannot convert %q to number", text)
			}
			return NewInteger(int64(f)), nil
		}
		return NewInteger(n), nil

	case targetType.Matches(TBoolean):
		switch {
		case src.VType.Matches(TBoolean):
			return src, nil
		case src.VType.Matches(TNumber):
			_as0, _ := src.AsNumber()
			return NewBoolean(_as0 != 0), nil
		default:
			text := ValToString(src)
			switch text {
			case "true":
				return NewBoolean(true), nil
			case "false":
				return NewBoolean(false), nil
			default:
				return NewBoolean(text != ""), nil
			}
		}

	case targetType.Equal(TAtom):
		return NewAtom(ValToString(src)), nil

	default:
		return Value{}, fmt.Errorf("make: unsupported target type %s", targetType)
	}
}

// MakeFieldValue converts a value to match a record field's type
// constraint. Exported for the same reason as MakeConvert — keeps
// the production lang's record-make path on the engine's canonical
// implementation.
func MakeFieldValue(val Value, constraint Value) (Value, error) {
	val = ResolveWordValue(val)

	if constraint.Data == nil {
		constraintType := constraint.VType
		if val.VType.Matches(constraintType) {
			return val, nil
		}
		return MakeConvert(val, constraintType)
	}

	unified, ok := Unify(constraint, val)
	if !ok {
		return Value{}, fmt.Errorf("value %s does not match constraint %s", val.String(), constraint.String())
	}
	return unified, nil
}

// ResolveFieldType resolves a record field's type constraint value.
//
// Three resolution strategies:
//  1. String matching a user-defined type name in DefStacks → replaced
//     with the defined type value (e.g., disjunctions by name).
//  2. Concrete list → evaluated as code in a sub-engine so that
//     expressions like [string or none] produce a disjunction.
//  3. Everything else passes through unchanged.
func ResolveFieldType(r *Registry, v Value) Value {
	if v.Data != nil && (v.VType.Matches(TString) || v.VType.Matches(TAtom) || v.IsWord()) {
		var name string
		if v.IsWord() {
			_as2, _ := v.AsWord()
			name = _as2.Name
		} else {
			name, _ = v.AsString()
		}
		if tv, ok := r.TopOfTypeStack(name); ok {
			if IsTypeBody(tv) {
				return tv
			}
		}
		if top, ok := r.TopOfDefStack(name); ok {
			if IsTypeBody(top) {
				return top
			}
		}
		return v
	}

	if v.VType.Equal(TList) && !v.IsTypedList() && !v.IsTableType() {
		elems := v.AsList()
		input := make([]Value, elems.Len())
		for i, e := range elems.Slice() {
			if (e.VType.Matches(TString) || e.VType.Matches(TAtom)) && e.Data != nil {
				name, _ := e.AsString()
				if r.Lookup(name) != nil {
					input[i] = NewWord(name)
					continue
				}
			}
			input[i] = e
		}
		sub := New(r)
		results, err := sub.Run(input)
		if err == nil && len(results) == 1 {
			return results[0]
		}
		return v
	}

	return v
}

// setPrototypeField sets a field value on the appropriate level of a
// prototype chain.
func setPrototypeField(proto *ObjectInstanceInfo, key string, val Value) {
	for p := proto; p != nil; p = p.Prototype {
		if _, ok := p.TypeRef.Fields.Get(key); ok {
			p.Fields.Set(key, val)
			return
		}
	}
}
