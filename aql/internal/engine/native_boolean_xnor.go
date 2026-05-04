package engine

func RegisterXnor(r *Registry) {
	// xnor (logical XNOR / equivalence): true when both args have the
	// same truth value. Synonym for `iff`.
	registerBinaryBoolWord(r, "xnor", func(a, b bool) bool { return a == b })
}
