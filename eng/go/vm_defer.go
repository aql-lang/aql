package eng

// The VM piece's interpreter-facing entry helpers: the designed
// defer-to-interpreter choke points (vmDefer / vmDeferAlt) and the
// fallback-island runner with the flow-escape contract. Regrouped in
// Stage 1b of the four-piece split (design/ENG-FOUR-PIECE.0.md) — the
// only callers are vm*.go.

// vmDefer builds a designed defer-to-interpreter error (the internal_error
// class runtimeShouldFallback resolves by re-running) AND reports it to the
// runtime-bail hook — the single choke point every reachable designed-defer
// site in the VM routes through, so the executed bail census sees each defer
// exactly once with a stable site tag.
func vmDefer(r *Registry, curDebug []SrcPos, pc int, site, msg string) error {
	r.noteBail(site, msg)
	return vmErrAt(curDebug, pc, msg)
}

// vmDeferAlt is vmDefer carrying a best-effort user-facing raise for the
// fence-blocked arm (BoruError.DeferAlt): when the interpreter re-run this
// defer requests is blocked by the effect fence, the caller surfaces alt —
// the rich diagnostic the defer site built over its live values — instead of
// the internal error. A nil alt is plain vmDefer.
func vmDeferAlt(r *Registry, curDebug []SrcPos, pc int, site, msg string, alt *BoruError) error {
	err := vmDefer(r, curDebug, pc, site, msg)
	if alt == nil {
		return err
	}
	ae, ok := err.(*BoruError)
	if !ok { //covergate:allow vmErrAt always builds a *BoruError (§compiler)
		return err
	}
	ae.DeferAlt = alt
	return ae
}

// runIslandResolved is RunResolved with the island flow-escape contract
// (Engine.flowUnwind): a break/continue escaping the run tears down the
// island's live frames and returns no values, leaving the registry FlowCtrl
// flag set for the VM to translate (escapedFlow).
func runIslandResolved(r *Registry, inputs, tokens []Value) ([]Value, error) {
	input := make([]Value, len(inputs)+len(tokens))
	copy(input, inputs)
	copy(input[len(inputs):], tokens)
	e := r.takeSubEngine()
	e.isTop = false
	e.startAt = len(inputs)
	e.flowUnwind = true
	res, err := e.Run(input)
	if len(res) > 0 {
		res = append([]Value(nil), res...)
	}
	e.isTop = false
	e.startAt = 0
	e.flowUnwind = false
	r.putSubEngine(e)
	return res, err
}
