package modules

import (
	"math"

	"github.com/aql-lang/aql/lang/go/native"
	"github.com/cockroachdb/apd/v3"
)

// BuildMathModule creates the "aql:math-util" native module. It registers the
// Go-implemented math words into an isolated sub-registry and returns a
// ModuleDesc with a "math" export containing FnDef wrappers for each word.
//
// After import, words are accessed via dot notation: MathUtil.sin, MathUtil.abs, etc.
//
// The basic arithmetic operations (add, sub, mul, div, mod, pow) remain
// built-in and do not require this module.
func BuildMathModule(parent *native.Registry) (native.ModuleDesc, error) {
	// Create an isolated sub-registry for the module's Go words.
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	// Register all math words into the sub-registry.
	for _, n := range MathNatives {
		subReg.RegisterNativeFunc(n)
	}

	// Build the export map with FnDef wrappers.
	exports := native.NewOrderedMap()

	// Unary operations: [Number] -> [Number]
	for _, name := range []string{
		"abs", "negate", "sign",
		"ceil", "floor", "round", "round-even", "trunc",
		"sqrt", "cbrt", "exp", "log", "log2", "log10", "logb",
		"sin", "cos", "tan", "asin", "acos", "atan",
	} {
		exports.Set(name, makeUnaryFnDef(name, subReg))
	}

	// Binary operations: [Number, Number] -> [Number]
	for _, name := range []string{"min", "max", "atan2", "hypot", "remainder", "copysign", "nextafter", "scalb"} {
		exports.Set(name, makeBinaryFnDef(name, subReg))
	}

	// Ternary operations: [Number, Number, Number] -> [Number]
	exports.Set("fma", makeTernaryFnDef("fma", subReg))

	// IEEE-754 classifier predicates: [Number] -> [Boolean].
	for _, name := range []string{"is-nan", "is-inf", "is-finite", "signbit"} {
		exports.Set(name, makeUnaryPredFnDef(name, subReg))
	}

	// Constants: [] -> [Float]
	exports.Set("pi", makeConstFnDef("math-pi", subReg))
	exports.Set("e", makeConstFnDef("math-e", subReg))

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		Src:     subReg,
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"MathUtil": exports},
	}
	return desc, nil
}

// makeUnaryFnDef creates a FnDef value that wraps a unary math word.
// Uses unnamed params so args are pushed directly onto the stack.
func makeUnaryFnDef(wordName string, subReg *native.Registry) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Signatures: []native.FnSig{
			{
				Params:  []native.FnParam{{Type: native.TNumber}},
				Returns: []*native.Type{native.TNumber},
				Impl:    native.AQL([]native.Value{native.NewWord(wordName)}), BarrierPos: -1,
			},
		},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

// makeUnaryPredFnDef wraps a unary math predicate word: [Number] ->
// [Boolean] (the IEEE classifiers is-nan / is-inf / is-finite / signbit).
func makeUnaryPredFnDef(wordName string, subReg *native.Registry) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Signatures: []native.FnSig{
			{
				Params:  []native.FnParam{{Type: native.TNumber}},
				Returns: []*native.Type{native.TBoolean},
				Impl:    native.AQL([]native.Value{native.NewWord(wordName)}), BarrierPos: -1,
			},
		},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

// makeBinaryFnDef creates a FnDef value that wraps a binary math word.
// Uses unnamed params so args are pushed directly onto the stack.
func makeBinaryFnDef(wordName string, subReg *native.Registry) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Signatures: []native.FnSig{
			{
				Params: []native.FnParam{
					{Type: native.TNumber},
					{Type: native.TNumber},
				},
				Returns: []*native.Type{native.TNumber},
				Impl:    native.AQL([]native.Value{native.NewWord(wordName)}), BarrierPos: -1,
			},
		},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

// makeTernaryFnDef creates a FnDef value that wraps a ternary math word
// ([Number, Number, Number] -> [Number]) — used for fma.
func makeTernaryFnDef(wordName string, subReg *native.Registry) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Signatures: []native.FnSig{
			{
				Params: []native.FnParam{
					{Type: native.TNumber},
					{Type: native.TNumber},
					{Type: native.TNumber},
				},
				Returns: []*native.Type{native.TNumber},
				Impl:    native.AQL([]native.Value{native.NewWord(wordName)}), BarrierPos: -1,
			},
		},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

// makeConstFnDef creates a FnDef value that wraps a zero-arg constant word.
func makeConstFnDef(wordName string, subReg *native.Registry) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Signatures: []native.FnSig{
			{
				Params:  []native.FnParam{},
				Returns: []*native.Type{native.TFloat},
				Impl:    native.AQL([]native.Value{native.NewWord(wordName)}), BarrierPos: -1,
			},
		},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

// MathNatives is the consolidated NativeFunc slice for the math module's
// Go-implemented words. Replaces the per-word register*Math* functions
// and the master registerAllMathWords aggregator.
var MathNatives = func() []native.NativeFunc {
	out := []native.NativeFunc{
		// abs: integer or decimal -> matching numeric.
		{
			Name: "abs",

			Signatures: []native.Signature{
				{
					Args: []*native.Type{native.TInteger},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						v, err := args[0].AsConcreteInteger()
						if err != nil {
							return nil, err
						}
						if v < 0 {
							v = -v
						}
						return []native.Value{native.NewInteger(v)}, nil
					}),
					Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
				},
				{
					Args: []*native.Type{native.TFloat},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						d, err := args[0].AsConcreteFloat()
						if err != nil {
							return nil, err
						}
						return []native.Value{native.NewFloat(math.Abs(d))}, nil
					}),
					Returns: []*native.Type{native.TFloat}, BarrierPos: -1,
				},
			},
		},
		// negate: integer or decimal.
		{
			Name: "negate",

			Signatures: []native.Signature{
				{
					Args: []*native.Type{native.TInteger},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						v, err := args[0].AsConcreteInteger()
						if err != nil {
							return nil, err
						}
						return []native.Value{native.NewInteger(-v)}, nil
					}),
					Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
				},
				{
					Args: []*native.Type{native.TFloat},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						d, err := args[0].AsConcreteFloat()
						if err != nil {
							return nil, err
						}
						return []native.Value{native.NewFloat(-d)}, nil
					}),
					Returns: []*native.Type{native.TFloat}, BarrierPos: -1,
				},
			},
		},
		// sign: integer or decimal -> integer (-1/0/1).
		{
			Name: "sign",

			Signatures: []native.Signature{
				{
					Args: []*native.Type{native.TInteger},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						v, err := args[0].AsConcreteInteger()
						if err != nil {
							return nil, err
						}
						switch {
						case v < 0:
							return []native.Value{native.NewInteger(-1)}, nil
						case v > 0:
							return []native.Value{native.NewInteger(1)}, nil
						default:
							return []native.Value{native.NewInteger(0)}, nil
						}
					}),
					Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
				},
				{
					Args: []*native.Type{native.TFloat},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						v, err := args[0].AsConcreteFloat()
						if err != nil {
							return nil, err
						}
						switch {
						case v < 0:
							return []native.Value{native.NewInteger(-1)}, nil
						case v > 0:
							return []native.Value{native.NewInteger(1)}, nil
						default:
							return []native.Value{native.NewInteger(0)}, nil
						}
					}),
					Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
				},
			},
		},
		// min / max — built from the BinaryIntOpNative + BinaryNumOpNative
		// pair so each word carries an integer overload and a decimal
		// overload, matching the historical RegisterBinaryIntOp +
		// RegisterBinaryNumOp split.
		// min / max ignore NaN (IEEE-2008 minNum/maxNum): if one operand
		// is NaN the other is returned, so the result is order-independent
		// (`min nan 5` and `min 5 nan` both give 5). Only when BOTH are
		// NaN is the result NaN. This treats NaN as "missing", which suits
		// a data/query language; see design/IEEE-754-COMPLIANCE.8.md.
		mergeBinaryNumNatives("min",
			native.BinaryIntOpNative("min", func(a, b int64) (int64, error) {
				if a < b {
					return a, nil
				}
				return b, nil
			}),
			native.BinaryNumOpNative("min", func(a, b float64) (float64, error) {
				if math.IsNaN(a) {
					return b, nil
				}
				if math.IsNaN(b) {
					return a, nil
				}
				if a < b {
					return a, nil
				}
				return b, nil
			}),
		),
		mergeBinaryNumNatives("max",
			native.BinaryIntOpNative("max", func(a, b int64) (int64, error) {
				if a > b {
					return a, nil
				}
				return b, nil
			}),
			native.BinaryNumOpNative("max", func(a, b float64) (float64, error) {
				if math.IsNaN(a) {
					return b, nil
				}
				if math.IsNaN(b) {
					return a, nil
				}
				if a > b {
					return a, nil
				}
				return b, nil
			}),
		),
		// ceil/floor/round/round-even/trunc: decimal -> integer.
		// `round` rounds halves away from zero; `round-even` rounds halves
		// to even (IEEE-754 roundTiesToEven, the default rounding mode).
		ceilFloorNative("ceil", math.Ceil),
		ceilFloorNative("floor", math.Floor),
		ceilFloorNative("round", math.Round),
		ceilFloorNative("round-even", math.RoundToEven),
		ceilFloorNative("trunc", math.Trunc),
	}

	// Unary numeric words. Integer/Float arguments stay on the Float
	// path (unchanged); a Big argument is routed per the word's exactness.
	//
	// apd-backed group — sqrt/cbrt/exp/log(ln)/log10: a Big argument is
	// computed EXACTLY to the active decimal context and returns a
	// BigDecimal (apd implements all five). math.Log is natural log, so
	// `log` pairs with apd's Ln.
	for _, p := range []struct {
		name    string
		floatFn func(float64) float64
		apdFn   func(c *apd.Context, d, x *apd.Decimal) (apd.Condition, error)
	}{
		{"sqrt", math.Sqrt, (*apd.Context).Sqrt},
		{"cbrt", math.Cbrt, (*apd.Context).Cbrt},
		{"exp", math.Exp, (*apd.Context).Exp},
		{"log", math.Log, (*apd.Context).Ln},
		{"log10", math.Log10, (*apd.Context).Log10},
	} {
		out = append(out, native.ApdUnaryNative(p.name, p.floatFn, p.apdFn))
	}

	// Float-only group — trig + log2/logb: apd has no exact form, so a
	// Big argument is accepted via the lossy AsFloatApprox projection and
	// the result is a Float.
	for _, p := range []struct {
		name string
		fn   func(float64) float64
	}{
		{"log2", math.Log2},
		{"sin", math.Sin},
		{"cos", math.Cos},
		{"tan", math.Tan},
		{"asin", math.Asin},
		{"acos", math.Acos},
		{"atan", math.Atan},
		{"logb", math.Logb}, // IEEE: the unbiased radix-2 exponent of |x|
	} {
		out = append(out, native.FloatUnaryBigNative(p.name, p.fn))
	}

	// atan2: standard binary-num overloads + an integer-integer overload.
	out = append(out, atan2Native())
	// hypot: same shape as atan2.
	out = append(out, hypotNative())

	// IEEE-754 classifier predicates: [Number] -> Boolean. Integers are
	// always finite (never NaN/Inf), so the Integer arg yields the
	// constant answer (is-finite → true, the rest → false / sign).
	out = append(out,
		classifierNative("is-nan", func(f float64) bool { return math.IsNaN(f) }),
		classifierNative("is-inf", func(f float64) bool { return math.IsInf(f, 0) }),
		classifierNative("is-finite", func(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }),
		classifierNative("signbit", math.Signbit),
	)

	// IEEE-754 binary float ops. Both compute fn(args[1], args[0]) so the
	// swap form reads naturally (`a remainder b` = remainder of a by b,
	// `a copysign b` = |a| with the sign of b), matching the b-op-a
	// convention of the core arithmetic words. `remainder` is the IEEE
	// round-to-nearest remainder — distinct from `mod` (truncated fmod).
	out = append(out,
		swapBinaryFloatNative("remainder", math.Remainder),
		swapBinaryFloatNative("copysign", math.Copysign),
		swapBinaryFloatNative("nextafter", math.Nextafter),
	)

	// scalb: `x scalb n` = x * 2^n (math.Ldexp). The exponent is the
	// forward operand; truncated to int. b-op-a convention so the swap
	// form reads naturally.
	out = append(out, native.NativeFunc{
		Name: "scalb",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TNumber, native.TNumber},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, _ := native.AsNumber(args[0])
				x, _ := native.AsNumber(args[1])
				return []native.Value{native.NewFloat(math.Ldexp(x, int(n)))}, nil
			}),
			Returns: []*native.Type{native.TFloat}, BarrierPos: -1,
		}},
	})

	// fma: fused multiply-add, `fma a b c` = a*b + c computed with a
	// single rounding (math.FMA). Forward form is canonical: args are
	// taken in written order, so a*b is the product and c the addend.
	out = append(out, native.NativeFunc{
		Name: "fma",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TNumber, native.TNumber, native.TNumber},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				a, _ := native.AsNumber(args[0])
				b, _ := native.AsNumber(args[1])
				c, _ := native.AsNumber(args[2])
				return []native.Value{native.NewFloat(math.FMA(a, b, c))}, nil
			}),
			Returns: []*native.Type{native.TFloat}, BarrierPos: -1,
		}},
	})

	// Math constants — zero-arg stack-only.
	out = append(out, native.NativeFunc{
		Name: "math-pi",

		Signatures: []native.Signature{{
			Args: []*native.Type{},
			Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				return []native.Value{native.NewFloat(math.Pi)}, nil
			}),
			Returns: []*native.Type{native.TFloat}, BarrierPos: 0,
		}},
	})
	out = append(out, native.NativeFunc{
		Name: "math-e",

		Signatures: []native.Signature{{
			Args: []*native.Type{},
			Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				return []native.Value{native.NewFloat(math.E)}, nil
			}),
			Returns: []*native.Type{native.TFloat}, BarrierPos: 0,
		}},
	})

	return out
}()

// mergeBinaryNumNatives combines an integer-overload NativeFunc and a
// number-overload NativeFunc (typically produced by BinaryIntOpNative
// and BinaryNumOpNative) into one NativeFunc, preserving signature
// order: integer overloads first, then number overloads.
func mergeBinaryNumNatives(name string, intNative, numNative native.NativeFunc) native.NativeFunc {
	sigs := make([]native.Signature, 0, len(intNative.Signatures)+len(numNative.Signatures))
	sigs = append(sigs, intNative.Signatures...)
	sigs = append(sigs, numNative.Signatures...)
	return native.NativeFunc{
		Name: name,

		Signatures: sigs,
	}
}

// ceilFloorNative builds a NativeFunc for ceil/floor/round/trunc-style
// words: decimal -> integer via int64(fn(d)).
func ceilFloorNative(name string, fn func(float64) float64) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TFloat},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				d, err := args[0].AsConcreteFloat()
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(fn(d)))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	}
}

// classifierNative builds a [Number] -> Boolean predicate word. It
// accepts any Number (Integer or Float) and projects to float64 before
// applying the test, so `5 is-finite` and `5.0 is-finite` both work.
func classifierNative(name string, pred func(float64) bool) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TNumber},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				f, _ := native.AsNumber(args[0])
				return []native.Value{native.NewBoolean(pred(f))}, nil
			}),
			Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
		}},
	}
}

// swapBinaryFloatNative builds a [Number, Number] -> Float word whose
// handler computes fn(args[1], args[0]) — the b-op-a convention — so the
// swap surface form `a word b` evaluates fn(a, b). Accepts any Number on
// either side (the result is always a Float).
func swapBinaryFloatNative(name string, fn func(a, b float64) float64) native.NativeFunc {
	handler := func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		a, _ := native.AsNumber(args[0])
		b, _ := native.AsNumber(args[1])
		return []native.Value{native.NewFloat(fn(b, a))}, nil
	}
	return native.NativeFunc{
		Name: name,

		Signatures: []native.Signature{
			{Args: []*native.Type{native.TNumber, native.TNumber}, Impl: native.Go(handler), Returns: []*native.Type{native.TFloat}, BarrierPos: -1},
		},
	}
}

// atan2Native builds the atan2 NativeFunc. Note the atan2 number
// overload swaps argument order historically (atan2(b, a)), so we use a
// custom binary-num builder rather than native.BinaryNumOpNative.
func atan2Native() native.NativeFunc {
	numHandler := func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		a, _ := native.AsNumber(args[0])
		b, _ := native.AsNumber(args[1])
		return []native.Value{native.NewFloat(math.Atan2(b, a))}, nil
	}
	return native.NativeFunc{
		Name: "atan2",

		Signatures: []native.Signature{
			{Args: []*native.Type{native.TFloat, native.TFloat}, Impl: native.Go(numHandler), Returns: []*native.Type{native.TFloat}, BarrierPos: -1},
			{Args: []*native.Type{native.TNumber, native.TFloat}, Impl: native.Go(numHandler), Returns: []*native.Type{native.TFloat}, BarrierPos: -1},
			{Args: []*native.Type{native.TFloat, native.TNumber}, Impl: native.Go(numHandler), Returns: []*native.Type{native.TFloat}, BarrierPos: -1},
			{
				Args: []*native.Type{native.TInteger, native.TInteger},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					a0, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					a1, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewFloat(math.Atan2(float64(a1), float64(a0)))}, nil
				}),
				Returns: []*native.Type{native.TFloat}, BarrierPos: -1,
			},
		},
	}
}

// hypotNative builds the hypot NativeFunc with the standard binary-num
// overloads plus the integer-integer overload.
func hypotNative() native.NativeFunc {
	base := native.BinaryNumOpNative("hypot", func(a, b float64) (float64, error) {
		return math.Hypot(a, b), nil
	})
	intSig := native.Signature{
		Args: []*native.Type{native.TInteger, native.TInteger},
		Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
			a0, err := args[0].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			a1, err := args[1].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []native.Value{native.NewFloat(math.Hypot(float64(a0), float64(a1)))}, nil
		}),
		Returns: []*native.Type{native.TFloat}, BarrierPos: -1,
	}
	return native.NativeFunc{
		Name: base.Name,

		Signatures: append(append([]native.Signature{}, base.Signatures...), intSig),
	}
}
