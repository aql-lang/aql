package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// isTypeLike returns true if a value looks like a type target for make
// (type literal with nil Data, record type, options type, table type, or object type).
func isTypeLike(v Value) bool {
	if v.Data == nil {
		return true
	}
	return v.IsRecordType() || v.IsOptionsType() || v.IsTableType() || v.IsObjectType()
}

func registerMake(r *Registry) {
	// makeRecord creates a record instance from a source value and options.
	makeRecord := func(recType RecordTypeInfo, srcVal Value, useBase bool) ([]Value, error) {
		fieldKeys := recType.Fields.Keys()
		result := NewOrderedMap()

		// Helper: fill result from a provided map, defaulting
		// missing fields based on useBase flag.
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
						// base:true — fill with base value for the field type.
						bv, err := baseValueForConstraint(constraint)
						if err != nil {
							return fmt.Errorf("make: field %q: %w", key, err)
						}
						result.Set(key, bv)
						continue
					}
					// Default: allow if none unifies with constraint.
					noneVal := NewTypeLiteral(TNone)
					if _, unifOK := Unify(constraint, noneVal); unifOK {
						result.Set(key, noneVal)
						continue
					}
					return fmt.Errorf("make: missing field %q", key)
				}
				converted, err := makeFieldValue(val, constraint)
				if err != nil {
					return fmt.Errorf("make: field %q: %w", key, err)
				}
				result.Set(key, converted)
			}
			return nil
		}

		// Map form: make RecType {x:1 y:"hello"}
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

		// Check if named or positional.
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
				converted, err := makeFieldValue(elems.Get(i), constraint)
				if err != nil {
					return nil, fmt.Errorf("make: field %q: %w", key, err)
				}
				result.Set(key, converted)
			}
		}

		return []Value{NewMap(result)}, nil
	}

	// parseOptions extracts make options from an options map.
	parseOptions := func(opts Value) (useBase bool, err error) {
		if !opts.VType.Equal(TMap) {
			return false, fmt.Errorf("make: options must be a map, got %s", opts.String())
		}
		m, ok := opts.Data.(*OrderedMap)
		if !ok {
			return false, fmt.Errorf("make: expected concrete options map")
		}
		if v, ok := m.Get("base"); ok {
			v = resolveWordValue(v)
			if b, bOk := v.Data.(bool); bOk && b {
				useBase = true
			}
		}
		return useBase, nil
	}

	// buildBasePrototype creates a prototype instance with base values for a
	// type that has no explicit prototype. If the type has a parent, it
	// recursively builds prototypes up the chain.
	var buildBasePrototype func(objType ObjectTypeInfo) (*ObjectInstanceInfo, error)
	buildBasePrototype = func(objType ObjectTypeInfo) (*ObjectInstanceInfo, error) {
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
				// Concrete default.
				fields.Set(key, constraint)
			} else {
				// Type literal: use base value for that type.
				bv, err := baseValueForConstraint(constraint)
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

	// makeObject creates an object instance from an ObjectTypeInfo, a map source,
	// and an optional prototype instance. The prototype provides inherited
	// field values via a prototype chain (like JavaScript prototypes).
	// If no prototype is given and the type has a parent, a base prototype
	// is auto-created with zero/default values.
	makeObject := func(objType ObjectTypeInfo, srcVal Value, prototype *ObjectInstanceInfo) ([]Value, error) {
		if !srcVal.VType.Equal(TMap) {
			return nil, fmt.Errorf("make: object values must be a map, got %s", srcVal.String())
		}
		provided, ok := srcVal.Data.(*OrderedMap)
		if !ok {
			return nil, fmt.Errorf("make: expected concrete map, got %s", srcVal.String())
		}

		// Build prototype chain if needed.
		if prototype == nil && objType.Parent != nil {
			var err error
			prototype, err = buildBasePrototype(*objType.Parent)
			if err != nil {
				return nil, err
			}
		}

		// Validate prototype type matches parent.
		if prototype != nil && objType.Parent != nil {
			if prototype.TypeRef.ID != objType.Parent.ID {
				return nil, fmt.Errorf("make: prototype type %s does not match parent type %s",
					prototype.TypeRef.Name, objType.Parent.Name)
			}
		}

		// Determine which fields belong to this type (own) vs prototype.
		allFields := objType.AllFields()

		// Reject unknown fields.
		for _, key := range provided.Keys() {
			if _, ok := allFields.Get(key); !ok {
				return nil, fmt.Errorf("make: unknown field %q for object type %s", key, objType.Name)
			}
		}

		// Only populate the type's own fields in this instance.
		ownFields := objType.Fields
		result := NewOrderedMap()

		for _, key := range ownFields.Keys() {
			constraint, _ := ownFields.Get(key)
			val, hasVal := provided.Get(key)

			if !hasVal {
				// Missing field: use default if constraint has a concrete value.
				if constraint.Data != nil {
					result.Set(key, constraint)
					continue
				}
				return nil, fmt.Errorf("make: missing field %q for object type %s", key, objType.Name)
			}

			val = resolveWordValue(val)

			if constraint.Data == nil {
				// Type-literal constraint: convert value to that type.
				if val.VType.Matches(constraint.VType) {
					result.Set(key, val)
				} else {
					converted, err := makeConvert(val, constraint.VType)
					if err != nil {
						return nil, fmt.Errorf("make: field %q: %w", key, err)
					}
					result.Set(key, converted)
				}
				continue
			}

			// Concrete default: accept any value of the same base type.
			if val.VType.Matches(constraint.VType) {
				result.Set(key, val)
			} else {
				converted, err := makeConvert(val, constraint.VType)
				if err != nil {
					return nil, fmt.Errorf("make: field %q: %w", key, err)
				}
				result.Set(key, converted)
			}
		}

		// If a provided field belongs to the prototype chain (inherited),
		// override it in the appropriate prototype level.
		if prototype != nil {
			for _, key := range provided.Keys() {
				if _, ownOk := ownFields.Get(key); !ownOk {
					// This field comes from a parent — set it on the prototype.
					val, _ := provided.Get(key)
					val = resolveWordValue(val)
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
	makePath := func(srcVal Value, abs bool) ([]Value, error) {
		switch {
		case srcVal.VType.Matches(TList) && srcVal.Data != nil:
			elems := srcVal.AsList()
			parts := make([]string, elems.Len())
			for i := 0; i < elems.Len(); i++ {
				parts[i] = valToString(elems.Get(i))
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

	// Position-agnostic: detect which arg is the type and which is the source.
	makeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		targetVal, srcVal := args[0], args[1]
		if !isTypeLike(targetVal) && isTypeLike(srcVal) {
			targetVal, srcVal = srcVal, targetVal
		}

		// Object type instance creation.
		if targetVal.IsObjectType() {
			objType, _ := targetVal.AsObjectType()
			return makeObject(objType, srcVal, nil)
		}

		// Path construction: make Path [a b c] or make Path "a/b/c"
		if targetVal.Data == nil && targetVal.VType.Equal(TPath) {
			return makePath(srcVal, false)
		}

		// Options type creation: make Options {x:1, y:String}
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

		// Record type instance creation.
		if targetVal.IsRecordType() {
			recType, _ := targetVal.AsRecordType()
			return makeRecord(recType, srcVal, false)
		}

		// Table type instance creation.
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

				// Check if named or positional.
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
						converted, err := makeFieldValue(val, constraint)
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
						converted, err := makeFieldValue(rowElems.Get(i), constraint)
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

		// Scalar type conversion.
		if targetVal.Data != nil {
			return nil, fmt.Errorf("make: first argument must be a type literal or record type, got %s", targetVal.String())
		}

		targetType := targetVal.VType
		if srcVal.VType.Matches(targetType) {
			return []Value{srcVal}, nil
		}

		result, err := makeConvert(srcVal, targetType)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	// 3-arg handler with prototype: make ObjType source prototype
	// Position-agnostic: finds object type, prototype instance, and source.
	makeWithPrototype := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		var targetVal, srcVal, protoVal Value
		for _, a := range args {
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

	// 3-arg handler: make RecType source {options}
	// Position-agnostic: finds type, source, and options.
	makeWithOpts := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		var targetVal, srcVal, optsVal Value
		for _, a := range args {
			switch {
			case isTypeLike(a) && targetVal.VType.Equal(Type{}):
				targetVal = a
			default:
				// Remaining args: one is source, one is options map.
				if srcVal.VType.Equal(Type{}) {
					srcVal = a
				} else {
					optsVal = a
				}
			}
		}
		// The options map should be the plain map argument (not a list).
		// If srcVal is a map and optsVal is a list (or vice versa), swap.
		if optsVal.VType.Equal(TList) && srcVal.VType.Equal(TMap) && srcVal.Data != nil {
			srcVal, optsVal = optsVal, srcVal
		}

		useBase, err := parseOptions(optsVal)
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

		// Path with options: make Path {abs:true} [a b c]
		if targetVal.Data == nil && targetVal.VType.Equal(TPath) {
			abs := false
			if optsMap := optsVal.AsMap(); optsMap != nil {
				if v, ok := optsMap.Get("abs"); ok && v.VType.Matches(TBoolean) {
					abs, _ = v.AsBoolean()
				}
			}
			return makePath(srcVal, abs)
		}

		// For non-record/object types, options are ignored — delegate to 2-arg.
		return makeHandler([]Value{srcVal, targetVal}, nil, nil, nil)
	}

	// --- New specific signatures (matched first by score) ---

	// Scalar: [ScalarType, Any] → Scalar conversion
	makeScalarHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		targetVal, srcVal := args[0], args[1]
		if targetVal.Data != nil {
			return nil, fmt.Errorf("make: expected a type literal, got %s", targetVal.String())
		}
		targetType := targetVal.VType
		// Path construction
		if targetType.Equal(TPath) {
			return makePath(srcVal, false)
		}
		if srcVal.VType.Matches(targetType) {
			return []Value{srcVal}, nil
		}
		result, err := makeConvert(srcVal, targetType)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	// Object 2-arg: [ObjectType, Map] → Object instance
	makeObjHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		targetVal, srcVal := args[0], args[1]
		if targetVal.IsObjectType() {
			objType, _ := targetVal.AsObjectType()
			return makeObject(objType, srcVal, nil)
		}
		return nil, fmt.Errorf("make: expected object type, got %s", targetVal.String())
	}

	// Array 2-arg: [Array, List] → Array instance
	makeArrayHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		srcVal := args[1]
		if !srcVal.VType.Equal(TList) || srcVal.Data == nil {
			return nil, fmt.Errorf("make: Array source must be a concrete list, got %s", srcVal.String())
		}
		return []Value{NewArray(srcVal.AsList().Slice())}, nil
	}

	// Scalar 3-arg: [ScalarType, Map, Any] → scalar with options (e.g. make Path {abs:true} [...])
	makeScalarOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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
		// For other scalar types with options, ignore options and delegate.
		return makeScalarHandler([]Value{targetVal, srcVal}, nil, nil, nil)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "make",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			// New specific signatures
			{Args: []Type{TScalarType, TMap, TAny}, Handler: makeScalarOptsHandler},
			{Args: []Type{TObjectType, TMap}, Handler: makeObjHandler},
			{Args: []Type{TArray, TList}, Handler: makeArrayHandler},
			{Args: []Type{TScalarType, TAny}, Handler: makeScalarHandler},
			// Existing position-agnostic signatures (fallback)
			{
				Args:    []Type{TObject, TAny, TObject},
				Handler: makeWithPrototype,
			},
			{
				Args:    []Type{TAny, TAny, TMap},
				Handler: makeWithOpts,
			},
			{
				Args:    []Type{TAny, TAny},
				Handler: makeHandler,
			},
		},
	})
}

// makeConvert converts a source value to a target scalar type for the make word.
func makeConvert(src Value, targetType Type) (Value, error) {
	switch {
	case targetType.Matches(TString):
		return NewString(valToString(src)), nil

	case targetType.Matches(TDecimal):
		text := valToString(src)
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return Value{}, fmt.Errorf("make: cannot convert %q to decimal", text)
		}
		return NewDecimal(f), nil

	case targetType.Matches(TNumber) || targetType.Matches(TInteger):
		text := valToString(src)
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			// Try parsing as float and truncating to integer.
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
			text := valToString(src)
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
		return NewAtom(valToString(src)), nil

	default:
		return Value{}, fmt.Errorf("make: unsupported target type %s", targetType)
	}
}

// makeFieldValue converts a value to match a record field's type constraint.
// If the constraint is a type literal, the value is converted to that type.
// If the value already matches, it is returned as-is.
func makeFieldValue(val Value, constraint Value) (Value, error) {
	// Resolve words to their semantic value first (e.g. word(false) → boolean false).
	val = resolveWordValue(val)

	// If the constraint is a type literal (Data==nil), convert the value.
	if constraint.Data == nil {
		constraintType := constraint.VType
		if val.VType.Matches(constraintType) {
			return val, nil
		}
		return makeConvert(val, constraintType)
	}

	// If the constraint has a concrete value, just check via unification.
	unified, ok := Unify(constraint, val)
	if !ok {
		return Value{}, fmt.Errorf("value %s does not match constraint %s", val.String(), constraint.String())
	}
	return unified, nil
}

// resolveWordValue converts a word value to its semantic value.
// Words named "true"/"false" become booleans, known type names become type
// literals, and other words become atoms (bare strings).
func resolveWordValue(v Value) Value {
	if !v.IsWord() {
		return v
	}
	_as1, _ := v.AsWord()
	name := _as1.Name
	switch name {
	case "true":
		return NewBoolean(true)
	case "false":
		return NewBoolean(false)
	default:
		if t, ok := typeNames[name]; ok {
			return NewTypeLiteral(t)
		}
		return NewAtom(name)
	}
}

// resolveFieldType resolves a record field's type constraint value.
//
// Three resolution strategies:
//  1. String matching a user-defined type name in DefStacks → replaced
//     with the defined type value (e.g., disjunctions by name).
//  2. Concrete list → evaluated as code in a sub-engine so that
//     expressions like [string or none] produce a disjunction.
//  3. Everything else passes through unchanged.
//
// Examples:
//
//	type OptStr (string or none)
//	record [x:number y:OptStr]              => record{x:number,y:string|none}
//	record [x:number y:[string or none]]    => record{x:number,y:string|none}
func resolveFieldType(r *Registry, v Value) Value {
	// Strategy 1: string, atom, or word matching a defined type name.
	if v.Data != nil && (v.VType.Matches(TString) || v.VType.Matches(TAtom) || v.IsWord()) {
		var name string
		if v.IsWord() {
			_as2, _ := v.AsWord()
			name = _as2.Name
		} else {
			name, _ = v.AsString()
		}
		stack := r.DefStacks[name]
		if len(stack) > 0 {
			top := stack[len(stack)-1]
			if isTypeValue(top) {
				return top
			}
		}
		return v
	}

	// Strategy 2: evaluate concrete list as code.
	if v.VType.Equal(TList) && !v.IsTypedList() && !v.IsTableType() {
		elems := v.AsList()
		// Promote strings that name registered functions to words,
		// since list elements inside pairs are parsed in data context.
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
		// If evaluation fails or produces multiple values, keep original.
		return v
	}

	return v
}

// setPrototypeField sets a field value on the appropriate level of a prototype
// chain. It walks down until it finds the prototype whose type owns the field.
func setPrototypeField(proto *ObjectInstanceInfo, key string, val Value) {
	for p := proto; p != nil; p = p.Prototype {
		if _, ok := p.TypeRef.Fields.Get(key); ok {
			p.Fields.Set(key, val)
			return
		}
	}
}
