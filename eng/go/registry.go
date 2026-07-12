package eng

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
)

// Registry is the kernel's shared state: function-name registrations,
// def/type stacks, capabilities, IO writers, check-mode state, control
// flow flags. Sub-engines share one Registry so state propagates
// naturally across nested Run calls.
//
// Concerns are grouped into sub-stores rather than living as flat
// fields:
//   - r.Defs    (*DefTable)    — def-name shadowing stacks
//   - r.Types   (*TypeTable)   — type-name shadowing stacks
//   - r.Check   (CheckState)   — static-analysis state
//
// New stack-like concerns should follow the same pattern.
type Registry struct {
	// Defs holds the stacked bodies for `def`-defined words. See deftable.go.
	Defs *DefTable
	// types holds named type definitions installed by the `type` word —
	// type literals, records, disjuncts, typed lists/maps, options,
	// records, object types, dependent scalars (DepInteger, DepString,
	// …), function-shape types (FnUndef), and predicate types
	// (FnDef/Function used as type-defining functions). *Type values
	// live here, not in defStacks, because they are NOT independently
	// callable — a predicate type Bbd is only ever consulted via type
	// operations (`def n:Bbd v`, `v is Bbd`, `inspect Bbd`), never
	// invoked as a free-standing fn.
	//
	// Stacked: each name maps to a stack of definitions. `type Foo X`
	// pushes; `untype Foo` pops. The top is the active type. Once a
	// stack empties the entry is removed from the map. This mirrors
	// `def`'s shadowing semantics so users can introduce a temporary
	// alias inside a sub-program and revert it without registry
	// surgery.
	Types *TypeTable // dynamic types installed by the `type` word; each push mints a fresh Type
	// Capabilities holds host-installed plugin slots. See capability.go.
	Capabilities *CapabilityRegistry
	// Ideals holds the type-kind descriptors — the registered,
	// dynamically controllable constructors `type` dispatches through.
	// See ideal.go and design/IDEAL.10.md.
	Ideals    *IdealRegistry
	Output    io.Writer // output writer for print/printstr and stdout
	ErrOutput io.Writer // error output writer for stderr
	Input     io.Reader // input reader for stdin
	// TapeConfig bounds the execution tape's growth (initial size, max
	// grows, growth factor). The zero value uses the defaults; hosts set
	// it via lang.Options. See eng/go/tape.go.
	TapeConfig TapeConfig
	// Modules owns module-loading state: the load set, the
	// module-ID counter, the host's init callback, and the native-
	// module resolver. See modules.go.
	Modules   *ModuleRegistry
	ParseFunc func(string) ([]Value, error) // parser callback (set externally to avoid circular import)
	// Contexts is the scoped context stack; top = current engine's context Store. See contextstack.go.
	Contexts *ContextStack
	// Args is the per-call args list stack. See argsstack.go.
	Args     *ArgsStack
	Manager  any            // external manager (e.g. UniversalManager) for SDK operations
	SDKCache map[string]any // cached SDK instances keyed by spec name
	BaseDir  string         // base directory for resolving relative file paths (set by loadFileModule)
	BaseFile string         // full path of the current source file ("" if none); surfaced by __file/__folder
	Source   string         // most recent source text for error reporting
	// ModuleScope marks a registry that runs a MODULE body (set by the
	// module-body runner, e.g. RunModuleBody in lang). Read by
	// InstallWordExtension: a module may extend a CORE word only with
	// signatures carrying at least one user-defined (non-kernel)
	// argument type — an all-kernel tuple (`add [Boolean Boolean]`)
	// would surprise importers (`add 1 {}` suddenly working) and breaks
	// forward compatibility the day core claims the tuple as a locked
	// signature. See design/OPEN-WORDS.0.md "Implementation notes".
	ModuleScope    bool
	errs           []error           // registration errors accumulated during setup
	ready          bool              // true after initial setup; triggers dynamic help generation
	OnRegisterHook func(name string) // called when a function is registered after startup
	builtinWords   map[string]bool   // names registered via Register (natives + host words); user def/undef must not shadow these

	// Check holds all static type-checking state, bundled together
	// so the predicate-sandbox / compile-sandbox work can
	// snapshot/restore one field instead of ten. It is a POINTER so a
	// module sub-registry can transiently share the parent compile
	// pass's analysis state (mode/emit/memos/counters) while still
	// resolving names in its own Defs/Types — the module-fn body
	// compilation refactor (design/module-fn-checkstate-ownership.1.md
	// §5b). Snapshot sites deep-clone the pointee (CheckState.Clone)
	// and restore IN PLACE so a transient sharer observes the rollback.
	Check *CheckState

	// FlowCtrl carries the active control-flow signal (break, continue,
	// ...). Set by the corresponding handlers; consumed by the engine's
	// Run loop. Lives on the registry rather than the engine so that
	// sub-engines (which share a registry) naturally propagate the
	// signal upward — the outer Run sees the flag after its handler
	// returns, without the signal having to ride the error channel.
	// See flowctrl.go.
	FlowCtrl FlowCtrl

	// TCO is the tail-call-optimisation surface (design/TCO-STAGED.10.md).
	// Lives on the registry (not the engine) so sub-engines sharing the
	// registry contribute to one count and obey one switch.
	TCO TCOState

	// pendingGen holds the GenSpec the `gen [...]` word produced,
	// awaiting consumption by the NEXT type constructor (refine /
	// class / fnsig / fn). `gen` returns no value precisely so that
	// `def Box gen [T] refine Record [...]` collection flows past it
	// to the constructor's result; the spec travels out-of-band
	// through this slot (generics plan D2, revised — the
	// trailing-stack delivery was defeated by def's forward
	// collection, which captures a produced value before the next
	// constructor can see the stack). Orphans are drained loudly at
	// end-of-Run.
	pendingGen *GenSpecInfo

	// FnBaselines is a stack of def-depth snapshots, one per currently
	// active fn-body execution. Pushed at every body-entry point that
	// also takes a defSnapshot for body-local-def cleanup; popped at
	// the matching cleanup site. The top is the snapshot of the
	// innermost enclosing fn.
	//
	// Closure-capture detection (fn_capture.go::computeCaptures) reads
	// the top: a referenced name with Defs.Depth(name) > baseline[name]
	// lives inside an enclosing fn (param or body-local) and is
	// captured; depth == baseline[name] means it lives at module /
	// global scope and stays dynamic. Nil baseline (empty stack) means
	// the construction is at top-level and nothing is captured.
	FnBaselines []map[string]int

	// gensymN is the monotonic counter behind the `gensym` word: each call
	// mints a fresh, never-colliding atom name `tmp$g<n>`. Used for
	// capture-free temporaries in (hand-written and, later, expanded) macros.
	// See design/MACROS-PHASE1.10.md §7. The name is lowercase + mixed-`$` so
	// it is a LEGAL word name (ValidateWordName: lowercase-only, all-`$`
	// reserved) — gensyms are used as binders (`def <gensym> …`).
	gensymN uint64

	// regID is a process-stable identity minted once at construction
	// (NewRegistry). It discriminates fn-analysis memo keys across
	// registries: a native module builds its own sub-registry, and a
	// module-private fn (`decide`, `apply-op`, …) analysed under a shared
	// check pass must not collide with a same-named, same-positioned
	// parent fn. FnAnalysisKey prefixes this id; read it via
	// AnalysisScopeID. See design/module-fn-checkstate-ownership.1.md §5a.
	regID uint64

	// macroCache memoizes macro expansions keyed on (macro name + operand
	// canon). See macroCacheGet / MacroCacheClear. Nil until first use.
	macroCache map[string][]Value

	// Invoker, when non-nil, executes a code BODY through the bytecode VM
	// instead of a fresh interpreter sub-engine. The body-running native
	// handlers (each/fold/scan/do/filter/… — every word that today spins up
	// `New(r).Run([inputs… body…])`) call InvokeBody, which routes here when
	// the VM is driving so the body runs as a compiled closure WITHOUT
	// re-entering the interpreter. Nil on a plain interpreter run, where
	// InvokeBody falls back to a sub-engine and behaviour is unchanged. The
	// VM installs and restores this around its run. See invoke.go.
	//
	// Because this is mutated (install/restore) on the shared registry for
	// the duration of a RunProgram, a single registry cannot drive two
	// compiled runs concurrently — see RunProgram's concurrency note. Each
	// concurrent run needs its own *Registry (ForkConcurrent / per-instance).
	// The seam passes the CALLING registry (the handler's own dispatch
	// registry — a per-connection fork, a module sub-registry) so a raw
	// token body's sub-engine fallback resolves names in the caller's
	// scope; forks inherit the field and pass THEMSELVES.
	Invoker func(reg *Registry, body Value, inputs []Value) ([]Value, error)

	// nestedRunner runs a compiled fn UNIT nested within the currently-active
	// VM run on this registry — the live-run twin of RunUnit (which starts a
	// FRESH run and so cannot be used mid-run, where vmRunning is already 1). It
	// is installed alongside Invoker for the duration of a RunProgram and lets a
	// stamped callback invoked synchronously during the run (a service handler)
	// execute on the VM instead of falling to CallAQL. Returns handled=false when
	// the ref belongs to a different program than the running one (a defensive,
	// normally-unreachable case), so InvokeCallback falls back to the interpreter.
	// Nil outside a run; a fork inherits it but never reaches it (a fresh fork is
	// idle, so InvokeCallback takes the RunUnit path there, not this one).
	nestedRunner func(ref *CompiledFnRef, args []Value) (result []Value, handled bool, err error)

	// vmRunning latches non-zero (via sync/atomic) for the duration of a
	// RunProgram on this registry. Because RunProgram installs/restores the
	// shared Invoker (and the run mutates other shared scopes — Contexts, Defs,
	// the island sub-engine), two overlapping runs on ONE registry would race.
	// The guard converts that misuse from a silent data race into a clear
	// error; concurrent runs must each use their own registry (ForkConcurrent,
	// which resets this). It does NOT gate nested SEQUENTIAL reuse (one run
	// fully finishes, resetting the flag, before the next begins) — the normal
	// RunCompiled path. It is a plain int32 (not atomic.Bool) so the Registry
	// stays copyable for ForkConcurrent's shallow clone.
	vmRunning int32

	// runtimeStamping arms detached fn-unit compilation (StampDetachedFn):
	// see EnableRuntimeStamping. Set only by the compiled execution entry
	// points; default false keeps plain interpreter runs stamp-free.
	runtimeStamping bool
	// stampLog collects stamp attempts for the -compile-report surface
	// (stamp_report.go). Created at arming; SHARED (pointer) with forks and
	// module sub-registries so one report covers the whole execution.
	stampLog *stampLog

	// interpRunDepth counts live interpreter Engine.Run activations on this
	// registry — the top-level run AND every re-entrant sub-engine run (module
	// loads, islands, higher-order bodies). It exists so a compiled RunProgram
	// can detect at its ENTRY that an interpreter run is already in flight — the
	// one cross-engine race the vmRunning CAS structurally cannot see, since that
	// flag only guards compiled-vs-compiled. The reverse direction
	// (interpreter-starts-while-compiled-active) stays caller-responsibility:
	// the VM's own islands re-enter Engine.Run on this same registry, so a
	// registry-level flag cannot distinguish a legitimate island from a foreign
	// concurrent run without goroutine identity. Atomic so a foreign goroutine's
	// run is visible to the entry check.
	interpRunDepth int32

	// debugEngines is the stack of live Engines running on this registry,
	// innermost last. Pushed/popped (defer-balanced) at each Engine.Run
	// boundary so a 0-arg introspection word (Debug.stack) can read the
	// CURRENT engine's data stack on demand — the only path a handler has
	// to the live residual stack, which it is not otherwise handed. Cost is
	// one slice append/trim per Run (not per step); reading is the only place
	// a Snapshot is taken, so a program that never calls Debug.stack pays
	// nothing beyond the push/pop. A plain slice is safe because it is
	// per-execution state: ForkConcurrent resets it to nil (like Args), so each
	// concurrent goroutine pushes onto its own stack and never the parent's
	// shared backing array.
	debugEngines []*Engine

	// Procs is the BEAM-style process runtime (see process.go): the pid
	// table, named-process registry, and root cancellation scope. It is a
	// POINTER created by NewRegistry, so ForkConcurrent's shallow copy
	// shares one runtime across every fork — the whole engine tree sees
	// one process table (PROCESSES.0.md §2). No goroutines start until
	// the first spawn.
	Procs *ProcessRuntime

	// Proc identifies the process THIS registry's goroutine runs as: set
	// on the fork by `spawn` before the goroutine starts, and lazily set
	// on a top-level registry the first time `self`/`receive` runs there
	// (the implicit main process, so a top-level program can converse
	// with the actors it spawns — PROCESSES.0.md §10). Nil until then.
	Proc *Process

	// enginePool holds idle reusable sub-engines for the interpreter's
	// body/element evaluation seams (InvokeBody, autoEvalList, paren and
	// interp-hole evaluation). Each pooled engine has reuseTape set, so a
	// hot loop reloads one tape in place instead of allocating a fresh
	// ~DefaultTapeInitialFloor-entry tape per body invocation — the same
	// reuse the VM's island engine already proved sound. Reentrancy is
	// safe by construction: a nested sub-run POPS a different engine (or
	// creates one) while its parent's engine is mid-Run and not in the
	// pool. A plain slice is safe for the same reason debugEngines is:
	// per-execution state, reset by ForkConcurrent so forks never share
	// the parent's engines (a pooled engine pins its creating registry).
	enginePool []*Engine

	// dispatchCache memoizes Lookup's aggregated dispatch table per name.
	// aggregateDispatch rebuilds a fresh []Signature + *FnDefInfo on every
	// word dispatch even in a hot loop where the name's bindings never
	// change (~14% of interpreter allocations — see
	// design/INTERPRETER-SPEED-PLAN.10.md #2). Each entry records the
	// DefTable generation (Defs.Gen(name)) the aggregate was built at; the
	// cache hits while that generation is unchanged and misses (rebuilds)
	// the moment any binding for the name changes. Per-execution state,
	// like enginePool: ForkConcurrent gives each fork a fresh holder so a
	// fork never serves the parent's aggregates against its own cloned
	// DefTable. Allocated once in NewRegistry and never reassigned — see
	// dispatchCacheState for why the holder itself must be share-safe.
	dispatchCache *dispatchCacheState
}

// dispatchCacheState is the mutex-guarded store behind Lookup's
// memoization. The guard exists because the registry a Lookup runs
// against is not always goroutine-confined: module FnDef wrappers carry
// their module's ONE sub-registry (FnDefInfo.Registry), so every
// concurrent execution — a `serve-raw` connection handler, a spawned
// process, an `await` branch — that dispatches a module word Lookups on
// that same shared registry (execFnDefLiteral's foreign-registry
// branch). ForkConcurrent isolates each fork's OWN cache, but it cannot
// isolate the sub-registry the forks all reach through the wrappers, so
// the store itself must be safe for concurrent use. The sub-registry's
// DefTable is read-only once the module is built, which is what makes
// the cache the sole mutation on that shared path. Uncontended (the
// common single-goroutine case) the lock costs a few ns per dispatch
// and allocates nothing, so the aggregate-reuse win the cache exists
// for is untouched. Methods are nil-receiver-safe in the DefTable
// style: a zero Registry simply runs uncached.
type dispatchCacheState struct {
	mu sync.Mutex
	m  map[string]dispatchCacheEntry
}

// dispatchCacheEntry is one memoized Lookup result plus the DefTable
// generation it is valid for. fn may be nil (name resolves to no
// FnDefInfo) — caching the negative avoids re-walking the stack for a
// value-only name on every dispatch.
type dispatchCacheEntry struct {
	gen int64
	fn  *FnDefInfo
}

// newDispatchCache returns an empty holder ready for use.
func newDispatchCache() *dispatchCacheState {
	return &dispatchCacheState{m: make(map[string]dispatchCacheEntry)}
}

// get returns the aggregate cached for name at exactly gen. The second
// result distinguishes a valid cached nil (a negative entry) from a
// miss.
func (c *dispatchCacheState) get(name string, gen int64) (*FnDefInfo, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	e, ok := c.m[name]
	c.mu.Unlock()
	if !ok || e.gen != gen {
		return nil, false
	}
	return e.fn, true
}

// put stores the aggregate for name at gen. Two goroutines missing on
// the same name both build and both store; last write wins, and either
// aggregate is valid for the generation it was built at.
func (c *dispatchCacheState) put(name string, gen int64, fn *FnDefInfo) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.m[name] = dispatchCacheEntry{gen: gen, fn: fn}
	c.mu.Unlock()
}

// reset drops every entry while keeping the holder (and its mutex) in
// place, so a registry shared across goroutines never has its cache
// pointer swapped out from under a concurrent reader.
func (c *dispatchCacheState) reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.m = make(map[string]dispatchCacheEntry)
	c.mu.Unlock()
}

// Engine-pool bounds. An engine whose tape grew past pooledTapeMaxEntries
// is dropped rather than pooled so a one-off giant body does not pin its
// buffer for the rest of the run; the pool itself is capped so pathological
// nesting cannot accumulate engines without bound.
const (
	maxPooledEngines     = 16
	pooledTapeMaxEntries = 4096
)

// takeSubEngine pops an idle reusable sub-engine from the pool, creating
// one (with reuseTape set) when the pool is empty. Pair with putSubEngine
// after the engine's Run returns; results must be copied out first —
// Run's result slice aliases the engine's tape, which the next Reload
// overwrites.
func (r *Registry) takeSubEngine() *Engine {
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

// putSubEngine returns an idle sub-engine to the pool. Engines whose tape
// grew beyond the pooling bound are dropped (GC'd) instead, and per-call
// configuration is cleared so nothing leaks into the next use.
func (r *Registry) putSubEngine(e *Engine) {
	if e == nil || len(r.enginePool) >= maxPooledEngines {
		return
	}
	if e.tape != nil && e.tape.capEntries() > pooledTapeMaxEntries {
		return
	}
	e.elemEvalRecordable = false
	// Release any Values still held in the forward-collection scratch
	// buffers before the engine idles in the pool. rearrangeForForward
	// leaves the last call's collected args (which can be large list/map
	// payloads) in rrValues/rrReordered; without clearing, a pooled engine
	// would pin that transient data for the pool's lifetime, and a later
	// shorter call would leave stale tail entries pinned. Zero every slot up
	// to capacity but keep the backing arrays so the buffers stay reusable.
	clear(e.rrValues[:cap(e.rrValues)])
	e.rrValues = e.rrValues[:0]
	clear(e.rrReordered[:cap(e.rrReordered)])
	e.rrReordered = e.rrReordered[:0]
	r.enginePool = append(r.enginePool, e)
}

// enterInterpRun / exitInterpRun bracket one interpreter Engine.Run activation
// on this registry (top-level and every re-entrant sub-engine run). Pair them
// with defer at the Run boundary so the count is balanced across normal returns,
// error returns, and panic unwinds.
func (r *Registry) enterInterpRun() {
	if r != nil {
		atomic.AddInt32(&r.interpRunDepth, 1)
	}
}

func (r *Registry) exitInterpRun() {
	if r != nil {
		atomic.AddInt32(&r.interpRunDepth, -1)
	}
}

// pushEngine / popEngine bracket one Engine.Run on this registry, tracking the
// current engine for on-demand stack introspection (Debug.stack). Pair with
// defer at the Run boundary so the stack stays balanced across every exit path.
func (r *Registry) pushEngine(e *Engine) {
	if r != nil {
		r.debugEngines = append(r.debugEngines, e)
	}
}

func (r *Registry) popEngine() {
	if r != nil && len(r.debugEngines) > 0 {
		r.debugEngines = r.debugEngines[:len(r.debugEngines)-1]
	}
}

// CurrentStack returns a copy of the live data stack of the innermost
// running engine — the values resolved to the left of its pointer. Returns
// (nil, false) when no engine is running on this registry. This is the
// read-only seam Debug.stack uses; the live tape is never exposed or mutated.
func (r *Registry) CurrentStack() ([]Value, bool) {
	if r == nil || len(r.debugEngines) == 0 {
		return nil, false
	}
	e := r.debugEngines[len(r.debugEngines)-1]
	if e == nil || e.tape == nil {
		return nil, false
	}
	snap := e.tape.Snapshot()
	end := e.pointer
	if end < 0 {
		end = 0
	}
	if end > len(snap) {
		end = len(snap)
	}
	// Keep only resolved DATA values; drop the structural markers the tape
	// carries to the left of the pointer (open parens, forwards, marks,
	// ends) so the result is the clean data stack, not the raw tape.
	out := make([]Value, 0, end)
	for _, v := range snap[:end] {
		if IsOpenParen(v) || IsCloseParen(v) || IsForward(v) ||
			IsMark(v) || IsMove(v) || IsEnd(v) || IsWord(v) {
			continue
		}
		out = append(out, v)
	}
	return out, true
}

// interpRunActive reports whether an interpreter Engine.Run is currently in
// flight on this registry. Read by RunProgram at its entry (before it spawns any
// island sub-engine), so a true result there means a DISTINCT interpreter run —
// a concurrent misuse — not the compiled run's own islands.
func (r *Registry) interpRunActive() bool {
	return r != nil && atomic.LoadInt32(&r.interpRunDepth) > 0
}

// canHostVM reports whether r can start a FRESH compiled run right now: no
// compiled run and no interpreter run is already in flight on it. A per-
// connection / per-process fork (ForkConcurrent) is idle, so a callback fires a
// VM run cleanly; the main registry mid-run is busy, so InvokeCallback routes
// the callback to the interpreter (CallAQL) instead — never racing the shared
// invoker/scopes a live run owns.
func (r *Registry) canHostVM() bool {
	return r != nil && atomic.LoadInt32(&r.vmRunning) == 0 && !r.interpRunActive()
}

// EnableRuntimeStamping arms detached fn-unit compilation (StampDetachedFn /
// StampFnValue): store words and codec resolution may then compile a
// runtime-constructed, capture-free fn body to a standalone unit so
// InvokeCallback runs it on the VM. Armed only by the COMPILED execution
// entry points (RunCompiled / RunCompiledStrict and the CLI's default mode)
// and inherited by module sub-registries — a plain interpreter run
// (`-no-compile`, `Run`) never stamps, keeping the mode contract exact.
// ForkConcurrent's shallow copy carries the flag to per-connection forks.
func (r *Registry) EnableRuntimeStamping() {
	if r != nil {
		r.runtimeStamping = true
		if r.stampLog == nil {
			r.stampLog = &stampLog{}
		}
	}
}

// InheritRuntimeStamping arms r exactly like parent — including SHARING the
// parent's stamp-attribution log, so a module sub-registry's stamps land in
// the one report. A no-op when the parent is unarmed.
func (r *Registry) InheritRuntimeStamping(parent *Registry) {
	if r == nil || parent == nil || !parent.runtimeStamping {
		return
	}
	r.runtimeStamping = true
	r.stampLog = parent.stampLog
}

// RuntimeStampingEnabled reports whether detached fn-unit compilation is
// armed on this registry (see EnableRuntimeStamping).
func (r *Registry) RuntimeStampingEnabled() bool {
	return r != nil && r.runtimeStamping
}

// DisableRuntimeStamping disarms detached fn-unit compilation — the inverse of
// EnableRuntimeStamping. It stops only NEW stamps: a callback already stamped
// keeps its VM path (InvokeCallback gates on the stored CompiledRef, not this
// flag), and the stamp-attribution log is left intact so StampReport still
// reads after the run. RunCompiled / RunCompiledStrict use it to restore the
// prior interpreter contract on return, so a compiled-mode request never leaks
// the armed flag into a later plain Run (`-no-compile`) on a reused instance.
func (r *Registry) DisableRuntimeStamping() {
	if r != nil {
		r.runtimeStamping = false
	}
}

// NextGensym mints the next fresh gensym name (`tmp$g<n>`, n starting at 1).
// Monotonic per registry; the `gensym` word wraps the result in an Atom. The
// name is a valid word identifier so it can be used as a `def` binder.
func (r *Registry) NextGensym() string {
	r.gensymN++
	return fmt.Sprintf("tmp$g%d", r.gensymN)
}

// macroCache memoizes macro expansions (design/MACROS-PHASE1.10.md §8). A
// macro's expansion depends ONLY on its template and the operand FORMS — never
// on runtime state — so it is deterministic and cacheable. The key is the
// macro name + the canon of its operands (NOT source Pos, which can collide
// across re-parsed sources / synthetic tokens), so identical calls anywhere
// reuse the expansion and a macro's per-call side effects (a `gensym`) fire
// once. Cleared whenever a macro is (re)constructed (MacroCacheClear, called
// by the `macro` definer) so a redefined macro re-expands. Nil until first use.
//
// macroCacheGet / macroCachePut / MacroCacheClear are the access API.
func (r *Registry) macroCacheGet(key string) ([]Value, bool) {
	if r.macroCache == nil {
		return nil, false
	}
	toks, ok := r.macroCache[key]
	return toks, ok
}

func (r *Registry) macroCachePut(key string, toks []Value) {
	if r.macroCache == nil {
		r.macroCache = make(map[string][]Value)
	}
	r.macroCache[key] = toks
}

// MacroCacheClear drops every memoized expansion. Called by the `macro`
// definer so (re)defining a macro invalidates stale expansions.
func (r *Registry) MacroCacheClear() {
	r.macroCache = nil
}

// CheckState aggregates the static type-checking state that used to
// live as ten loose fields on Registry. Bundling them serves two
// purposes:
//
//   - **Sandboxing.** A predicate body that runs under unify checks
//     should not mutate enclosing analysis state. With a single
//     struct, snapshot/restore is `saved := r.Check; defer func()
//     { r.Check = saved }()` rather than ten parallel assignments.
//   - **Discoverability.** Anyone reading `Registry` can see the
//     check-mode footprint at a glance instead of scanning ten
//     adjacent declarations.
type CheckState struct {
	// CurCallPos is a TRANSIENT scratch: carrierResults writes the current
	// call's source position here immediately before invoking a sig's
	// ReturnsFn, so a ReturnsFn that needs the call site (e.g. `make Array`
	// stamping a typed-Array carrier's make-site identity) can read it — the
	// ReturnsFunc signature carries no pos. Overwritten on every dispatch; not
	// persistent state. Zero = unknown (synthetic/top-level).
	CurCallPos SrcPos
	// Mode toggles static type-checking execution. When true, the
	// engine runs the same dispatch/matching machinery but carries
	// type-only Carrier values instead of concrete payloads, and
	// replaces signature handlers with carrier-typed return
	// propagation (see Signature.Returns). Diagnostics are
	// accumulated into Diagnostics rather than returned as hard
	// errors.
	Mode        bool
	Diagnostics []CheckDiagnostic

	// FnSummaries caches carrier return-stacks for user-defined fn
	// bodies keyed by (name + "#" + argTypesJoined). Populated by
	// analyseFnBody; re-entrant calls (recursion) consult this
	// cache to break cycles and converge on a fixed point.
	FnSummaries map[string][]Value

	// FnInflight tracks which (name, arg-types) analyses are
	// currently running so that recursive calls can bail out with
	// a placeholder instead of looping.
	FnInflight map[string]bool

	// FnNameInflight counts, per fn NAME, how many of its body analyses
	// are on the stack. A recursive self-call with a DIFFERENT arg shape
	// has a different FnInflight key, so it does not bail — it re-analyses
	// the same body tokens under the narrowed args. Those re-analyses must
	// not RE-EMIT body diagnostics: the first (non-recursive) analysis of
	// the same body already reports any real error, while a call-shape that
	// narrowed a param to a strict Any can spuriously fail dispatch
	// (the trie fuzzy-go recursion's `kid-items`/`get` cascade). When a
	// name is already in-flight, SuppressBodyErrors is raised for the
	// re-entry so only the canonical analysis's diagnostics stand.
	FnNameInflight map[string]int

	// SuppressBodyErrors, when > 0, drops error-level diagnostics emitted
	// during a recursive fn-body RE-ENTRY (see FnNameInflight). Sound: the
	// re-entry re-runs body tokens the outer analysis already checked.
	SuppressBodyErrors int

	// CaughtBodyDepth, when > 0, marks analysis running inside an
	// error-CATCHING body — `do [body]` traps every body error and
	// surfaces it as an Error VALUE, so a guaranteed-runtime-error mirror
	// (a strict-accessor static miss, a provable index OOB, a make
	// construction failure) that fires in the region is NOT a program
	// error and must stay silent (`do [{a:1} !. b] error [dot message]`
	// is a working program). Raised around doListReturnsFn's body run;
	// consulted by CheckAddUniqueDiagnostic and emitIndexOOB.
	CaughtBodyDepth int

	// NestedBodyDepth, when > 0, marks analysis running inside ANY nested
	// body region (RunCarrierBodyWithDefs — if/case branches, loop bodies,
	// quotation/closure bodies). A diagnostic that is only sound for
	// unconditionally-reached code (the top-level unconditional-raise
	// mirror) consults it: a branch or loop body may never execute, so
	// firing there would flag working guard idioms.
	NestedBodyDepth int

	// InflightBails counts Any-placeholder bail-outs taken by
	// recursive calls of UNCHECKED fns (declared returns use the
	// declaration instead and don't count). AnalyseFnBody compares
	// the counter around a body run to know whether its summary was
	// computed under the weakest hypothesis and needs refinement
	// before being cached (design/checker-accuracy-review.10.md A2).
	InflightBails int

	// Emit is the bytecode recorder seam (EmitRecorder). A real
	// *EmitState — installed by the compile entry points after Begin —
	// turns the check pass into the bytecode recording pass (Stage 1 of
	// design/aql-bytecode-plan.0.md): every dispatch through
	// carrierResults records a classified call event and Finalize
	// linearises the trace into a Program. A plain check runs against
	// the inactive no-op recorder (Begin installs it). READ through
	// CheckState.Recorder(), which substitutes the no-op for a nil
	// field; write only from the pass entry points / probe forks.
	Emit EmitRecorder

	// Strict enables the STRICT-MODE advisory surface (`aql check
	// --strict`): every committed dispatch over a dynamic operand emits a
	// non-gating dynamic_dispatch info, making the gradual frontier loud
	// (checker-accuracy-review.10.md "--strict mode"). Persistent config —
	// set by the caller BEFORE Begin (which must not reset it).
	Strict bool

	// Compiling marks a REAL compile pass (CompileCheck / RunCompiled),
	// whose recorded events become an executed Program. A plain `aql check`
	// leaves it false even though fn-body analysis arms transient Emit
	// states. Some check-only precision relaxations (modelling a runtime-
	// arity-variable result as a consumable value) are sound for diagnostics
	// but would feed the recorder an arity the VM cannot honour, so they are
	// gated to !Compiling. Set by the compile entry points after Begin.
	Compiling bool

	// FnAnalysisCounts tracks distinct body analyses (memo misses)
	// per fn DEFINITION SITE (fnQuotaKey: scope + name + body position,
	// NOT bare name — every higher-order closure shares a synthetic
	// "<word>$body" name, so a name-only key pooled unrelated closures
	// across the whole program). Past FnAnalysisQuota the analyser stops
	// re-running the body for new arg shapes — it answers from the
	// declaration or dynamic(Any) — and emits ONE analysis_truncated
	// diagnostic naming the fn, so heavy polymorphic use degrades loudly
	// instead of silently eating the whole step budget
	// (design/checker-accuracy-review.10.md A9).
	FnAnalysisCounts map[string]int

	// StepCount is the running total of engine steps consumed by
	// the current check run, summed across every sub-engine. Used
	// with StepBudget to cap total analysis effort.
	StepCount int

	// StepBudget is the maximum total steps the check run may
	// consume. The "unset" sentinel is -1 — that's what gets
	// substituted with DefaultCheckStepBudget at run time. A real
	// zero is honored as "abort on the first step", which is
	// rarely useful but unambiguous. Once the running count
	// exceeds the resolved budget, the engine emits a
	// step_budget_exceeded diagnostic and returns immediately.
	StepBudget int

	// BudgetTripped is set to true after the first budget overshoot
	// so we emit at most one diagnostic per check run.
	BudgetTripped bool

	// SuppressedRuntimeError is set when a word that is deliberately
	// lenient in check mode skips an error the interpreter raises at
	// runtime — an orphan `gen [...]` (gen_without_constructor), an
	// `unpack` of a missing key (unpack_error). The bytecode compiler
	// reads it after the check pass: the compiled stream elides such
	// words, so it would silently succeed where the interpreter errors —
	// the program is therefore uncompilable and must fall back.
	SuppressedRuntimeError bool

	// AmbiguousGradualSplit is set when matchSignature's forward/stack
	// split for a dispatch depended on whether a GENUINELY MIXED gradual
	// carrier (a Disjunct with some alternatives conforming to a more-
	// specific overload's slot and some not — e.g. `and 0 false` typed
	// Disjunct(Integer,Boolean)) matched that overload. At runtime the
	// concrete value resolves the split one way; the check pass picked the
	// other (a less-specific overload that forward-collects instead of
	// grabbing the carrier from the stack). The two splits produce
	// different result stacks, so the bytecode compiler reads this after
	// the check pass and refuses — the program is uncompilable and must
	// fall back to the interpreter. Dispatch itself is unchanged; this is
	// a compile-time advisory only.
	AmbiguousGradualSplit bool

	// DefsInstalled records the names (and source positions) that
	// the user's program defined during a check run via the def
	// word. Populated by RecordCheckDef; consulted at end of run
	// to emit unused_def warnings.
	DefsInstalled map[string]SrcPos

	// DefsUsed records names looked up via Registry.Lookup or
	// simple-value substitution in check mode. Used to filter out
	// defs that were referenced at least once.
	DefsUsed map[string]bool

	// ContextTypes is a best-effort record of keys that user code
	// wrote to a Store during a check run. The value is the
	// last-seen carrier type for that key, joined via JoinCarriers
	// on repeated writes. Used by get's ReturnsFn so subsequent
	// reads can produce a typed carrier rather than falling back to
	// Any. Shared across the entire check run — not keyed by store
	// identity. It remains the COMPATIBILITY FALLBACK for any store
	// the shape minting misses (design/checker-precision-fronts.0.md
	// §2 stage 3 retires it only when every reader is store-shaped);
	// store-identity-keyed typing lives on StoreShapeInfo carriers
	// (store_shape.go).
	ContextTypes map[string]Value

	// CtxShapes maps a LIVE context-store layer to its abstract
	// StoreShapeInfo carrier, minted lazily by the `context` word's
	// check-mode ReturnsFn (CheckState.ContextShape). Pointer-keyed
	// deliberately: check mode never runs the runtime COW replace, so
	// a layer's pointer is stable for its scope, and holding it as a
	// key keeps the layer reachable (no recycled-allocation aliasing).
	// Per-pass state — reset by Begin, header-cloned by Clone (the
	// shape pointees stay shared; all shape mutation is join-only, so
	// a sandbox leak can only widen — see store_shape.go).
	CtxShapes map[*StoreInstanceInfo]Value

	// MethodShapes maps a dynamic method-read carrier's value ID to the
	// resolved MEMBER value — the trivial-delegation wrapper FnDef a
	// get-family read surfaced from a shape-instance container (a logger /
	// span / instrument / rand handle whose check-mode ReturnsFn instance
	// resolves method SIGNATURES; the runtime instance carries per-call
	// state, so the member itself must never bake — the freeze-gate).
	// Minted by the accessor ReturnsFn via NoteMethodShape (which vets the
	// member: delegation wrapper, named, foreign sub-registry, no genuine
	// 0-arg overload — the miscompile-E auto-dispatch guard's class stays
	// out); consumed by the compile pass's shaped-method model
	// (tryShapedMethodDispatch, method_shape.go). Per-pass state — reset by
	// Begin, header-cloned by Clone (members are immutable values).
	MethodShapes map[string]Value

	// PendingMethodApply threads ONE modelled shaped-method dispatch from
	// tryShapedMethodDispatch into recordDispatchOutcome (set immediately
	// before the model's carrierResults call, consumed by
	// tryRecordMethodApply — the first specialist in the outcome chain — so
	// the member's native never records as a check-time CALL_NATIVE, which
	// would bake the shape instance's state: the freeze-gate). Transient
	// within a single dispatch; reset by Begin.
	PendingMethodApply *PendingMethodApply

	// CodeEffectDepth counts nested code-effect body analyses
	// (AnalyseCodeEffectCarrier — the typed-code-value producer). A
	// stored code body that itself reads stored code (`quote [ops get
	// 0 do]`) would recurse through the element-read producer
	// unboundedly, so the producer declines past depth 1 — nested code
	// stays dynamic(Any), a stage-2/3 precision
	// (design/checker-precision-fronts.0.md §1).
	CodeEffectDepth int

	// FnBodyDepth counts the AnalyseFnBody nesting around the
	// current dispatch. Diagnostics emitted while it is positive
	// come from a fn BODY — code that runs at call time, not at the
	// point of analysis — so an undefined_word there may be a legal
	// forward reference (the documented mutual-recursion idiom:
	// `def isod fn […isev…] def isev fn […isod…]`). Such
	// diagnostics are tagged FnBody and rescued at end of pass when
	// the name has a binding by then (RescueForwardRefDiagnostics).
	FnBodyDepth int

	// ArgsFrameUnnamed reports whether the fn body CURRENTLY under analysis
	// (the one whose args projection is on top of r.Args) has at least one
	// UNNAMED (stack-flowing) parameter. Set and save/restored by
	// runFnBodyOnce in lockstep with the args projection push, so it always
	// reflects the innermost active body. `args` / `args.N` folds soundly to
	// a frame local only when every param is NAMED; with an unnamed param the
	// input stays live on the body stack and folding args.N strands it
	// (a compile≠interpret divergence — design/EDGE-SPEC-FINDINGS.0.md §4), so
	// specialWordResults refuses the program in compile mode when this is set.
	ArgsFrameUnnamed bool

	// FnNameStack is the stack of NAMED fn bodies currently under analysis
	// (one entry per AnalyseFnBody whose fn has a name). Its top is the fn
	// whose body is executing; used to attribute body-local defs and
	// undefined_word diagnostics to their enclosing fn and to record the
	// caller→callee edge for each dispatch. Anonymous bodies (name "") are
	// transparent — not pushed — so attribution lands on the nearest named
	// ancestor. Pushed/popped in lockstep with FnBodyDepth.
	FnNameStack []string

	// FnBinders maps a NAME to the set of fn names that bind it (as a
	// parameter or a body-local def) somewhere in the pass. With FnCallGraph
	// it is the SOUND basis for the dynamic-scope undefined-word rescue: a
	// fn-body reference to a name only ever bound in a per-call frame is
	// rescued iff a binder of that name can actually REACH the reading fn
	// through the call graph (RescueForwardRefDiagnostics). This replaces the
	// reverted "bound anywhere in the pass" rescue, which masked a genuinely-
	// undefined name that merely shared a name with an unrelated fn's param.
	FnBinders map[string]map[string]bool

	// FnCallGraph maps a fn name to the set of fn names its body calls,
	// recorded at each nested AnalyseFnBody entry (including the self-edge of
	// a recursive fn). Its transitive closure answers "can a binder of X
	// reach the fn that reads X" for the dynamic-scope rescue.
	FnCallGraph map[string]map[string]bool
}

// DefaultCheckStepBudget caps total check-mode steps across all
// sub-engines. Chosen to comfortably fit typical programs
// (thousands of words) while preventing pathological runaways.
const DefaultCheckStepBudget = 500_000

// CheckSeverity classifies a diagnostic as an error, warning, or info.
// Errors indicate a real type/signature violation that prevents
// successful execution. Warnings flag suspicious patterns that are
// still type-correct. Info is everything else (missing annotation,
// budget overshoot, etc.).
type CheckSeverity string

const (
	SeverityError   CheckSeverity = "error"
	SeverityWarning CheckSeverity = "warning"
	SeverityInfo    CheckSeverity = "info"
)

// checkCodeSeverity maps a diagnostic code to its default severity.
// Unknown codes default to SeverityInfo so new codes don't
// accidentally trip CI gates until they're classified.
var checkCodeSeverity = map[string]CheckSeverity{
	"no_signature":          SeverityError,
	"undefined_word":        SeverityError,
	"incomparable":          SeverityError,
	"fn_body_error":         SeverityError,
	"branch_error":          SeverityError,
	"type_error":            SeverityError,
	"uncalled_function":     SeverityError,
	"unreachable_signature": SeverityWarning,
	"partial_dispatch":      SeverityWarning,
	"analysis_truncated":    SeverityInfo,
	// Every emit site (CheckListIndex / CheckAtIndices / the module
	// insert-at/remove-at mirrors) fires only on a PROVABLY out-of-range
	// index over a statically-known length, and every consuming word
	// (getr / at / set / ArrayUtil.*) errors at runtime on that index —
	// there is no lenient consumer, so the diagnostic is a guaranteed
	// runtime failure, not a suspicion. Promoted from SeverityWarning
	// (accessors are REQUIRED reads: a static miss must gate).
	"index_out_of_range":   SeverityError,
	"missing_returns":      SeverityWarning,
	"step_budget_exceeded": SeverityWarning,
	"body_error":           SeverityWarning,
	// The Micron naming rule: a type bound under Scalar/Micron whose
	// name does not end in the "on" suffix (micron.go).
	"micron_name": SeverityError,
	// A strict read (getr/dotr) whose miss is statically decidable — a
	// known Micron kind with a concrete unknown key (native_micron.go), a
	// concrete map / class schema / module export the key provably misses
	// (native_accessor.go getrNodeReturns / getrObjectReturns,
	// native_module_types.go), or a statically-None strict-read parent.
	"not_found": SeverityError,
	// A strict read whose CONTAINER is provably the wrong shape — a
	// concrete list read with a non-integer key ("getr: expected a map").
	"getr_error": SeverityError,
	// `unpack` against a CONCRETE source map that provably lacks a
	// requested key (native_unpack.go's check-mode mirror).
	"unpack_error": SeverityError,
	// A `raise` on the top-level straight line — outside every fn body,
	// branch, loop, and catching `do` — unconditionally errors the
	// program (native_error_raise.go raiseReturns).
	"unconditional_raise": SeverityError,
	// Concrete-operand arithmetic that provably faults at runtime, on the
	// same top-level straight line: an int64 overflow (integer_overflow,
	// the runtime's own code) or an uncoded arithmetic raise — div/mod by
	// a static zero, pow's negative exponent (native_math.go
	// returnsIntArithChecked / returnsDivMod).
	"integer_overflow": SeverityError,
	"arith_error":      SeverityError,
	// `convert` of a PROVEN-Float source into a Big target — the one
	// type-decidable convert refusal (native_type.go convertScalarReturns).
	"convert_error": SeverityError,
	// `set` of a field outside a class instance's CLOSED schema
	// (native_storage.go setClassInstanceReturns — the runtime's own code).
	"sealed_field": SeverityError,
	// Dry-pass mirrors of PURE words over concrete literals
	// (eng/go/drypass.go DryPassReturns): each code is the runtime's own —
	// a failing Assert comparison, a malformed codec/parse literal, an
	// out-of-range module list edit, a reify shape violation, and the
	// outside-a-template macro words.
	"assertion_failure": SeverityError,
	"decode_error":      SeverityError,
	"parse_error":       SeverityError,
	"reify_error":       SeverityError,
	"unquote_error":     SeverityError,
	"splice_error":      SeverityError,
	// Parse.spec's check-mode dry pass (aql:parse): a concrete
	// whole-grammar map with a decidable shape error — unknown section,
	// mistyped token/action/matcher/abnf/rule entries.
	"parse_bad_spec":    SeverityError,
	"parse_bad_action":  SeverityError,
	"parse_bad_matcher": SeverityError,
	"parse_bad_abnf":    SeverityError,
	"parse_bad_rule":    SeverityError,
	// MiniLang.micron's check-mode dry pass (aql:minilang): a
	// value-decidable registration shape error — a non-Micron or
	// builtin kind, a non-Function builder, or a spec-Map grammar
	// with a decidable document error (bad sections, no gate token,
	// a user-set tag).
	"micron_literal": SeverityError,
	// Generics (design/GENERICS.10.md §9.2).
	"constraint_violation": SeverityError,
	"unbound_param":        SeverityError,
	"arity_mismatch":       SeverityError,
	"static_warning":       SeverityWarning,
	// Housekeeping / structural (previously set inline at the emit sites —
	// the table is the single source of truth; TestCheckSeverityTableComplete
	// gates that every emitted code has an entry).
	"unused_def":            SeverityWarning,
	"unreachable_branch":    SeverityWarning,
	"record_shape_mismatch": SeverityError,
	"fold_error":            SeverityError,
	// A typed Patrun (`patrun T`) whose `add` stores a CONCRETE value the
	// checker can prove is not a T (native_patrun.go — the static mirror of
	// the runtime add guard).
	"patrun_error": SeverityError,
	// Advisory (non-gating): a readability nudge, not a defect.
	"forward_strands_operand": SeverityInfo,
	"mixed_form_call":         SeverityInfo,
	// A parked word whose plan filled a slot with a dispatching word
	// committed early at a statement boundary (the else-less-guard
	// shape) — correct, but the source reads ambiguously; an explicit
	// `[]` else or `end` makes the intent loud. See
	// design/FORWARD-COLLECTION-PHASES.10.md.
	"speculative_forward_commit": SeverityInfo,
	// Advisory in CHECK mode by design: a ref-family misuse (`usurp 5`) is
	// lenient under analysis (the value may be a gradual carrier) and raises
	// for real at runtime — the check-mode diagnostic is a hint, not a gate.
	"illegal_ref": SeverityInfo,
	// A8: a macro/DSL expansion the checker cannot run statically degrades
	// to a dynamic value; the advisory makes the precision loss visible.
	"macro_not_expandable": SeverityInfo,
	// Strict-mode advisory: a committed dispatch over a dynamic operand.
	"dynamic_dispatch": SeverityInfo,
	// Advisory (non-gating): an `x is T` guard whose binding's static type
	// already entails T — the check cannot fail, and the dead guard misleads
	// readers about reachable states (completion plan 2.3; the article's
	// "unnecessary defensive check" residue). Emitted by ApplyGuardNarrowing.
	"redundant_guard": SeverityInfo,
	// RESERVED — no emit site yet (completion plan 4.4 / G6). The general
	// "options-looking map literal flows into a slot with no Options schema"
	// lint is BLOCKED ON PRECISION: atom-spelled and string-spelled map keys
	// are indistinguishable post-parse (`{a:1} cmp {'a':1}` → 0 — OrderedMap
	// keys are plain strings), so EVERY concrete map argument would qualify
	// and the rule cannot separate an options idiom from a data map (merge /
	// inject / make inputs) — far below the ~100% on-corpus advisory bar.
	// The per-family remedy shipped instead: option-consuming words declare
	// an Options schema on their opts slot (`convert`'s convertOptsPattern;
	// the emit family's EmitOptsSchema), which turns an unknown key into a
	// hard dispatch rejection at check AND run time. Classified here so a
	// future precise emitter inherits the intended severity.
	"options_key_unchecked": SeverityInfo,
}

// SeverityFor returns the default severity classification for a
// diagnostic code. Exported so consumers can tag custom codes.
func SeverityFor(code string) CheckSeverity {
	if s, ok := checkCodeSeverity[code]; ok {
		return s
	}
	return SeverityInfo
}

// CheckDiagnostic is a single static type-check finding.
type CheckDiagnostic struct {
	Code     string        `json:"code"`               // short stable code, e.g. "missing_returns", "no_signature"
	Detail   string        `json:"detail"`             // human-readable description
	Word     string        `json:"word,omitempty"`     // word name relevant to the diagnostic, if any
	Row      int           `json:"row,omitempty"`      // 1-based line number, 0 if unknown
	Col      int           `json:"col,omitempty"`      // 1-based column number, 0 if unknown
	Severity CheckSeverity `json:"severity,omitempty"` // default severity from checkCodeSeverity; empty = info
	FnBody   bool          `json:"fnBody,omitempty"`   // emitted during fn-body analysis (call-time code) — see RescueForwardRefDiagnostics
	FnName   string        `json:"fnName,omitempty"`   // enclosing named fn for an FnBody diagnostic — the reader for the dynamic-scope rescue

	// RuntimeMirror marks a diagnostic that mirrors a GUARANTEED runtime
	// error over exactly-known operands (design/CHECKER-COMPLETION.0.md):
	// the finding gates `aql check`, but the recording MODEL underneath it
	// is exact — the program compiles and raises the identical error at
	// runtime (a trap, the VM RET check, the same pure handler) — so the
	// compile pipeline does NOT refuse on it (CompileCheck / Vm.compile
	// skip mirrors in their error-diagnostic refusal). Contrast a
	// model-undermining diagnostic (undefined_word, no_signature), where
	// dispatch did not resolve and the recording is a guess.
	RuntimeMirror bool `json:"runtimeMirror,omitempty"`

	// CaughtAtRuntime marks a would-be-error diagnostic emitted inside an
	// error-TRAPPING region (`do [...]` — CaughtBodyDepth): the runtime
	// traps the error there, so the program is not wrong. AddDiagnostic
	// downgrades such findings to SeverityInfo centrally and stamps this
	// flag — the information (the expression always raises, and where the
	// trap is) survives without a false error verdict.
	CaughtAtRuntime bool `json:"caughtAtRuntime,omitempty"`

	// --- Structured diagnostic payload (design/DIAGNOSTICS.0.md). ---
	// The additive rich layer rendered by RenderCheckDiagnostic beneath
	// the stable one-line `check:` header; every field omitempty so the
	// JSON/LSP wire shape only grows.

	// Src is the offending token's source text — the caret width for
	// the rich source excerpt (falls back to Word when empty).
	Src string `json:"src,omitempty"`
	// Notes are freestanding explanatory lines (`= note: …`).
	Notes []string `json:"notes,omitempty"`
	// Suggestions are actionable fixes (`= help: …`).
	Suggestions []DiagSuggestion `json:"suggestions,omitempty"`
}

// NewRegistry creates an empty registry.
//
// The returned Registry has no built-in capabilities — no file
// operations, no format registry, no SQL store. The host package
// installs those via Registry.SetCapability before running user code.
// See capability.go for the plugin contract.
func NewRegistry() (*Registry, error) {
	// Surface any error accumulated while building the package-level
	// builtin type table (a malformed builtinDecls or an unknown
	// well-known path). These are init-time programmer errors that used
	// to panic; per ADR-005 they are reported here instead.
	if err := builtinInitError(); err != nil {
		return nil, err
	}
	r := &Registry{
		Defs:         NewDefTable(),
		Contexts:     NewContextStack(),
		Args:         NewArgsStack(),
		Types:        NewDynamicTypeTable(),
		Capabilities: NewCapabilityRegistry(),
		Ideals:       NewIdealRegistry(),
		Modules:      NewModuleRegistry(),
		Output:       os.Stdout,
		ErrOutput:    os.Stderr,
		Input:        os.Stdin,
		SDKCache:     make(map[string]any),
		// StepBudget uses -1 as the "unset, use the project default"
		// sentinel. The Go zero (0) is honored as "abort on the first
		// step" so callers who want that have an unambiguous way to
		// express it; callers who omit the field get the default
		// without the historical zero-as-magic overload.
		// Emit starts as the inactive no-op recorder — never nil — so
		// recorder calls are always safe (CheckState.Recorder() covers
		// registries constructed without NewRegistry).
		Check: &CheckState{StepBudget: -1, Emit: theInactiveEmit},
		Procs: NewProcessRuntime(),
		// The dispatch cache is allocated here and never reassigned, so
		// a registry shared across goroutines (a module sub-registry
		// reached through wrapper dispatch) always sees one stable,
		// mutex-guarded holder.
		dispatchCache: newDispatchCache(),
	}
	// Mint a process-stable scope id so fn-analysis memo keys can be
	// namespaced per registry (parent vs module sub-registry). A
	// ForkConcurrent shallow-copy inherits the parent's id, which is fine
	// — forks isolate execution scopes but never run check-mode analysis
	// concurrently with the parent.
	r.regID = atomic.AddUint64(&regIDCounter, 1)
	registerKernelIdeals(r)
	return r, nil
}

// regIDCounter mints the monotonic per-registry scope ids assigned in
// NewRegistry. Process-global and atomic so id minting is race-free even
// when hosts construct registries from multiple goroutines.
var regIDCounter uint64

// AnalysisScopeID returns the registry's process-stable scope id, used to
// namespace fn-analysis memo keys (FnAnalysisKey) so a module
// sub-registry's fns cannot alias a parent's under a shared check pass.
// Returns 0 for a nil registry.
func (r *Registry) AnalysisScopeID() uint64 {
	if r == nil {
		return 0
	}
	return r.regID
}

// SetParseFunc sets the parser callback used by file-based import.
func (r *Registry) SetParseFunc(fn func(string) ([]Value, error)) {
	r.ParseFunc = fn
}

// MarkReady signals that initial setup is complete. Subsequent Register
// calls will trigger dynamic help example generation via OnRegisterHook.
func (r *Registry) MarkReady() {
	r.ready = true
}

// PushFnBaseline records a def-depth snapshot as the entry point of a
// new fn-body execution. Must be paired with PopFnBaseline at the
// matching cleanup site (same place the per-call defSnapshot's cleanup
// runs). Closure-capture detection reads TopFnBaseline to decide which
// names a body-Word reference belongs to an enclosing fn vs the
// module / global scope.
func (r *Registry) PushFnBaseline(snap map[string]int) {
	r.FnBaselines = append(r.FnBaselines, snap)
}

// PopFnBaseline removes the innermost fn-body baseline. Safe to call on
// an empty stack (no-op) so cleanup paths can run unconditionally.
func (r *Registry) PopFnBaseline() {
	n := len(r.FnBaselines)
	if n == 0 {
		return
	}
	r.FnBaselines = r.FnBaselines[:n-1]
}

// TopFnBaseline returns the innermost enclosing-fn def-depth snapshot,
// or nil if no fn body is currently executing (top-level construction).
func (r *Registry) TopFnBaseline() map[string]int {
	n := len(r.FnBaselines)
	if n == 0 {
		return nil
	}
	return r.FnBaselines[n-1]
}

// Register adds one or more signatures to a named function.
// Sentinel resolution: any sig with `BarrierPos == BarrierAllForward`
// (-1) is lifted to `len(Args)` by upsertFnDef. Stack-only sigs must
// set `BarrierPos: 0` explicitly. Signatures are stored in a
// FnDefInfo entry in DefStacks.
//
// There is only one registration entry point now — the historical
// `RegisterStackOnly` wrapper was retired in favor of an explicit
// per-sig `BarrierPos`.
func (r *Registry) Register(name string, sigs ...Signature) {
	for _, sig := range sigs {
		if sig.TotalArgs() > MaxArgs {
			r.errs = append(r.errs, fmt.Errorf("signature for %q has %d args, max is %d", name, sig.TotalArgs(), MaxArgs))
			return
		}
	}
	// Record the name as a built-in word. Register is the native /
	// host-API word-registration path (RegisterNativeFunc and the public
	// (*AQL).Register both route here); user `def`s install through
	// InstallFnDef / DefTable.Push and never reach here. So this set is
	// exactly the core vocabulary whose bindings `def` / `undef` must
	// protect — see IsBuiltinWord. A `def <builtin> fn […]` is not a
	// redefinition but a MERGE (word extension, design/OPEN-WORDS.0.md);
	// the Locked flag stamped here is what pins every natively
	// registered signature against replacement/removal and keeps it
	// first in match order.
	if r.builtinWords == nil {
		r.builtinWords = make(map[string]bool)
	}
	r.builtinWords[name] = true
	for i := range sigs {
		sigs[i].Locked = true
	}
	r.upsertFnDef(name, sigs...)
	if r.ready && r.OnRegisterHook != nil {
		r.OnRegisterHook(name)
	}
}

// reservedLiterals are the value literals the parser produces directly
// (not registered words), so they never appear in builtinWords yet must
// also be protected from redefinition.
var reservedLiterals = map[string]bool{"true": true, "false": true, "none": true, "inf": true, "nan": true}

// IsBuiltinWord reports whether name is a core word that user code must
// not redefine or undefine: a word registered via Register (every
// native / kernel word, plus host words added through (*AQL).Register)
// or a reserved literal (true / false / none). User `def`s never reach
// Register, so they are never flagged here.
func (r *Registry) IsBuiltinWord(name string) bool {
	if reservedLiterals[name] {
		return true
	}
	return r != nil && r.builtinWords[name]
}

// RegisteredWordNames returns the sorted set of words that are actually
// dispatchable in THIS registry: every name registered via Register (kernel
// natives + host words, tracked in builtinWords) unioned with the live
// def-bound names. Unlike the static help catalog, this never reports a
// name that would fail with undefined_word (e.g. words documented but moved
// to an unimported module) and never omits a host registration that lacks a
// help entry. Used by Debug.words.
func (r *Registry) RegisteredWordNames() []string {
	if r == nil {
		return nil
	}
	seen := make(map[string]bool, len(r.builtinWords))
	for name := range r.builtinWords {
		seen[name] = true
	}
	for _, name := range r.Defs.Names() {
		seen[name] = true
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// upsertFnDef finds or creates a FnDefInfo at the top of DefStacks[name]
// and appends the given compiled signatures. If the top entry is already
// a FnDefInfo, its Signatures are updated in place. Otherwise a new
// FnDefInfo is pushed.
//
// Sentinel resolution: `BarrierPos == BarrierAllForward` (-1) means
// "no `|` boundary specified — default this sig to all-forward
// dispatch." Resolved here to `len(Args)`. Stack-only sigs MUST set
// `BarrierPos: 0` explicitly at the call site; there is no per-word
// "stack default" mode (the ForwardArgs flag and RegisterStackOnly
// method were retired in the BarrierPos cleanup).
func (r *Registry) upsertFnDef(name string, sigs ...Signature) {
	for i := range sigs {
		// Normalize the positional Args/Patterns constructor-convenience
		// fields into Params so every stored sig is Params-authoritative
		// for the kernel's matchers.
		normalizeSig(&sigs[i])
		if sigs[i].BarrierPos == BarrierAllForward {
			sigs[i].BarrierPos = sigs[i].TotalArgs()
		}
	}
	// If the top of the stack is already a FnDefInfo, update it in place.
	if top, ok := r.Defs.Top(name); ok {
		if fnDef, ok := top.Data.(FnDefInfo); ok {
			fnDef.Signatures = append(fnDef.Signatures, sigs...)
			SortSignatures(fnDef.Signatures)
			fnDef.MaxForwardArgs = calcMaxForwardArgs(fnDef.Signatures)
			top.Data = fnDef
			r.Defs.Replace(name, top)
			return
		}
	}
	// No existing FnDefInfo on top — push a new one.
	fnDef := FnDefInfo{
		Name:       name,
		Signatures: append([]Signature(nil), sigs...),
	}
	SortSignatures(fnDef.Signatures)
	fnDef.MaxForwardArgs = calcMaxForwardArgs(fnDef.Signatures)
	r.Defs.Push(name, NewFnDef(fnDef))
}

// calcMaxForwardArgs returns the maximum number of forward args
// needed across all signatures. Under the unified dispatch rule the
// forward limit is exactly `sig.BarrierPos`, which upsertFnDef
// resolves at registration: `BarrierAllForward` (-1) becomes
// `len(Args)`, `0` stays as explicit all-stack, intermediates pass
// through. This tells the engine how far ahead to scan and
// pre-evaluate paren expressions before signature matching.
func calcMaxForwardArgs(sigs []Signature) int {
	max := 0
	for i := range sigs {
		n := sigs[i].BarrierPos
		if n > max {
			max = n
		}
	}
	return max
}

// Lookup returns the top FnDefInfo for a name from DefStacks, or nil.
//
// Lookup deliberately does NOT record a check-mode "use" of the name
// because it is called from internal machinery (installDef, undef,
// match dispatch) that would inflate use counts. User-code usage is
// recorded by the engine.stepWord paths (simple-value substitution
// and the post-Lookup dispatch path).
func (r *Registry) Lookup(name string) *FnDefInfo {
	// The dispatchCache serves ONLY the interpreter's runtime-execution
	// path (Check inactive). Check and compile passes deliberately get a
	// fresh aggregate every call: their carrier-disjointness /
	// unmatched-dispatch refusal proofs (carrier.go) and the emitter
	// (callable_words.go, emit.go) compare a matched `*Signature` against
	// `&Lookup(word).Signatures[i]` by POINTER identity — a contract that
	// assumes each Lookup yields its own aggregate. A stable cached
	// aggregate would make those identity tests hold across calls where
	// they must not, flipping a refusal into a (wrong) compile. Runtime
	// dispatch does no such identity test, so caching there is sound and
	// is where the hot-loop win lives.
	if r.Check.IsActive() {
		return r.lookupUncached(name)
	}
	gen := r.Defs.Gen(name)
	if fn, ok := r.dispatchCache.get(name, gen); ok {
		return fn
	}
	fn := r.lookupUncached(name)
	r.dispatchCache.put(name, gen, fn)
	return fn
}

// lookupUncached aggregates the dispatch table for name from its binding
// stack, with no memoization. Lookup wraps it with the generation-keyed
// dispatchCache; call this directly only when a fresh (uncached) aggregate
// is required.
func (r *Registry) lookupUncached(name string) *FnDefInfo {
	stack := r.Defs.Stack(name)
	// Collect every FnDefInfo binding for the name, newest-first. Each
	// entry holds only its OWN overloads; the dispatch table is the union
	// across the stack (overloading across stacked defs of one name).
	//
	// A word-extension CLONE (Extends != "") carries the base word's
	// COMPLETE signature list plus its merge, so it OCCLUDES every
	// deeper entry — unioning past it would resurrect the pre-merge
	// version of any tuple the clone replaced (the stale overload would
	// race the replacement at equal score). Stopping here is also what
	// makes `undef` restore the exact previous state: popping the clone
	// re-exposes whatever the walk stopped short of.
	var entries []FnDefInfo
	for i := len(stack) - 1; i >= 0; i-- {
		if fnDef, ok := stack[i].Data.(FnDefInfo); ok {
			entries = append(entries, fnDef)
			if fnDef.Extends != "" {
				break
			}
		}
	}
	if len(entries) == 0 {
		return nil
	}
	return r.aggregateDispatch(name, entries)
}

// aggregateDispatch builds the cross-stack dispatch view for name from the
// per-entry own-signature slices (newest-first). It unions every entry's
// non-fallback overloads, sorts them with SortSignatures (most specific
// first, fallbacks last), and appends a single synthetic 0-arg Fallback when
// the name has any AQL-bodied overload — reproducing what the old carry-
// forward + in-place fallback injection produced on the top DefStack entry,
// but derived on demand so each stored entry stays its own authored unit
// (needed by targeted undef and overlap detection). Metadata (Registry,
// Anonymous, Captured) is taken from the newest entry.
func (r *Registry) aggregateDispatch(name string, entries []FnDefInfo) *FnDefInfo {
	top := entries[0]

	// Fast path: a single entry with no AQL body (a pure native word)
	// needs neither a union nor a synthetic fallback — its own sorted
	// Signatures already ARE the dispatch table.
	if len(entries) == 1 {
		hasBody := false
		for i := range top.Signatures {
			if len(top.Signatures[i].body()) > 0 {
				hasBody = true
				break
			}
		}
		if !hasBody {
			return &top
		}
	}

	sigs := make([]Signature, 0, len(top.Signatures)+1)
	hasAQL := false
	for _, e := range entries {
		for _, s := range e.Signatures {
			if s.Fallback {
				continue
			}
			sigs = append(sigs, s)
			if len(s.body()) > 0 {
				hasAQL = true
			}
		}
	}
	if hasAQL {
		sigs = append(sigs, r.fnFallbackSig(name))
	}
	SortSignatures(sigs)
	return &FnDefInfo{
		Name:           name,
		Signatures:     sigs,
		MaxForwardArgs: calcMaxForwardArgs(sigs),
		Registry:       top.Registry,
		Anonymous:      top.Anonymous,
		Macro:          top.Macro,
		Captured:       top.Captured,
		MiniKind:       top.MiniKind,
		// A word-extension clone's provenance marker rides the aggregate:
		// when the newest entry is a clone the walk stopped there, so the
		// aggregate IS the clone's view — a `name/r` reference (ResolveRef
		// wraps the aggregate) must stay recognisable at export transplant.
		Extends: top.Extends,
	}
}

// fnFallbackSig builds the synthetic 0-arg catch-all signature for an
// AQL-defined word. It courtesy-dispatches a 0-arg overload when one
// exists, otherwise raises a clean "no matching signature" error (with a
// forward-collection hint when the word takes args). Injected into the
// dispatch aggregate by aggregateDispatch; never stored on an authored entry.
func (r *Registry) fnFallbackSig(name string) Signature {
	return Signature{
		Fallback:   true,
		BarrierPos: 0,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			top, ok := r.Defs.Top(name)
			if !ok {
				return nil, fmt.Errorf("undefined: %s", name)
			}
			// A 0-arg fn courtesy-dispatches its 0-arg sig. Every other
			// shape (no 0-arg sig, a plain Function, any other binding)
			// is an unmatched call. A fn that TAKES arguments and reached
			// this 0-arg fallback (e.g. `inc inc 5`, `f a g b` — forward
			// collection could not gather them) is intercepted UPSTREAM:
			// stepWord raises the rich sigError before the fallback is
			// dispatched (the fnCourtesyDispatches guard in engine.go), so
			// the only shape reaching the Impl without a 0-arg handler
			// falls straight through to the shared no-match error.
			if _, ok := top.Data.(FnDefInfo); ok {
				if fn := r.Lookup(name); fn != nil {
					for i := range fn.Signatures {
						sig := &fn.Signatures[i]
						if sig.TotalArgs() == 0 && sig.dispatchHandler() != nil && !sig.Fallback {
							return sig.dispatchHandler()(nil, nil, nil, r)
						}
					}
				}
			}
			return nil, r.AqlError("signature_error", noMatchDetail(name), name)
		}),
	}
}

// Match finds the best matching signature for a function name given the
// resolved stack state and word modifiers.
func (r *Registry) Match(name string, resolved []Value, modifiers WordInfo) *MatchResult {
	fnDef := r.Lookup(name)
	if fnDef == nil {
		return nil
	}
	return MatchSignature(fnDef.Signatures, resolved, modifiers)
}

// InitRootContext initializes the root context Store with the __sys key.
// The __sys value is a Store/System instance containing system configuration.
// All containers at every depth are Stores.
func (r *Registry) InitRootContext() {
	root := &StoreInstanceInfo{
		TypeName: "Ideal/Store",
		Data:     make(map[string]Value),
	}

	// Create the System store.
	sysStore := &StoreInstanceInfo{
		TypeName: "Ideal/Store/System",
		Data:     make(map[string]Value),
	}

	// fs: a Store with {mem: false, impl: None}
	fsStore := &StoreInstanceInfo{
		TypeName: "Ideal/Store",
		Data:     make(map[string]Value),
	}
	fsStore.Set("mem", NewBoolean(false))
	fsStore.Set("impl", NewTypeLiteral(TNone))
	sysStore.Set("fs", NewStoreValue(TStore, fsStore))

	// __val: a Store for user-defined values
	valStore := &StoreInstanceInfo{
		TypeName: "Ideal/Store",
		Data:     make(map[string]Value),
	}
	sysStore.Set("__val", NewStoreValue(TStore, valStore))

	root.Set("__sys", NewStoreValue(TStore, sysStore))
	r.Contexts.PushExisting(root)
}

// Err returns the first registration error, or nil if none occurred.
func (r *Registry) Err() error {
	if len(r.errs) == 0 {
		return nil
	}
	return r.errs[0]
}

// --- Shared helpers used by multiple builtin files ---

// UnaryNumOpNative builds a NativeFunc for a unary numeric operation with
// two overloads: [integer] -> [decimal] and [decimal] -> [decimal]. This
// is the value-returning sibling of RegisterUnaryNumOp; use it when
// composing a NativeFunc slice instead of mutating a Registry.
func UnaryNumOpNative(name string, op func(float64) float64) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		v, _ := AsNumber(args[0])
		return []Value{NewFloat(op(v))}, nil
	}
	return NativeFunc{
		Name: name,

		Signatures: []Signature{
			{Args: []*Type{TInteger}, Impl: Go(handler), Returns: []*Type{TFloat}, BarrierPos: -1},
			{Args: []*Type{TFloat}, Impl: Go(handler), Returns: []*Type{TFloat}, BarrierPos: -1},
		},
	}
}

// BinaryNumOpNative builds a NativeFunc for a binary numeric operation
// with three float-typed overloads matching RegisterBinaryNumOp:
// [decimal, decimal], [number, decimal], and [decimal, number].
func BinaryNumOpNative(name string, op func(a, b float64) (float64, error)) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a, _ := AsNumber(args[0])
		b, _ := AsNumber(args[1])
		result, err := op(a, b)
		if err != nil {
			return nil, err
		}
		return []Value{NewFloat(result)}, nil
	}
	return NativeFunc{
		Name: name,

		Signatures: []Signature{
			{Args: []*Type{TFloat, TFloat}, Impl: Go(handler), Returns: []*Type{TFloat}, BarrierPos: -1},
			{Args: []*Type{TNumber, TFloat}, Impl: Go(handler), Returns: []*Type{TFloat}, BarrierPos: -1},
			{Args: []*Type{TFloat, TNumber}, Impl: Go(handler), Returns: []*Type{TFloat}, BarrierPos: -1},
		},
	}
}

// BinaryIntOpNative builds a NativeFunc for a binary integer operation
// with one signature [integer, integer] -> [integer]. The
// value-returning sibling of RegisterBinaryIntOp.
func BinaryIntOpNative(name string, op func(a, b int64) (int64, error)) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a, _ := args[0].AsConcreteInteger()
		b, _ := args[1].AsConcreteInteger()
		result, err := op(a, b)
		if err != nil {
			return nil, err
		}
		return []Value{NewInteger(result)}, nil
	}
	return NativeFunc{
		Name: name,

		Signatures: []Signature{
			{Args: []*Type{TInteger, TInteger}, Impl: Go(handler), Returns: []*Type{TInteger}, BarrierPos: -1},
		},
	}
}

// ValToString converts any scalar Value to its string representation.
func ValToString(v Value) string {
	if !IsConcrete(v) {
		return v.String()
	}
	switch {
	case v.IsDepScalar():
		// Must come before TString/TInteger/etc. matches: the
		// lattice override makes DepString.ConformsTo(TString) true,
		// so without this case AsString would crash on the wrong
		// payload type.
		return renderDepScalar(v)
	case v.Parent.ConformsTo(TString):
		_as8, _ := AsString(v)
		return _as8
	case IsAtom(v):
		_as9, _ := AsAtom(v)
		return _as9
	case v.Parent.ConformsTo(TFloat):
		_as10, _ := AsFloat(v)
		return formatFloat(_as10)
	case v.Parent.ConformsTo(TInteger):
		_as11, _ := AsInteger(v)
		return strconv.FormatInt(_as11, 10)
	case v.Parent.ConformsTo(TBoolean):
		_as12, _ := AsBoolean(v)
		if _as12 {
			return "true"
		}
		return "false"
	case IsPathon(v):
		_as13, _ := AsPathon(v)
		return _as13.String()
	case IsWord(v):
		_as14, _ := AsWord(v)
		return _as14.Name
	default:
		return v.String()
	}
}

// ContextStoreLookup looks up a key in the registry's context store,
// walking the prototype chain. Returns the value and true if found.
func ContextStoreLookup(r *Registry, key string) (Value, bool) {
	store := r.Contexts.Top()
	if store == nil {
		return Value{}, false
	}
	return store.Get(key)
}

// ContextSet stores a key-value pair in the root context store.
// Convenience method for programmatic setup (e.g. tests, query setup).
func (r *Registry) ContextSet(key string, val Value) {
	store := r.Contexts.Top()
	if store == nil {
		r.InitRootContext()
		store = r.Contexts.Top()
	}
	store.Set(key, val)
}

// IsKnownPart reports whether part is already used by any registered
// type — builtin or dynamic. Used to enforce part-name uniqueness when
// installing a new `type Foo …`.
func (r *Registry) IsKnownPart(part string) bool {
	if Builtin.parts[part] {
		return true
	}
	if r != nil && r.Types != nil && r.Types.parts[part] {
		return true
	}
	return false
}

// RegisterPart records part as used by this Registry's dynamic types
// so subsequent IsKnownPart calls flag it. Idempotent.
func (r *Registry) RegisterPart(part string) {
	if r == nil || r.Types == nil {
		return
	}
	r.Types.parts[part] = true
}

// ResolveTypeLiteralDef checks whether a bare type literal (Data==nil) has
// a richer definition installed under the same name (e.g. an ClassTypeInfo
// from RegisterResource or a `type Foo object {…}` binding). If so it
// returns that value; otherwise it returns the original unchanged. This
// lets the parser eagerly resolve all type names while the engine still
// picks up installed ObjectType defs.
//
// User-defined types now live in r.Types (post-§5.2); the DefStacks
// fallback is retained only for value-side ObjectType installations
// from outside the type word (e.g. legacy RegisterResource paths).
func ResolveTypeLiteralDef(v Value, reg *Registry) Value {
	if v.Data != nil || reg == nil || v.Carrier {
		return v
	}
	// A type literal IS its lattice node (by-value copy), so the
	// canonical identity is the value's own ID, not v.Parent.ID (the
	// supertype's ID).
	name := TypeNameByID(v.ID)
	if name == "" {
		return v
	}
	if top, ok := reg.Defs.Top(name); ok && (IsClassType(top) || IsResourceType(top)) {
		return top
	}
	return v
}

// StoreKey converts a Value to a string key for the store.
func StoreKey(v Value) string {
	if !IsConcrete(v) {
		return v.Parent.String()
	}
	if IsWord(v) {
		_as15, _ := AsWord(v)
		return _as15.Name
	}
	if v.Parent.ConformsTo(TString) {
		_as16, _ := AsString(v)
		return _as16
	}
	if IsAtom(v) {
		_as17, _ := AsAtom(v)
		return _as17
	}
	if v.Parent.ConformsTo(TInteger) {
		n, _ := AsInteger(v)
		return strconv.FormatInt(n, 10)
	}
	if v.Parent.ConformsTo(TFloat) {
		f, _ := AsFloat(v)
		return FormatFloat(f)
	}
	if v.Parent.ConformsTo(TBoolean) {
		b, _ := AsBoolean(v)
		if b {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%v", v.Data)
}

// RegisterNativeFunc installs a NativeFunc into the registry, converts
// native-authored Signature, and registers with the appropriate precedence.
//
// The function name is validated against the language-fundamental
// word-name rule (ValidateWordName in word_name.go): must begin with
// [a-z] and contain only [a-z0-9-]. Engine-internal markers (`__`-
// prefixed) are exempt. A bad name accumulates into r.errs — callers
// can check r.Err() before relying on the registration.
func (r *Registry) RegisterNativeFunc(fn NativeFunc) {
	if err := ValidateWordName(fn.Name); err != nil {
		r.errs = append(r.errs, err)
		return
	}
	for _, sig := range fn.Signatures {
		// The native author fills a plain Signature (positional Args + a Go
		// Handler); Register → upsertFnDef → normalizeSig synthesizes the
		// authoritative Params from Args+Patterns. Only the WORD-level fields
		// (CompileEffect, Callable) fold onto each sig here. `BarrierAllForward`
		// (-1) is the "default all-forward" sentinel; `0` is explicit all-stack —
		// upsertFnDef resolves it once.
		s := sig
		s.CompileEffect |= fn.CompileEffect
		if s.Callable == nil {
			s.Callable = fn.Callable
		}
		r.Register(fn.Name, s)
	}
}

// CallAQL invokes an AQL function value (FnDefInfo) with a pre-matched
// signature and arguments in a sub-engine. The caller is responsible for
// signature matching — use MatchFnSig to find the matching sig.
// `captures` is the FnDefInfo's lexical-closure binding list (may be
// nil); they're installed as defs alongside named params and torn down
// at exit.
//
//	sig := MatchFnSig(fn, args)
//	result, err := r.CallAQL(sig, args, fnDef.Captured)
func (r *Registry) CallAQL(sig *FnSig, args []Value, captures []CapturedBinding) ([]Value, error) {
	// Build token sequence (same as InstallFnDef handler).
	var tokens []Value
	var names []string

	// Push the fn-entry baseline before installing anything. Inner
	// fn constructions inside this body consult TopFnBaseline to
	// identify enclosing-fn-local bindings.
	r.PushFnBaseline(r.Defs.Snapshot())

	// Push args list onto the args stack.
	argsCopy := make([]Value, len(args))
	copy(argsCopy, args)
	argsList := NewList(argsCopy)
	if err := r.Args.Push(argsList); err != nil {
		r.PopFnBaseline()
		return nil, err
	}

	// Install lexical captures first so params (installed below)
	// shadow same-named captures — innermost binding wins.
	for _, cb := range captures {
		InstallFrameBinding(r, cb.Name, cb.Value)
		names = append(names, cb.Name)
	}

	// args is in top-first sig order (matchSignature convention):
	// args[0] is the value that filled sig position 0 = outer-stack top.
	//
	// Named params: bind args[i] to Params[i].Name. The first declared
	// name binds to the outer-stack top.
	//
	// Unnamed params: push args[i] into the body's frame in i-order
	// (args[0] first, args[N-1] last). The body therefore sees args[0]
	// at the bottom of its frame and args[N-1] on top, the same
	// convention InstallFnDef's handler closure uses. This is the ONE
	// arg-flow convention across every fn dispatch path post-
	// SIG-ORDER-REFACTOR.0: no reordering anywhere.
	for i, p := range sig.Params {
		if p.Name != "" {
			arg := args[i]
			if arg.Parent.Equal(TList) && !arg.Quoted {
				arg.Quoted = true
			}
			InstallFrameBinding(r, p.Name, arg)
			names = append(names, p.Name)
		} else {
			tokens = append(tokens, args[i])
		}
	}
	// The unnamed-arg prefix assembled above is call-site-resolved data;
	// stepping starts after it (arguments are inert — the sub-engine twin
	// of FrameOpenInfo.ArgSpan; design/ARG-SEMANTICS-UNIFICATION.0.md).
	unnamedCount := len(tokens)
	body := make([]Value, len(sig.body()))
	copy(body, sig.body())
	tokens = append(tokens, body...)

	// Snapshot DefStacks lengths before body execution so we can
	// clean up any defs created during body execution (Issue 2
	// from AQL-DX-REPORT: def leakage from fn bodies).
	defSnapshot := r.Defs.Snapshot()

	// Evaluate in a sub-engine with higher step limit for complex bodies.
	sub := NewTop(r)
	sub.startAt = unnamedCount
	result, err := sub.Run(tokens)

	// Cleanup: pop args stack, undef named params + captures, then
	// clean up any defs that were created during body execution. A
	// Pop error here means the args stack is nil — a misconfigured
	// registry; surface it only if sub.Run didn't already fail (the
	// run error is more informative).
	if _, popErr := r.Args.Pop(); popErr != nil && err == nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
		err = popErr
	}
	r.PopFnBaseline()
	for i := len(names) - 1; i >= 0; i-- {
		UninstallDef(r, names[i])
	}

	// Remove defs that were added during body execution.
	// Collect names first, then clean up outside the range loop
	// to avoid mutating DefStacks during iteration (UninstallDef
	// triggers InstallFnDef → Register → upsertFnDef which can
	// modify DefStacks entries for other names).
	var toClean []string
	for _, name := range r.Defs.Names() {
		if r.Defs.Depth(name) > defSnapshot[name] {
			toClean = append(toClean, name)
		}
	}
	for _, name := range toClean {
		target := defSnapshot[name]
		// Pop entries down to the snapshot length. Use a bounded
		// loop to avoid infinite looping if UninstallDef's rebuild
		// creates new entries.
		for attempts := 0; attempts < 100 && r.Defs.Depth(name) > target; attempts++ {
			UninstallDef(r, name)
		}
	}

	if err != nil {
		// Return the body's error UNWRAPPED: the historical `CallAQL:`
		// prefix leaked an internal name into every error that crossed
		// a fn-call / import boundary and broke *AqlError type
		// assertions downstream (decision DX report finding 4).
		return nil, err
	}

	// Mirror the frame collapse's unnamed-arg DISCARD (stepCloseParen's
	// ReturnCheck handling): residuals beyond the declared return count
	// are unconsumed unnamed args sitting at the bottom of the scope —
	// call-scoped data, trimmed up to unnamedCount. Leaking them into the
	// caller's stream would let a resolved fn-value argument re-step and
	// fire there (the inert-arguments invariant,
	// design/ARG-SEMANTICS-UNIFICATION.0.md). Trim ONLY — this path has
	// never enforced return count/type (guard predicates signal failure
	// via a None residual, lambdas auto-declare [Any] over side-effect
	// bodies), so the frame path's validation errors are deliberately not
	// mirrored here; that asymmetry is pre-existing and documented.
	// Undeclared returns keep the historical flow-through (the residual
	// IS the return), matching the frame path, which emits no ReturnCheck
	// in that case.
	if len(sig.Returns) > 0 && unnamedCount > 0 {
		if extra := len(result) - len(sig.Returns); extra > 0 {
			if extra > unnamedCount {
				extra = unnamedCount
			}
			result = result[extra:]
		}
	}
	return result, nil
}

// --- Error construction -------------------------------------------------

// AqlError constructs an AqlError that picks up the registry's source
// text automatically. Replaces the recurring `makeAqlError(code,
// detail, name, r.Source, "")` pattern across handlers — handlers
// just call `r.AqlError("signature_error", "no match for "+name,
// name)` and source threading is handled centrally.
//
// Use AqlErrorHint when a hint string is needed.
func (r *Registry) AqlError(code, detail, word string) error {
	src := ""
	if r != nil {
		src = r.Source
	}
	return makeAqlError(code, detail, word, src, "")
}

// AqlErrorHint is AqlError with an explicit hint string.
func (r *Registry) AqlErrorHint(code, detail, word, hint string) error {
	src := ""
	if r != nil {
		src = r.Source
	}
	return makeAqlError(code, detail, word, src, hint)
}

// ResolveTypedName resolves a name to its bound value. Post the
// TYPE-UNIFORM Phase 4 collapse there is a single binding store
// (DefTable) holding both type and value bindings, so this is one
// lookup: the capitalisation convention keeps type names and value
// names disjoint, so a name is bound at most one way.
func (r *Registry) ResolveTypedName(name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	return r.Defs.Top(name)
}

// TopTypeBody returns the body of name's active binding when that
// binding is a *type* binding (installed by a capitalised `def`), and
// (zero Value, false) otherwise — including when name is unbound or
// bound only as a value.
func (r *Registry) TopTypeBody(name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	if e, ok := r.Defs.TopEntry(name); ok && e.TypeDef != nil {
		return e.Body, true
	}
	return Value{}, false
}

// LookupTypeName returns the active lattice *Type for name: the minted
// def of a dynamic type binding (in the DefTable), or an external
// builtin registered by name. Returns nil if name names no type.
func (r *Registry) LookupTypeName(name string) *Type {
	if r == nil {
		return nil
	}
	if e, ok := r.Defs.TopEntry(name); ok && e.TypeDef != nil {
		return e.TypeDef
	}
	return r.Types.LookupBuiltinByName(name)
}

// ResolveTypedNameValue resolves a Value-shaped type reference to its
// concrete type value, capturing the source name when the input was
// a Word. Returns the resolved value, the source name (empty if v
// wasn't a Word), and ok=false only when v WAS a Word but couldn't
// be resolved through r.types or DefStacks.
//
// Replaces the
// `if v.IsWord() { w, _ := v.AsWord(); typeName = w.Name; if tv, ok :=
// r.types[w.Name]; ok { v = tv } else if ds := r.defStacks[w.Name];
// len(ds) > 0 { v = ds[len(ds)-1] } }` pattern in `defTypedHandler`,
// `is`, `inspect`, and `typeof` — extracting the name capture so
// downstream error messages can surface "type Bbd" rather than the
// rendered value form.
func (r *Registry) ResolveTypedNameValue(v Value) (resolved Value, name string, ok bool) {
	if !IsWord(v) {
		return v, "", true
	}
	w, _ := AsWord(v)
	rv, hit := r.ResolveTypedName(w.Name)
	if !hit {
		return v, w.Name, false
	}
	return rv, w.Name, true
}

// RunPredicate invokes a predicate-type fn against a candidate
// value, applying the None-or-value contract. Returns the
// predicate's output, a `matched` flag (true when the result is
// not-None), and an error for malformed predicates or invocation
// failures.
//
// The constraint must be a TFnDef or TFunction value carrying
// FnDefInfo with a single-arg first signature. Predicate types
// from `type Foo fn [x:Any Any [body]]` always satisfy this; other
// shapes return an error.
//
// CheckMode short-circuit: when r.Check.Mode is true the predicate
// body would run against carrier-typed input, which the body's
// `(x is String)`/`(x gte 10)`/etc. checks can't usefully evaluate
// (carriers fail those checks → every typed binding errors). Under
// CheckMode this helper returns (candidate, matched=true, nil) so
// downstream typed-def installation proceeds; the predicate's real
// behaviour is exercised at runtime.
//
// Sandboxing: predicate bodies are user-controlled fn bodies that
// could otherwise mutate registry state during a unify check.
// runPredicateSandboxed snapshots r.types and r.ctxStack before the
// CallAQL invocation and restores them on return — additions to
// r.types via `type Foo …` and pushes onto the context stack are
// rolled back. r.defStacks is already protected by CallAQL's own
// snapshot.
func (r *Registry) RunPredicate(constraint, candidate Value) (out Value, matched bool, err error) {
	if !constraint.Parent.Equal(TFnDef) && !constraint.Parent.Equal(TFunction) {
		return Value{}, false, fmt.Errorf("RunPredicate: constraint is not a fn (got %s)", constraint.Parent.String())
	}
	fnDef, ok := constraint.Data.(FnDefInfo)
	if !ok {
		return Value{}, false, fmt.Errorf("RunPredicate: constraint has invalid payload (got %T)", constraint.Data)
	}
	predSig, ok := fnDef.FirstOwnSig()
	if !ok || len(predSig.Params) != 1 {
		return Value{}, false, fmt.Errorf("RunPredicate: predicate must take exactly one argument")
	}
	// CheckMode: accept the binding without running the body. Real
	// predicate behaviour is asserted at runtime; here we only need
	// the analyser to keep flowing past the typed slot.
	if r != nil && r.Check.Mode {
		return candidate, true, nil
	}
	// Input-type gate: a predicate's declared input type acts as a
	// pre-filter. `"x" is Pos` for `Pos fn [[n:Integer] …]` rejects
	// at this gate without running the body, because the predicate
	// body's behavior on a non-Integer input is undefined (and
	// cross-type comparators like `gt` produce confusing answers).
	// Skip the gate for the empty case (input declared as Any or
	// unset) — those predicates explicitly accept any input.
	if inputT := predSig.Params[0].Type; inputT != nil && !inputT.Equal(TAny) {
		if IsBareTypeNode(candidate) { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			// Bare type literal: skip the gate (the literal IS a type,
			// not an inhabitant — predicate has no value to test).
		} else if !candidate.Parent.ConformsTo(inputT) {
			return candidate, false, nil
		}
	}
	// Sandbox the call so a mischievous predicate body can't mutate
	// r.types or the context stack out from under the surrounding
	// program.
	saved := snapshotPredicateState(r)
	defer restorePredicateState(r, saved)

	// InvokeCallback runs the predicate body on the VM when it compiled to a unit
	// (nested in a live run, or fresh on an idle registry) and falls back to
	// CallAQL — the interpreter — otherwise. The predicate sandbox (above) wraps
	// either engine identically.
	result, err := InvokeCallback(r, predSig, []Value{candidate}, fnDef.Captured)
	if err != nil {
		return Value{}, false, err
	}
	if len(result) != 1 {
		return Value{}, false, fmt.Errorf("RunPredicate: predicate must return exactly one value, got %d", len(result))
	}
	out = result[0]
	// A predicate signals "doesn't match" by returning:
	//  - None — sentinel value or bare type literal (IsNoneShape).
	//  - Boolean false — but ONLY when the predicate's input domain
	//    doesn't include Boolean. The `n gt 0` style predicate
	//    (input=Integer, body→Boolean) uses Boolean as a verdict, so
	//    `false` means "no match". A Boolean-domain predicate like
	//    `def Flag fn [[b:Boolean] [Boolean] [b]]` legitimately
	//    accepts `false` as a value, so we must NOT short-circuit on
	//    Boolean returns when the input type accepts Boolean.
	//
	// When the body returns Boolean true (verdict form), the
	// candidate flows through unchanged — this preserves the
	// typed-def Reparent invariant (def x:Pos 5 ⇒ x's payload is 5,
	// not Boolean true). For value-transforming bodies (`guard val`)
	// the non-Boolean output IS the new value.
	if IsNoneShape(out) {
		return out, false, nil
	}
	inputT := predSig.Params[0].Type
	booleanIsValue := inputT != nil && TBoolean.ConformsTo(inputT)
	if !booleanIsValue && out.Parent != nil && out.Parent.Equal(TBoolean) && out.Data != nil {
		if b, ok := out.Data.(BoolPayload); ok {
			if !b.B {
				return out, false, nil
			}
			return candidate, true, nil
		}
	}
	return out, true, nil
}
