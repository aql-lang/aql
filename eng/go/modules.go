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
	loaded map[string]bool
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
	return &ModuleRegistry{loaded: make(map[string]bool)}
}

// IsLoaded reports whether the named native module has already been
// loaded (so a second import is a no-op rather than a re-installation).
func (m *ModuleRegistry) IsLoaded(name string) bool {
	if m == nil || m.loaded == nil {
		return false
	}
	return m.loaded[name]
}

// MarkLoaded records that the named native module has been loaded.
func (m *ModuleRegistry) MarkLoaded(name string) {
	if m == nil {
		return
	}
	if m.loaded == nil {
		m.loaded = make(map[string]bool)
	}
	m.loaded[name] = true
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
