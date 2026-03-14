package engine

func registerImplies(r *Registry) {
	registerBinaryBoolOp(r, "implies", 1, func(a, b bool) bool { return !a || b })
}
