package eng

// vmCompiledRuntime is the bytecode runner's CompiledRuntime — the real
// implementation of core's S4 seam (design/ENG-FOUR-PIECE.0.md). It
// owns the stamped-ref freshness/JIT-re-stamp dance, the C1 effect
// fence around the attempt, and the internal-error degrade decision;
// core's InvokeCallback sees only ran/declined.
type vmCompiledRuntime struct{}

func init() { InstallCompiledRuntime(vmCompiledRuntime{}) }

func (vmCompiledRuntime) InvokeCompiled(r *Registry, sig *Signature, args []Value) ([]Value, error, bool) {
	ref := CompiledRef(sig)
	if ref != nil && ref.Prog != nil && !ref.depsFresh(r) {
		ref = ref.jitRestamp(r)
	}
	if ref == nil || ref.Prog == nil {
		return nil, nil, false
	}
	// The writer fence is armed around the attempt because a DETACHED
	// callback fires after the enclosing compiled run disarmed its own
	// fence: without the wrap, a callback that PRINTS and then bails
	// would leave the ledger untouched and the interpreter retry would
	// emit the output a second time. Nested invocations are already
	// armed; the second wrap only double-counts, and the fence reads
	// deltas, not magnitudes.
	disarm := r.ArmEffectFence()
	effectsAt := r.Effects.Count()
	res, err, ran := invokeCompiledUnit(r, ref, args)
	disarm()
	if !ran {
		return nil, nil, false
	}
	if !IsInternalErr(err) {
		return res, err, true
	}
	// C1 effect fence (effects.go): the interpreter retry re-runs the
	// whole body, so it is sound only while the failed unit emitted NO
	// observable effect — a callback that wrote to the peer and THEN
	// bailed must surface the internal_error rather than double its
	// output.
	if r.Effects.Count() != effectsAt {
		return nil, err, true
	}
	return nil, nil, false
}

func (vmCompiledRuntime) StampDetached(r *Registry, fd FnDefInfo, pos SrcPos) {
	// fd arrives with the stamp-event name applied by the caller; the
	// ref lands on the shared *BoruImpl pointer (stampCompiledRef), so
	// the same copy serves both calls.
	if ref, stampOK := StampDetachedFn(r, fd, pos); stampOK {
		stampCompiledRef(fd, ref)
	}
}
