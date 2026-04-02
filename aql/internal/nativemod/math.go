package nativemod

import (
	"math"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
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
	registerAllMathWords(subReg)

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

	modID := parent.NextModuleID()
	desc := engine.ModuleDesc{
		ID:      modID,
		Exports: map[string]*engine.OrderedMap{"math": exports},
	}
	parent.Modules[modID] = desc
	return desc, nil
}

// makeUnaryFnDef creates a FnDef value that wraps a unary math word.
// Uses unnamed params so args are pushed directly onto the stack.
func makeUnaryFnDef(wordName string, subReg *engine.Registry) engine.Value {
	fnDef := engine.FnDefInfo{
		Sigs: []engine.FnSig{
			{
				Params:  []engine.FnParam{{Type: engine.TNumber}},
				Returns: []engine.Type{engine.TNumber},
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
		Sigs: []engine.FnSig{
			{
				Params: []engine.FnParam{
					{Type: engine.TNumber},
					{Type: engine.TNumber},
				},
				Returns: []engine.Type{engine.TNumber},
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
		Sigs: []engine.FnSig{
			{
				Params:  []engine.FnParam{},
				Returns: []engine.Type{engine.TDecimal},
				Body:    []engine.Value{engine.NewWord(wordName)},
			},
		},
		Registry: subReg,
	}
	return engine.NewFnDef(fnDef)
}

// registerAllMathWords registers the Go-implemented math words into a registry.
// These are the internal implementations used by the FnDef wrappers.
func registerAllMathWords(r *engine.Registry) {
	registerAbs(r)
	registerNegate(r)
	registerSign(r)
	registerMin(r)
	registerMax(r)

	registerCeil(r)
	registerFloor(r)
	registerRound(r)
	registerTrunc(r)

	registerSqrt(r)
	registerCbrt(r)
	registerExp(r)
	registerLog(r)
	registerLog2(r)
	registerLog10(r)

	registerSin(r)
	registerCos(r)
	registerTan(r)
	registerAsin(r)
	registerAcos(r)
	registerAtan(r)
	registerAtan2(r)
	registerHypot(r)

	registerMathConstants(r)
}

// --- Registration functions (Go implementations) ---

func registerAbs(r *engine.Registry) {
	r.Register("abs", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			v := args[0].AsInteger()
			if v < 0 {
				v = -v
			}
			return []engine.Value{engine.NewInteger(v)}, nil
		},
	})
	r.Register("abs", engine.Signature{
		Args: []engine.Type{engine.TDecimal},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewDecimal(math.Abs(args[0].AsDecimal()))}, nil
		},
	})
}

func registerNegate(r *engine.Registry) {
	r.Register("negate", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(-args[0].AsInteger())}, nil
		},
	})
	r.Register("negate", engine.Signature{
		Args: []engine.Type{engine.TDecimal},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewDecimal(-args[0].AsDecimal())}, nil
		},
	})
}

func registerSign(r *engine.Registry) {
	r.Register("sign", engine.Signature{
		Args: []engine.Type{engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			v := args[0].AsInteger()
			switch {
			case v < 0:
				return []engine.Value{engine.NewInteger(-1)}, nil
			case v > 0:
				return []engine.Value{engine.NewInteger(1)}, nil
			default:
				return []engine.Value{engine.NewInteger(0)}, nil
			}
		},
	})
	r.Register("sign", engine.Signature{
		Args: []engine.Type{engine.TDecimal},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			v := args[0].AsDecimal()
			switch {
			case v < 0:
				return []engine.Value{engine.NewInteger(-1)}, nil
			case v > 0:
				return []engine.Value{engine.NewInteger(1)}, nil
			default:
				return []engine.Value{engine.NewInteger(0)}, nil
			}
		},
	})
}

func registerMin(r *engine.Registry) {
	engine.RegisterBinaryIntOp(r, "min", func(a, b int64) (int64, error) {
		if a < b {
			return a, nil
		}
		return b, nil
	})
	engine.RegisterBinaryNumOp(r, "min", func(a, b float64) (float64, error) {
		if a < b {
			return a, nil
		}
		return b, nil
	})
}

func registerMax(r *engine.Registry) {
	engine.RegisterBinaryIntOp(r, "max", func(a, b int64) (int64, error) {
		if a > b {
			return a, nil
		}
		return b, nil
	})
	engine.RegisterBinaryNumOp(r, "max", func(a, b float64) (float64, error) {
		if a > b {
			return a, nil
		}
		return b, nil
	})
}

func registerCeil(r *engine.Registry) {
	r.Register("ceil", engine.Signature{
		Args: []engine.Type{engine.TDecimal},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(math.Ceil(args[0].AsDecimal())))}, nil
		},
	})
}

func registerFloor(r *engine.Registry) {
	r.Register("floor", engine.Signature{
		Args: []engine.Type{engine.TDecimal},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(math.Floor(args[0].AsDecimal())))}, nil
		},
	})
}

func registerRound(r *engine.Registry) {
	r.Register("round", engine.Signature{
		Args: []engine.Type{engine.TDecimal},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(math.Round(args[0].AsDecimal())))}, nil
		},
	})
}

func registerTrunc(r *engine.Registry) {
	r.Register("trunc", engine.Signature{
		Args: []engine.Type{engine.TDecimal},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(math.Trunc(args[0].AsDecimal())))}, nil
		},
	})
}

func registerSqrt(r *engine.Registry)  { engine.RegisterUnaryNumOp(r, "sqrt", math.Sqrt) }
func registerCbrt(r *engine.Registry)  { engine.RegisterUnaryNumOp(r, "cbrt", math.Cbrt) }
func registerExp(r *engine.Registry)   { engine.RegisterUnaryNumOp(r, "exp", math.Exp) }
func registerLog(r *engine.Registry)   { engine.RegisterUnaryNumOp(r, "log", math.Log) }
func registerLog2(r *engine.Registry)  { engine.RegisterUnaryNumOp(r, "log2", math.Log2) }
func registerLog10(r *engine.Registry) { engine.RegisterUnaryNumOp(r, "log10", math.Log10) }
func registerSin(r *engine.Registry)   { engine.RegisterUnaryNumOp(r, "sin", math.Sin) }
func registerCos(r *engine.Registry)   { engine.RegisterUnaryNumOp(r, "cos", math.Cos) }
func registerTan(r *engine.Registry)   { engine.RegisterUnaryNumOp(r, "tan", math.Tan) }
func registerAsin(r *engine.Registry)  { engine.RegisterUnaryNumOp(r, "asin", math.Asin) }
func registerAcos(r *engine.Registry)  { engine.RegisterUnaryNumOp(r, "acos", math.Acos) }
func registerAtan(r *engine.Registry)  { engine.RegisterUnaryNumOp(r, "atan", math.Atan) }

func registerAtan2(r *engine.Registry) {
	engine.RegisterBinaryNumOp(r, "atan2", func(a, b float64) (float64, error) {
		return math.Atan2(b, a), nil
	})
	r.Register("atan2", engine.Signature{
		Args: []engine.Type{engine.TInteger, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewDecimal(math.Atan2(float64(args[1].AsInteger()), float64(args[0].AsInteger())))}, nil
		},
	})
}

func registerHypot(r *engine.Registry) {
	engine.RegisterBinaryNumOp(r, "hypot", func(a, b float64) (float64, error) {
		return math.Hypot(a, b), nil
	})
	r.Register("hypot", engine.Signature{
		Args: []engine.Type{engine.TInteger, engine.TInteger},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewDecimal(math.Hypot(float64(args[0].AsInteger()), float64(args[1].AsInteger())))}, nil
		},
	})
}

func registerMathConstants(r *engine.Registry) {
	r.RegisterStackOnly("math-pi", engine.Signature{
		Args: []engine.Type{},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewDecimal(math.Pi)}, nil
		},
	})
	r.RegisterStackOnly("math-e", engine.Signature{
		Args: []engine.Type{},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewDecimal(math.E)}, nil
		},
	})
}
