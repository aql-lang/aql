package eng

import "strings"

// Methods on CheckState — the static-analysis state bundle defined in
// registry.go. Grouped here so the static-checker surface is one file
// rather than three: previously these lived as r.* methods spread
// across util.go and carrier.go.
//
// Call shape: callers reach the receiver via the Registry field, e.g.
// `r.Check.IsActive()`. CheckState methods have pointer receivers
// because most mutate state (Diagnostics, DefsUsed, …); r.Check is
// addressable so Go auto-takes &r.Check at the call site.

// Clone returns a deep copy of the analysis state: scalar fields are
// copied, the maps (FnSummaries, FnInflight, FnAnalysisCounts,
// DefsInstalled, DefsUsed, ContextTypes) and the Diagnostics slice are
// cloned so a sandboxed run's in-place mutation cannot bleed into the
// snapshot (and a restore cannot bleed back). Emit is copied by pointer
// (the recorder is shared, not snapshotted). Used by the predicate /
// compile sandboxes, which since the Check-pointer conversion
// (design/module-fn-checkstate-ownership.1.md §3.2) must snapshot the
// POINTEE rather than alias it.
func (c *CheckState) Clone() *CheckState {
	if c == nil {
		return nil
	}
	cp := *c // scalars + Emit pointer + map/slice headers
	if c.Diagnostics != nil {
		cp.Diagnostics = append([]CheckDiagnostic(nil), c.Diagnostics...)
	}
	cp.FnSummaries = cloneMap(c.FnSummaries)
	cp.FnInflight = cloneMap(c.FnInflight)
	cp.FnNameInflight = cloneMap(c.FnNameInflight)
	cp.FnAnalysisCounts = cloneMap(c.FnAnalysisCounts)
	cp.DefsInstalled = cloneMap(c.DefsInstalled)
	cp.DefsUsed = cloneMap(c.DefsUsed)
	cp.ContextTypes = cloneMap(c.ContextTypes)
	cp.CtxShapes = cloneMap(c.CtxShapes)
	cp.MethodShapes = cloneMap(c.MethodShapes)
	cp.FnBinders = cloneNestedSet(c.FnBinders)
	cp.FnCallGraph = cloneNestedSet(c.FnCallGraph)
	if c.FnNameStack != nil {
		cp.FnNameStack = append([]string(nil), c.FnNameStack...)
	}
	return &cp
}

// cloneNestedSet deep-copies a name→set map so a sandbox's mutation of an
// inner set cannot bleed into the snapshot (cloneMap only copies the outer
// header, leaving the inner maps shared).
func cloneNestedSet(m map[string]map[string]bool) map[string]map[string]bool {
	if m == nil {
		return nil
	}
	cp := make(map[string]map[string]bool, len(m))
	for k, inner := range m {
		cp[k] = cloneMap(inner)
	}
	return cp
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return nil
	}
	cp := make(map[K]V, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// IsActive reports whether check mode is currently on. Handlers consult
// this to short-circuit side effects during static analysis.
func (c *CheckState) IsActive() bool {
	return c != nil && c.Mode
}

// SkipsSideEffect reports whether a side-effecting operation should be
// suppressed by check mode. Equivalent to IsActive today — kept
// distinct so the policy can be refined per category later (file write
// vs network vs store mutation) without churning every call site.
func (c *CheckState) SkipsSideEffect() bool {
	return c.IsActive()
}

// ModelsEffects reports whether this registry runs with CONCRETE values but
// SUBSTITUTED effect backends — the third mode a module body executed during
// `boru check` runs in. See CheckState.ModelEffects for the contract and for
// the two classes that stay real.
func (c *CheckState) ModelsEffects() bool {
	return c != nil && c.ModelEffects
}

// PushSpecBaseline / PopSpecBaseline bracket one SPECULATIVE check region
// with its def-depth snapshot (see CheckState.SpecBaselines): pushed by the
// rolled-back nested-body run and the fn-body analysis, consulted by the
// `undef` handler via Registry.SpecUndefBlocked.
func (c *CheckState) PushSpecBaseline(snap map[string]int) {
	c.SpecBaselines = append(c.SpecBaselines, snap)
}

func (c *CheckState) PopSpecBaseline() {
	c.SpecBaselines = c.SpecBaselines[:len(c.SpecBaselines)-1]
}

// SpecUndefBlocked reports whether an `undef` of name inside the CURRENT
// speculative check region would pop a binding that PREDATES the region —
// the deletion the model must not commit: the region may never execute at
// run time, so leaking the pop flagged `undefined_word` on clean programs
// (the wrapped-undef FP class — an `undef` in a skipped error handler, an
// each body over an empty list, an uncalled fn body). False outside check
// mode, outside any speculative region, and for bindings pushed INSIDE the
// region (frame params, body defs — their depth exceeds the entry
// snapshot), so teardown and body-local undef are untouched.
func (r *Registry) SpecUndefBlocked(name string) bool {
	if !r.Check.IsActive() || len(r.Check.SpecBaselines) == 0 {
		return false
	}
	base := r.Check.SpecBaselines[len(r.Check.SpecBaselines)-1]
	return r.Defs.Depth(name) <= base[name]
}

// Begin enables check mode and resets the per-pass state (diagnostics,
// step count, budget flag, defs-installed/used, context-type tracking).
// Returns a function that switches mode off when called — typically via
// `defer`. Diagnostics gathered during the pass remain accessible on
// Diagnostics for the caller to inspect after the deferred function
// runs.
func (c *CheckState) Begin() func() {
	if c == nil {
		return func() {}
	}
	c.Mode = true
	c.Diagnostics = nil
	c.StepCount = 0
	c.BudgetTripped = false
	c.SuppressedRuntimeError = false
	c.AmbiguousGradualSplit = false
	c.DefsInstalled = nil
	c.DefsUsed = nil
	c.FnNameStack = nil
	c.FnBinders = nil
	c.FnCallGraph = nil
	c.ContextTypes = nil
	c.CtxShapes = nil
	c.MethodShapes = nil
	c.PendingMethodApply = nil
	c.InflightBails = 0
	c.FnNameInflight = nil
	c.SuppressBodyErrors = 0
	c.FnAnalysisCounts = nil
	c.Emit = theInactiveEmit
	c.CodeEffectDepth = 0
	c.FnBodyDepth = 0
	c.CaughtBodyDepth = 0
	c.NestedBodyDepth = 0
	c.CondBodyDepth = 0
	c.LoopBodyDepth = 0
	c.SpecBaselines = nil
	c.ArgsFrameUnnamed = false
	// Compiling marks a REAL compile pass; the compile entry points set it
	// true AFTER this Begin (via BeginCompilePass). Reset it here so it is
	// a proper per-pass flag — a later plain check on a reused registry
	// must not inherit a prior compile's true.
	c.Compiling = false
	// Arm process-wide ID minting for the pass's lifetime: the emit
	// recorder keys provenance on Value.IDs minted at creation, so every
	// value created while ANY pass is live must carry one (see
	// checkPassDepth in value.go). The decrement is once-guarded — done
	// closures ride defer AND t.Cleanup in places, and a double decrement
	// would drive the counter negative, eliding IDs inside a later
	// legitimate pass.
	checkPassDepth.Add(1)
	ended := false
	return func() {
		c.Mode = false
		if !ended {
			ended = true
			checkPassDepth.Add(-1)
		}
	}
}

// BeginCompilePass is Begin() plus the compile-pass arming ritual shared
// by every bytecode-recording entry point (lang's CompileCheck, boru:vm's
// Vm.compile): install a fresh EmitState, mark the pass as Compiling, and
// drop the fn-body memos so bodies re-analyse — and re-record — under
// THIS pass (a summary cached by an earlier plain check would leave its
// compiled unit empty). One shared helper is what keeps the ritual's
// pieces from going missing in a hand-rolled copy: Vm.compile shipped
// without the Compiling flag for exactly that reason.
func (c *CheckState) BeginCompilePass() func() {
	done := c.Begin()
	if c == nil {
		return done
	}
	c.Emit = NewEmitState()
	c.Compiling = true
	c.FnSummaries = nil
	c.FnInflight = nil
	return done
}

// AddDiagnostic appends a diagnostic to the active check run. Safe to
// call outside of check mode — it simply records the finding. If the
// diagnostic's Severity is empty, the default mapping from its Code is
// applied via SeverityFor.
func (c *CheckState) AddDiagnostic(d CheckDiagnostic) {
	if c == nil {
		return
	}
	if d.Severity == "" {
		d.Severity = SeverityFor(d.Code)
	}
	// A recursive fn-body re-entry (a self-call with a different arg shape that
	// re-runs the same body tokens) must not re-emit body errors — the outer,
	// non-recursive analysis of the same body already reports any real defect,
	// whereas the narrowed re-entry can spuriously fail dispatch. Drop only the
	// emergent error-level dispatch diagnostics; warnings/info still flow.
	if c.SuppressBodyErrors > 0 && d.Severity == SeverityError {
		switch d.Code {
		case "no_signature", "undefined_word", "uncalled_function", "branch_error",
			// fn_body_error: an assumed-dispatch analysis runs a body against
			// args the REAL match already rejected (checkModeAssumeSig's
			// post-trap continuation) — a body run that HALTS under those args
			// (a splice operand bound to a typed param) is the same cascade
			// noise as the dispatch codes above; the honest diagnostic is the
			// no_signature / trap at the call site.
			"fn_body_error":
			return
		}
	}
	// Inside an error-TRAPPING region (`do [...]`, CaughtBodyDepth) the
	// runtime catches every body error, so an error-severity finding there
	// is not a program error — re-attribute it centrally: downgrade to
	// info and stamp CaughtAtRuntime, keeping the finding visible without
	// a false verdict. This covers EVERY error family uniformly (the
	// guaranteed-error mirrors, undefined_word, no_signature, …) instead
	// of each emitter special-casing the region.
	if c.CaughtBodyDepth > 0 && d.Severity == SeverityError {
		d.Severity = SeverityInfo
		d.CaughtAtRuntime = true
	}
	if c.FnBodyDepth > 0 {
		d.FnBody = true
		// Attribute the finding to the innermost NAMED fn on the stack — the
		// reader for the dynamic-scope undefined-word rescue. Empty when the
		// only enclosing body is anonymous, which the rescue treats as "no
		// reader" (never dynamic-rescued).
		if n := len(c.FnNameStack); n > 0 {
			d.FnName = c.FnNameStack[n-1]
		}
	}
	c.Diagnostics = append(c.Diagnostics, d)
}

// TruncateDiagnostics drops every diagnostic recorded after position
// n. Used by bounded fixed-point analyses (AnalyseLoopBody, the fold
// accumulator iteration) so that only the FINAL round's diagnostics
// survive — earlier rounds run against not-yet-stable bindings.
func (c *CheckState) TruncateDiagnostics(n int) {
	if c == nil || n < 0 || n >= len(c.Diagnostics) {
		return
	}
	c.Diagnostics = c.Diagnostics[:n]
}

// IsolateEmit swaps in a FRESH EmitState (sharing the registry) for the
// duration of a throwaway evaluation, returning a restore func. It is the
// hermetic complement to Suspend: Suspend keeps the SAME EmitState (only
// stopping recording), so a nested eval's interned consts and RememberOriginal
// entries still pollute the live EmitState's pool. The dynamic-help example
// eval fires from OnRegisterHook on EVERY fn registration — including the
// program's own `def f fn […]` DURING compilation — so without a swap its
// example run leaks compile-time state (e.g. a generated `['a' 'b']` sample
// list) into the program's own EmitState, corrupting a later operand's compile.
// Swapping to a throwaway pool, discarded on restore, contains it fully.
func (c *CheckState) IsolateEmit() func() {
	if c == nil {
		return func() {}
	}
	saved := c.Emit
	c.Emit = newIsolatedEmit(c.Recorder())
	return func() { c.Emit = saved }
}

// IsolateBudget snapshots the check-mode step budget (StepCount +
// BudgetTripped) for the duration of a throwaway evaluation, returning a
// restore func. It is the budget-channel complement to IsolateEmit /
// TruncateDiagnostics in the dynamic-help hermetic eval: the synthetic
// example run shares the registry's CheckState, so steps it consumes count
// against the REAL program's shared budget (StepCount is per-registry, and
// the engine's check loop short-circuits every subsequent sub-engine once
// BudgetTripped is set). A documentation example that runs many steps — most
// acutely a RECURSIVE macro, which loops to the step ceiling — would then
// exhaust the program's budget before its own later statements are reached.
// Snapshotting and restoring the counters contains the synthetic eval fully,
// so it can never abort the program's compile. No-op outside check mode.
func (c *CheckState) IsolateBudget() func() {
	if c == nil {
		return func() {}
	}
	savedCount := c.StepCount
	savedTripped := c.BudgetTripped
	return func() {
		c.StepCount = savedCount
		c.BudgetTripped = savedTripped
	}
}

// RecordDef remembers a name the user bound during a check run so
// end-of-run analysis can flag defs that were never referenced. Names
// starting with "_" (engine internals) are ignored.
func (c *CheckState) RecordDef(name string, pos SrcPos) {
	if !c.IsActive() || name == "" || strings.HasPrefix(name, "_") {
		return
	}
	if c.DefsInstalled == nil {
		c.DefsInstalled = map[string]SrcPos{}
	}
	c.DefsInstalled[name] = pos
	// "Ever-used during the run" semantics: a use recorded against this name
	// at ANY point counts. The tracker is flat (name-keyed, no scope id), so
	// resetting the use on every rebind produced false unused_def warnings
	// wherever a name is legitimately read and then re-bound — loop-carried
	// flags re-bound each iteration (decision's found/best-pri/done) and a
	// name reused as an independent local across sibling quotations where only
	// one reads it (sort's var-destructure counters). The one use the reset
	// was really protecting against — a fn's construction-time self-reference,
	// which must NOT count as a use — is now suppressed precisely at its
	// source (checkFnBodyAtConstruction snapshots/restores this name's use
	// flag), so dropping the blanket reset keeps uncalled-fn detection
	// (TestCheckUnusedDefFn) while clearing the read-then-rebind FPs. The only
	// residual is a benign FN: a genuinely dead VALUE rebind
	// (`def x 1  x print  def x 2`) is no longer flagged — acceptable, and in
	// line with the tracker's already-documented forward-ref FN.
}

// RecordUse is the exported wrapper over recordUse for callers outside the
// eng package — notably module export resolution (lang/native), which records
// each reference-exported public word as a use so unused_def does not falsely
// flag the entire public API.
func (c *CheckState) RecordUse(name string) { c.recordUse(name) }

// recordUse marks a name as referenced during check mode. Safe to call
// unconditionally; outside check mode it is a no-op. Used by
// Registry.Lookup and stepWord's simple-value path.
func (c *CheckState) recordUse(name string) {
	if !c.IsActive() || name == "" {
		return
	}
	if c.DefsUsed == nil {
		c.DefsUsed = map[string]bool{}
	}
	c.DefsUsed[name] = true
}

// EmitUnusedDefDiagnostics walks the set of defs installed during a
// check run and emits an unused_def warning for any name that was
// never referenced. Call this at the end of a check pass, before
// returning the CheckResult.
func (c *CheckState) EmitUnusedDefDiagnostics() {
	if c == nil {
		return
	}
	for name, pos := range c.DefsInstalled {
		if c.DefsUsed[name] {
			continue
		}
		c.AddDiagnostic(CheckDiagnostic{
			Code:   "unused_def",
			Detail: "def " + name + " is never used",
			Word:   name,
			Row:    pos.Row,
			Col:    pos.Col,
		})
	}
}

// RescueForwardRefDiagnostics drops undefined_word diagnostics that
// were emitted INSIDE a fn-body analysis (FnBody tag) for names that
// have a binding by the end of the pass. A fn body runs at CALL
// time, when the whole program's defs exist — so a body reference to
// a later definition is the documented forward-reference idiom
// (recursion via forward ref, mutual recursion: lang/spec/
// recursion.tsv §3), not a defect; the install-time body analysis
// just runs too early to see it. Names still unbound at end of pass
// keep their diagnostic (a genuine typo). Top-level (non-FnBody)
// uses before a def keep theirs too — those genuinely error at run
// time.
//
// Known limitation: a top-level CALL placed before the dependent
// def (`def f fn […g…] f 1 def g …`) errors at run time but is
// rescued here — the checker doesn't order call sites against defs.
//
// Call at end of a check pass, before reading Diagnostics.
func (r *Registry) RescueForwardRefDiagnostics() {
	if r == nil || r.Check.Diagnostics == nil {
		return
	}
	kept := r.Check.Diagnostics[:0]
	for _, d := range r.Check.Diagnostics {
		if d.Code == "undefined_word" && d.FnBody && d.Word != "" {
			// Module-scope forward reference: the name has a binding by end of
			// pass (recursion, mutual recursion, a later top-level def).
			if _, bound := r.Defs.Top(d.Word); bound || r.Lookup(d.Word) != nil {
				continue
			}
			// Dynamic-scope reference: the name lives only in a per-call frame
			// (a fn parameter or a body-local def), popped before end of pass,
			// but boru's dynamic scoping makes it visible to a fn REACHED from
			// the binder's frame. Rescue iff some fn that binds the name can
			// actually reach the reading fn through the call graph — the SOUND
			// condition. A name merely bound by an unrelated fn that never
			// calls the reader (`def f fn [[] [x]] def g fn [[x:Integer] [1]] f`)
			// stays flagged: it genuinely errors at run time.
			if r.Check.dynamicScopeReachable(d.Word, d.FnName) {
				continue
			}
		}
		kept = append(kept, d)
	}
	r.Check.Diagnostics = kept
}

// recordCallEdge notes that fn `caller` dispatches fn `callee` (both named).
// The self-edge of a recursive fn is recorded too — it is what makes a
// body-local binding visible to a same-fn read on a sibling branch across a
// recursive frame. No-op for the top-level caller (empty name).
func (c *CheckState) recordCallEdge(caller, callee string) {
	if caller == "" || callee == "" {
		return
	}
	if c.FnCallGraph == nil {
		c.FnCallGraph = map[string]map[string]bool{}
	}
	m := c.FnCallGraph[caller]
	if m == nil {
		m = map[string]bool{}
		c.FnCallGraph[caller] = m
	}
	m[callee] = true
}

// RecordFnBinder attributes a binding of `name` to the innermost named fn
// currently under analysis (top of FnNameStack) — a body-local def, or (via
// the AnalyseFnBody param loop) a parameter. No-op outside check mode, at the
// top level (empty stack), or for engine-internal ($-/_-prefixed) names.
func (c *CheckState) RecordFnBinder(name string) {
	if !c.IsActive() || name == "" || len(c.FnNameStack) == 0 {
		return
	}
	if name[0] == '_' || name[0] == '$' {
		return
	}
	fn := c.FnNameStack[len(c.FnNameStack)-1]
	if fn == "" {
		return
	}
	if c.FnBinders == nil {
		c.FnBinders = map[string]map[string]bool{}
	}
	m := c.FnBinders[name]
	if m == nil {
		m = map[string]bool{}
		c.FnBinders[name] = m
	}
	m[fn] = true
}

// dynamicScopeReachable reports whether some fn that binds `name` can reach
// `reader` (the fn whose body referenced it) through the recorded call graph
// — i.e. a runtime call stack exists where `reader` executes while a binder
// frame of `name` is live. Parameters/captures are frame-lifetime, so the
// answer is sound for them; a body-local binder is sound for the recursion
// idiom (the def precedes the reaching call) with a documented narrow residual
// when a local is bound only AFTER the call that reaches the reader.
func (c *CheckState) dynamicScopeReachable(name, reader string) bool {
	if reader == "" {
		return false
	}
	binders := c.FnBinders[name]
	if len(binders) == 0 {
		return false
	}
	for b := range binders {
		if c.callReaches(b, reader) {
			return true
		}
	}
	return false
}

// callReaches reports whether fn `from` transitively calls `to` in the
// recorded call graph. A fn reaches itself ONLY via an actual recursion edge
// (from→…→from), never trivially — so a non-recursive fn's own body-local
// binding is not treated as visible to a same-fn read on a sibling branch.
func (c *CheckState) callReaches(from, to string) bool {
	if len(c.FnCallGraph) == 0 {
		return false
	}
	seen := map[string]bool{}
	var walk func(n string) bool
	walk = func(n string) bool {
		for callee := range c.FnCallGraph[n] {
			if callee == to {
				return true
			}
			if !seen[callee] {
				seen[callee] = true
				if walk(callee) {
					return true
				}
			}
		}
		return false
	}
	return walk(from)
}

// RecordContextSet records (key → carrier) for the given store-set
// call. Called from `set`'s ReturnsFn. Repeated writes to the same key
// join their carrier types via JoinCarriers so the recorded type
// reflects every write. Safe to call outside check mode — it becomes a
// no-op.
func (c *CheckState) RecordContextSet(key string, carrier Value) {
	if !c.IsActive() || key == "" {
		return
	}
	if c.ContextTypes == nil {
		c.ContextTypes = map[string]Value{}
	}
	if existing, ok := c.ContextTypes[key]; ok {
		c.ContextTypes[key] = JoinCarriers(existing, carrier)
		return
	}
	c.ContextTypes[key] = carrier
}

// LookupContextType returns the carrier recorded for the given key via
// a prior set, or (Any-carrier, false) when the key has not been
// observed in this check run.
func (c *CheckState) LookupContextType(key string) (Value, bool) {
	if c == nil {
		return NewCarrier(TAny), false
	}
	if v, ok := c.ContextTypes[key]; ok {
		return v, true
	}
	return NewCarrier(TAny), false
}

// (Moved from micron.go with the family split — these are generic
// check-mode helpers, not Micron content.)
//
// CheckAddUniqueDiagnostic adds a check-mode diagnostic unless an
// identical one (code+detail+position) is already recorded — ReturnsFns
// run once per analysed call shape, and a body can be analysed under
// several shapes. Every caller mirrors a GUARANTEED runtime error over
// exactly-known operands, so the diagnostic is stamped RuntimeMirror
// (the compile pipeline does not refuse on it — the recording model is
// exact) and inside an error-catching `do` body AddDiagnostic
// re-attributes it to a caught info finding. A caught (downgraded)
// entry never blocks a later REAL emission of the same finding at
// another site, so the dedupe skips it.
func CheckAddUniqueDiagnostic(r *Registry, code, detail, word string, pos SrcPos) {
	CheckAddUnique(r, CheckDiagnostic{
		Code:          code,
		Detail:        detail,
		Word:          word,
		Row:           pos.Row,
		Col:           pos.Col,
		RuntimeMirror: true,
	})
}

// CheckAddUnique is CheckAddUniqueDiagnostic's dedupe over a diagnostic the
// caller shapes itself — for a finding that must NOT be stamped
// RuntimeMirror because the compile pipeline should refuse on it. That is
// the MODEL-UNDERMINING class (eng/go/CLAUDE.md): a mirror promises the
// program compiles and then raises the identical error, which is false when
// dispatch itself did not resolve (`no_signature`, `undefined_word`,
// `uncalled_function` — there is no call to compile).
func CheckAddUnique(r *Registry, d CheckDiagnostic) {
	for _, prev := range r.Check.Diagnostics {
		if prev.Code == d.Code && prev.Detail == d.Detail &&
			prev.Row == d.Row && prev.Col == d.Col && !prev.CaughtAtRuntime {
			return
		}
	}
	r.Check.AddDiagnostic(d)
}
