package engine

import "math"

func registerCos(r *Registry) {
	registerUnaryNumOp(r, "cos", func(x float64) float64 { return math.Cos(x) })
}
