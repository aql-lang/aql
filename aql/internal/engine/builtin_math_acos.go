package engine

import "math"

func registerAcos(r *Registry) {
	registerUnaryNumOp(r, "acos", func(x float64) float64 { return math.Acos(x) })
}
