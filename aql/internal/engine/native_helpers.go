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

// registerBinaryMathWord registers a word with a single [TNumber, TNumber]
// signature. The fn receives (farther, nearest) as float64 and returns the
// result. When both inputs are integers and intFn is non-nil, intFn is
// called with int64 args instead, preserving integer output type.
// Extra signatures (e.g. temporal) can be appended via extraSigs.
// numericBinaryHandler builds the standard [TNumber, TNumber] handler
// for a binary math word. It reads sig args in sig order — args[0] is
// the first arg the matcher bound, args[1] is the second — and feeds
// them straight into the supplied math function. No arg reordering.
//
// The earlier `registerBinaryMathWord` helper that used to live here
// was deleted as part of the §1.4 cleanup: it wrapped this handler
// AND auto-registered the whole word, hiding an internal arg
// reversal (`fn(args[1], args[0])`) that encoded a no-longer-needed
// "prefix-as-canonical" reading. With the unified matcher, every
// native word should look the same shape at the call site:
//
//	r.RegisterNativeFunc(NativeFunc{
//	    Name: "...",
//	    ForwardPrecedence: true,
//	    Signatures: []NativeSig{
//	        {Args: []Type{TNumber, TNumber}, Handler: numericBinaryHandler(intFn, decFn), ReturnsFn: ReturnsNumericBinary()},
//	        // …extra sigs (date, duration, ...) inline as needed.
//	    },
//	})
func numericBinaryHandler(
	intFn func(a, b int64) (Value, error),
	decFn func(a, b float64) (Value, error),
) Handler {
	return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if intFn != nil && args[0].VType.Matches(TInteger) && args[1].VType.Matches(TInteger) {
			a, _ := args[0].AsConcreteInteger()
			b, _ := args[1].AsConcreteInteger()
			return singleResult(intFn(a, b))
		}
		a, _ := args[0].AsNumber()
		b, _ := args[1].AsNumber()
		return singleResult(decFn(a, b))
	}
}

// singleResult wraps a (Value, error) pair into ([]Value, error) for handlers.
func singleResult(v Value, err error) ([]Value, error) {
	if err != nil {
		return nil, err
	}
	return []Value{v}, nil
}

// CoerceBoolean is re-exported by aliases.go (defined in aqleng).
