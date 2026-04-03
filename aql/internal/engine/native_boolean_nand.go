package engine

func registerNand(r *Registry) {
	registerBinaryBoolOp(r, "nand", func(a, b bool) bool { return !(a && b) })
}
