package engine

func RegisterNor(r *Registry) {
	registerBinaryBoolWord(r, "nor", func(a, b bool) bool { return !(a || b) })
}
