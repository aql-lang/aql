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
//	make *Type *Type {opts}           three-arg shape with arbitrary options
//	make *Type Any                   two-arg fallback
//
// Mirrors the production lang `make` (formerly in
// lang/go/engine/native_type_make_helpers.go); the handlers are
// ported verbatim. Lang re-exports the helpers via aliases so any
// callers that reach into the package-private surface keep working.
// The `make` word registration has moved to
// lang/go/engine/native_make.go. The Make* handlers below are exported
// algorithm primitives — lang's registration wires the dispatch
// table without forking the algorithm.

// isTypeLike returns true if v looks like a type target for make
// (type literal with nil Data, record type, options type, table
// type, or object type).
func isTypeLike(v Value) bool {
	if v.Data == nil {
		return true
	}
	return IsRecordType(v) || IsOptionsType(v) || IsTableType(v) || IsObjectType(v)
}

// MakeRecord creates a record instance from a source value and
// options. Used by `make` (via the Record Ideal's Instantiate) and
// by the 3-arg make-with-options path.
func MakeRecord(recType RecordTypeInfo, srcVal Value, useBase bool) ([]Value, error) {
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
		provided, err := AsMutableMap(srcVal)
		if err != nil {
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
	elems, _ := AsList(srcVal)

	isNamed := elems.Len() > 0 && elems.Get(0).VType.Equal(TMap)
	if isNamed {
		if _, err := AsMutableMap(elems.Get(0)); err != nil {
			isNamed = false
		}
	}

	if isNamed {
		provided := NewOrderedMap()
		for _, elem := range elems.Slice() {
			if !elem.VType.Equal(TMap) {
				return nil, fmt.Errorf("make: mixed named and positional fields")
			}
			m, err := AsMutableMap(elem)
			if err != nil {
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
	m, err := AsMutableMap(opts)
	if err != nil {
		return false, fmt.Errorf("make: expected concrete options map")
	}
	if v, ok := m.Get("base"); ok {
		v = ResolveWordValue(v)
		if b, bErr := AsBoolean(v); bErr == nil && b {
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
// MakeObject is the exported wrapper around the internal object
// construction path. Used by lang-side `def x:T body` to build a
// Person-typed ObjectInstance from a raw Map body when the typed
// binding's constraint is an ObjectType — closes the
// structural-vs-nominal dispatch gap for object types.
func MakeObject(objType ObjectTypeInfo, srcVal Value, prototype *ObjectInstanceInfo) ([]Value, error) {
	return makeObject(objType, srcVal, prototype)
}

func makeObject(objType ObjectTypeInfo, srcVal Value, prototype *ObjectInstanceInfo) ([]Value, error) {
	if !srcVal.VType.Equal(TMap) {
		return nil, fmt.Errorf("make: object values must be a map, got %s", srcVal.String())
	}
	provided, err := AsMutableMap(srcVal)
	if err != nil {
		return nil, fmt.Errorf("make: expected concrete map, got %s", srcVal.String())
	}

	if prototype == nil && objType.Parent != nil {
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

	instanceType := objType.Type
	if instanceType == nil {
		instanceType = TObject
	}
	return []Value{NewObjectInstance(instanceType, ObjectInstanceInfo{
		TypeRef:   &objType,
		Fields:    result,
		Prototype: prototype,
	})}, nil
}

// makePath creates a Path value from a source (list or string).
func makePath(srcVal Value, abs bool) ([]Value, error) {
	switch {
	case srcVal.VType.Matches(TList) && srcVal.Data != nil:
		elems, _ := AsList(srcVal)
		parts := make([]string, elems.Len())
		for i := 0; i < elems.Len(); i++ {
			parts[i] = ValToString(elems.Get(i))
		}
		return []Value{NewPath(parts, abs)}, nil
	case srcVal.VType.Matches(TString) && srcVal.Data != nil:
		s, _ := AsString(srcVal)
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

// MakeHandler is the position-agnostic 2-arg make dispatcher.
func MakeHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	if !isTypeLike(targetVal) && isTypeLike(srcVal) {
		targetVal, srcVal = srcVal, targetVal
	}

	targetVal = ResolveTypeLiteralDef(targetVal, reg)

	// Structural kinds (object / record / table) instantiate through
	// the Ideal registry — see ideal.go and lang/doc/design/IDEAL.0.md.
	if reg != nil {
		if ideal := reg.Ideals.For(targetVal); ideal != nil && ideal.Instantiate != nil {
			return ideal.Instantiate(targetVal, srcVal, reg)
		}
	}

	if targetVal.Data == nil && targetVal.VType.Equal(TPath) {
		return makePath(srcVal, false)
	}

	if targetVal.VType.Equal(TOptions) && targetVal.Data == nil {
		if !srcVal.VType.Equal(TMap) || srcVal.Data == nil {
			return nil, fmt.Errorf("make: Options requires a concrete map")
		}
		src, err := AsMutableMap(srcVal)
		if err != nil {
			return nil, fmt.Errorf("make: Options requires a concrete map")
		}
		return []Value{NewOptionsType(src)}, nil
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

// MakeTable instantiates a table value — a list of record-conforming
// rows — from a table type and a list of row data. Each row may be
// positional or named. Backs the Table Ideal's Instantiate.
func MakeTable(tt TableTypeInfo, srcVal Value) ([]Value, error) {
	recType := tt.Record
	if !srcVal.VType.Equal(TList) {
		return nil, fmt.Errorf("make: table values must be a list of row lists, got %s", srcVal.String())
	}
	if srcVal.Data == nil {
		return nil, fmt.Errorf("make: table values must be a concrete list, got type literal")
	}
	rows, _ := AsList(srcVal)
	fieldKeys := recType.Fields.Keys()
	resultRows := make([]Value, 0, rows.Len())

	for rowIdx, rowVal := range rows.Slice() {
		if !rowVal.VType.Equal(TList) {
			return nil, fmt.Errorf("make: table row %d must be a list, got %s", rowIdx, rowVal.String())
		}
		if rowVal.Data == nil {
			return nil, fmt.Errorf("make: table row %d must be a concrete list, got type literal", rowIdx)
		}
		rowElems, _ := AsList(rowVal)

		isNamed := rowElems.Len() > 0 && rowElems.Get(0).VType.Equal(TMap)
		if isNamed {
			if _, err := AsMutableMap(rowElems.Get(0)); err != nil {
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
				m, err := AsMutableMap(elem)
				if err != nil {
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

// registerKernelIdeals installs the kernel type-kind descriptors with
// their dispatch predicate (Accepts) and value constructor
// (Instantiate). The type-level constructor (Ideal.Construct) is
// filled in by the language layer's installIdeals — type construction
// reuses the surface-registered object/record handlers. Called from
// NewRegistry so every Registry, including the bare eng spec runner,
// can `make` the structural kinds.
func registerKernelIdeals(r *Registry) {
	r.Ideals.Register(&Ideal{
		Name:    "Object",
		Enabled: true,
		Accepts: func(v Value) bool {
			return (v.Data == nil && v.VType.Equal(TObject)) || IsObjectType(v)
		},
		Instantiate: func(typ, data Value, _ *Registry) ([]Value, error) {
			objType, err := AsObjectType(typ)
			if err != nil {
				return nil, fmt.Errorf("make: expected a constructed object type, got %s", typ.String())
			}
			return makeObject(objType, data, nil)
		},
	})
	r.Ideals.Register(&Ideal{
		Name:    "Record",
		Enabled: true,
		Accepts: func(v Value) bool {
			return (v.Data == nil && v.VType.Equal(TRecord)) || IsRecordType(v)
		},
		Instantiate: func(typ, data Value, _ *Registry) ([]Value, error) {
			recType, err := AsRecordType(typ)
			if err != nil {
				return nil, fmt.Errorf("make: expected a constructed record type, got %s", typ.String())
			}
			return MakeRecord(recType, data, false)
		},
	})
	r.Ideals.Register(&Ideal{
		Name:    "Table",
		Enabled: true,
		Accepts: func(v Value) bool {
			return (v.Data == nil && v.VType.Equal(TTable)) || IsTableType(v)
		},
		Instantiate: func(typ, data Value, _ *Registry) ([]Value, error) {
			tt, err := AsTableType(typ)
			if err != nil {
				return nil, fmt.Errorf("make: expected a constructed table type, got %s", typ.String())
			}
			return MakeTable(tt, data)
		},
	})
}

// MakeWithPrototype is the 3-arg make-with-prototype dispatcher.
func MakeWithPrototype(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	resolved := make([]Value, len(args))
	for i, a := range args {
		resolved[i] = ResolveTypeLiteralDef(a, reg)
	}
	var targetVal, srcVal, protoVal Value
	for _, a := range resolved {
		switch {
		case IsObjectType(a) && targetVal.VType.Equal(nil):
			targetVal = a
		case IsObjectInstance(a):
			protoVal = a
		default:
			srcVal = a
		}
	}

	if !IsObjectType(targetVal) {
		return nil, fmt.Errorf("make: prototype can only be used with object types, got %s", targetVal.String())
	}
	if !IsObjectInstance(protoVal) {
		return nil, fmt.Errorf("make: prototype must be an object instance, got %s", protoVal.String())
	}

	objType, _ := AsObjectType(targetVal)
	protoInfo, _ := AsObjectInstance(protoVal)
	return makeObject(objType, srcVal, &protoInfo)
}

// MakeWithOpts is the 3-arg make-with-options dispatcher.
func MakeWithOpts(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	var targetVal, srcVal, optsVal Value
	for _, a := range args {
		resolved := ResolveTypeLiteralDef(a, reg)
		switch {
		case isTypeLike(resolved) && targetVal.VType.Equal(nil):
			targetVal = resolved
		default:
			if srcVal.VType.Equal(nil) {
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

	if IsObjectType(targetVal) {
		objType, _ := AsObjectType(targetVal)
		return makeObject(objType, srcVal, nil)
	}

	if IsRecordType(targetVal) {
		recType, _ := AsRecordType(targetVal)
		return MakeRecord(recType, srcVal, useBase)
	}

	if targetVal.Data == nil && targetVal.VType.Equal(TPath) {
		abs := false
		if optsMap, _ := AsMap(optsVal); optsMap != nil {
			if v, ok := optsMap.Get("abs"); ok && v.VType.Matches(TBoolean) {
				abs, _ = AsBoolean(v)
			}
		}
		return makePath(srcVal, abs)
	}

	// Pass reg through so the Ideal-registry dispatch in MakeHandler
	// can reach the structural kinds (e.g. a table target with opts).
	return MakeHandler([]Value{srcVal, targetVal}, nil, nil, reg)
}

// MakeScalarHandler converts a scalar value to a target scalar type.
func MakeScalarHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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

// MakeObjHandler is the 2-arg [ObjectType, Map] make handler.
func MakeObjHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	targetVal = ResolveTypeLiteralDef(targetVal, reg)
	if reg != nil {
		if ideal := reg.Ideals.For(targetVal); ideal != nil && ideal.Instantiate != nil {
			return ideal.Instantiate(targetVal, srcVal, reg)
		}
	}
	if IsObjectType(targetVal) {
		objType, _ := AsObjectType(targetVal)
		return makeObject(objType, srcVal, nil)
	}
	return nil, fmt.Errorf("make: expected object type, got %s", targetVal.String())
}

// MakeArrayHandler is the 2-arg [Array, List] make handler.
func MakeArrayHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	srcVal := args[1]
	if !srcVal.VType.Equal(TList) || srcVal.Data == nil {
		return nil, fmt.Errorf("make: Array source must be a concrete list, got %s", srcVal.String())
	}
	srcList, _ := AsList(srcVal)
	return []Value{NewArray(srcList.Slice())}, nil
}

// MakeScalarOptsHandler is the 3-arg [ScalarType, Map, Any] make handler.
func MakeScalarOptsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	targetVal, optsVal, srcVal := args[0], args[1], args[2]
	if targetVal.Data == nil && targetVal.VType.Equal(TPath) {
		abs := false
		if optsMap, _ := AsMap(optsVal); optsMap != nil {
			if v, ok := optsMap.Get("abs"); ok && v.VType.Matches(TBoolean) {
				abs, _ = AsBoolean(v)
			}
		}
		return makePath(srcVal, abs)
	}
	return MakeScalarHandler([]Value{targetVal, srcVal}, nil, nil, nil)
}

// MakeConvert converts a source value to a target scalar type.
// Exported so production lang and downstream tooling can reuse the
// same scalar-coercion logic that backs `make`.
func MakeConvert(src Value, targetType *Type) (Value, error) {
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
			_as0, _ := AsNumber(src)
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
	if v.Data != nil && (v.VType.Matches(TString) || v.VType.Matches(TAtom) || IsWord(v)) {
		var name string
		if IsWord(v) {
			_as2, _ := AsWord(v)
			name = _as2.Name
		} else {
			name, _ = AsString(v)
		}
		if tv, ok := r.TopTypeBody(name); ok {
			if IsTypeBody(tv) {
				return tv
			}
		}
		if top, ok := r.Defs.Top(name); ok {
			if IsTypeBody(top) {
				return top
			}
		}
		return v
	}

	if v.VType.Equal(TList) && !IsTypedList(v) && !IsTableType(v) {
		elems, _ := AsList(v)
		input := make([]Value, elems.Len())
		for i, e := range elems.Slice() {
			if (e.VType.Matches(TString) || e.VType.Matches(TAtom)) && e.Data != nil {
				name, _ := AsString(e)
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
