package engine

func registerAnd(r *Registry) {
	registerBinaryBoolWord(r, "and", func(a, b bool) bool { return a && b })
}
