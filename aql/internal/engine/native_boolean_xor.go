package engine

func registerXor(r *Registry) {
	registerBinaryBoolOp(r, "xor", func(a, b bool) bool { return a != b })
}
