package engine

import "fmt"

// registerUnaryStringWord registers a word that takes a single string arg
// (TString or TAtom), applies fn, and returns a NewString result.
// Covers the common pattern used by upper, lower, and similar transforms.
func registerUnaryStringWord(r *Registry, name string, fn func(string) string) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		s, ok := args[0].Data.(string)
		if !ok {
			return nil, fmt.Errorf("%s: expected string, got %s", name, args[0].String())
		}
		return []Value{NewString(fn(s))}, nil
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:              name,
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TString}, Handler: handler, Returns: []Type{TString}},
			{Args: []Type{TAtom}, Handler: handler, Returns: []Type{TString}},
		},
	})
}

// registerBinaryBoolWord registers a word that takes two boolean-ish
// args, applies fn, and returns a NewBoolean result. Non-boolean
// inputs are coerced via CoerceBoolean (same rules as `convert
// boolean`). Covers the common pattern used by xor, nand.
func registerBinaryBoolWord(r *Registry, name string, fn func(a, b bool) bool) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a := CoerceBoolean(args[0])
		b := CoerceBoolean(args[1])
		return []Value{NewBoolean(fn(a, b))}, nil
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:              name,
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TBoolean, TBoolean}, Handler: handler, Returns: []Type{TBoolean}},
			{Args: []Type{TAny, TAny}, Handler: handler, Returns: []Type{TBoolean}},
		},
	})
}

// registerBinaryMathWord registers a word with a single [TNumber, TNumber]
// signature. The fn receives (farther, nearest) as float64 and returns the
// result. When both inputs are integers and intFn is non-nil, intFn is
// called with int64 args instead, preserving integer output type.
// Extra signatures (e.g. temporal) can be appended via extraSigs.
func registerBinaryMathWord(
	r *Registry,
	name string,
	fn func(a, b float64) (Value, error),
	intFn func(a, b int64) (Value, error),
	extraSigs ...NativeSig,
) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		// When both args are integers, use the integer path to preserve type.
		if intFn != nil && args[0].VType.Matches(TInteger) && args[1].VType.Matches(TInteger) {
			a, _ := args[0].AsInteger()
			b, _ := args[1].AsInteger()
			return singleResult(intFn(b, a))
		}
		a, _ := args[0].AsNumber()
		b, _ := args[1].AsNumber()
		return singleResult(fn(b, a))
	}

	// Static type-check annotation: mirror the handler's intra-signature
	// value-dependence with ReturnsNumericBinary — when both args are
	// integers, the carrier result is Integer, otherwise Decimal.
	sigs := []NativeSig{
		{Args: []Type{TNumber, TNumber}, Handler: handler, ReturnsFn: ReturnsNumericBinary()},
	}
	sigs = append(sigs, extraSigs...)

	r.RegisterNativeFunc(NativeFunc{
		Name:              name,
		ForwardPrecedence: true,
		Signatures:        sigs,
	})
}

// singleResult wraps a (Value, error) pair into ([]Value, error) for handlers.
func singleResult(v Value, err error) ([]Value, error) {
	if err != nil {
		return nil, err
	}
	return []Value{v}, nil
}

// CoerceBoolean converts any value to a boolean using the same rules
// as `convert boolean`: booleans pass through, numbers are non-zero,
// none is false, lists/maps are non-empty, "true"/"false" parse
// literally, all other values are non-empty.
func CoerceBoolean(v Value) bool {
	switch {
	case v.VType.Matches(TBoolean):
		b, _ := v.AsBoolean()
		return b
	case v.VType.Matches(TNumber):
		n, _ := v.AsNumber()
		return n != 0
	case v.VType.Equal(TNone):
		return false
	case v.VType.Equal(TList):
		if v.Data == nil {
			return false
		}
		if elems, ok := v.Data.([]Value); ok {
			return len(elems) > 0
		}
		// Non-[]Value list backings (table types, query builders) are truthy.
		return true
	case v.VType.Equal(TMap):
		if v.Data == nil {
			return false
		}
		if om, ok := v.Data.(*OrderedMap); ok {
			return om.Len() > 0
		}
		// Non-*OrderedMap map backings (record/options/child types) are truthy.
		return true
	}
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
