package engine

func RegisterIff(r *Registry) {
	// iff (logical biconditional / equivalence): true when both args
	// have the same truth value.
	registerBinaryBoolWord(r, "iff", func(a, b bool) bool { return a == b })
}
