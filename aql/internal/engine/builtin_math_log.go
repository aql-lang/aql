package engine

import "math"

func registerLog(r *Registry) {
	registerUnaryNumOp(r, "log", func(x float64) float64 { return math.Log(x) })
}
