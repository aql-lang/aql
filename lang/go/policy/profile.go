package policy

// Profile is the raw, pre-resolution JSON shape of a policy file.
// Use Profile.Compile() to resolve the extends chain and produce an
// evaluable *Compiled (which satisfies Policy).
//
// Profile is intentionally a plain data carrier — no methods that
// hit the evaluator. Compilation is one explicit step.
type Profile struct {
	Version int               `json:"version,omitempty"`
	Name    string            `json:"name,omitempty"`
	Extends string            `json:"extends,omitempty"`
	Limits  Limits            `json:"limits,omitempty"`
	Scopes  map[string]*Scope `json:"scopes,omitempty"`
}

// Scope is the recursive policy unit: a words block gating directly
// addressable operations, an optional install flag, and an optional
// subscope map (used by modules for per-module export control).
type Scope struct {
	// Install reports whether the underlying capability is installed.
	// Nil = default = true. Explicit false uninstalls the capability
	// at registry-construction time.
	Install *bool `json:"install,omitempty"`

	// Words is the rule block gating operations in this scope.
	Words WordsBlock `json:"words"`

	// Scopes holds nested subscopes (e.g. per-module rules under
	// modules.scopes). Nil if the scope has no sub-structure.
	Scopes map[string]*Scope `json:"scopes,omitempty"`
}

// Installed reports the resolved install decision. Absent = true.
func (s *Scope) Installed() bool {
	if s == nil || s.Install == nil {
		return true
	}
	return *s.Install
}

// WordsBlock is the rule block: a default decision applied when no
// rule matches, plus an ordered list of rules. Last-match-wins:
// later rules override earlier matches.
type WordsBlock struct {
	Default Effect `json:"default,omitempty"`
	Rules   []Rule `json:"rules,omitempty"`
}

// Rule is one entry in a WordsBlock. Exactly one of Allow or Deny
// must be non-empty; both are always arrays (no comma-soup, no
// single strings). Where carries optional argument predicates.
type Rule struct {
	Allow []string         `json:"allow,omitempty"`
	Deny  []string         `json:"deny,omitempty"`
	Where map[string][]any `json:"where,omitempty"`
}

// Effect returns the rule's effect — EffectAllow if Allow is set,
// EffectDeny if Deny is set. Behaviour is undefined if neither is
// set; the validator catches that case at load time.
func (r *Rule) Effect() Effect {
	if len(r.Allow) > 0 {
		return EffectAllow
	}
	return EffectDeny
}

// Names returns the op-name list this rule matches against (Allow
// or Deny, whichever is set). Empty when neither is set.
func (r *Rule) Names() []string {
	if len(r.Allow) > 0 {
		return r.Allow
	}
	return r.Deny
}

// Validate checks the raw Profile shape: scope names from the known
// set, install flag only on capability scopes (or modules /
// per-module), global op names from the enum, rule shape (exactly
// one of allow/deny). Returns the first error encountered. Schema
// errors fail loudly; semantic errors (a profile that denies
// everything) do not.
func (p *Profile) Validate() error {
	if p == nil {
		return nil
	}
	for name, scope := range p.Scopes {
		if !isKnownScope(name) {
			return profileError("unknown scope %q (known: %v)", name, KnownScopes)
		}
		if scope == nil {
			continue
		}
		if scope.Install != nil && !canInstall(name) {
			return profileError("scope %q does not support install=%v",
				name, *scope.Install)
		}
		if err := validateWordsBlock(name, scope.Words); err != nil {
			return err
		}
		// Subscopes: only "modules" supports per-module subscopes today.
		// Future scopes may add more; treat unknown subscope as
		// permitted (forward-compat) but apply the same rule shape.
		for subName, sub := range scope.Scopes {
			if name == "modules" {
				// Per-module subscope. Install allowed; words allowed.
				if err := validateWordsBlock(name+"."+subName, sub.Words); err != nil {
					return err
				}
			} else {
				return profileError("scope %q does not support subscopes", name)
			}
		}
	}
	// Validate the global scope's op names against the enum.
	if g := p.Scopes["global"]; g != nil {
		for ri, rule := range g.Words.Rules {
			for _, n := range rule.Names() {
				if !IsGlobalOp(n) {
					return profileError(
						"global rule #%d names unknown global op %q (known: %v)",
						ri, n, GlobalOps)
				}
			}
		}
	}
	return nil
}

func isKnownScope(name string) bool {
	for _, k := range KnownScopes {
		if k == name {
			return true
		}
	}
	return false
}

// canInstall reports whether scope supports install=false. Global
// and engine are intrinsic to the policy mechanism — schema rejects
// install on them.
func canInstall(scope string) bool {
	switch scope {
	case "global", "engine":
		return false
	}
	return true
}

func validateWordsBlock(scopeName string, w WordsBlock) error {
	switch w.Default {
	case "", EffectAllow, EffectDeny:
		// ok
	default:
		return profileError("scope %q: default must be \"allow\" or \"deny\" (got %q)",
			scopeName, w.Default)
	}
	for ri, rule := range w.Rules {
		switch {
		case len(rule.Allow) > 0 && len(rule.Deny) > 0:
			return profileError("scope %q rule #%d has both allow and deny",
				scopeName, ri)
		case len(rule.Allow) == 0 && len(rule.Deny) == 0:
			return profileError("scope %q rule #%d has neither allow nor deny",
				scopeName, ri)
		}
	}
	return nil
}
