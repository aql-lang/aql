package engine

func registerAnd(r *Registry) {
	registerBinaryBoolOp(r, "and", func(a, b bool) bool { return a && b })
}
