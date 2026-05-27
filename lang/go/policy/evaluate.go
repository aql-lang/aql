package policy

// Name returns the compiled profile's resolved name.
func (c *Compiled) Name() string { return c.name }

// Limits returns the resolved limits, with MaxSubEngineDepth
// defaulted if unset.
func (c *Compiled) Limits() Limits {
	l := c.limits
	if l.MaxSubEngineDepth == 0 {
		l.MaxSubEngineDepth = DefaultMaxSubEngineDepth
	}
	return l
}

// Installed reports whether scope is installed. Absent scope or
// nil Install field defaults to true (allow-everything default).
func (c *Compiled) Installed(scope string) bool {
	s, ok := c.scopes[scope]
	if !ok {
		return true
	}
	return s.Installed()
}

// Scope returns a snapshot of the named scope, or a zero-value
// Scope if absent. The returned Scope is safe to inspect for
// install/words/scopes fields but should not be retained or
// mutated; the underlying maps are shared with the Compiled
// instance.
func (c *Compiled) Scope(name string) Scope {
	if s, ok := c.scopes[name]; ok && s != nil {
		return *s
	}
	return Scope{}
}

// scope returns the scope by name, or nil if absent.
func (c *Compiled) scope(name string) *Scope {
	return c.scopes[name]
}

// Check is the main dispatch path. See package doc for the
// algorithm; the structure is:
//
//  1. If the scope is uninstalled, refuse with capability_not_installed
//     (the most specific error code; takes precedence over global
//     denials so the user sees "this engine doesn't do X" rather than
//     "policy denied X by default").
//  2. For `modules.call`, descend ONLY into the per-module subscope —
//     the outer modules.words is the import gate, not the call gate.
//  3. Run global hard caps bound to (scope, op).
//  4. Evaluate the scope's words block (last-match-wins).
//
// Returns nil on allow, *Denied on refusal.
func (c *Compiled) Check(scopeName, op string, args Args) error {
	scope := c.scope(scopeName)

	// 1. Install check first. Capability_not_installed is more
	// specific than permission_denied; surfacing it first gives
	// users the right error to act on.
	if scope != nil && !scope.Installed() {
		return &Denied{
			Code:    CodeCapabilityNotInstalled,
			Scope:   scopeName,
			Op:      op,
			Profile: c.name,
			Blame:   scopeName + ".install=false",
			Args:    args,
		}
	}

	// 2. Module per-export call: only the subscope gates this.
	// Imports are checked separately with op="import".
	if scopeName == "modules" && op == "call" {
		return c.checkModuleCall(scope, args)
	}

	// 3. Global hard caps bound to (scope, op).
	for _, g := range GlobalsFor(scopeName, op) {
		if err := c.CheckGlobal(g); err != nil {
			// Promote the scope/op so the blame names the actual
			// site, not the global re-entry.
			if d, ok := err.(*Denied); ok {
				d.Scope = scopeName
				d.Op = op
				d.Args = args
			}
			return err
		}
	}

	// 4. Scope rule.
	if scope == nil {
		// Absent scope = allow-all default.
		return nil
	}
	if decision, ruleIdx := evalWords(scope.Words, op, args); decision != EffectAllow {
		blame := scopeName + ".words default=deny"
		if ruleIdx >= 0 {
			blame = blameForRule(scopeName, ruleIdx)
		}
		return &Denied{
			Code:    CodePermissionDenied,
			Scope:   scopeName,
			Op:      op,
			Profile: c.name,
			Blame:   blame,
			Args:    args,
		}
	}
	return nil
}

// checkModuleCall handles op="call" on the modules scope. The outer
// modules.words gates imports; per-export rules live in the
// per-module subscope. Absent subscope = inherit allow-all.
func (c *Compiled) checkModuleCall(scope *Scope, args Args) error {
	if scope == nil {
		return nil
	}
	modName, _ := args["module"].(string)
	exportName, _ := args["export"].(string)
	sub, ok := scope.Scopes[modName]
	if !ok || sub == nil {
		return nil
	}
	if !sub.Installed() {
		return &Denied{
			Code:    CodeCapabilityNotInstalled,
			Scope:   "modules",
			Op:      "call",
			Profile: c.name,
			Blame:   "modules.scopes." + modName + ".install=false",
			Args:    args,
		}
	}
	if decision, ruleIdx := evalWords(sub.Words, exportName, args); decision != EffectAllow {
		blame := "modules.scopes." + modName + ".words default=deny"
		if ruleIdx >= 0 {
			blame = "modules.scopes." + modName + ".words rule #" + itoa(ruleIdx)
		}
		return &Denied{
			Code:    CodePermissionDenied,
			Scope:   "modules",
			Op:      "call",
			Profile: c.name,
			Blame:   blame,
			Args:    args,
		}
	}
	return nil
}

// CheckGlobal verifies a single global hard-cap name.
func (c *Compiled) CheckGlobal(name string) error {
	scope := c.scope("global")
	if scope == nil {
		// No global block = allow-all default.
		return nil
	}
	if decision, ruleIdx := evalWords(scope.Words, name, nil); decision != EffectAllow {
		blame := "global." + name
		if ruleIdx >= 0 {
			blame = blame + " (rule #" + itoa(ruleIdx) + ")"
		} else {
			blame = blame + " (default=deny)"
		}
		return &Denied{
			Code:    CodePermissionDenied,
			Scope:   "global",
			Op:      name,
			Profile: c.name,
			Blame:   blame,
		}
	}
	return nil
}

// CheckWord verifies a kernel-word name against the engine scope.
// Exposed as a dedicated method so eng/ can consume just this
// one-method shim (WordChecker) without importing the rest of policy.
func (c *Compiled) CheckWord(name string) error {
	return c.Check("engine", name, nil)
}

// evalWords runs the last-match-wins rule loop. Returns the final
// decision and the index of the matching rule (or -1 if none
// matched, in which case the default applied).
func evalWords(w WordsBlock, op string, args Args) (Effect, int) {
	decision := w.Default
	if decision == "" {
		decision = EffectAllow
	}
	matchedIdx := -1
	for i, rule := range w.Rules {
		if !GlobAny(rule.Names(), op) {
			continue
		}
		if !whereMatches(rule.Where, args) {
			continue
		}
		decision = rule.Effect()
		matchedIdx = i
	}
	return decision, matchedIdx
}

// whereMatches reports whether all where-predicates accept args.
// Empty where = match anything. Each predicate is a key→value-list:
// the corresponding args[key] must match at least one value in the
// list. Values match by Glob for strings, deep equality for others
// (numbers, bools).
func whereMatches(where map[string][]any, args Args) bool {
	if len(where) == 0 {
		return true
	}
	for key, values := range where {
		if !whereKeyMatches(values, args[key]) {
			return false
		}
	}
	return true
}

// whereKeyMatches reports whether actual satisfies at least one of
// the values. String values use Glob; numeric values use equality
// (with int64/float64 interconversion); booleans use equality.
// maxBytes is a special key handled as "actual ≤ value".
func whereKeyMatches(values []any, actual any) bool {
	if actual == nil {
		// If no actual was supplied, predicates pass vacuously —
		// they're constraining a property the caller didn't measure.
		// Wrappers that care must supply the arg.
		return true
	}
	for _, v := range values {
		if matchOne(v, actual) {
			return true
		}
	}
	return false
}

func matchOne(want, got any) bool {
	switch w := want.(type) {
	case string:
		gs, ok := got.(string)
		if !ok {
			return false
		}
		return Glob(w, gs)
	case bool:
		gb, ok := got.(bool)
		return ok && w == gb
	case int:
		return numEq(int64(w), got)
	case int64:
		return numEq(w, got)
	case float64:
		return numEq(int64(w), got)
	}
	return false
}

func numEq(want int64, got any) bool {
	switch g := got.(type) {
	case int:
		return int64(g) == want
	case int64:
		return g == want
	case float64:
		return int64(g) == want
	}
	return false
}

// blameForRule formats the diagnostic for a rule-indexed denial.
func blameForRule(scope string, idx int) string {
	return scope + ".words rule #" + itoa(idx)
}

// itoa is a small integer-to-string for blame strings without
// pulling in strconv (which the package doesn't otherwise need).
// Handles non-negative indices; falls back to "?" for negative.
func itoa(n int) string {
	if n < 0 {
		return "?"
	}
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
