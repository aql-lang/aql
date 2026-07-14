package eng

// polyNoMatchRaise executes a poly's FAITHFUL no-match raise (plan 3c): when
// the record carried a PolyNoMatchSpec — the check pass proved, at the failed-
// dispatch tape state it recovered from, that the interpreter's sigError
// diagnostic is rebuildable from the runtime operand window — the VM raises
// the byte-identical signature_error here instead of deferring the whole run
// to the interpreter. Rebuild: the spec's window indices reproduce sigError's
// two tape tuples (the WRITTEN tuple its notes render, and the SECONDARY
// stack-prefix tuple its reorder probe falls back to); the diagnostic itself
// is then the same shared builder over the same inputs (noMatchDiag +
// reorderHintFor — both pure functions of the word, its live table, and the
// tuples). Returns nil — keeping the caller's sound defer — when no spec was
// recorded or a runtime drift guard trips: a signature table whose length
// changed since the record (the record-time arity screen no longer holds), a
// NARROWER-arity overload now present (its collection could match where this
// raise claims nothing can), or an index outside the window.
func (vc *vmContext) polyNoMatchRaise(r *Registry, pr *PolyRef, fn *FnDefInfo, window []Value, curDebug []SrcPos, pc int) error {
	spec := pr.NoMatch
	if spec == nil || fn == nil || len(fn.Signatures) != spec.NSigs {
		return nil
	}
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if !s.Fallback && s.TotalArgs() < pr.Arity {
			return nil
		}
	}
	written, ok := tupleAt(window, spec.Written)
	if !ok {
		return nil
	}
	stackTuple, ok := tupleAt(window, spec.StackTuple)
	if !ok {
		return nil
	}
	// sigError's two-probe cascade: the value-based reorder probe over the
	// written tuple first, the stack-prefix tuple second (engine.go:sigError).
	reorder := reorderHintFor(pr.Word, fn, written)
	if reorder == "" {
		reorder = reorderHintFor(pr.Word, fn, stackTuple)
	}
	ae := noMatchDiag(r.Source, pr.Word, fn, written, spec.Pos, reorder)
	return stampAt(ae, curDebug, pc, r)
}

// tupleAt rebuilds a recorded tape tuple from operand-window indices.
func tupleAt(window []Value, idx []int) ([]Value, bool) {
	vals := make([]Value, len(idx))
	for i, j := range idx {
		if j < 0 || j >= len(window) {
			return nil, false
		}
		vals[i] = window[j]
	}
	return vals, true
}
