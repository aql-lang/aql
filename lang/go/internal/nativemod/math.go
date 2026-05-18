package nativemod

import (
	"math"

	"github.com/aql-lang/aql/lang/go/native"
)

// BuildMathModule creates the "aql:math" native module. It registers the
// Go-implemented math words into an isolated sub-registry and returns a
// ModuleDesc with a "math" export containing FnDef wrappers for each word.
//
// After import, words are accessed via dot notation: math.sin, math.abs, etc.
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
		"ceil", "floor", "round", "trunc",
		"sqrt", "cbrt", "exp", "log", "log2", "log10",
		"sin", "cos", "tan", "asin", "acos", "atan",
	} {
		exports.Set(name, makeUnaryFnDef(name, subReg))
	}

	// Binary operations: [Number, Number] -> [Number]
	for _, name := range []string{"min", "max", "atan2", "hypot"} {
		exports.Set(name, makeBinaryFnDef(name, subReg))
	}

	// Constants: [] -> [Decimal]
	exports.Set("pi", makeConstFnDef("math-pi", subReg))
	exports.Set("e", makeConstFnDef("math-e", subReg))

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"math": exports},
	}
	return desc, nil
}

// makeUnaryFnDef creates a FnDef value that wraps a unary math word.
// Uses unnamed params so args are pushed directly onto the stack.
func makeUnaryFnDef(wordName string, subReg *native.Registry) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{
			{
				Params:  []native.FnParam{{Type: native.TNumber}},
				Returns: []*native.Type{native.TNumber},
				Body:    []native.Value{native.NewWord(wordName)},
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
		Sigs: []native.FnSig{
			{
				Params: []native.FnParam{
					{Type: native.TNumber},
					{Type: native.TNumber},
				},
				Returns: []*native.Type{native.TNumber},
				Body:    []native.Value{native.NewWord(wordName)},
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
		Sigs: []native.FnSig{
			{
				Params:  []native.FnParam{},
				Returns: []*native.Type{native.TDecimal},
				Body:    []native.Value{native.NewWord(wordName)},
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
			Name:        "abs",
			ForwardArgs: true,
			Signatures: []native.NativeSig{
				{
					Args: []*native.Type{native.TInteger},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						v, err := args[0].AsConcreteInteger()
						if err != nil {
							return nil, err
						}
						if v < 0 {
							v = -v
						}
						return []native.Value{native.NewInteger(v)}, nil
					},
					Returns: []*native.Type{native.TInteger},
				},
				{
					Args: []*native.Type{native.TDecimal},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						d, err := args[0].AsConcreteDecimal()
						if err != nil {
							return nil, err
						}
						return []native.Value{native.NewDecimal(math.Abs(d))}, nil
					},
					Returns: []*native.Type{native.TDecimal},
				},
			},
		},
		// negate: integer or decimal.
		{
			Name:        "negate",
			ForwardArgs: true,
			Signatures: []native.NativeSig{
				{
					Args: []*native.Type{native.TInteger},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						v, err := args[0].AsConcreteInteger()
						if err != nil {
							return nil, err
						}
						return []native.Value{native.NewInteger(-v)}, nil
					},
					Returns: []*native.Type{native.TInteger},
				},
				{
					Args: []*native.Type{native.TDecimal},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						d, err := args[0].AsConcreteDecimal()
						if err != nil {
							return nil, err
						}
						return []native.Value{native.NewDecimal(-d)}, nil
					},
					Returns: []*native.Type{native.TDecimal},
				},
			},
		},
		// sign: integer or decimal -> integer (-1/0/1).
		{
			Name:        "sign",
			ForwardArgs: true,
			Signatures: []native.NativeSig{
				{
					Args: []*native.Type{native.TInteger},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
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
					},
					Returns: []*native.Type{native.TInteger},
				},
				{
					Args: []*native.Type{native.TDecimal},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						v, err := args[0].AsConcreteDecimal()
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
					},
					Returns: []*native.Type{native.TInteger},
				},
			},
		},
		// min / max — built from the BinaryIntOpNative + BinaryNumOpNative
		// pair so each word carries an integer overload and a decimal
		// overload, matching the historical RegisterBinaryIntOp +
		// RegisterBinaryNumOp split.
		mergeBinaryNumNatives("min",
			native.BinaryIntOpNative("min", func(a, b int64) (int64, error) {
				if a < b {
					return a, nil
				}
				return b, nil
			}),
			native.BinaryNumOpNative("min", func(a, b float64) (float64, error) {
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
				if a > b {
					return a, nil
				}
				return b, nil
			}),
		),
		// ceil/floor/round/trunc: decimal -> integer.
		ceilFloorNative("ceil", math.Ceil),
		ceilFloorNative("floor", math.Floor),
		ceilFloorNative("round", math.Round),
		ceilFloorNative("trunc", math.Trunc),
	}

	// Unary float -> float words. Each becomes a NativeFunc with two
	// overloads ([integer] and [decimal]) returning Decimal, courtesy of
	// native.UnaryNumOpNative.
	for _, p := range []struct {
		name string
		fn   func(float64) float64
	}{
		{"sqrt", math.Sqrt},
		{"cbrt", math.Cbrt},
		{"exp", math.Exp},
		{"log", math.Log},
		{"log2", math.Log2},
		{"log10", math.Log10},
		{"sin", math.Sin},
		{"cos", math.Cos},
		{"tan", math.Tan},
		{"asin", math.Asin},
		{"acos", math.Acos},
		{"atan", math.Atan},
	} {
		out = append(out, native.UnaryNumOpNative(p.name, p.fn))
	}

	// atan2: standard binary-num overloads + an integer-integer overload.
	out = append(out, atan2Native())
	// hypot: same shape as atan2.
	out = append(out, hypotNative())

	// Math constants — zero-arg stack-only.
	out = append(out, native.NativeFunc{
		Name:        "math-pi",
		ForwardArgs: false,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{},
			Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				return []native.Value{native.NewDecimal(math.Pi)}, nil
			},
			Returns: []*native.Type{native.TDecimal},
		}},
	})
	out = append(out, native.NativeFunc{
		Name:        "math-e",
		ForwardArgs: false,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{},
			Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				return []native.Value{native.NewDecimal(math.E)}, nil
			},
			Returns: []*native.Type{native.TDecimal},
		}},
	})

	return out
}()

// mergeBinaryNumNatives combines an integer-overload NativeFunc and a
// number-overload NativeFunc (typically produced by BinaryIntOpNative
// and BinaryNumOpNative) into one NativeFunc, preserving signature
// order: integer overloads first, then number overloads.
func mergeBinaryNumNatives(name string, intNative, numNative native.NativeFunc) native.NativeFunc {
	sigs := make([]native.NativeSig, 0, len(intNative.Signatures)+len(numNative.Signatures))
	sigs = append(sigs, intNative.Signatures...)
	sigs = append(sigs, numNative.Signatures...)
	return native.NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures:  sigs,
	}
}

// ceilFloorNative builds a NativeFunc for ceil/floor/round/trunc-style
// words: decimal -> integer via int64(fn(d)).
func ceilFloorNative(name string, fn func(float64) float64) native.NativeFunc {
	return native.NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TDecimal},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				d, err := args[0].AsConcreteDecimal()
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(fn(d)))}, nil
			},
			Returns: []*native.Type{native.TInteger},
		}},
	}
}

// atan2Native builds the atan2 NativeFunc. Note the atan2 number
// overload swaps argument order historically (atan2(b, a)), so we use a
// custom binary-num builder rather than native.BinaryNumOpNative.
func atan2Native() native.NativeFunc {
	numHandler := func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		a, _ := native.AsNumber(args[0])
		b, _ := native.AsNumber(args[1])
		return []native.Value{native.NewDecimal(math.Atan2(b, a))}, nil
	}
	return native.NativeFunc{
		Name:        "atan2",
		ForwardArgs: true,
		Signatures: []native.NativeSig{
			{Args: []*native.Type{native.TDecimal, native.TDecimal}, Handler: numHandler, Returns: []*native.Type{native.TDecimal}},
			{Args: []*native.Type{native.TNumber, native.TDecimal}, Handler: numHandler, Returns: []*native.Type{native.TDecimal}},
			{Args: []*native.Type{native.TDecimal, native.TNumber}, Handler: numHandler, Returns: []*native.Type{native.TDecimal}},
			{
				Args: []*native.Type{native.TInteger, native.TInteger},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					a0, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					a1, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewDecimal(math.Atan2(float64(a1), float64(a0)))}, nil
				},
				Returns: []*native.Type{native.TDecimal},
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
	intSig := native.NativeSig{
		Args: []*native.Type{native.TInteger, native.TInteger},
		Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
			a0, err := args[0].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			a1, err := args[1].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []native.Value{native.NewDecimal(math.Hypot(float64(a0), float64(a1)))}, nil
		},
		Returns: []*native.Type{native.TDecimal},
	}
	return native.NativeFunc{
		Name:        base.Name,
		ForwardArgs: true,
		Signatures:  append(append([]native.NativeSig{}, base.Signatures...), intSig),
	}
}
