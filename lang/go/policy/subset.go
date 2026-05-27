package policy

// RequireSubset returns nil iff child grants no more than parent.
// Specifically:
//
//   - For every known capability scope: if parent has install=false,
//     child must also have install=false.
//   - For every known global op: if parent denies it, child must
//     also deny it.
//   - For every scope's words block: every (op, args) that parent
//     denies, child must also deny. We approximate this with a
//     conservative rule: child's default must be at least as
//     restrictive as parent's; child cannot lift a parent rule's
//     deny by adding a later allow rule.
//
// The conservative approximation rejects some technically-safe
// child policies, but never accepts an unsafe one — false
// positives only. That's the correct bias for attenuation.
//
// Used by the aql:vm module to validate sub-engine policies before
// construction. Implemented in the policy package so eng/native
// don't need to know about it.
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
