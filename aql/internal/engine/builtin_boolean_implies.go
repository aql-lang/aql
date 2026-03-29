package engine

func registerImplies(r *Registry) {
	registerBinaryBoolOp(r, "implies", func(a, b bool) bool { return !a || b })
}
