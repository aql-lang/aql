package engine

// defName extracts a word name from a Value that is either a word or a
// string. Used by def, undef, type, untype handlers.
func defName(v Value) string {
	if v.IsWord() {
		_as0, _ := v.AsWord()
		return _as0.Name
	}
	_as1, _ := v.AsString()
	return _as1
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
