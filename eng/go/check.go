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
	cp.EverBound = cloneMap(c.EverBound)
	cp.FnAnalysisCounts = cloneMap(c.FnAnalysisCounts)
	cp.DefsInstalled = cloneMap(c.DefsInstalled)
	cp.DefsUsed = cloneMap(c.DefsUsed)
	cp.ContextTypes = cloneMap(c.ContextTypes)
	return &cp
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
	c.EverBound = nil
	c.ContextTypes = nil
	c.ArrayPoison = map[SrcPos]bool{} // fresh per run; SHARED (not cloned) across branches within the run
	c.InflightBails = 0
	c.FnNameInflight = nil
	c.SuppressBodyErrors = 0
	c.FnAnalysisCounts = nil
	c.Emit = nil
	c.FnBodyDepth = 0
	// Compiling marks a REAL compile pass; the compile entry points
	// (CompileCheck / RunCompiled) set it true AFTER this Begin. Reset it here so
	// it is a proper per-pass flag — a later plain check on a reused registry
	// must not inherit a prior compile's true.
	c.Compiling = false
	return func() {
		c.Mode = false
	}
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
		case "no_signature", "undefined_word", "uncalled_function", "branch_error":
			return
		}
	}
	if c.FnBodyDepth > 0 {
		d.FnBody = true
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
	fresh := NewEmitState()
	if saved != nil {
		fresh.reg = saved.reg
	}
	c.Emit = fresh
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
			Code:     "unused_def",
			Detail:   "def " + name + " is never used",
			Word:     name,
			Row:      pos.Row,
			Col:      pos.Col,
			Severity: SeverityWarning,
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
			// Dynamic-scope reference: the name only ever lives in a per-call
			// frame (a fn parameter or a body-local def), popped before end of
			// pass, but AQL's dynamic scoping makes it visible to a fn called
			// within that frame. Rescue it iff it was bound SOMEWHERE in the
			// pass (recursion.tsv: a dynamically-called `g` reading the caller's
			// `n`; a body-local `def acc2` read across a recursive frame). A
			// name bound nowhere stays flagged as a genuine typo.
			if r.Check.EverBound[d.Word] {
				continue
			}
		}
		kept = append(kept, d)
	}
	r.Check.Diagnostics = kept
}

// RecordBoundName notes that `name` was bound (as a def, fn parameter, or
// capture) at some point in the pass. Monotone: never cleared as frames
// unwind, so the dynamic-scope rescue can later recognise a fn-body reference
// to a name that only ever lives in a per-call frame. No-op outside check mode
// and for empty / internal ($-prefixed, __-prefixed) names.
func (c *CheckState) RecordBoundName(name string) {
	if !c.IsActive() || name == "" {
		return
	}
	if name[0] == '_' || name[0] == '$' {
		return // engine-internal / synthetic bindings are never user references
	}
	if c.EverBound == nil {
		c.EverBound = map[string]bool{}
	}
	c.EverBound[name] = true
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

// PoisonArray marks the typed mutable-Array carrier with make-site identity
// `id` as non-strict-readable: a later `arr get` on it must fall back to gradual
// Any rather than its element type. Called when a `set` writes a non-conforming
// value or the array escapes off-carrier. Monotone (never cleared) and shared
// across branch clones, so the taint holds on every path. No-op for the zero id
// (an untracked array) or outside check mode.
func (c *CheckState) PoisonArray(id SrcPos) {
	if !c.IsActive() || (id == SrcPos{}) {
		return
	}
	if c.ArrayPoison == nil {
		c.ArrayPoison = map[SrcPos]bool{}
	}
	c.ArrayPoison[id] = true
}

// ArrayPoisoned reports whether the typed-Array carrier with identity `id` has
// been poisoned in this check run (see PoisonArray). A zero id (untracked array)
// is always treated as poisoned so `get` declines to Any.
func (c *CheckState) ArrayPoisoned(id SrcPos) bool {
	if c == nil || (id == SrcPos{}) {
		return true
	}
	return c.ArrayPoison[id]
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
