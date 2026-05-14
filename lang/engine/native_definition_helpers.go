package engine

// defName extracts a word name from a Value that is either a word,
// an atom, or a string. Used by def, undef, type, untype handlers.
// /q-marked sig positions deliver Atoms; bare-word slots deliver
// Words; quoted-string slots deliver Strings.
func defName(v Value) string {
	if v.IsWord() {
		_as0, _ := v.AsWord()
		return _as0.Name
	}
	if v.IsAtom() {
		s, _ := v.AsAtom()
		return s
	}
	s, _ := v.AsString()
	return s
}

// defStackOnly returns true if the name word carries the /s modifier,
// indicating the defined word should be stack-only (not forward
// precedence).
func defStackOnly(v Value) bool {
	if v.IsWord() {
		_as2, _ := v.AsWord()
		return _as2.ForceStack
	}
	return false
}
