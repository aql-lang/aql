package engine

func registerNand(r *Registry) {
	registerBinaryBoolOp(r, "nand", 2, func(a, b bool) bool { return !(a && b) })
}
