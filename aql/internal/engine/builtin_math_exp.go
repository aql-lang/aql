package engine

import "math"

func registerExp(r *Registry) {
	registerUnaryNumOp(r, "exp", func(x float64) float64 { return math.Exp(x) })
}
