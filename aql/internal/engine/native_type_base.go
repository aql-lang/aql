package engine

import "fmt"

func registerBase(r *Registry) {
	baseHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		v := args[0]
		t := v.VType
		result, err := baseValue(t)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "base",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{TAny},
			Handler: baseHandler,
			// base returns the zero value of the arg's described
			// type; at the carrier level, this is the arg's type.
			ReturnsFn: ReturnsIdentity(0),
		}},
	})
}

// baseValue returns the zero/default value for a given type, similar to Go's
// zero values. Used by both the "base" word and "make" with base:true option.
func baseValue(t Type) (Value, error) {
	switch {
	case t.Matches(TInteger):
		return NewInteger(0), nil
	case t.Matches(TDecimal):
		return NewDecimal(0), nil
	case t.Matches(TNumber):
		return NewInteger(0), nil
	case t.Matches(TString):
		return NewString(""), nil
	case t.Matches(TBoolean):
		return NewBoolean(false), nil
	case t.Matches(TList):
		return NewList([]Value{}), nil
	case t.Matches(TMap):
		return NewMap(NewOrderedMap()), nil
	case t.Matches(TNone):
		return NewTypeLiteral(TNone), nil
	case t.Matches(TAtom):
		return NewAtom(""), nil
	default:
		return Value{}, fmt.Errorf("base: unsupported type %s", t.String())
	}
}

// baseValueForConstraint returns the base value for a field constraint.
// For type literals, returns the zero value directly.
// For disjunctions (e.g. string|none), returns the base of the first
// non-none alternative.
func baseValueForConstraint(constraint Value) (Value, error) {
	if constraint.IsDisjunct() {
		di, _ := constraint.AsDisjunct()
		for _, alt := range di.Alternatives {
			if alt.Data == nil && !alt.VType.Equal(TNone) {
				return baseValue(alt.VType)
			}
		}
		// All alternatives are none.
		return NewTypeLiteral(TNone), nil
	}
	if constraint.Data == nil {
		return baseValue(constraint.VType)
	}
	return Value{}, fmt.Errorf("base: cannot determine base value for %s", constraint.String())
}
