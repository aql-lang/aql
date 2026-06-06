package native

import (
	"fmt"
	"math"
	"math/big"

	"github.com/cockroachdb/apd/v3"
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
				Handler: numericBinaryHandler(towerOps{
					intFn: func(a, b int64) (Value, error) {
						c, ok := checkedAddInt(b, a)
						if !ok {
							return Value{}, integerOverflowError("add", a, b)
						}
						return NewInteger(c), nil
					},
					bigFn: func(a, b *big.Int) (Value, error) { return NewBigInteger(new(big.Int).Add(b, a)), nil },
					decFn: func(ctx *apd.Context, a, b *apd.Decimal) (Value, error) { return apdBin(ctx.Add, b, a) },
					fltFn: func(a, b float64) (Value, error) { return NewFloat(b + a), nil },
				}),
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
				Handler: numericBinaryHandler(towerOps{
					intFn: func(a, b int64) (Value, error) {
						c, ok := checkedSubInt(b, a)
						if !ok {
							return Value{}, integerOverflowError("sub", a, b)
						}
						return NewInteger(c), nil
					},
					bigFn: func(a, b *big.Int) (Value, error) { return NewBigInteger(new(big.Int).Sub(b, a)), nil },
					decFn: func(ctx *apd.Context, a, b *apd.Decimal) (Value, error) { return apdBin(ctx.Sub, b, a) },
					fltFn: func(a, b float64) (Value, error) { return NewFloat(b - a), nil },
				}),
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
			Handler: numericBinaryHandler(towerOps{
				intFn: func(a, b int64) (Value, error) {
					c, ok := checkedMulInt(b, a)
					if !ok {
						return Value{}, integerOverflowError("mul", a, b)
					}
					return NewInteger(c), nil
				},
				bigFn: func(a, b *big.Int) (Value, error) { return NewBigInteger(new(big.Int).Mul(b, a)), nil },
				decFn: func(ctx *apd.Context, a, b *apd.Decimal) (Value, error) { return apdBin(ctx.Mul, b, a) },
				fltFn: func(a, b float64) (Value, error) { return NewFloat(b * a), nil },
			}),
			ReturnsFn: ReturnsNumericBinary(), BarrierPos: -1,
		}},
	},
	{
		Name: "div",

		Signatures: []NativeSig{{
			Args: []*Type{TNumber, TNumber},
			Handler: numericBinaryHandler(towerOps{
				intFn: func(a, b int64) (Value, error) {
					// Integer division by zero has no defined result
					// (there is no integer infinity) — it stays a hard
					// error. The Float path below follows IEEE-754 instead.
					if a == 0 {
						return Value{}, fmt.Errorf("division by zero")
					}
					return NewInteger(b / a), nil
				},
				bigFn: func(a, b *big.Int) (Value, error) {
					// BigInteger division truncates toward zero (like Integer).
					if a.Sign() == 0 {
						return Value{}, fmt.Errorf("division by zero")
					}
					return NewBigInteger(new(big.Int).Quo(b, a)), nil
				},
				decFn: func(ctx *apd.Context, a, b *apd.Decimal) (Value, error) {
					// BigDecimal division rounds to the context (decimal128);
					// apd's DivisionByZero trap surfaces zero divisors as an
					// error.
					return apdBin(ctx.Quo, b, a)
				},
				fltFn: func(a, b float64) (Value, error) {
					// Float division by zero is IEEE-754: x/0 → ±inf,
					// 0/0 → nan. Go's float division already produces
					// these, so we do NOT special-case a == 0.
					return NewFloat(b / a), nil
				},
			}),
			ReturnsFn: ReturnsNumericBinary(), BarrierPos: -1,
		}},
	},
	{
		Name: "mod",

		Signatures: []NativeSig{{
			Args: []*Type{TNumber, TNumber},
			Handler: numericBinaryHandler(towerOps{
				intFn: func(a, b int64) (Value, error) {
					// Integer modulo by zero stays a hard error (no
					// integer infinity / NaN). The Float path is IEEE.
					if a == 0 {
						return Value{}, fmt.Errorf("modulo by zero")
					}
					return NewInteger(b % a), nil
				},
				bigFn: func(a, b *big.Int) (Value, error) {
					if a.Sign() == 0 {
						return Value{}, fmt.Errorf("modulo by zero")
					}
					return NewBigInteger(new(big.Int).Rem(b, a)), nil // truncated remainder, sign of dividend
				},
				decFn: func(ctx *apd.Context, a, b *apd.Decimal) (Value, error) { return apdBin(ctx.Rem, b, a) },
				fltFn: func(a, b float64) (Value, error) {
					// Float modulo by zero is IEEE-754: math.Mod(x, 0)
					// returns NaN. No special-case error.
					return NewFloat(math.Mod(b, a)), nil
				},
			}),
			ReturnsFn: ReturnsNumericBinary(), BarrierPos: -1,
		}},
	},
	{
		Name: "pow",

		Signatures: []NativeSig{{
			Args: []*Type{TNumber, TNumber},
			Handler: numericBinaryHandler(towerOps{
				intFn: func(a, b int64) (Value, error) {
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
				bigFn: func(a, b *big.Int) (Value, error) {
					if a.Sign() < 0 {
						return Value{}, fmt.Errorf("pow: negative exponent")
					}
					return NewBigInteger(new(big.Int).Exp(b, a, nil)), nil
				},
				decFn: func(ctx *apd.Context, a, b *apd.Decimal) (Value, error) { return apdBin(ctx.Pow, b, a) },
				fltFn: func(a, b float64) (Value, error) { return NewFloat(math.Pow(b, a)), nil },
			}),
			ReturnsFn: ReturnsNumericBinary(), BarrierPos: -1,
		}},
	},
	{
		Name: "with-decimal",
		Signatures: []NativeSig{{
			Args:       []*Type{TMap, TList},
			Handler:    withDecimalHandler,
			NoEvalArgs: map[int]bool{1: true},
			Returns:    []*Type{TAny}, BarrierPos: -1,
		}},
	},
}

// withDecimalHandler runs a body block with a scoped BigDecimal rounding
// context. `with-decimal {precision: N, rounding: "half-even"} [body]`
// pushes the override onto the CoW context stack, evaluates the body
// tokens (so every BigDecimal op inside — arithmetic and apd-backed
// transcendentals — uses the override), then pops. Unknown / omitted
// keys fall back to the decimal128 default (34 digits, round-half-even).
func withDecimalHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	opts := args[0]
	body := args[1]
	if !IsConcrete(body) || !body.Parent.ConformsTo(TList) {
		return nil, r.AqlError("type_error", "with-decimal: body must be a concrete list", "with-decimal")
	}

	r.Contexts.Push(r.Contexts.Top())
	defer r.Contexts.Pop()
	store := r.Contexts.Top()
	if store != nil && IsConcrete(opts) {
		if m, _ := AsMap(opts); m != nil {
			if pv, ok := m.Get("precision"); ok {
				store.Set(decimalPrecKey, pv)
			}
			if rv, ok := m.Get("rounding"); ok {
				store.Set(decimalRoundKey, rv)
			}
		}
	}

	lst, _ := AsList(body)
	return doEvalList(r, lst.Slice())
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
