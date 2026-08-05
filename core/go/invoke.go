package core

import "errors"

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
	//
	// A code-body word (each/fold/do/…) does NOT strip a dispatch ascription
	// from its result: it is inline value-routing (design/OPEN-WORDS.1.md
	// §9), transparent like a paren group or an if-branch, so the ascription
	// flows to the consuming dispatch — which matches the compiled path,
	// where `as` folds at compile time and the ascription rides the static
	// value flow to that same dispatch. (Only a fn/lambda/module return, an
	// abstraction boundary with a declared signature, strips.)
	return RunResolved(r, inputs, BodyTokens(body))
}

// InvokeCallback runs a runtime fn VALUE (given its matched signature and the
// per-call args) against the VM when the sig carries a compiled unit whose
// program is stamped AND r can host a fresh run, else falling back to CallBoru —
// the tree-walking interpreter — with the fn's captures. It is the single seam
// every native callback word (serve-raw, spawn, service/codec endpoints)
// dispatches through, so retiring the interpreter for reducible callback bodies
// is one routing decision rather than an edit per word.
//
// Correctness is fail-safe: a nil CompiledRef, an un-stamped ref (a body the
// compiler refused, or a run that never reached Finalize), or a busy registry
// all fall to CallBoru, whose values and error taxonomy are unchanged. When the
// VM path IS taken, RunUnit executes the exact unit the differential gates prove
// equivalent to the interpreter.
//
// A callback fires AFTER the enclosing RunProgram returned (serve-raw handling a
// connection, a spawned process), so — unlike an in-program island — there is no
// outer RunCompiled to catch a VM bailout and re-run on the interpreter. This seam
// therefore applies RunCompiled's own discipline itself: if the compiled unit
// raises an internal_error (a VM/lowering soundness assertion or a recovered
// handler panic, the class RunCompiled resolves by falling back), retry on
// CallBoru so the peer sees the interpreter's canonical result/error instead of a
// raw internal_error leaking to the network. Genuine boru runtime errors
// (signature_error, type_error, …) are the interpreter's answer too and are
// returned as-is.
func InvokeCallback(r *Registry, sig *Signature, args []Value, captures []CapturedBinding) ([]Value, error) {
	// sig is the matched signature (callers dispatch it via MatchFnSig and check
	// it non-nil first — serve-raw's handler dispatch is the canonical caller).
	// depsFresh guards RUNTIME-stamped refs (StampDetachedFn): a module dep
	// rebound or shadowed since the stamp means the frozen unit would diverge
	// from live resolution. A stale DETACHED ref first tries the JIT re-stamp
	// (REFUSAL-CLOSURE.0 §7c) — recompile against the live bindings, bounded
	// by the ref's try budget — and only a declined re-stamp takes the
	// interpreter, where the seam previously degraded permanently.
	// The compiled fast path lives behind the CompiledRuntime seam
	// (compiled_runtime.go, Stage 1 of the four-piece split): the VM
	// piece owns ref freshness, the JIT re-stamp, the C1 effect fence,
	// and the internal-error degrade decision. ran=false — including a
	// bailed unit with no observable effect — leaves the interpreter
	// path to the code below.
	if res, err, ran := compiledRuntime.InvokeCompiled(r, sig, args); ran {
		return res, err
	}
	// Observability seam (interp_entry.go): the callback seam's interpreter
	// fallback — its own name so the C4 decline tag can attach later without
	// conflating it with a direct CallBoru.
	r.noteInterp("InvokeCallback:callboru")
	return r.CallBoru(sig, args, captures)
}

// isInternalErr reports whether err is an internal_error-class BoruError — the
// signal runVMEntry stamps on a recovered VM panic / lowering soundness bailout,
// and the exact class RunCompiled resolves by re-running on the interpreter. The
// callback seam mirrors that decision so a post-program bailout degrades to the
// interpreter rather than surfacing a raw compiler bug to a network peer.
func IsInternalErr(err error) bool {
	var ae *BoruError
	return errors.As(err, &ae) && ae.Code == "internal_error"
}

// IsInternalError is the exported face of isInternalErr, for run-side seams
// outside eng that take the same degrade-to-interpreter decision on a stamped
// unit's result (await's per-branch fork runs — native_temporal_await.go).
func IsInternalError(err error) bool { return IsInternalErr(err) }

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
func RunPooledSub(r *Registry, input []Value, elemEvalRecordable bool) ([]Value, error) {
	// Observability seam (interp_entry.go): the per-element sub-evaluation path.
	r.noteInterp("runPooledSub")
	sub := r.TakeSubEngine()
	sub.ElemEvalRecordable = elemEvalRecordable
	res, err := sub.Run(input)
	if err != nil {
		r.PutSubEngine(sub)
		return nil, err
	}
	out := make([]Value, len(res))
	copy(out, res)
	r.PutSubEngine(sub)
	return out, nil
}

// bodyTokens returns the executable token sequence for a code body: a concrete
// list's elements (the common case — `[mul 2]`), or the value itself wrapped
// as a singleton for a non-list body. Mirrors what the handlers extracted via
// `AsList(body).Slice()` before splicing.
func BodyTokens(body Value) []Value {
	if lst, err := AsList(body); err == nil && !lst.IsNil() {
		return lst.Slice()
	}
	return []Value{body}
}
