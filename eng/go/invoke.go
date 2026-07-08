package eng

// InvokeBody executes a code BODY against per-call inputs and returns the
// residual value stack (bottom→top) — exactly as the body-running native
// handlers did when they spun up `New(r).Run([inputs… bodyTokens…])`.
//
// This is the single seam through which every higher-order / code-body word
// (each/fold/scan/do/filter/case/where/group/having/order/outer/inner/select)
// runs its body. It exists so the bytecode VM can drive body execution
// WITHOUT re-entering the interpreter: when r.Invoker is set (the VM is
// running), the body is a compiled closure and execution re-enters the VM;
// when it is nil (a plain interpreter run) a fresh sub-engine runs the
// reconstructed token stream, so behaviour is byte-identical to the pre-seam
// handlers (design plan P1).
//
// inputs are spliced BEFORE the body tokens, matching the handlers' historical
// `input[0..k-1]=inputs; input[k..]=bodyTokens` layout, so the body sees its
// per-call values exactly where the original code placed them.
func InvokeBody(r *Registry, body Value, inputs []Value) ([]Value, error) {
	if r.Invoker != nil {
		return r.Invoker(r, body, inputs)
	}
	// Pooled + resolved: the engine and its tape are reused across
	// invocations (runPooledSub / the registry sub-engine pool), and the
	// inputs enter as RESOLVED stack data rather than being re-stepped —
	// arguments are inert (design/ARG-SEMANTICS-UNIFICATION.0.md, via
	// RunResolved's start offset).
	return RunResolved(r, inputs, bodyTokens(body))
}

// runPooledSub runs input on a pooled reusable sub-engine and returns a
// caller-owned COPY of the results. It is the shared seam behind every
// per-element sub-evaluation (higher-order bodies, list/paren/interp-hole
// auto-evaluation): pooling means a hot loop reloads one tape in place
// instead of allocating a fresh ~DefaultTapeInitialFloor-entry tape per
// invocation. The result copy is mandatory — Engine.Run's result slice
// aliases the engine's tape (Tape.TakeAll), which the pool's next Reload
// overwrites — and it also stops a retained result from pinning the whole
// tape buffer the way the previous spin-up-per-call path did.
//
// elemEvalRecordable configures the sub-engine's container-element
// recording flag (see Engine.elemEvalRecordable); pass false when the
// caller does not evaluate recordable container elements.
func runPooledSub(r *Registry, input []Value, elemEvalRecordable bool) ([]Value, error) {
	sub := r.takeSubEngine()
	sub.elemEvalRecordable = elemEvalRecordable
	res, err := sub.Run(input)
	if err != nil {
		r.putSubEngine(sub)
		return nil, err
	}
	out := make([]Value, len(res))
	copy(out, res)
	r.putSubEngine(sub)
	return out, nil
}

// bodyTokens returns the executable token sequence for a code body: a concrete
// list's elements (the common case — `[mul 2]`), or the value itself wrapped
// as a singleton for a non-list body. Mirrors what the handlers extracted via
// `AsList(body).Slice()` before splicing.
func bodyTokens(body Value) []Value {
	if lst, err := AsList(body); err == nil && !lst.IsNil() {
		return lst.Slice()
	}
	return []Value{body}
}
