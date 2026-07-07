package eng

// Registry-scoped sub-engine pool.
//
// Interpreter callback execution (InvokeBody's no-Invoker branch and the
// per-element body runs in higher-order words) historically spun up a
// fresh `New(r)` engine per invocation. Each spawn allocates and zeroes a
// tape at the initial-size floor (1024 entries × 160-byte Values ≈ 164KB),
// which dominates per-element cost in hot loops — measured at ~92µs /
// ~189KB per spawn vs ~9µs / 112B when the engine and its tape are reused
// via reuseTape (design/SUB-ENGINE-MAIN-TAPE-REVIEW.0.md §3).
//
// The pool generalises the bytecode VM's island-runner pattern
// (vmContext.islandEng + Tape.Reload) to the interpreter: RunPooled
// acquires an engine (reusing a parked one when available), runs the
// tokens, and parks the engine again. Engine.Run already clears all
// per-run scratch state on the reuseTape path (marks, voidGroups,
// traceNote, pointer), so a pooled run is observably identical to a
// fresh `New(r).Run` — same registry, same step limit, same isolation.
//
// Concurrency: the pool is intentionally unsynchronised, matching the
// registry's single-goroutine contract (Defs, Args, Contexts are equally
// unsynchronised). Code that runs engines on another goroutine must fork
// first (ForkConcurrent), and the fork starts with an EMPTY pool — a
// pooled engine holds a pointer to the registry it was built on, so
// sharing pooled engines across a fork would alias the parent.
//
// Nesting is naturally safe: an acquire POPS the engine from the pool, so
// a body that re-enters RunPooled (an `each` inside an `each`) acquires a
// different engine; the outer one is returned only after the inner run
// finished and its own release ran.

const (
	// maxPooledEngines bounds how many parked engines a registry keeps.
	// Deeper nesting simply allocates fresh engines that are dropped on
	// release once the pool is full.
	maxPooledEngines = 8
	// maxPooledTapeEntries drops an engine whose tape grew past this many
	// entries instead of parking it, so one deep run cannot pin a large
	// buffer (entries × 160B) in the pool for the rest of the process.
	maxPooledTapeEntries = 4096
)

// acquireEngine returns a parked reusable engine or builds a fresh one.
// The returned engine always has reuseTape set and non-top semantics —
// callers that need top semantics set isTop for the run (releaseEngine
// resets it).
func (r *Registry) acquireEngine() *Engine {
	if n := len(r.enginePool); n > 0 {
		e := r.enginePool[n-1]
		r.enginePool[n-1] = nil
		r.enginePool = r.enginePool[:n-1]
		return e
	}
	e := New(r)
	e.reuseTape = true
	return e
}

// releaseEngine parks e for reuse. Per-acquire fields are reset to the
// `New` defaults so no configuration leaks between pooled runs; engines
// whose tape grew past maxPooledTapeEntries are dropped rather than
// parked (the next acquire rebuilds a floor-sized one).
func (r *Registry) releaseEngine(e *Engine) {
	if e == nil || len(r.enginePool) >= maxPooledEngines {
		return
	}
	if e.tape != nil && e.tape.Cap() > maxPooledTapeEntries {
		return
	}
	e.isTop = false
	e.trace = nil
	e.traceNote = ""
	e.recorder = nil
	e.source = ""
	e.elemEvalRecordable = false
	r.enginePool = append(r.enginePool, e)
}

// RunPooled executes tokens exactly as `New(r).Run(tokens)` would, but
// reuses a registry-pooled engine (and its tape) across invocations. Use
// it for per-element / per-callback body runs on the interpreter hot
// path; behaviour is identical to a fresh sub-engine run.
//
// The residual is COPIED out of the engine before release: Run's return
// value aliases the tape's backing array under an ownership-transfer
// contract (Tape.TakeAll — "the Engine builds a fresh tape on every
// Run"), and a pooled engine's next Reload would otherwise clobber a
// residual slice the caller retained. The copy is a handful of Values —
// noise against the ~164KB tape spawn it replaces.
//
// On panic the engine is simply not returned to the pool — Engine.Run
// re-initialises all per-run state at entry, but not mid-run state torn
// by an unwind, so dropping the engine is the conservative choice.
func RunPooled(r *Registry, tokens []Value) ([]Value, error) {
	return runPooled(r, tokens, false)
}

// RunPooledTop is RunPooled with NewTop semantics for the one run: an
// unhandled break/continue at end-of-Run is an error (not propagated),
// and a handler panic is recovered into a clean internal_error. Used by
// callback sites that historically spawned `NewTop(r)` (behave bodies).
// NewTop and New share the same step limit, so top-ness is the only
// difference.
func RunPooledTop(r *Registry, tokens []Value) ([]Value, error) {
	return runPooled(r, tokens, true)
}

func runPooled(r *Registry, tokens []Value, top bool) ([]Value, error) {
	e := r.acquireEngine()
	e.isTop = top
	res, err := e.Run(tokens)
	if len(res) > 0 {
		res = append([]Value(nil), res...)
	}
	r.releaseEngine(e)
	return res, err
}
