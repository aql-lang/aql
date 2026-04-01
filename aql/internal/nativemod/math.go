package nativemod

import (
	"math"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// RegisterMath registers the extended math words provided by the "aql:math"
// native module. These include unary operations (abs, negate, sign), rounding
// (ceil, floor, round, trunc), roots and exponentials (sqrt, cbrt, exp, log,
// log2, log10), trigonometry (sin, cos, tan, asin, acos, atan, atan2, hypot),
// min/max, and constants (math-pi, math-e).
//
// The basic arithmetic operations (add, sub, mul, div, mod, pow) remain
// built-in and do not require this module.
func RegisterMath(r *engine.Registry) {
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

// --- Unary operations ---

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

// --- Min/Max ---

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

// --- Rounding ---

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

// --- Roots, exponentials, logarithms ---

func registerSqrt(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "sqrt", func(x float64) float64 { return math.Sqrt(x) })
}

func registerCbrt(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "cbrt", func(x float64) float64 { return math.Cbrt(x) })
}

func registerExp(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "exp", func(x float64) float64 { return math.Exp(x) })
}

func registerLog(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "log", func(x float64) float64 { return math.Log(x) })
}

func registerLog2(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "log2", func(x float64) float64 { return math.Log2(x) })
}

func registerLog10(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "log10", func(x float64) float64 { return math.Log10(x) })
}

// --- Trigonometry ---

func registerSin(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "sin", func(x float64) float64 { return math.Sin(x) })
}

func registerCos(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "cos", func(x float64) float64 { return math.Cos(x) })
}

func registerTan(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "tan", func(x float64) float64 { return math.Tan(x) })
}

func registerAsin(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "asin", func(x float64) float64 { return math.Asin(x) })
}

func registerAcos(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "acos", func(x float64) float64 { return math.Acos(x) })
}

func registerAtan(r *engine.Registry) {
	engine.RegisterUnaryNumOp(r, "atan", func(x float64) float64 { return math.Atan(x) })
}

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

// --- Constants ---

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
