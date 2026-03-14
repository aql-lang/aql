package engine

import "math"

func registerSqrt(r *Registry) {
	registerUnaryNumOp(r, "sqrt", func(x float64) float64 { return math.Sqrt(x) })
}
