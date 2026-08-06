package eng

// Runtime (detached) fn-unit stamping — design/RUNTIME-STAMPING.0.md.
//
// The compile-time store-fn bake (recordCallOperands → compileStoredFnUnit →
// stampCompiledRef) stamps a CompiledFnRef only onto a fn value that is a
// concrete const in a CompileStoresFn slot of the program CURRENTLY being
// compiled. A fn value constructed at runtime — a service handler lambda
// added inside an interpreted module-fn body, a custom codec fn resolved
// from a codec map — is never visible to any compile pass, so its sig
// carries no ref and InvokeCallback permanently falls back to CallBoru.
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
// mismatch degrades to CallBoru, which resolves the live binding exactly as
// the interpreter would.

// StampDetachedFn compiles a runtime-constructed, capture-free fn VALUE's
// FIRST stampable own-sig body to a standalone one-unit Program and returns
// its CompiledFnRef — the single-sig entry the predicate-type constructor
// and the module-load sweep use (their values are single-overload by
// construction). Multi-overload values stamp EVERY own sig through the
// value-level loops (StampFnValue / StampFnValueInPlace →
// StampDetachedSig, REFUSAL-CLOSURE §7b).
func StampDetachedFn(r *Registry, fd FnDefInfo, pos SrcPos) (*CompiledFnRef, bool) {
	si, ok := firstStampableSig(fd)
	if !ok {
		// Not recorded: native-word bindings swept by the module-load loop
		// land here — a shape that was never a compile candidate is report
		// noise, not an attempt.
		return nil, false
	}
	return StampDetachedSig(r, fd, si, pos)
}

// StampDetachedSig is the per-signature detached stamp: it compiles
// fd.Signatures[sigIdx]'s body to a standalone one-unit Program and returns
// its CompiledFnRef. It runs only when runtime stamping is armed on r
// (EnableRuntimeStamping — the compiled execution entry points). The compile
// is fully isolated: it runs on a ForkConcurrent copy of r carrying a FRESH
// CheckState (Registry.Check is a shared pointer the fork's shallow copy
// would otherwise alias — the compile pass must not touch the parent's live
// check state). Refusal returns (nil, false) and leaves r untouched.
//
// The caller contract is ForkConcurrent's: invoke from the goroutine that
// owns r (store words and codec resolution run on the registry executing
// them, so this holds at every trigger site).
func StampDetachedSig(r *Registry, fd FnDefInfo, sigIdx int, pos SrcPos) (*CompiledFnRef, bool) {
	if r == nil || !r.RuntimeStampingEnabled() {
		return nil, false
	}
	if sigIdx < 0 || sigIdx >= len(fd.Signatures) || !storedSigEligible(&fd.Signatures[sigIdx]) {
		return nil, false
	}
	fork := r.ForkConcurrent()
	// Fresh check state: the fork's shallow copy aliases r.Check (a shared
	// *CheckState); arming a compile pass on the alias would trash the
	// parent's live diagnostics/emit state. Mirror NewRegistry's init
	// (StepBudget sentinel -1, inactive recorder).
	fork.Check = &CheckState{StepBudget: -1, Emit: TheInactiveEmit}
	defer fork.Check.BeginCompilePass()()
	// BeginCompilePass installs a concrete *EmitState; the two-value cast
	// (never-failing here) keeps this panic-free without an unreachable
	// guard branch — every EmitState method below is nil-receiver-safe and
	// compileStoredFnUnit declines on a nil state.
	es, _ := fork.Check.Recorder().(*EmitState)
	es.BindRegistry(fork)
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
	// An identity-less capture value (minted at pure runtime, where the
	// mode-gated ID elision skips minting) cannot key its positional capture
	// slot, so StartFnCompile's identity gate would refuse the unit
	// (REFUSAL-CLOSURE.0 §7a). For a DETACHED unit the capture is per-ref
	// and FROZEN — ref.Captures carries the construction-time snapshot — so
	// minting a fresh identity on a CLONE of the captured slice is confined
	// to this unit's compile: body reads resolve to the slot by the minted
	// ID exactly like an ID-bearing capture, and no shared published value
	// is mutated (the input's Captured backing array stays untouched). The
	// compile pass armed above keeps GenerateID live.
	for i := range fd.Captured {
		if fd.Captured[i].Value.ID == "" {
			cloned := append([]CapturedBinding(nil), fd.Captured...)
			for j := range cloned {
				if cloned[j].Value.ID == "" {
					cloned[j].Value.ID = GenerateID(IDPrefixForType(cloned[j].Value.Parent))
				}
			}
			fd.Captured = cloned
			break
		}
	}
	// Deps are read off the PRE-analysis def table (the fork clone equals r
	// here), so the snapshot below describes exactly what the body resolved
	// against — the analysis inside compileStoredFnUnit installs and restores
	// its own body-local bindings.
	deps := es.storedHandlerDeps(fd.Signatures[sigIdx].Body())
	unit, ok := es.compileStoredFnUnit(fd, sigIdx, pos)
	if !ok {
		// The probe's latched reason when it gave one; the report printer
		// substitutes a generic text for an empty reason (a refusal path
		// that never reached MarkUncompilable).
		r.RecordStampEvent(StampEvent{Name: fd.Name, Pos: pos, Reason: es.storedFnProbeReason})
		return nil, false
	}
	ref := &CompiledFnRef{Unit: unit, depNames: deps}
	// A capturing body compiled with its captures as trailing param slots
	// (compileStoredFnUnit passes fd.Captured to the closure compile — the
	// same slot layout OpPushClosure uses); the ref carries the captured
	// VALUES so RunUnit / runUnitNested bind them at every invoke. The
	// values are the construction-time snapshots the interpreter's dispatch
	// installs — frozen in fd.Captured, name-sorted, so slot order is
	// deterministic.
	for _, cb := range fd.Captured {
		ref.Captures = append(ref.Captures, cb.Value)
	}
	es.storedFnRefs = append(es.storedFnRefs, ref)
	prog, _, ok := es.Finalize(nil)
	if !ok || prog == nil || ref.Prog == nil {
		// REACHABLE, sound per-body fallback. compileStoredFnUnit's probe/real
		// passes only RECORD this body's events (and any nested fn as a
		// sub-unit); they do not LOWER the recorded units. Finalize does — its
		// per-unit lowering loop (emit.go, "fn <name>: " + reason) can refuse a
		// SUB-UNIT that the outer body's compile accepted. A runtime-constructed
		// fn whose body defines a nested fn that consumes a loop result (Stage-2
		// boundary: "consumes loop results") is the concrete case — the outer
		// compileStoredFnUnit returns ok, then Finalize declines on the inner
		// unit. This is the same "the real pass CAN decline after a clean probe"
		// shape compileStoredParamBody documents; the value stays plain and
		// interprets, byte-identically. (Graduated from //covergate:allow: the
		// variation sweep's module-body transform over a sift row reaches it —
		// design/COVERAGE-ALLOWLIST.10.md graduation.)
		r.RecordStampEvent(StampEvent{Name: fd.Name, Pos: pos, Reason: "finalize left the unit unstamped"})
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
	// Arm the JIT re-stamp box (REFUSAL-CLOSURE.0 §7c): the stamp inputs ride
	// the ref so a later dep rebind re-compiles against the live bindings at
	// invoke time (jitRestamp) instead of degrading permanently to CallBoru.
	// fd here carries the §7a identity-minted capture clone, so a re-stamp
	// needs no re-clone.
	ref.restamp = &restampBox{fd: fd, sigIdx: sigIdx, pos: pos}
	r.RecordStampEvent(StampEvent{Name: fd.Name, Pos: pos, Stamped: true})
	return ref, true
}

// restampMaxTries bounds the TOTAL re-compiles one detached ref may pay
// across its lifetime: a dep that keeps rebinding between invokes would
// otherwise cost a compile per invoke — after the budget the seam stays on
// CallBoru, which resolves the live binding exactly as the interpreter.
const restampMaxTries = 3

// jitRestamp is InvokeCallback's stale-ref recovery (REFUSAL-CLOSURE.0 §7c):
// when a detached ref's depSnap no longer matches the live def table, re-run
// StampDetachedFn against the CURRENT bindings and return the fresh twin —
// each re-stamp snapshots the new generations, so a stable rebind pays one
// compile and then runs on the VM again. Returns nil when the seam should
// take the interpreter instead: a compile-time ref (no box), an exhausted
// try budget, or a declined re-stamp (stamping disarmed, the body now
// refusing). The box mutex serialises concurrent invokers of one shared sig
// — the winner compiles, the rest reuse its twin; StampDetachedFn itself
// runs on the CALLER's registry per its ForkConcurrent contract.
func (ref *CompiledFnRef) JitRestamp(r *Registry) *CompiledFnRef {
	box := ref.restamp
	if box == nil {
		return nil
	}
	box.mu.Lock()
	defer box.mu.Unlock()
	if box.cur != nil && box.cur.DepsFresh(r) {
		return box.cur
	}
	if box.tries >= restampMaxTries {
		return nil
	}
	box.tries++
	nr, ok := StampDetachedSig(r, box.fd, box.sigIdx, box.pos)
	if !ok {
		return nil
	}
	box.cur = nr
	return nr
}

// StampFnValue is the value-level entry over StampDetachedFn: given a fn
// VALUE (an FnDefInfo payload), it compiles the body and returns a CLONE of
// the value whose own boru sig carries the ref. The clone is deliberate — the
// input may be a published, shared value (a module binding, an interned
// const), and mutating its shared *BoruImpl from a store word would race
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
	// Refuse an already-stamped value wholesale (first stamp wins: a
	// compile-time stamp or an earlier detached one already carries the VM
	// edge for the sigs it accepted; re-stamping is the §7c box's job).
	for i := range fd.Signatures {
		if CompiledRef(&fd.Signatures[i]) != nil {
			return v, false
		}
	}
	// EVERY stampable own sig compiles to its OWN unit and ref
	// (REFUSAL-CLOSURE §7b): the callback seam dispatches through
	// MatchFnSig, so the matched sig's Impl ref is the sig table. A sig
	// whose body declines stays plain and interprets — per-sig, fail-safe.
	// The sig slice clones once (and each stamped impl clones) so the stamp
	// never writes through a shared pointer of a published value.
	var sigs []Signature
	for i := range fd.Signatures {
		if !storedSigEligible(&fd.Signatures[i]) {
			continue
		}
		ref, ok := StampDetachedSig(r, fd, i, v.Pos())
		if !ok {
			continue
		}
		if sigs == nil {
			sigs = make([]Signature, len(fd.Signatures))
			copy(sigs, fd.Signatures)
		}
		na := *(sigs[i].Impl.(*BoruImpl))
		na.Compiled = ref
		sigs[i].Impl = &na
	}
	if sigs == nil {
		// Nothing stamped: a Go-backed / fallback-only value (a built-in
		// codec), or every eligible body declined.
		return v, false
	}
	fd.Signatures = sigs
	out := v
	out.Data = fd
	return out, true
}

// StampFnValueInPlace is StampFnValue for PRE-PUBLICATION values: it stamps
// the ref onto the value's own shared *BoruImpl (stampCompiledRef) instead of
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
		if CompiledRef(&fd.Signatures[i]) != nil {
			return false
		}
	}
	// Per-sig stamps onto the value's own shared impls (pre-publication —
	// see the doc above): every stampable own sig gets its own ref
	// (REFUSAL-CLOSURE §7b); a declining body leaves that sig plain.
	any := false
	for i := range fd.Signatures {
		if !storedSigEligible(&fd.Signatures[i]) {
			continue
		}
		ref, ok := StampDetachedSig(r, fd, i, v.Pos())
		if !ok {
			continue
		}
		fd.Signatures[i].Impl.(*BoruImpl).Compiled = ref
		any = true
	}
	return any
}
