package engine

func RegisterNand(r *Registry) {
	registerBinaryBoolWord(r, "nand", func(a, b bool) bool { return !(a && b) })
}
