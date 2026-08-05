package eng

// dispatchRematch executes OpDispatchRematch (see the opcode doc): a
// statically-failed dispatch whose window held carriers re-runs the match
// over the LIVE values (window[i] = sig position i, the callPoly layout)
// against the word's live registry binding. NO MATCH — the expected case,
// this compiled an ERROR row — raises the shared rich diagnostic built over
// the concrete values, byte-identical to the interpreter's sigError at the
// same point. A MATCH means the static model was wrong (a refined runtime
// tag, a satisfied predicate); the tail was truncated at this terminal op,
// so the run defers to the interpreter. Always returns a non-nil error.
func (vc *vmContext) dispatchRematch(ds *DispatchSpec, stack []Value, curDebug []SrcPos, pc int) error {
	r := vc.r
	if len(stack) < ds.NArgs {
		return vmErrAt(curDebug, pc, "DISPATCH_REMATCH underflow at "+ds.Word)
	}
	window := make([]Value, ds.NArgs)
	for i := 0; i < ds.NArgs; i++ {
		window[i] = stack[len(stack)-1-i]
	}
	var sigs []Signature
	if fn := r.Lookup(ds.Word); fn != nil {
		sigs = fn.Signatures
	}
	if mr := MatchSignature(sigs, window, WordInfo{ArgCount: -1}); mr != nil && mr.Sig != nil && !mr.Sig.Fallback {
		return vmDefer(r, curDebug, pc, "vm:rematch-matched",
			"DISPATCH_REMATCH at "+ds.Word+" matched at run time where the static model failed; deferring to the interpreter")
	}
	// The diagnostic renders over the RENDER BOUND (window[off:off+n]) — the
	// written tuple sigError's forward-else-stack derivation yields, proven a
	// contiguous slice of the window by ID at record time. The match above
	// ran over the FULL window (the view the failed static match examined).
	if ds.NWritten < 1 || ds.WrittenOff < 0 || ds.WrittenOff+ds.NWritten > len(window) {
		return vmErrAt(curDebug, pc, "DISPATCH_REMATCH written bound out of range at "+ds.Word)
	}
	ae := RuntimeNoMatch(r, ds.Word, window[ds.WrittenOff:ds.WrittenOff+ds.NWritten])
	ae.Row, ae.Col = ds.Pos.Row, ds.Pos.Col
	return stampAt(ae, curDebug, pc, r)
}
