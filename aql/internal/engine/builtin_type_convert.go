package engine

import (
	"fmt"
	"strconv"
	"strings"
)

func registerConvert(r *Registry) {
	const defaultSize = 222

	// truncate limits a string to maxLen characters.
	truncate := func(s string, maxLen int) string {
		if maxLen < 0 {
			maxLen = 0
		}
		if len(s) > maxLen {
			return s[:maxLen]
		}
		return s
	}

	// convertTo performs the actual conversion.
	convertTo := func(src Value, targetType Type, variant string, size int) (Value, error) {
		switch {
		case targetType.Matches(TString):
			// Convert to string.
			if variant == "" {
				return NewString(truncate(valToString(src), size)), nil
			}
			// Variant-based string conversion (only for integer numbers).
			if !src.VType.Matches(TInteger) {
				return Value{}, fmt.Errorf("convert: variant %q only supported for integer to string", variant)
			}
			n := src.AsInteger()
			var s string
			switch variant {
			case "hex":
				s = strconv.FormatInt(n, 16)
			case "HEX":
				s = strings.ToUpper(strconv.FormatInt(n, 16))
			case "bin":
				s = strconv.FormatInt(n, 2)
			case "oct":
				s = strconv.FormatInt(n, 8)
			default:
				return Value{}, fmt.Errorf("convert: unknown string variant %q", variant)
			}
			return NewString(truncate(s, size)), nil

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
			if variant == "" {
				n, err := strconv.ParseInt(text, 10, 64)
				if err != nil {
					return Value{}, fmt.Errorf("convert: cannot convert %q to number", text)
				}
				return NewInteger(n), nil
			}
			var base int
			switch variant {
			case "hex":
				base = 16
			case "bin":
				base = 2
			case "oct":
				base = 8
			default:
				return Value{}, fmt.Errorf("convert: unknown number variant %q", variant)
			}
			n, err := strconv.ParseInt(text, base, 64)
			if err != nil {
				return Value{}, fmt.Errorf("convert: cannot convert %q to number (base %d)", text, base)
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
			// Convert to atom.
			return NewAtom(valToString(src)), nil

		default:
			return Value{}, fmt.Errorf("convert: unsupported target type %s", targetType)
		}
	}

	// 2-arg: convert value type
	convert2Handler := func(args []Value) ([]Value, error) {
		src := args[0]
		if args[1].Data != nil {
			return nil, fmt.Errorf("convert: second argument must be a type literal")
		}
		result, err := convertTo(src, args[1].VType, "", defaultSize)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	// 3-arg: convert value type <base-or-settings>
	// The third argument is either a string base shorthand (e.g. "hex")
	// or a settings map (e.g. {base:hex, size:3}).
	convert3Handler := func(args []Value) ([]Value, error) {
		src := args[0]
		if args[1].Data != nil {
			return nil, fmt.Errorf("convert: second argument must be a type literal")
		}
		base := ""
		size := defaultSize
		if args[2].VType.Equal(TMap) && args[2].Data != nil {
			m := args[2].AsMap()
			if v, ok := m.Get("base"); ok {
				base = valToString(v)
			}
			if v, ok := m.Get("size"); ok && v.VType.Matches(TInteger) {
				size = int(v.AsInteger())
			}
		} else {
			base = valToString(args[2])
		}
		result, err := convertTo(src, args[1].VType, base, size)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	r.Register("convert",
		// 3-arg variant registered first (higher score from more args)
		Signature{
			Args:    []Type{TAny, TAny, TAny},
			Handler: convert3Handler,
		},
		Signature{
			Args:    []Type{TAny, TAny},
			Handler: convert2Handler,
		},
	)
}
