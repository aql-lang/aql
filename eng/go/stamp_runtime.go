package eng

// Runtime (detached) fn-unit stamping — design/RUNTIME-STAMPING.0.md.
//
// The compile-time store-fn bake (recordCallOperands → compileStoredFnUnit →
// stampCompiledRef) stamps a CompiledFnRef only onto a fn value that is a
// concrete const in a CompileStoresFn slot of the program CURRENTLY being
// compiled. A fn value constructed at runtime — a service handler lambda
// added inside an interpreted module-fn body, a custom codec fn resolved
// from a codec map — is never visible to any compile pass, so its sig
// carries no ref and InvokeCallback permanently falls back to CallAQL.
//
// StampDetachedFn closes that gap: it compiles such a body to a standalone
// one-unit *Program OUTSIDE any whole-program pass, on an isolated fork of
// the live registry, and returns a ref the existing InvokeCallback seam runs
// via RunUnit / runUnitNested with zero changes to its happy path. Refusal
// is silent and per-body — the caller keeps the plain value and the
// interpreter behaviour is byte-identical (slow, never wrong).
//
// Freshness: a detached ref outlives the compile fork, so the compile-time
// NotifyNameRebound poisoning cannot observe later rebinds of the module
// names its body reads. Instead the ref carries a depSnap — per dep name,
// the DefTable shadow depth and mutation generation at stamp time — which
// InvokeCallback validates on every invoke (CompiledFnRef.depsFresh); any
// mismatch degrades to CallAQL, which resolves the live binding exactly as
// the interpreter would.

// StampDetachedFn compiles a runtime-constructed, capture-free fn VALUE's
// single own-sig body to a standalone one-unit Program and returns its
// CompiledFnRef. It runs only when runtime stamping is armed on r
// (EnableRuntimeStamping — the compiled execution entry points). The compile
// is fully isolated: it runs on a ForkConcurrent copy of r carrying a FRESH
// CheckState (Registry.Check is a shared pointer the fork's shallow copy
// would otherwise alias — the compile pass must not touch the parent's live
// check state). Refusal returns (nil, false) and leaves r untouched.
//
// The caller contract is ForkConcurrent's: invoke from the goroutine that
// owns r (store words and codec resolution run on the registry executing
// them, so this holds at every trigger site).
func StampDetachedFn(r *Registry, fd FnDefInfo, pos SrcPos) (*CompiledFnRef, bool) {
	if r == nil || !r.RuntimeStampingEnabled() {
		return nil, false
	}
	if len(fd.Captured) > 0 {
		// A capturing fn needs its captures resolved to compiled homes in the
		// constructing scope — the OpPushClosure path's territory, not a
		// detached unit's. Deferred; the interpreter handles it unchanged.
		return nil, false
	}
	if _, ok := storedFnUnitEligible(fd); !ok {
		return nil, false
	}
	fork := r.ForkConcurrent()
	// Fresh check state: the fork's shallow copy aliases r.Check (a shared
	// *CheckState); arming a compile pass on the alias would trash the
	// parent's live diagnostics/emit state. Mirror NewRegistry's init
	// (StepBudget sentinel -1, inactive recorder).
	fork.Check = &CheckState{StepBudget: -1, Emit: theInactiveEmit}
	defer fork.Check.BeginCompilePass()()
	// BeginCompilePass installs a concrete *EmitState; the two-value cast
	// (never-failing here) keeps this panic-free without an unreachable
	// guard branch — every EmitState method below is nil-receiver-safe and
	// compileStoredFnUnit declines on a nil state.
	es, _ := fork.Check.Recorder().(*EmitState)
	es.bindRegistry(fork)
	if es != nil {
		// Detached compiles run in GRADUAL-Any nesting mode: an Any arg
		// flowing into a nested callee's Any param binds a gradual carrier
		// (see EmitState.storedGradualDepth), so a handler calling an
		// `st:Any` helper that reads `st.kv` compiles instead of refusing
		// on the first field access. Safe here and only here — this fork
		// owns its program and Finalize, so any gradual-caused failure is
		// one silently declined stamp.
		es.storedGradualDepth = 1
	}
	// Deps are read off the PRE-analysis def table (the fork clone equals r
	// here), so the snapshot below describes exactly what the body resolved
	// against — the analysis inside compileStoredFnUnit installs and restores
	// its own body-local bindings.
	deps := es.storedFnDeps(fd)
	unit, ok := es.compileStoredFnUnit(fd, pos)
	if !ok {
		return nil, false
	}
	ref := &CompiledFnRef{Unit: unit, depNames: deps}
	es.storedFnRefs = append(es.storedFnRefs, ref)
	prog, _, ok := es.Finalize(nil)
	if !ok || prog == nil || ref.Prog == nil {
		// Finalize refused (the real compile marked the state uncompilable)
		// or left the ref unstamped — the value stays plain and interprets.
		return nil, false
	}
	if len(deps) > 0 {
		ref.depSnap = make(map[string]depSnapEntry, len(deps))
		for name := range deps {
			ref.depSnap[name] = depSnapEntry{Depth: r.Defs.Depth(name), Gen: r.Defs.Gen(name)}
		}
	} else {
		// A body with no module deps still marks itself runtime-stamped with
		// an empty (non-nil) snapshot: vacuously fresh forever, but the field
		// keeps "detached ref" distinguishable from "compile-time ref".
		ref.depSnap = map[string]depSnapEntry{}
	}
	return ref, true
}

// StampFnValue is the value-level entry over StampDetachedFn: given a fn
// VALUE (an FnDefInfo payload), it compiles the body and returns a CLONE of
// the value whose own AQL sig carries the ref. The clone is deliberate — the
// input may be a published, shared value (a module binding, an interned
// const), and mutating its shared *AQLImpl from a store word would race
// concurrent readers; the compile-time stampCompiledRef mutates only
// pre-publication interned consts. On any decline (not a fn value, already
// stamped, capturing, ineligible shape, refusing body, policy off) it
// returns the input unchanged with ok=false, so callers may use the returned
// value unconditionally.
func StampFnValue(r *Registry, v Value) (Value, bool) {
	fd, ok := v.Data.(FnDefInfo)
	if !ok {
		return v, false
	}
	// Locate the stamp target — the first own (non-fallback) AQL body sig,
	// mirroring stampCompiledRef's choice — while refusing an already-stamped
	// value (first stamp wins: a compile-time stamp or an earlier detached
	// one already carries the VM edge).
	target, aImpl := -1, (*AQLImpl)(nil)
	for i := range fd.Signatures {
		if fd.Signatures[i].CompiledRef() != nil {
			return v, false
		}
		if target < 0 && !fd.Signatures[i].Fallback {
			if a, isAQL := fd.Signatures[i].Impl.(*AQLImpl); isAQL {
				target, aImpl = i, a
			}
		}
	}
	if target < 0 {
		// A Go-backed or fallback-only fn value (a built-in codec) — nothing
		// to stamp; it already dispatches natively.
		return v, false
	}
	ref, ok := StampDetachedFn(r, fd, v.Pos())
	if !ok {
		return v, false
	}
	// Clone the sig slice and the target impl so the stamp never writes
	// through a shared pointer of a published value.
	sigs := make([]Signature, len(fd.Signatures))
	copy(sigs, fd.Signatures)
	na := *aImpl
	na.Compiled = ref
	sigs[target].Impl = &na
	fd.Signatures = sigs
	out := v
	out.Data = fd
	return out, true
}

// StampFnValueInPlace is StampFnValue for PRE-PUBLICATION values: it stamps
// the ref onto the value's own shared *AQLImpl (stampCompiledRef) instead of
// cloning. Callers must guarantee the value has not escaped to concurrent
// readers — the one sanctioned site is module load (RunModuleBody), where the
// module's def bindings and its export map share impl pointers and both must
// see the stamp, and nothing outside the loading goroutine holds the value
// yet. Returns false on any decline, leaving the value untouched.
func StampFnValueInPlace(r *Registry, v Value) bool {
	fd, ok := v.Data.(FnDefInfo)
	if !ok {
		return false
	}
	for i := range fd.Signatures {
		if fd.Signatures[i].CompiledRef() != nil {
			return false
		}
	}
	ref, ok := StampDetachedFn(r, fd, v.Pos())
	if !ok {
		return false
	}
	return stampCompiledRef(fd, ref)
}
