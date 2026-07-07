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
		return r.Invoker(body, inputs)
	}
	toks := bodyTokens(body)
	input := make([]Value, len(inputs)+len(toks))
	copy(input, inputs)
	copy(input[len(inputs):], toks)
	// Pooled: identical semantics to New(r).Run(input), but the engine and
	// its tape are reused across invocations — the fresh-spawn tape floor
	// (~164KB zeroed per call) dominated per-element cost in hot loops.
	// See engine_pool.go.
	return RunPooled(r, input)
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
