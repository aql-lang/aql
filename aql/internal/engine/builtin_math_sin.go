package engine

import "math"

func registerSin(r *Registry) {
	registerUnaryNumOp(r, "sin", func(x float64) float64 { return math.Sin(x) })
}
