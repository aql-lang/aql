package engine

import "fmt"

// isTruthy converts a Value to a boolean using the same rules as convert boolean:
// - booleans: direct value
// - numbers: non-zero is true
// - strings: "true" is true, "false" and "" are false, non-empty is true
// - atoms: same as string conversion
// - none: false
// - lists/maps: non-empty is true
func isTruthy(v Value) bool {
	switch {
	case v.VType.Matches(TBoolean):
		return v.AsBoolean()
	case v.VType.Matches(TInteger):
		return v.AsInteger() != 0
	case v.VType.Equal(TNone):
		return false
	case v.VType.Equal(TList):
		if elems, ok := v.Data.([]Value); ok {
			return len(elems) > 0
		}
		return true
	case v.VType.Equal(TMap):
		if om, ok := v.Data.(*OrderedMap); ok {
			return om.Len() > 0
		}
		return true
	default:
		text := valToString(v)
		switch text {
		case "true":
			return true
		case "false", "":
			return false
		default:
			return text != ""
		}
	}
}

// evalArg evaluates an if-word argument. If the value is a list, it is
// evaluated as code (like do). Otherwise the scalar value is returned as-is.
func evalArg(r *Registry, v Value) ([]Value, error) {
	if v.VType.Equal(TList) && !v.IsTypedList() && !v.IsTableType() {
		elems := v.AsList()
		sub := New(r)
		input := make([]Value, len(elems))
		copy(input, elems)
		return sub.Run(input)
	}
	return []Value{v}, nil
}

func registerIf(r *Registry) {
	// if: [any, any, any] -> [any] — 3-arg (condition, then, else)
	if3Handler := func(args []Value) ([]Value, error) {
		condResults, err := evalArg(r, args[0])
		if err != nil {
			return nil, fmt.Errorf("if: %w", err)
		}
		if len(condResults) == 0 {
			return nil, fmt.Errorf("if: condition produced no value")
		}
		cond := isTruthy(condResults[len(condResults)-1])

		if cond {
			result, err := evalArg(r, args[1])
			if err != nil {
				return nil, fmt.Errorf("if: %w", err)
			}
			return result, nil
		}
		result, err := evalArg(r, args[2])
		if err != nil {
			return nil, fmt.Errorf("if: %w", err)
		}
		return result, nil
	}

	// if: [any, any] -> [any] — 2-arg (condition, then)
	if2Handler := func(args []Value) ([]Value, error) {
		condResults, err := evalArg(r, args[0])
		if err != nil {
			return nil, fmt.Errorf("if: %w", err)
		}
		if len(condResults) == 0 {
			return nil, fmt.Errorf("if: condition produced no value")
		}
		cond := isTruthy(condResults[len(condResults)-1])

		if cond {
			result, err := evalArg(r, args[1])
			if err != nil {
				return nil, fmt.Errorf("if: %w", err)
			}
			return result, nil
		}
		return nil, nil
	}

	r.Register("if",
		Signature{
			Args:    []Type{TAny, TAny, TAny},
			Handler: if3Handler,
		},
		Signature{
			Args:    []Type{TAny, TAny},
			Handler: if2Handler,
		},
	)
}
