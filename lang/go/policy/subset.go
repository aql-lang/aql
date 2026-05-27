package policy

// RequireSubset returns nil iff child grants no more than parent.
//
// DEPRECATED — UNSOUND. This check only compares scope defaults
// and install flags; it does NOT account for per-rule denies. A
// parent with default-allow plus a specific deny rule (e.g.
// fileops default-allow with `deny: read /secret/*`) is treated
// as if the child may allow everything in that scope. The child
// can omit the rule and the sub-engine gains access the parent
// explicitly blocked. See PR #99 review (chatgpt-codex-connector,
// 2026-05-27) for the original report.
//
// The correct replacement is policy.Compose(parent, child), used by
// the aql:vm module: a composed policy routes every check through
// BOTH layers, so the parent's denies always apply regardless of
// the child's rule structure. There is no way for a child rule to
// lift a parent deny because the parent gets to vote independently.
//
// This function remains exported for backwards compatibility but
// callers should migrate to Compose. New callers should NOT use
// RequireSubset for attenuation enforcement.
func RequireSubset(child, parent Policy) error {
	if parent == nil {
		// No parent policy = parent allows everything. Anything goes.
		return nil
	}
	pCompiled, ok := parent.(*Compiled)
	if !ok {
		return profileError("RequireSubset: parent is not *Compiled")
	}
	cCompiled, ok := child.(*Compiled)
	if !ok {
		return profileError("RequireSubset: child is not *Compiled")
	}

	// 1. Globals.
	for _, g := range GlobalOps {
		if pCompiled.CheckGlobal(g) != nil && cCompiled.CheckGlobal(g) == nil {
			return &Denied{
				Code:    CodePolicyAttenuation,
				Profile: cCompiled.name,
				Blame:   "child grants global." + g + " but parent denies it",
			}
		}
	}

	// 2. install:false propagation.
	for scopeName := range KnownScopesMap() {
		pInstalled := pCompiled.Installed(scopeName)
		cInstalled := cCompiled.Installed(scopeName)
		if !pInstalled && cInstalled {
			return &Denied{
				Code:    CodePolicyAttenuation,
				Profile: cCompiled.name,
				Blame:   "child installs scope " + scopeName + " but parent has install=false",
			}
		}
	}

	// 3. Words defaults — conservative check: if parent denies by
	// default in a scope, child must also deny by default (or have
	// install=false, which is even more restrictive).
	for scopeName := range KnownScopesMap() {
		pScope := pCompiled.scope(scopeName)
		cScope := cCompiled.scope(scopeName)
		if pScope == nil {
			// Parent absent = parent default-allow. Child can be anything.
			continue
		}
		pDefault := pScope.Words.Default
		if pDefault == "" {
			pDefault = EffectAllow
		}
		if pDefault == EffectDeny {
			if cScope == nil {
				// Child absent = child default-allow. Child is broader.
				return &Denied{
					Code:    CodePolicyAttenuation,
					Profile: cCompiled.name,
					Blame:   "child default-allows scope " + scopeName + " but parent default-denies",
				}
			}
			if !cScope.Installed() {
				continue // child is more restrictive — fine
			}
			cDefault := cScope.Words.Default
			if cDefault == "" {
				cDefault = EffectAllow
			}
			if cDefault == EffectAllow {
				return &Denied{
					Code:    CodePolicyAttenuation,
					Profile: cCompiled.name,
					Blame:   "child default-allows scope " + scopeName + " but parent default-denies",
				}
			}
		}
	}

	return nil
}

// KnownScopesMap returns a set view of KnownScopes for membership
// tests in subset checks. Kept as a small helper to avoid linear
// scans in the loops above.
func KnownScopesMap() map[string]struct{} {
	out := make(map[string]struct{}, len(KnownScopes))
	for _, k := range KnownScopes {
		out[k] = struct{}{}
	}
	return out
}
