package engine

func registerAnd(r *Registry) {
	registerBinaryBoolOp(r, "and", 2, func(a, b bool) bool { return a && b })
}
