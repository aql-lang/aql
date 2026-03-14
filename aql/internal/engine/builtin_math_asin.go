package engine

import "math"

func registerAsin(r *Registry) {
	registerUnaryNumOp(r, "asin", func(x float64) float64 { return math.Asin(x) })
}
