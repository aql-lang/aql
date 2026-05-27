package policy

// Compiled is the post-resolution, evaluable form of a Profile. It
// satisfies the Policy interface.
//
// Compiled is immutable after construction; methods are safe for
// concurrent use.
type Compiled struct {
	name   string
	scopes map[string]*Scope
	limits Limits
}

// Compile resolves p's extends chain (looking up parents via the
// supplied resolver) and returns an evaluable *Compiled. resolver
// may be nil if p has no extends.
//
// Merging policy:
//   - child's Limits override parent's per-field (zero = inherit).
//   - child's Scopes are merged with parent's; for each shared
//     scope, child's Rules are appended (so later overrides win),
//     child's Default overrides parent's (zero-value = inherit), and
//     child's Install overrides parent's.
//   - per-module subscopes merge by the same rule recursively.
//
// Returns an error if any extends in the chain is unresolvable, or
// the resolved profile fails Validate().
func (p *Profile) Compile(resolver func(name string) (*Profile, error)) (*Compiled, error) {
	merged, err := resolveChain(p, resolver, nil)
	if err != nil {
		return nil, err
	}
	if err := merged.Validate(); err != nil {
		return nil, err
	}
	out := &Compiled{
		name:   merged.Name,
		scopes: merged.Scopes,
		limits: merged.Limits,
	}
	if out.scopes == nil {
		out.scopes = map[string]*Scope{}
	}
	return out, nil
}

// resolveChain walks the extends chain bottom-up, merging parent
// into child as it returns. visited tracks names to detect cycles.
func resolveChain(p *Profile, resolver func(name string) (*Profile, error), visited map[string]bool) (*Profile, error) {
	if p == nil {
		return &Profile{}, nil
	}
	if p.Extends == "" || resolver == nil {
		return p, nil
	}
	if visited == nil {
		visited = map[string]bool{}
	}
	if visited[p.Extends] {
		return nil, profileError("extends cycle through %q", p.Extends)
	}
	visited[p.Extends] = true

	parent, err := resolver(p.Extends)
	if err != nil {
		return nil, profileError("extends %q: %s", p.Extends, err)
	}
	parentResolved, err := resolveChain(parent, resolver, visited)
	if err != nil {
		return nil, err
	}
	return mergeProfile(parentResolved, p), nil
}

// mergeProfile applies child on top of parent. parent is consumed
// (its Scopes map may be reused); callers should not retain it.
func mergeProfile(parent, child *Profile) *Profile {
	out := &Profile{
		Version: child.Version,
		Name:    child.Name,
		Limits:  mergeLimits(parent.Limits, child.Limits),
		Scopes:  map[string]*Scope{},
	}
	if out.Version == 0 {
		out.Version = parent.Version
	}
	if out.Name == "" {
		out.Name = parent.Name
	}
	for name, ps := range parent.Scopes {
		out.Scopes[name] = cloneScope(ps)
	}
	for name, cs := range child.Scopes {
		if existing, ok := out.Scopes[name]; ok {
			out.Scopes[name] = mergeScope(existing, cs)
		} else {
			out.Scopes[name] = cloneScope(cs)
		}
	}
	return out
}

func mergeLimits(parent, child Limits) Limits {
	out := parent
	if child.TimeoutMs != 0 {
		out.TimeoutMs = child.TimeoutMs
	}
	if child.MaxStepBudget != 0 {
		out.MaxStepBudget = child.MaxStepBudget
	}
	if child.MaxStackDepth != 0 {
		out.MaxStackDepth = child.MaxStackDepth
	}
	if child.MaxMemoryBytes != 0 {
		out.MaxMemoryBytes = child.MaxMemoryBytes
	}
	if child.MaxOutputBytes != 0 {
		out.MaxOutputBytes = child.MaxOutputBytes
	}
	if child.MaxSubEngineDepth != 0 {
		out.MaxSubEngineDepth = child.MaxSubEngineDepth
	}
	return out
}

func mergeScope(parent, child *Scope) *Scope {
	if parent == nil {
		return cloneScope(child)
	}
	if child == nil {
		return cloneScope(parent)
	}
	out := &Scope{
		Install: parent.Install,
		Words: WordsBlock{
			Default: parent.Words.Default,
			Rules:   append([]Rule(nil), parent.Words.Rules...),
		},
	}
	if child.Install != nil {
		out.Install = child.Install
	}
	if child.Words.Default != "" {
		out.Words.Default = child.Words.Default
	}
	out.Words.Rules = append(out.Words.Rules, child.Words.Rules...)
	// Subscopes: merge recursively.
	if parent.Scopes != nil || child.Scopes != nil {
		out.Scopes = map[string]*Scope{}
		for n, s := range parent.Scopes {
			out.Scopes[n] = cloneScope(s)
		}
		for n, s := range child.Scopes {
			if existing, ok := out.Scopes[n]; ok {
				out.Scopes[n] = mergeScope(existing, s)
			} else {
				out.Scopes[n] = cloneScope(s)
			}
		}
	}
	return out
}

func cloneScope(s *Scope) *Scope {
	if s == nil {
		return nil
	}
	out := &Scope{
		Install: s.Install,
		Words: WordsBlock{
			Default: s.Words.Default,
			Rules:   append([]Rule(nil), s.Words.Rules...),
		},
	}
	if s.Scopes != nil {
		out.Scopes = map[string]*Scope{}
		for n, sub := range s.Scopes {
			out.Scopes[n] = cloneScope(sub)
		}
	}
	return out
}
