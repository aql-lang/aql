package engine

import "math"

func registerTan(r *Registry) {
	registerUnaryNumOp(r, "tan", func(x float64) float64 { return math.Tan(x) })
}
