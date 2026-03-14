package engine

import "math"

func registerAtan(r *Registry) {
	registerUnaryNumOp(r, "atan", func(x float64) float64 { return math.Atan(x) })
}
