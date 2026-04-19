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
			{Args: []Type{TString}, Handler: handler},
			{Args: []Type{TAtom}, Handler: handler},
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

	// Static type-check annotation: TNumber+TNumber returns a Number.
	// Intra-signature value-dependence (Integer+Integer→Integer,
	// otherwise Decimal) is modelled at the carrier level as a widened
	// TNumber for now; a finer split is future work.
	sigs := []NativeSig{
		{Args: []Type{TNumber, TNumber}, Handler: handler, Returns: []Type{TNumber}},
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
