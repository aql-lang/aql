package engine

import (
	"fmt"
	"strconv"
)

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
		elems := srcVal.AsList()

		// Check if named or positional.
		isNamed := len(elems) > 0 && elems[0].VType.Equal(TMap)
		if isNamed {
			if _, ok := elems[0].Data.(*OrderedMap); !ok {
				isNamed = false
			}
		}

		if isNamed {
			provided := NewOrderedMap()
			for _, elem := range elems {
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
			if len(elems) != len(fieldKeys) {
				return nil, fmt.Errorf("make: expected %d values, got %d",
					len(fieldKeys), len(elems))
			}
			for i, key := range fieldKeys {
				constraint, _ := recType.Fields.Get(key)
				converted, err := makeFieldValue(elems[i], constraint)
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
			if v.VType.Matches(TBoolean) && v.Data.(bool) {
				useBase = true
			}
		}
		return useBase, nil
	}

	makeHandler := func(args []Value) ([]Value, error) {
		targetVal := args[0]
		srcVal := args[1]

		// Record type instance creation.
		if targetVal.IsRecordType() {
			recType := targetVal.AsRecordType()
			return makeRecord(recType, srcVal, false)
		}

		// Table type instance creation.
		if targetVal.IsTableType() {
			tableType := targetVal.AsTableType()
			recType := tableType.Record

			if !srcVal.VType.Equal(TList) {
				return nil, fmt.Errorf("make: table values must be a list of row lists, got %s", srcVal.String())
			}
			rows := srcVal.AsList()
			fieldKeys := recType.Fields.Keys()
			resultRows := make([]Value, 0, len(rows))

			for rowIdx, rowVal := range rows {
				if !rowVal.VType.Equal(TList) {
					return nil, fmt.Errorf("make: table row %d must be a list, got %s", rowIdx, rowVal.String())
				}
				rowElems := rowVal.AsList()

				// Check if named or positional.
				isNamed := len(rowElems) > 0 && rowElems[0].VType.Equal(TMap)
				if isNamed {
					if _, ok := rowElems[0].Data.(*OrderedMap); !ok {
						isNamed = false
					}
				}

				result := NewOrderedMap()
				if isNamed {
					provided := NewOrderedMap()
					for _, elem := range rowElems {
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
					if len(rowElems) != len(fieldKeys) {
						return nil, fmt.Errorf("make: table row %d: expected %d values, got %d",
							rowIdx, len(fieldKeys), len(rowElems))
					}
					for i, key := range fieldKeys {
						constraint, _ := recType.Fields.Get(key)
						converted, err := makeFieldValue(rowElems[i], constraint)
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

	// 3-arg handler: make RecType source {options}
	makeWithOpts := func(args []Value) ([]Value, error) {
		targetVal := args[0]
		srcVal := args[1]
		optsVal := args[2]

		useBase, err := parseOptions(optsVal)
		if err != nil {
			return nil, err
		}

		if targetVal.IsRecordType() {
			recType := targetVal.AsRecordType()
			return makeRecord(recType, srcVal, useBase)
		}

		// For non-record types, options are ignored — delegate to 2-arg.
		return makeHandler(args[:2])
	}

	r.Register("make",
		Signature{
			Args:    []Type{TAny, TAny, TMap},
			Handler: makeWithOpts,
		},
		Signature{
			Args:    []Type{TAny, TAny},
			Handler: makeHandler,
		},
	)
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
			return NewBoolean(src.AsNumber() != 0), nil
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
// Words named "true"/"false" become booleans, "none" becomes a type literal,
// and other words become atoms (bare strings).
func resolveWordValue(v Value) Value {
	if !v.IsWord() {
		return v
	}
	name := v.AsWord().Name
	switch name {
	case "true":
		return NewBoolean(true)
	case "false":
		return NewBoolean(false)
	case "None":
		return NewTypeLiteral(TNone)
	default:
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
	// Strategy 1: string matching a defined type name.
	if v.Data != nil && v.VType.Matches(TString) {
		name := v.AsString()
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
		input := make([]Value, len(elems))
		for i, e := range elems {
			if (e.VType.Matches(TString) || e.VType.Matches(TAtom)) && e.Data != nil {
				name := e.AsString()
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
