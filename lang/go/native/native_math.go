package native

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/cockroachdb/apd/v3"
)

// mathNatives are the basic arithmetic words: add, sub, mul, div,
// mod, pow. Each shares a [TNumber, TNumber] base signature wired
// through numericBinaryHandler; add/sub additionally carry the
// temporal overloads (date+CalendarDuration, datetime+ClockDuration, etc.)
// and add carries the [TString, TScalar] / [TScalar, TString]
// string-concatenation overloads, used when AT LEAST ONE input is a
// String (the other Scalar coerces to its string form). Two
// non-String scalars match neither overload and are a type error.
//
// All [TNumber, TNumber] handlers compute b op a (i.e.
// args[1] op args[0]). Under §1.4 the swap form `a op b` is the
// preferred surface syntax, and binds args[0]=b, args[1]=a; the
// b-op-a body therefore yields the natural reading (`10 sub 3` → 7,
// `10 div 3` → 3). The mirror forms (`op a b`, `b op a`, `b a op`)
// produce the reversed result.
// integerOverflowError reports an int64 arithmetic overflow. Until
// Integer becomes arbitrary-precision (Phase 1 of
// design/INTEGER-OVERFLOW-STRATEGY.5.md), integer add/sub/mul/pow that
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

// returnsIntArithChecked wraps ReturnsNumericBinary for add / sub / mul /
// pow: two concrete int64 operands that PROVABLY fault at runtime — an
// int64 overflow (the checkedXxxInt guards) or pow's negative exponent —
// are a guaranteed error, flagged on the top-level straight line with the
// byte-identical runtime detail (integerOverflowError / the pow message).
// The residual model is unchanged (the numeric-binary carrier): the flag
// gates, and every non-top-level / non-concrete / non-int64 shape — Big
// leaves never overflow, Float saturates per IEEE — takes the base path
// untouched, so compile-pass behaviour is byte-identical.
func returnsIntArithChecked(op string, faultFn func(a, b int64) error) ReturnsFunc {
	base := ReturnsNumericBinary()
	return func(args []Value, r *Registry) []Value {
		checkBigFloatMix(r, args)
		if atUncaughtTopLevel(r) && len(args) == 2 &&
			IsConcrete(args[0]) && IsConcrete(args[1]) &&
			args[0].Parent.ConformsTo(TInteger) && args[1].Parent.ConformsTo(TInteger) &&
			!args[0].Parent.ConformsTo(TBigInteger) && !args[1].Parent.ConformsTo(TBigInteger) {
			a, aerr := args[0].AsConcreteInteger()
			b, berr := args[1].AsConcreteInteger()
			if aerr == nil && berr == nil {
				if err := faultFn(a, b); err != nil {
					code, detail := "arith_error", err.Error()
					var ae *AqlError
					if errors.As(err, &ae) {
						code, detail = ae.Code, ae.Detail
					}
					eng.CheckAddUniqueDiagnostic(r, code, detail, op, args[0].Pos)
				}
			}
		}
		return base(args, r)
	}
}

// The faultFns mirror the intFn bodies exactly (same operand order, same
// error constructors) so the static detail is byte-identical to runtime.
func addIntFault(a, b int64) error {
	if _, ok := checkedAddInt(b, a); !ok {
		return integerOverflowError("add", a, b)
	}
	return nil
}

func subIntFault(a, b int64) error {
	if _, ok := checkedSubInt(b, a); !ok {
		return integerOverflowError("sub", a, b)
	}
	return nil
}

func mulIntFault(a, b int64) error {
	if _, ok := checkedMulInt(b, a); !ok {
		return integerOverflowError("mul", a, b)
	}
	return nil
}

func powIntFault(a, b int64) error {
	if a < 0 {
		return fmt.Errorf("pow: negative exponent %d", a)
	}
	if _, ok := checkedPowInt(b, a); !ok {
		return integerOverflowError("pow", a, b)
	}
	return nil
}

var mathNatives = []NativeFunc{
	{
		Name: "add",

		Signatures: []Signature{
			{
				Args: []*Type{TNumber, TNumber},
				Impl: Go(numericBinaryHandler(towerOps{
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
				})),
				ReturnsFn: returnsIntArithChecked("add", addIntFault), BarrierPos: -1,
			},
			// String concatenation requires AT LEAST ONE String operand;
			// the other may be any Scalar (coerced via ValToString). Two
			// non-String scalars (e.g. `add true 1`, `add true false`)
			// match NEITHER overload and raise a no-signature error — the type
			// system expresses "string-or-bust" directly rather than
			// silently stringifying any Scalar pair. See
			// design/WAT-AUDIT.5.md §G.
			{Args: []*Type{TString, TScalar}, Impl: Go(addConcatHandler), ReturnsFn: ReturnsAddConcat(), BarrierPos: -1},
			{Args: []*Type{TScalar, TString}, Impl: Go(addConcatHandler), ReturnsFn: ReturnsAddConcat(), BarrierPos: -1},
			// The temporal overloads (Date+CalendarDuration, Instant+ClockDuration, …)
			// moved to aql:time-util — the module owns the duration types and
			// exports add/sub word extensions built by
			// TemporalArithmeticExtensions (below); import transplants them.
		},
	},
	{
		Name: "sub",

		Signatures: []Signature{
			{
				Args: []*Type{TNumber, TNumber},
				Impl: Go(numericBinaryHandler(towerOps{
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
				})),
				ReturnsFn: returnsIntArithChecked("sub", subIntFault), BarrierPos: -1,
			},
			// Temporal sub overloads: moved to aql:time-util — see the
			// add note above and TemporalArithmeticExtensions.
		},
	},
	{
		Name: "mul",

		Signatures: []Signature{{
			Args: []*Type{TNumber, TNumber},
			Impl: Go(numericBinaryHandler(towerOps{
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
			})),
			ReturnsFn: returnsIntArithChecked("mul", mulIntFault), BarrierPos: -1,
		}},
	},
	{
		Name: "div",
		// div by a static-zero integer divisor raises (returnsDivMod drops the
		// result on that path); the flag lets a closure body ending in it —
		// `do [1 div 0]` — compile as a divergent terminal instead of islanding.
		CompileEffect: CompileValueDiverges,

		Signatures: []Signature{{
			Args: []*Type{TNumber, TNumber},
			Impl: Go(numericBinaryHandler(towerOps{
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
			})),
			ReturnsFn: returnsDivMod("division by zero"), BarrierPos: -1,
		}},
	},
	{
		Name: "mod",
		// mod by a static-zero integer divisor raises — same divergent-terminal
		// treatment as div (see above).
		CompileEffect: CompileValueDiverges,

		Signatures: []Signature{{
			Args: []*Type{TNumber, TNumber},
			Impl: Go(numericBinaryHandler(towerOps{
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
			})),
			ReturnsFn: returnsDivMod("modulo by zero"), BarrierPos: -1,
		}},
	},
	{
		Name: "pow",

		Signatures: []Signature{{
			Args: []*Type{TNumber, TNumber},
			Impl: Go(numericBinaryHandler(towerOps{
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
			})),
			ReturnsFn: returnsIntArithChecked("pow", powIntFault), BarrierPos: -1,
		}},
	},
	{
		Name: "with-decimal",
		// with-decimal {opts} [body] — runs a 0-input body inside a scoped
		// BigDecimal rounding context (opts at sig 0, body at sig 1). Like `do`,
		// the body nets its single residual; the handler pushes the context around
		// InvokeBody so the VM-run body's BigDecimal ops read the override.
		Callable: &CallableSpec{BodyPos: 1, BodyOut: 1, Inputs: func(_ []Value) []Value {
			return []Value{}
		}},
		Signatures: []Signature{{
			Args:       []*Type{TMap, TList},
			Impl:       Go(withDecimalHandler),
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
	// The body is a quotation list under the interpreter, or a compiled CLOSURE
	// when the bytecode VM drives this dispatch (the body operand lowered to
	// OpPushClosure). Both run within the scoped context below.
	closure := IsCompiledClosure(body)
	if !closure && (!IsConcrete(body) || !body.Parent.ConformsTo(TList)) {
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

	if closure {
		// Drive the closure through the InvokeBody seam (the VM re-entrant
		// runner) WITHIN the pushed context, so every BigDecimal op inside reads
		// the override exactly as the interpreter's doEvalList does.
		return InvokeBody(r, body, nil)
	}
	lst, _ := AsList(body)
	return doEvalList(r, lst.Slice())
}

func addConcatHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewString(ValToString(args[1]) + ValToString(args[0]))}, nil
}

// The temporal add/sub overload pairs share one implementation each: the
// sub side is the add side with the duration negated (neg=true). Each
// builder returns the Handler for one (temporal, duration) sig pair.

// dateCalArith builds the Date ± CalendarDuration → Date handler.
func dateCalArith(neg bool) Handler {
	sign := 1
	if neg {
		sign = -1
	}
	return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := AsDate(args[0])
		cd, _ := AsCalendarDuration(args[1])
		return []Value{NewDate(t.AddDate(sign*cd.Years, sign*cd.Months, sign*cd.Days))}, nil
	}
}

// dateTimeClkArith builds the DateTime ± ClockDuration → DateTime handler.
func dateTimeClkArith(neg bool) Handler {
	return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := AsDateTime(args[0])
		d, _ := AsClockDuration(args[1])
		if neg {
			d = -d
		}
		return []Value{NewDateTime(t.Add(d))}, nil
	}
}

// instantClkArith builds the Instant ± ClockDuration → Instant handler.
func instantClkArith(neg bool) Handler {
	return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		t := AsInstant(args[0])
		d, _ := AsClockDuration(args[1])
		if neg {
			d = -d
		}
		return []Value{NewInstant(t.Add(d))}, nil
	}
}

// addDateClkHandler (Date + ClockDuration → DateTime) has no sub
// counterpart, so it stays a plain handler.
func addDateClkHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	t := AsDate(args[0])
	d, _ := AsClockDuration(args[1])
	return []Value{NewDateTime(t.Add(d))}, nil
}

// TemporalArithmeticExtensions builds the add / sub WORD-EXTENSION
// clones aql:time-util exports (design/OPEN-WORDS.0.md §2.4 / §6
// migration 1): the temporal overloads that historically sat on the
// core words, now anchored to the module's per-import duration mints —
// which is exactly what satisfies the module-scope user-type rule at
// transplant. The handlers stay here beside the other math handlers;
// only the signature tuples reference the minted types. BuildTimeModule
// places each clone in the export map under its base name, so importing
// the module transplants the overloads onto the importer's bare
// add / sub (and TimeUtil.add / TimeUtil.sub work namespaced).
func TemporalArithmeticExtensions(tt TemporalModuleTypes) []FnDefInfo {
	return []FnDefInfo{
		NewWordExtension("add", []Signature{
			{Args: []*Type{TDate, tt.CalendarDuration}, Impl: Go(dateCalArith(false)), Returns: []*Type{TDate}, BarrierPos: -1},
			{Args: []*Type{TDateTime, tt.ClockDuration}, Impl: Go(dateTimeClkArith(false)), Returns: []*Type{TDateTime}, BarrierPos: -1},
			{Args: []*Type{TInstant, tt.ClockDuration}, Impl: Go(instantClkArith(false)), Returns: []*Type{TInstant}, BarrierPos: -1},
			{Args: []*Type{TDate, tt.ClockDuration}, Impl: Go(addDateClkHandler), Returns: []*Type{TDateTime}, BarrierPos: -1},
		}),
		NewWordExtension("sub", []Signature{
			{Args: []*Type{TDate, tt.CalendarDuration}, Impl: Go(dateCalArith(true)), Returns: []*Type{TDate}, BarrierPos: -1},
			{Args: []*Type{TDateTime, tt.ClockDuration}, Impl: Go(dateTimeClkArith(true)), Returns: []*Type{TDateTime}, BarrierPos: -1},
			{Args: []*Type{TInstant, tt.ClockDuration}, Impl: Go(instantClkArith(true)), Returns: []*Type{TInstant}, BarrierPos: -1},
		}),
	}
}
