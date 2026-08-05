package core

import "sync/atomic"

// Observability seams for the runtime-independence program
// (design/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md, C4 + Phase 10): two
// arm-able hooks that make silent engine decisions observable WITHOUT
// changing behaviour, the way the stamp log does for stamping decisions
// (stamp_report.go). Both are TEST SEAMS (design/TEST-SEAMS.10.md), not API:
// production code never arms them, an unarmed hook costs one atomic load at
// each emit point, and tests restore via the returned disarm func.
//
//   - The INTERPRETER-ENTRY hook observes every entry into tree-walking
//     machinery (Engine.Run, RunResolved, CallBoru, runPooledSub, and
//     InvokeCallback's CallBoru fallback). The frontier suite uses it to
//     assert "this program ran with no unattributed interpreter execution" —
//     the C4 end-state invariant — at entry points (REPL, exec), callback
//     seams, and post-Stage-J the public Run itself.
//   - The RUNTIME-BAIL hook observes every DESIGNED VM defer-to-interpreter
//     (the internal_error class RunCompiled resolves by re-running): the
//     compile-time census is structurally blind to these, so the executed
//     bail census (TestRuntimeBailCensus, plan Phase 10) needs its own
//     instrument.
//
// The hook holders live on the Registry as pointers, so ForkConcurrent's
// shallow copy shares them into concurrent forks for free; module
// sub-registries inherit them explicitly (InheritObserveHooks) alongside the
// writers and the effect ledger.

// InterpEntry is one observed entry into interpreter machinery.
type InterpEntry struct {
	// Seam names the entry point: "Engine.Run", "RunResolved", "CallBoru",
	// "runPooledSub", or "InvokeCallback:callboru" (the callback seam's
	// interpreter fallback — distinguished so its C4 decline tag can attach
	// when attribution lands).
	Seam string
	// Attribution is the C4 carve-out tag this entry belongs to ("check-mode"
	// today; decline/oracle tags land with plan Phase 10). "" = unattributed —
	// interpreter execution the end-state invariant does not permit.
	Attribution string
	// CheckMode reports whether the registry was in check mode at entry (the
	// compiler front-end executing RunInCheckMode words — always attributed).
	CheckMode bool
}

// BailEvent is one designed VM defer-to-interpreter (a runtime bail): the VM
// raised the internal_error that makes RunCompiled re-run the source on the
// interpreter (or, post effect-fence, propagate).
type BailEvent struct {
	// Site names the defer site class: "vm:poly-no-match",
	// "vm:poly-nout-drift", "vm:user-poly-unresolved", "vm:user-poly-drift",
	// "vm:user-poly-no-match", "vm:rematch-matched",
	// "vm:shaped-method-not-appliable", "vm:splice-active-payload",
	// "vm:dyn-scope-miss", "vm:dyn-scope-dispatching",
	// "vm:dyn-scope-active-token", "vm:dyn-scope-data-miss",
	// "vm:dyn-scope-data-class", "vm:dyn-scope-data-active-token",
	// "vm:dyn-frame-replay".
	Site string
	// Reason is the defer's message text (what vmErrAt wraps).
	Reason string
}

// interpEntryHook holds the armed interpreter-entry callback. The atomic
// pointer keeps the unarmed fast path to a single load with no lock — the
// emit points sit on the interpreter's hottest paths (per-element
// runPooledSub) — while staying race-free against arming from another
// goroutine (the -race concurrency gates run forks against a parent-armed
// hook).
type interpEntryHook struct {
	fn atomic.Pointer[func(InterpEntry)]
}

// bailHook holds the armed runtime-bail callback (same discipline).
type bailHook struct {
	fn atomic.Pointer[func(BailEvent)]
}

// ArmInterpEntryHook installs fn as the interpreter-entry observer and
// returns the disarm func (test seam — pair with defer/t.Cleanup). A
// registry assembled without NewRegistry has no holder and arms nothing
// (no-op disarm).
func (r *Registry) ArmInterpEntryHook(fn func(InterpEntry)) func() {
	if r.interpHook == nil {
		return func() {}
	}
	r.interpHook.fn.Store(&fn)
	return func() { r.interpHook.fn.Store(nil) }
}

// ArmRuntimeBailHook installs fn as the runtime-bail observer and returns
// the disarm func (test seam). Same holder discipline as ArmInterpEntryHook.
func (r *Registry) ArmRuntimeBailHook(fn func(BailEvent)) func() {
	if r.bailHook == nil {
		return func() {}
	}
	r.bailHook.fn.Store(&fn)
	return func() { r.bailHook.fn.Store(nil) }
}

// InheritObserveHooks shares the parent's hook holders (and nothing else)
// into a module sub-registry, alongside the writer/effect-ledger threading in
// RunModuleBody — a module body's interpreter entries and runtime bails must
// report to the SAME observers the enclosing request armed.
func (r *Registry) InheritObserveHooks(parent *Registry) {
	r.interpHook = parent.interpHook
	r.bailHook = parent.bailHook
	r.coverHook = parent.coverHook
	r.coverSources = parent.coverSources
}

// noteInterp emits one interpreter-entry observation when the hook is armed.
// Nil-safe on both the registry and the holder.
func (r *Registry) noteInterp(seam string) {
	if r == nil || r.interpHook == nil {
		return
	}
	fp := r.interpHook.fn.Load()
	if fp == nil {
		return
	}
	att := r.interpAttribution
	check := r.analysisActive()
	if check {
		att = "check-mode"
	}
	(*fp)(InterpEntry{Seam: seam, Attribution: att, CheckMode: check})
}

// SetInterpAttribution installs tag as the C4 attribution context for
// interpreter entries on this registry and returns the restore func — the
// compiled-mode entry points bracket their SANCTIONED fallback re-runs with
// it ("fallback:refusal", "fallback:runtime-bail") so the interp-entry hook
// reports the re-run's entries as attributed. Pair with defer.
func (r *Registry) SetInterpAttribution(tag string) func() {
	prev := r.interpAttribution
	r.interpAttribution = tag
	return func() { r.interpAttribution = prev }
}

// noteBail emits one runtime-bail observation when the hook is armed.
func (r *Registry) NoteBail(site, reason string) {
	if r == nil || r.bailHook == nil {
		return
	}
	fp := r.bailHook.fn.Load()
	if fp == nil {
		return
	}
	(*fp)(BailEvent{Site: site, Reason: reason})
}
