package engine

func RegisterXor(r *Registry) {
	registerBinaryBoolWord(r, "xor", func(a, b bool) bool { return a != b })
}
