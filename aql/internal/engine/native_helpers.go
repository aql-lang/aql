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

// registerBinaryBoolWord registers a word that takes two TBoolean args,
// applies fn, and returns a NewBoolean result.
// Covers the common pattern used by and, xor, nand.
func registerBinaryBoolWord(r *Registry, name string, fn func(a, b bool) bool) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a, _ := args[0].AsBoolean()
		b, _ := args[1].AsBoolean()
		return []Value{NewBoolean(fn(a, b))}, nil
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:              name,
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:    []Type{TBoolean, TBoolean},
			Handler: handler,
			Returns: []Type{TBoolean},
		}},
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

// coerceBoolean converts any value to a boolean using the same rules
// as `convert boolean`: booleans pass through, numbers are non-zero,
// "true"/"false" parse literally, all other values are non-empty.
func coerceBoolean(v Value) bool {
	switch {
	case v.VType.Matches(TBoolean):
		b, _ := v.AsBoolean()
		return b
	case v.VType.Matches(TNumber):
		n, _ := v.AsNumber()
		return n != 0
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
