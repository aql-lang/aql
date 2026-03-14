package engine

import "math"

func registerLog10(r *Registry) {
	registerUnaryNumOp(r, "log10", func(x float64) float64 { return math.Log10(x) })
}
