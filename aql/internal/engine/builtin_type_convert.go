package engine

import (
	"fmt"
	"strconv"
	"strings"
)

func registerConvert(r *Registry) {
	// convertTo performs the actual conversion.
	convertTo := func(src Value, targetType Type, base string) (Value, error) {
		switch {
		case targetType.Matches(TString):
			// Convert to string.
			if base == "" {
				return NewString(valToString(src)), nil
			}
			// Base-based string conversion (only for integer numbers).
			if !src.VType.Matches(TInteger) {
				return Value{}, fmt.Errorf("convert: base %q only supported for integer to string", base)
			}
			n := src.AsInteger()
			var s string
			switch base {
			case "hex":
				s = strconv.FormatInt(n, 16)
			case "HEX":
				s = strings.ToUpper(strconv.FormatInt(n, 16))
			case "bin":
				s = strconv.FormatInt(n, 2)
			case "oct":
				s = strconv.FormatInt(n, 8)
			default:
				return Value{}, fmt.Errorf("convert: unknown base %q", base)
			}
			return NewString(s), nil

		case targetType.Matches(TDecimal):
			// Convert to decimal.
			text := valToString(src)
			f, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return Value{}, fmt.Errorf("convert: cannot convert %q to decimal", text)
			}
			return NewDecimal(f), nil

		case targetType.Matches(TNumber) || targetType.Matches(TInteger):
			// Convert to number.
			text := valToString(src)
			if base == "" {
				n, err := strconv.ParseInt(text, 10, 64)
				if err != nil {
					return Value{}, fmt.Errorf("convert: cannot convert %q to number", text)
				}
				return NewInteger(n), nil
			}
			var numBase int
			switch base {
			case "hex":
				numBase = 16
			case "bin":
				numBase = 2
			case "oct":
				numBase = 8
			default:
				return Value{}, fmt.Errorf("convert: unknown base %q", base)
			}
			n, err := strconv.ParseInt(text, numBase, 64)
			if err != nil {
				return Value{}, fmt.Errorf("convert: cannot convert %q to number (base %d)", text, numBase)
			}
			return NewInteger(n), nil

		case targetType.Matches(TBoolean):
			// Convert to boolean.
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
			return Value{}, fmt.Errorf("convert: unsupported target type %s", targetType)
		}
	}

	// 2-arg: convert ScalarType Scalar
	// args[0] = ScalarType literal (target, forward), args[1] = Scalar (source, stack)
	convert2Handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		targetType := args[0]
		src := args[1]
		if targetType.Data != nil {
			return nil, fmt.Errorf("convert: first argument must be a type literal, got %s", targetType.VType)
		}
		result, err := convertTo(src, targetType.VType, "")
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	// 3-arg: convert ScalarType Options Scalar
	// args[0] = ScalarType literal (target, forward), args[1] = Options map (forward), args[2] = Scalar (source, stack)
	convert3Handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		targetType := args[0]
		opts := args[1]
		src := args[2]
		if targetType.Data != nil {
			return nil, fmt.Errorf("convert: first argument must be a type literal, got %s", targetType.VType)
		}

		base := ""
		if opts.Data != nil {
			m := opts.AsMap()
			if m != nil {
				if bv, ok := m.Get("base"); ok {
					base = valToString(bv)
				}
			}
		}

		result, err := convertTo(src, targetType.VType, base)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	// Options pattern for 3-arg variant: {base?: String|None}
	baseOpts := NewOrderedMap()
	baseOpts.Set("base", NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)}))
	optsPattern := NewOptionsType(baseOpts)

	r.Register("convert",
		// 3-arg variant registered first (higher score from more args)
		Signature{
			Args:     []Type{TScalarType, TMap, TScalar},
			Patterns: map[int]Value{1: optsPattern},
			Handler:  convert3Handler,
		},
		Signature{
			Args:    []Type{TScalarType, TScalar},
			Handler: convert2Handler,
		},
	)
}
