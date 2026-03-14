package engine

import "math"

func registerLog2(r *Registry) {
	registerUnaryNumOp(r, "log2", func(x float64) float64 { return math.Log2(x) })
}
