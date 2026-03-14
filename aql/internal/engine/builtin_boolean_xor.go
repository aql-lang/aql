package engine

func registerXor(r *Registry) {
	registerBinaryBoolOp(r, "xor", 1, func(a, b bool) bool { return a != b })
}
