package engine

func RegisterAnd(r *Registry) {
	registerBinaryBoolWord(r, "and", func(a, b bool) bool { return a && b })
}
