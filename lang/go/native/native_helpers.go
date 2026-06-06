package native

import (
	"math/big"

	"github.com/cockroachdb/apd/v3"
)

// Numeric leaf ranks for the arithmetic "tower". The exact leaves form a
// widest-wins ladder (Integer < BigInteger < BigDecimal); Float is the
// inexact leaf and sits off the ladder (it infects Integer but is an
// ERROR opposite a Big leaf — see numericBinaryHandler).
const (
	leafInteger    = iota // int64
	leafBigInteger        // *big.Int
	leafBigDecimal        // *apd.Decimal
	leafFloat             // float64 — special, not on the exact ladder
)

// numLeaf classifies a numeric value's leaf. Tested most-specific first
// because BigInteger/BigDecimal both ConformsTo TNumber but neither
// conforms to TInteger/TFloat.
func numLeaf(v Value) int {
	switch {
	case v.Parent.ConformsTo(TBigDecimal):
		return leafBigDecimal
	case v.Parent.ConformsTo(TBigInteger):
		return leafBigInteger
	case v.Parent.ConformsTo(TFloat):
		return leafFloat
	default:
		return leafInteger
	}
}

// decimal128 is the default rounding context for BigDecimal ops: IEEE-754
// decimal128 — 34 significant digits, round-half-even. Threaded to every
// apd op; a future `with-decimal` block word (Phase C) overrides it.
var decimal128 = func() *apd.Context {
	c := apd.BaseContext.WithPrecision(34)
	c.Rounding = apd.RoundHalfEven
	return c
}()

// Context-store keys under which a `with-decimal` block records its
// scoped precision / rounding override. They live on the normal CoW
// context stack, so nesting and unwinding come for free.
const (
	decimalPrecKey  = "$decimal-precision"
	decimalRoundKey = "$decimal-rounding"
)

// decimalRounders maps the rounding names accepted by `with-decimal`
// (both AQL-hyphenated and apd's own underscore spellings) to apd
// Rounders. Anything unrecognised falls back to round-half-even.
var decimalRounders = map[string]apd.Rounder{
	"half-even": apd.RoundHalfEven, "half_even": apd.RoundHalfEven,
	"half-up": apd.RoundHalfUp, "half_up": apd.RoundHalfUp,
	"half-down": apd.RoundHalfDown, "half_down": apd.RoundHalfDown,
	"down": apd.RoundDown, "up": apd.RoundUp,
	"ceiling": apd.RoundCeiling, "floor": apd.RoundFloor,
}

// decimalContext returns the active BigDecimal rounding context: the
// package default decimal128 unless a surrounding `with-decimal` block
// has pushed a precision/rounding override onto the context store.
func decimalContext(r *Registry) *apd.Context {
	if r == nil || r.Contexts == nil {
		return decimal128
	}
	pv, pok := ContextStoreLookup(r, decimalPrecKey)
	rv, rok := ContextStoreLookup(r, decimalRoundKey)
	if !pok && !rok {
		return decimal128
	}
	prec := uint32(34)
	if pok {
		if n, err := pv.AsConcreteInteger(); err == nil && n > 0 {
			prec = uint32(n)
		}
	}
	rounding := apd.RoundHalfEven
	if rok {
		if s, err := rv.AsConcreteString(); err == nil {
			if mapped, ok := decimalRounders[s]; ok {
				rounding = mapped
			}
		}
	}
	c := apd.BaseContext.WithPrecision(prec)
	c.Rounding = rounding
	return c
}

// asBigOperand projects an exact integer value (Integer or BigInteger) to
// *big.Int. Only called when the widest leaf is BigInteger, so the value
// is one of those two.
func asBigOperand(v Value) *big.Int {
	if v.Parent.ConformsTo(TBigInteger) {
		n, _ := AsBigInteger(v)
		return n
	}
	n, _ := v.AsConcreteInteger()
	return big.NewInt(n)
}

// asApdOperand projects any exact numeric value (Integer/BigInteger/
// BigDecimal) to *apd.Decimal. Only called when the widest leaf is
// BigDecimal.
func asApdOperand(v Value) *apd.Decimal {
	switch {
	case v.Parent.ConformsTo(TBigDecimal):
		d, _ := AsBigDecimal(v)
		return d
	case v.Parent.ConformsTo(TBigInteger):
		n, _ := AsBigInteger(v)
		d, _, _ := apd.NewFromString(n.String())
		return d
	default:
		n, _ := v.AsConcreteInteger()
		return apd.New(n, 0)
	}
}

// towerOps bundles a binary math word's per-leaf implementations. Each
// closure follows the kernel b-op-a convention: it receives (a, b) =
// (args[0], args[1]) and computes `b OP a`. A nil closure means the leaf
// is unsupported (matchSignature/typing keeps that case out).
type towerOps struct {
	intFn func(a, b int64) (Value, error)
	bigFn func(a, b *big.Int) (Value, error)
	decFn func(ctx *apd.Context, a, b *apd.Decimal) (Value, error)
	fltFn func(a, b float64) (Value, error)
}

// numericBinaryHandler builds the [TNumber, TNumber] handler for a binary
// math word as a leaf-rank tower. Args are in sig order (args[0], args[1]).
//
//   - Float opposite a Big leaf → [aql/type_error] (the exact Big types
//     never silently degrade to binary Float; convert explicitly).
//   - Float opposite Integer/Float → the Float path, exactly as before
//     (preserves Integer⊕Float→Float and the IEEE NaN behaviour; AsNumber
//     still works because neither side is Big).
//   - Both exact → the widest leaf wins (Integer<BigInteger<BigDecimal):
//     int64 (unchanged), *big.Int, or *apd.Decimal at the decimal context.
func numericBinaryHandler(ops towerOps) Handler {
	return func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		la, lb := numLeaf(args[0]), numLeaf(args[1])
		if la == leafFloat || lb == leafFloat {
			other := la
			if la == leafFloat {
				other = lb
			}
			if other == leafBigInteger || other == leafBigDecimal {
				return nil, bigFloatMixError(r, args[0], args[1])
			}
			a, _ := AsNumber(args[0])
			b, _ := AsNumber(args[1])
			return singleResult(ops.fltFn(a, b))
		}
		w := la
		if lb > w {
			w = lb
		}
		switch w {
		case leafBigDecimal:
			return singleResult(ops.decFn(decimalContext(r), asApdOperand(args[0]), asApdOperand(args[1])))
		case leafBigInteger:
			return singleResult(ops.bigFn(asBigOperand(args[0]), asBigOperand(args[1])))
		default:
			a, _ := args[0].AsConcreteInteger()
			b, _ := args[1].AsConcreteInteger()
			return singleResult(ops.intFn(a, b))
		}
	}
}

// apdBin runs a binary apd Context op into a fresh Decimal and wraps the
// result as a BigDecimal, mapping any apd error (division by zero,
// exponent overflow, …) to the caller. Informational Conditions
// (Rounded/Inexact) are expected for div/pow and ignored.
func apdBin(fn func(d, x, y *apd.Decimal) (apd.Condition, error), x, y *apd.Decimal) (Value, error) {
	out := new(apd.Decimal)
	if _, err := fn(out, x, y); err != nil {
		return Value{}, err
	}
	return NewBigDecimal(out), nil
}

// bigFloatMixError reports an arithmetic mix of an arbitrary-precision Big
// type with a binary Float — disallowed so exactness is never silently
// lost. The user must convert one operand explicitly.
func bigFloatMixError(r *Registry, a, b Value) error {
	return r.AqlError("type_error",
		"cannot mix "+a.Parent.Leaf()+" and "+b.Parent.Leaf()+
			" in arithmetic — convert one explicitly (a Big type never silently becomes a binary Float)",
		"")
}

// singleResult wraps a (Value, error) pair into ([]Value, error) for handlers.
func singleResult(v Value, err error) ([]Value, error) {
	if err != nil {
		return nil, err
	}
	return []Value{v}, nil
}

// ApdUnaryNative builds a unary transcendental word that stays on the
// Float path for Integer/Float arguments (byte-identical to
// UnaryNumOpNative) but, for a Big argument, computes the result EXACTLY
// to the active decimal rounding context and returns a BigDecimal. Used
// for the apd-backed transcendentals (sqrt, ln/log, exp, log10) where the
// arbitrary-precision result is meaningful.
//
//	floatFn — the float64 implementation (Integer/Float args).
//	apdFn   — the apd.Context method (e.g. (*apd.Context).Sqrt), applied
//	          to a Big arg projected to *apd.Decimal.
func ApdUnaryNative(name string, floatFn func(float64) float64, apdFn func(c *apd.Context, d, x *apd.Decimal) (apd.Condition, error)) NativeFunc {
	floatHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		v, _ := AsNumber(args[0])
		return []Value{NewFloat(floatFn(v))}, nil
	}
	bigHandler := func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		out := new(apd.Decimal)
		if _, err := apdFn(decimalContext(r), out, asApdOperand(args[0])); err != nil {
			return nil, r.AqlError("math_error", name+": "+err.Error(), name)
		}
		return []Value{NewBigDecimal(out)}, nil
	}
	return NativeFunc{
		Name: name,
		Signatures: []NativeSig{
			{Args: []*Type{TInteger}, Handler: floatHandler, Returns: []*Type{TFloat}, BarrierPos: -1},
			{Args: []*Type{TFloat}, Handler: floatHandler, Returns: []*Type{TFloat}, BarrierPos: -1},
			{Args: []*Type{TBigInteger}, Handler: bigHandler, Returns: []*Type{TBigDecimal}, BarrierPos: -1},
			{Args: []*Type{TBigDecimal}, Handler: bigHandler, Returns: []*Type{TBigDecimal}, BarrierPos: -1},
		},
	}
}

// FloatUnaryBigNative builds a unary numeric word that ALWAYS returns
// Float, accepting Big arguments via the deliberately-lossy AsFloatApprox
// projection. Used for the transcendentals apd cannot compute exactly
// (trig, cbrt, log2, logb): a Big argument is accepted for convenience
// but the result is necessarily an approximate binary Float.
func FloatUnaryBigNative(name string, floatFn func(float64) float64) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		v, _ := AsFloatApprox(args[0])
		return []Value{NewFloat(floatFn(v))}, nil
	}
	return NativeFunc{
		Name: name,
		Signatures: []NativeSig{
			{Args: []*Type{TInteger}, Handler: handler, Returns: []*Type{TFloat}, BarrierPos: -1},
			{Args: []*Type{TFloat}, Handler: handler, Returns: []*Type{TFloat}, BarrierPos: -1},
			{Args: []*Type{TBigInteger}, Handler: handler, Returns: []*Type{TFloat}, BarrierPos: -1},
			{Args: []*Type{TBigDecimal}, Handler: handler, Returns: []*Type{TFloat}, BarrierPos: -1},
		},
	}
}

// CoerceBoolean is re-exported by aliases.go (defined in aqleng).
