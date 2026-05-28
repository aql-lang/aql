package native

import "fmt"

// binaryNatives covers the core bitwise / binary operators on the
// 64-bit signed Integer type. See lang/doc/design/BINARY-OPERATIONS.0.md.
//
// Word names are `b`-prefixed to disambiguate from the boolean
// connectives (`and`, `or`, `xor`, `not`) which short-circuit on
// truthiness instead of operating on bit patterns.
//
// Argument convention: the canonical surface form reads as
// "value OP count" for shifts:
//
//	8 bsl 2   => 32     # 8 << 2
//	8 bsr 2   => 2      # 8 >> 2  (sign-extending)
//	-1 busr 60 => 15    # logical right-shift, zero-fill
//
// The handler convention `args[1] OP args[0]` (swap form) matches
// every other binary word in AQL.
var binaryNatives = []NativeFunc{
	{
		Name: "band",

		Signatures: []NativeSig{{
			Args:    []*Type{TInteger, TInteger},
			Handler: bandHandler,
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "bor",

		Signatures: []NativeSig{{
			Args:    []*Type{TInteger, TInteger},
			Handler: borHandler,
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "bxor",

		Signatures: []NativeSig{{
			Args:    []*Type{TInteger, TInteger},
			Handler: bxorHandler,
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "bnot",

		Signatures: []NativeSig{{
			Args:    []*Type{TInteger},
			Handler: bnotHandler,
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "bsl",

		Signatures: []NativeSig{{
			Args:    []*Type{TInteger, TInteger},
			Handler: bslHandler,
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "bsr",

		Signatures: []NativeSig{{
			Args:    []*Type{TInteger, TInteger},
			Handler: bsrHandler,
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "busr",

		Signatures: []NativeSig{{
			Args:    []*Type{TInteger, TInteger},
			Handler: busrHandler,
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	},
}

func intPair(args []Value) (lhs, rhs int64, err error) {
	rhs, err = args[0].AsConcreteInteger()
	if err != nil {
		return 0, 0, err
	}
	lhs, err = args[1].AsConcreteInteger()
	if err != nil {
		return 0, 0, err
	}
	return lhs, rhs, nil
}

func bandHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	a, b, err := intPair(args)
	if err != nil {
		return nil, err
	}
	return []Value{NewInteger(a & b)}, nil
}

func borHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	a, b, err := intPair(args)
	if err != nil {
		return nil, err
	}
	return []Value{NewInteger(a | b)}, nil
}

func bxorHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	a, b, err := intPair(args)
	if err != nil {
		return nil, err
	}
	return []Value{NewInteger(a ^ b)}, nil
}

func bnotHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	x, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, err
	}
	return []Value{NewInteger(^x)}, nil
}

// bslHandler: `value bsl count` → value << count. Shifts >= 64
// saturate to 0; negative counts error.
func bslHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	x, n, err := intPair(args)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, r.AqlError("binary_error",
			fmt.Sprintf("bsl: shift count must be non-negative, got %d", n), "bsl")
	}
	if n >= 64 {
		return []Value{NewInteger(0)}, nil
	}
	return []Value{NewInteger(x << uint(n))}, nil
}

// bsrHandler: arithmetic (sign-extending) right shift.
// `value bsr count` → value >> count with sign-fill. n >= 64 yields
// 0 for non-negative inputs and -1 for negative inputs.
func bsrHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	x, n, err := intPair(args)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, r.AqlError("binary_error",
			fmt.Sprintf("bsr: shift count must be non-negative, got %d", n), "bsr")
	}
	if n >= 64 {
		if x < 0 {
			return []Value{NewInteger(-1)}, nil
		}
		return []Value{NewInteger(0)}, nil
	}
	return []Value{NewInteger(x >> uint(n))}, nil
}

// busrHandler: logical (unsigned) right shift. `value busr count` →
// (uint64(value) >> count) reinterpreted as int64. Vacated high bits
// zero-fill.
func busrHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	x, n, err := intPair(args)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, r.AqlError("binary_error",
			fmt.Sprintf("busr: shift count must be non-negative, got %d", n), "busr")
	}
	if n >= 64 {
		return []Value{NewInteger(0)}, nil
	}
	return []Value{NewInteger(int64(uint64(x) >> uint(n)))}, nil
}
