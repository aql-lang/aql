package eng

import "fmt"

// ModuleRegistry tracks module-loading state for a Registry. It owns:
//   - the load set for native modules (so `import aql:foo` is idempotent),
//   - the module-ID sequence (so each loaded module gets a unique
//     internal name),
//   - the host's module-init callback (run when a sub-registry is
//     created for a fresh module),
//   - the native-module resolver (the bridge from `aql:<name>` to a
//     ModuleDesc).
//
// Extracted from Registry so module-loading state lives in one place
// instead of as four loose fields. The public method surface stays
// minimal: most interaction is via the resolver/init-func callbacks
// the host installs.
type ModuleRegistry struct {
	// loaded maps a native-module name to the ModuleDesc it resolved to.
	// Presence means "already resolved" (so the resolver runs at most once
	// per registry); the cached desc lets a re-import re-bind the module's
	// namespace defs without re-resolving — needed because a fn-body /
	// property-body import installs the namespace via InstallDef, which
	// CallAQL's def-cleanup then strips, leaving the module marked loaded
	// but its `pkg` name unbound. See resolveNativeMod (§11b.1).
	loaded map[string]ModuleDesc
	seq    int

	// InitFunc is called when a fresh sub-Registry is minted for a
	// module so the host can register extension words on it. Mirrors
	// the OnRegisterHook contract but fires once per sub-registry.
	InitFunc func(*Registry)

	// Resolver resolves `aql:<name>` native module imports to a
	// ModuleDesc. The kernel doesn't know how the host finds modules;
	// it just calls this and uses the result.
	Resolver func(name string, r *Registry) (ModuleDesc, error)
}

// NewModuleRegistry returns an empty module registry.
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{loaded: make(map[string]ModuleDesc)}
}

// InheritConfig copies the module CONFIG — the host-installed callbacks
// (InitFunc and Resolver) — from parent into m as a single unit, leaving
// m's own per-registry runtime state (the loaded set and the ID sequence)
// untouched. Module-body spin-up sites MUST call this instead of copying
// the callback fields one at a time: the field-by-field copying is exactly
// how the Resolver came to be silently dropped, which broke
// `import "aql:math"` from file-imported modules (native-module imports
// only worked from the top-level script). A new config field added here is
// then inherited at every spin-up site by default.
func (m *ModuleRegistry) InheritConfig(parent *ModuleRegistry) {
	if m == nil || parent == nil {
		return
	}
	m.InitFunc = parent.InitFunc
	m.Resolver = parent.Resolver
}

// IsLoaded reports whether the named native module has already been
// loaded (so a second import re-binds from the cached desc rather than
// re-resolving).
func (m *ModuleRegistry) IsLoaded(name string) bool {
	if m == nil || m.loaded == nil {
		return false
	}
	_, ok := m.loaded[name]
	return ok
}

// LoadedDesc returns the cached ModuleDesc for an already-loaded native
// module. The bool is false when the module has not been loaded.
func (m *ModuleRegistry) LoadedDesc(name string) (ModuleDesc, bool) {
	if m == nil || m.loaded == nil {
		return ModuleDesc{}, false
	}
	d, ok := m.loaded[name]
	return d, ok
}

// MarkLoaded records that the named native module has been loaded,
// caching its ModuleDesc so a later re-import can re-bind the namespace
// without re-resolving.
func (m *ModuleRegistry) MarkLoaded(name string, desc ModuleDesc) {
	if m == nil {
		return
	}
	if m.loaded == nil {
		m.loaded = make(map[string]ModuleDesc)
	}
	m.loaded[name] = desc
}

// NextID generates a fresh unique module identifier of the form
// "mod_<n>". Each call increments the counter.
func (m *ModuleRegistry) NextID() string {
	if m == nil {
		return "mod_0"
	}
	m.seq++
	return fmt.Sprintf("mod_%d", m.seq)
}
