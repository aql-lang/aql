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
	c.DefsInstalled = nil
	c.DefsUsed = nil
	c.ContextTypes = nil
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
	c.Diagnostics = append(c.Diagnostics, d)
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
	// Any prior "use" count for this name was against an older
	// (now-shadowed) def or against a lookup during def setup — reset
	// so only uses AFTER this install count.
	delete(c.DefsUsed, name)
}

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
