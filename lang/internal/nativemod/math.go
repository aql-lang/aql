package nativemod

import (
	"math"

	"github.com/aql-lang/aql/lang/engine"
)

// BuildMathModule creates the "aql:math" native module. It registers the
// Go-implemented math words into an isolated sub-registry and returns a
// ModuleDesc with a "math" export containing FnDef wrappers for each word.
//
// After import, words are accessed via dot notation: math.sin, math.abs, etc.
//
// The basic arithmetic operations (add, sub, mul, div, mod, pow) remain
// built-in and do not require this module.
func BuildMathModule(parent *engine.Registry) (engine.ModuleDesc, error) {
	// Create an isolated sub-registry for the module's Go words.
	subReg, err := engine.DefaultRegistry()
	if err != nil {
		return engine.ModuleDesc{}, err
	}

	// Register all math words into the sub-registry.
	for _, n := range MathNatives {
		subReg.RegisterNativeFunc(n)
	}

	// Build the export map with FnDef wrappers.
	exports := engine.NewOrderedMap()

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
	desc := engine.ModuleDesc{
		ID:      modID,
		Exports: map[string]*engine.OrderedMap{"math": exports},
	}
	return desc, nil
}

// makeUnaryFnDef creates a FnDef value that wraps a unary math word.
// Uses unnamed params so args are pushed directly onto the stack.
func makeUnaryFnDef(wordName string, subReg *engine.Registry) engine.Value {
	fnDef := engine.FnDefInfo{
		Name: wordName,
		Sigs: []engine.FnSig{
			{
				Params:  []engine.FnParam{{Type: engine.TNumber}},
				Returns: []*engine.Type{engine.TNumber},
				Body:    []engine.Value{engine.NewWord(wordName)},
			},
		},
		Registry: subReg,
	}
	return engine.NewFnDef(fnDef)
}

// makeBinaryFnDef creates a FnDef value that wraps a binary math word.
// Uses unnamed params so args are pushed directly onto the stack.
func makeBinaryFnDef(wordName string, subReg *engine.Registry) engine.Value {
	fnDef := engine.FnDefInfo{
		Name: wordName,
		Sigs: []engine.FnSig{
			{
				Params: []engine.FnParam{
					{Type: engine.TNumber},
					{Type: engine.TNumber},
				},
				Returns: []*engine.Type{engine.TNumber},
				Body:    []engine.Value{engine.NewWord(wordName)},
			},
		},
		Registry: subReg,
	}
	return engine.NewFnDef(fnDef)
}

// makeConstFnDef creates a FnDef value that wraps a zero-arg constant word.
func makeConstFnDef(wordName string, subReg *engine.Registry) engine.Value {
	fnDef := engine.FnDefInfo{
		Name: wordName,
		Sigs: []engine.FnSig{
			{
				Params:  []engine.FnParam{},
				Returns: []*engine.Type{engine.TDecimal},
				Body:    []engine.Value{engine.NewWord(wordName)},
			},
		},
		Registry: subReg,
	}
	return engine.NewFnDef(fnDef)
}

// MathNatives is the consolidated NativeFunc slice for the math module's
// Go-implemented words. Replaces the per-word register*Math* functions
// and the master registerAllMathWords aggregator.
var MathNatives = func() []engine.NativeFunc {
	out := []engine.NativeFunc{
		// abs: integer or decimal -> matching numeric.
		{
			Name:        "abs",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{
				{
					Args: []*engine.Type{engine.TInteger},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						v, err := args[0].AsConcreteInteger()
						if err != nil {
							return nil, err
						}
						if v < 0 {
							v = -v
						}
						return []engine.Value{engine.NewInteger(v)}, nil
					},
					Returns: []*engine.Type{engine.TInteger},
				},
				{
					Args: []*engine.Type{engine.TDecimal},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						d, err := args[0].AsConcreteDecimal()
						if err != nil {
							return nil, err
						}
						return []engine.Value{engine.NewDecimal(math.Abs(d))}, nil
					},
					Returns: []*engine.Type{engine.TDecimal},
				},
			},
		},
		// negate: integer or decimal.
		{
			Name:        "negate",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{
				{
					Args: []*engine.Type{engine.TInteger},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						v, err := args[0].AsConcreteInteger()
						if err != nil {
							return nil, err
						}
						return []engine.Value{engine.NewInteger(-v)}, nil
					},
					Returns: []*engine.Type{engine.TInteger},
				},
				{
					Args: []*engine.Type{engine.TDecimal},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						d, err := args[0].AsConcreteDecimal()
						if err != nil {
							return nil, err
						}
						return []engine.Value{engine.NewDecimal(-d)}, nil
					},
					Returns: []*engine.Type{engine.TDecimal},
				},
			},
		},
		// sign: integer or decimal -> integer (-1/0/1).
		{
			Name:        "sign",
			ForwardArgs: true,
			Signatures: []engine.NativeSig{
				{
					Args: []*engine.Type{engine.TInteger},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						v, err := args[0].AsConcreteInteger()
						if err != nil {
							return nil, err
						}
						switch {
						case v < 0:
							return []engine.Value{engine.NewInteger(-1)}, nil
						case v > 0:
							return []engine.Value{engine.NewInteger(1)}, nil
						default:
							return []engine.Value{engine.NewInteger(0)}, nil
						}
					},
					Returns: []*engine.Type{engine.TInteger},
				},
				{
					Args: []*engine.Type{engine.TDecimal},
					Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
						v, err := args[0].AsConcreteDecimal()
						if err != nil {
							return nil, err
						}
						switch {
						case v < 0:
							return []engine.Value{engine.NewInteger(-1)}, nil
						case v > 0:
							return []engine.Value{engine.NewInteger(1)}, nil
						default:
							return []engine.Value{engine.NewInteger(0)}, nil
						}
					},
					Returns: []*engine.Type{engine.TInteger},
				},
			},
		},
		// min / max — built from the BinaryIntOpNative + BinaryNumOpNative
		// pair so each word carries an integer overload and a decimal
		// overload, matching the historical RegisterBinaryIntOp +
		// RegisterBinaryNumOp split.
		mergeBinaryNumNatives("min",
			engine.BinaryIntOpNative("min", func(a, b int64) (int64, error) {
				if a < b {
					return a, nil
				}
				return b, nil
			}),
			engine.BinaryNumOpNative("min", func(a, b float64) (float64, error) {
				if a < b {
					return a, nil
				}
				return b, nil
			}),
		),
		mergeBinaryNumNatives("max",
			engine.BinaryIntOpNative("max", func(a, b int64) (int64, error) {
				if a > b {
					return a, nil
				}
				return b, nil
			}),
			engine.BinaryNumOpNative("max", func(a, b float64) (float64, error) {
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
	// engine.UnaryNumOpNative.
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
		out = append(out, engine.UnaryNumOpNative(p.name, p.fn))
	}

	// atan2: standard binary-num overloads + an integer-integer overload.
	out = append(out, atan2Native())
	// hypot: same shape as atan2.
	out = append(out, hypotNative())

	// Math constants — zero-arg stack-only.
	out = append(out, engine.NativeFunc{
		Name:        "math-pi",
		ForwardArgs: false,
		Signatures: []engine.NativeSig{{
			Args: []*engine.Type{},
			Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				return []engine.Value{engine.NewDecimal(math.Pi)}, nil
			},
			Returns: []*engine.Type{engine.TDecimal},
		}},
	})
	out = append(out, engine.NativeFunc{
		Name:        "math-e",
		ForwardArgs: false,
		Signatures: []engine.NativeSig{{
			Args: []*engine.Type{},
			Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				return []engine.Value{engine.NewDecimal(math.E)}, nil
			},
			Returns: []*engine.Type{engine.TDecimal},
		}},
	})

	return out
}()

// mergeBinaryNumNatives combines an integer-overload NativeFunc and a
// number-overload NativeFunc (typically produced by BinaryIntOpNative
// and BinaryNumOpNative) into one NativeFunc, preserving signature
// order: integer overloads first, then number overloads.
func mergeBinaryNumNatives(name string, intNative, numNative engine.NativeFunc) engine.NativeFunc {
	sigs := make([]engine.NativeSig, 0, len(intNative.Signatures)+len(numNative.Signatures))
	sigs = append(sigs, intNative.Signatures...)
	sigs = append(sigs, numNative.Signatures...)
	return engine.NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures:  sigs,
	}
}

// ceilFloorNative builds a NativeFunc for ceil/floor/round/trunc-style
// words: decimal -> integer via int64(fn(d)).
func ceilFloorNative(name string, fn func(float64) float64) engine.NativeFunc {
	return engine.NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []engine.NativeSig{{
			Args: []*engine.Type{engine.TDecimal},
			Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
				d, err := args[0].AsConcreteDecimal()
				if err != nil {
					return nil, err
				}
				return []engine.Value{engine.NewInteger(int64(fn(d)))}, nil
			},
			Returns: []*engine.Type{engine.TInteger},
		}},
	}
}

// atan2Native builds the atan2 NativeFunc. Note the atan2 number
// overload swaps argument order historically (atan2(b, a)), so we use a
// custom binary-num builder rather than engine.BinaryNumOpNative.
func atan2Native() engine.NativeFunc {
	numHandler := func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
		a, _ := engine.AsNumber(args[0])
		b, _ := engine.AsNumber(args[1])
		return []engine.Value{engine.NewDecimal(math.Atan2(b, a))}, nil
	}
	return engine.NativeFunc{
		Name:        "atan2",
		ForwardArgs: true,
		Signatures: []engine.NativeSig{
			{Args: []*engine.Type{engine.TDecimal, engine.TDecimal}, Handler: numHandler, Returns: []*engine.Type{engine.TDecimal}},
			{Args: []*engine.Type{engine.TNumber, engine.TDecimal}, Handler: numHandler, Returns: []*engine.Type{engine.TDecimal}},
			{Args: []*engine.Type{engine.TDecimal, engine.TNumber}, Handler: numHandler, Returns: []*engine.Type{engine.TDecimal}},
			{
				Args: []*engine.Type{engine.TInteger, engine.TInteger},
				Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
					a0, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					a1, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []engine.Value{engine.NewDecimal(math.Atan2(float64(a1), float64(a0)))}, nil
				},
				Returns: []*engine.Type{engine.TDecimal},
			},
		},
	}
}

// hypotNative builds the hypot NativeFunc with the standard binary-num
// overloads plus the integer-integer overload.
func hypotNative() engine.NativeFunc {
	base := engine.BinaryNumOpNative("hypot", func(a, b float64) (float64, error) {
		return math.Hypot(a, b), nil
	})
	intSig := engine.NativeSig{
		Args: []*engine.Type{engine.TInteger, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			a0, err := args[0].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			a1, err := args[1].AsConcreteInteger()
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewDecimal(math.Hypot(float64(a0), float64(a1)))}, nil
		},
		Returns: []*engine.Type{engine.TDecimal},
	}
	return engine.NativeFunc{
		Name:        base.Name,
		ForwardArgs: true,
		Signatures:  append(append([]engine.NativeSig{}, base.Signatures...), intSig),
	}
}
