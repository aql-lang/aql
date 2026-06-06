package native

import (
	"fmt"
	"math"
)

// mathNatives are the basic arithmetic words: add, sub, mul, div,
// mod, pow. Each shares a [TNumber, TNumber] base signature wired
// through numericBinaryHandler; add/sub additionally carry the
// temporal overloads (date+CalDuration, datetime+ClkDuration, etc.)
// and add carries the [TScalar, TScalar] string-concatenation
// overload used when both inputs coerce to strings.
//
// All [TNumber, TNumber] handlers compute b op a (i.e.
// args[1] op args[0]). Under §1.4 the swap form `a op b` is the
// preferred surface syntax, and binds args[0]=b, args[1]=a; the
// b-op-a body therefore yields the natural reading (`10 sub 3` → 7,
// `10 div 3` → 3). The mirror forms (`op a b`, `b op a`, `b a op`)
// produce the reversed result.
// integerOverflowError reports an int64 arithmetic overflow. Until
// Integer becomes arbitrary-precision (Phase 1 of
// design/INTEGER-OVERFLOW-STRATEGY.0.md), integer add/sub/mul/pow that
// cross the int64 range raise this rather than silently wrapping
// two's-complement (the WAT Exhibit K bug). The policy is uniform across
// every integer arithmetic word, mirroring the existing division-by-zero
// error. The float64 handlers are unaffected (they saturate to ±Inf per
// IEEE-754).
func integerOverflowError(op string, a, b int64) error {
	return &AqlError{
		Code: "integer_overflow",
		Detail: fmt.Sprintf(
			"integer overflow in %s: %d %s %d does not fit in the Integer range (-9223372036854775808..9223372036854775807)",
			op, b, op, a),
		Hint: "convert an operand to a Float (e.g. add a decimal point) if an approximate result is acceptable",
	}
}

// checkedAddInt, checkedSubInt, checkedMulInt return (result, true) on
// success and (_, false) on int64 overflow. They test the overflow
// condition before computing so they never rely on wrap behaviour.
func checkedAddInt(a, b int64) (int64, bool) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, false
	}
	return a + b, true
}

func checkedSubInt(a, b int64) (int64, bool) {
	if (b < 0 && a > math.MaxInt64+b) || (b > 0 && a < math.MinInt64+b) {
		return 0, false
	}
	return a - b, true
}

func checkedMulInt(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	// MinInt64 * -1 has no positive counterpart; guard before the
	// division check (Go defines MinInt64 / -1 as wrap, so c/b == a
	// would otherwise mask the overflow).
	if (a == math.MinInt64 && b == -1) || (b == math.MinInt64 && a == -1) {
		return 0, false
	}
	c := a * b
	if c/b != a {
		return 0, false
	}
	return c, true
}

// checkedPowInt computes base**exp (exp >= 0) by square-and-multiply,
// returning false on int64 overflow at any intermediate step.
func checkedPowInt(base, exp int64) (int64, bool) {
	result := int64(1)
	for exp > 0 {
		if exp%2 == 1 {
			var ok bool
			if result, ok = checkedMulInt(result, base); !ok {
				return 0, false
			}
		}
		exp /= 2
		if exp > 0 {
			var ok bool
			if base, ok = checkedMulInt(base, base); !ok {
				return 0, false
			}
		}
	}
	return result, true
}

var mathNatives = []NativeFunc{
	{
		Name: "add",

		Signatures: []NativeSig{
			{
				Args: []*Type{TNumber, TNumber},
				Handler: numericBinaryHandler(
					func(a, b int64) (Value, error) {
						c, ok := checkedAddInt(b, a)
						if !ok {
							return Value{}, integerOverflowError("add", a, b)
						}
						return NewInteger(c), nil
					},
					func(a, b float64) (Value, error) { return NewFloat(b + a), nil },
				),
				ReturnsFn: ReturnsNumericBinary(), BarrierPos: -1,
			},
			{Args: []*Type{TScalar, TScalar}, Handler: addConcatHandler, Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TDate, TCalDuration}, Handler: addDateCalHandler, Returns: []*Type{TDate}, BarrierPos: -1},
			{Args: []*Type{TDateTime, TClkDuration}, Handler: addDateTimeClkHandler, Returns: []*Type{TDateTime}, BarrierPos: -1},
			{Args: []*Type{TInstant, TClkDuration}, Handler: addInstantClkHandler, Returns: []*Type{TInstant}, BarrierPos: -1},
			{Args: []*Type{TDate, TClkDuration}, Handler: addDateClkHandler, Returns: []*Type{TDateTime}, BarrierPos: -1},
		},
	},
	{
		Name: "sub",

		Signatures: []NativeSig{
			{
				Args: []*Type{TNumber, TNumber},
				Handler: numericBinaryHandler(
					func(a, b int64) (Value, error) {
						c, ok := checkedSubInt(b, a)
						if !ok {
							return Value{}, integerOverflowError("sub", a, b)
						}
						return NewInteger(c), nil
					},
					func(a, b float64) (Value, error) { return NewFloat(b - a), nil },
				),
				ReturnsFn: ReturnsNumericBinary(), BarrierPos: -1,
			},
			{Args: []*Type{TDate, TCalDuration}, Handler: subDateCalHandler, Returns: []*Type{TDate}, BarrierPos: -1},
			{Args: []*Type{TDateTime, TClkDuration}, Handler: subDateTimeClkHandler, Returns: []*Type{TDateTime}, BarrierPos: -1},
			{Args: []*Type{TInstant, TClkDuration}, Handler: subInstantClkHandler, Returns: []*Type{TInstant}, BarrierPos: -1},
		},
	},
	{
		Name: "mul",

		Signatures: []NativeSig{{
			Args: []*Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) {
					c, ok := checkedMulInt(b, a)
					if !ok {
						return Value{}, integerOverflowError("mul", a, b)
					}
					return NewInteger(c), nil
				},
				func(a, b float64) (Value, error) { return NewFloat(b * a), nil },
			),
			ReturnsFn: ReturnsNumericBinary(), BarrierPos: -1,
		}},
	},
	{
		Name: "div",

		Signatures: []NativeSig{{
			Args: []*Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) {
					if a == 0 {
						return Value{}, fmt.Errorf("division by zero")
					}
					return NewInteger(b / a), nil
				},
				func(a, b float64) (Value, error) {
					if a == 0 {
						return Value{}, fmt.Errorf("division by zero")
					}
					return NewFloat(b / a), nil
				},
			),
			ReturnsFn: ReturnsNumericBinary(), BarrierPos: -1,
		}},
	},
	{
		Name: "mod",

		Signatures: []NativeSig{{
			Args: []*Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) {
					if a == 0 {
						return Value{}, fmt.Errorf("modulo by zero")
					}
					return NewInteger(b % a), nil
				},
				func(a, b float64) (Value, error) {
					if a == 0 {
						return Value{}, fmt.Errorf("modulo by zero")
					}
					return NewFloat(math.Mod(b, a)), nil
				},
			),
			ReturnsFn: ReturnsNumericBinary(), BarrierPos: -1,
		}},
	},
	{
		Name: "pow",

		Signatures: []NativeSig{{
			Args: []*Type{TNumber, TNumber},
			Handler: numericBinaryHandler(
				func(a, b int64) (Value, error) {
					// Compute b ** a under §1.4 swap-form preference.
					if a < 0 {
						return Value{}, fmt.Errorf("pow: negative exponent %d", a)
					}
					result, ok := checkedPowInt(b, a)
					if !ok {
						return Value{}, integerOverflowError("pow", a, b)
					}
					return NewInteger(result), nil
				},
				func(a, b float64) (Value, error) { return NewFloat(math.Pow(b, a)), nil },
			),
			ReturnsFn: ReturnsNumericBinary(), BarrierPos: -1,
		}},
	},
}

func addConcatHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewString(ValToString(args[1]) + ValToString(args[0]))}, nil
}

func addDateCalHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsDate(args[0])
	cd, _ := AsCalDuration(args[1])
	return []Value{NewDate(t.AddDate(cd.Years, cd.Months, cd.Days))}, nil
}

func addDateTimeClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsDateTime(args[0])
	d, _ := AsClkDuration(args[1])
	return []Value{NewDateTime(t.Add(d))}, nil
}

func addInstantClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsInstant(args[0])
	d, _ := AsClkDuration(args[1])
	return []Value{NewInstant(t.Add(d))}, nil
}

func addDateClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsDate(args[0])
	d, _ := AsClkDuration(args[1])
	return []Value{NewDateTime(t.Add(d))}, nil
}

func subDateCalHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsDate(args[0])
	cd, _ := AsCalDuration(args[1])
	return []Value{NewDate(t.AddDate(-cd.Years, -cd.Months, -cd.Days))}, nil
}

func subDateTimeClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsDateTime(args[0])
	d, _ := AsClkDuration(args[1])
	return []Value{NewDateTime(t.Add(-d))}, nil
}

func subInstantClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsInstant(args[0])
	d, _ := AsClkDuration(args[1])
	return []Value{NewInstant(t.Add(-d))}, nil
}
