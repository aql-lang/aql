package engine

func registerNand(r *Registry) {
	registerBinaryBoolWord(r, "nand", func(a, b bool) bool { return !(a && b) })
}
