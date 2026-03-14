package engine

import "math"

func registerCbrt(r *Registry) {
	registerUnaryNumOp(r, "cbrt", func(x float64) float64 { return math.Cbrt(x) })
}
