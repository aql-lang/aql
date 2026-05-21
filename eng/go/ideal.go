package eng

// An Ideal is the archetype of a type-kind — the registered,
// dynamically controllable descriptor for a family of types (Object,
// Record, Table, …). It is the `type` constructor turned into data:
// the kernel routes `type ‹base› arg` through the Ideal registry
// instead of a hard-coded switch. See lang/doc/design/IDEAL.0.md.
//
// This is Phase 1 of that design — the type-level constructor path
// only (Accepts + Construct). Value-level operations (instantiate,
// unify, format) join the struct in later phases.
type Ideal struct {
	// Name is the kind's user-facing name ("Object", "Record", …).
	Name string
	// Enabled gates the kind. A disabled Ideal is skipped by the
	// registry resolver, exactly like an unavailable capability slot.
	Enabled bool
	// Accepts reports whether base is a type value of this kind — the
	// dispatch predicate the registry consults to route `type`.
	Accepts func(base Value) bool
	// Construct builds a type of this kind from a base type value and
	// a construction argument. It is the body of `type ‹base› arg`.
	Construct func(base, arg Value, r *Registry) ([]Value, error)
	// Instantiate builds a concrete value of a type of this kind from
	// the type value and source data. It is the body of `make ‹typ›
	// data`.
	Instantiate func(typ, data Value, r *Registry) ([]Value, error)
}

// IdealRegistry holds the type-kind descriptors for one Registry.
// It mirrors CapabilityRegistry: per-Registry, named, dynamically
// extendable. Type-kinds and capabilities share the same first-class
// status but live in separate registries — a kind is a type-system
// descriptor, a capability is a host-effect slot.
type IdealRegistry struct {
	byName  map[string]*Ideal
	ordered []*Ideal
}

// NewIdealRegistry returns an empty Ideal registry.
func NewIdealRegistry() *IdealRegistry {
	return &IdealRegistry{byName: make(map[string]*Ideal)}
}

// Register installs an Ideal. Re-registering a name replaces the
// descriptor in the name index but keeps the original resolver
// position — kernel kinds register first and so win Accepts ties.
func (ir *IdealRegistry) Register(id *Ideal) {
	if ir == nil || id == nil || id.Name == "" {
		return
	}
	if _, dup := ir.byName[id.Name]; !dup {
		ir.ordered = append(ir.ordered, id)
	} else {
		for i, existing := range ir.ordered {
			if existing.Name == id.Name {
				ir.ordered[i] = id
				break
			}
		}
	}
	ir.byName[id.Name] = id
}

// Get returns the Ideal registered under name, or nil.
func (ir *IdealRegistry) Get(name string) *Ideal {
	if ir == nil {
		return nil
	}
	return ir.byName[name]
}

// Match returns the first Ideal, in registration order, whose Accepts
// predicate claims base — regardless of whether it is enabled. Returns
// nil when no kind claims base. Use For for dispatch (it additionally
// requires the kind to be enabled); use Match to tell a disabled kind
// apart from an unknown one when raising an error.
func (ir *IdealRegistry) Match(base Value) *Ideal {
	if ir == nil {
		return nil
	}
	for _, id := range ir.ordered {
		if id.Accepts != nil && id.Accepts(base) {
			return id
		}
	}
	return nil
}

// For resolves the Ideal that governs base for dispatch — the first
// matching Ideal (see Match) that is also enabled. Returns nil when no
// enabled kind claims base.
func (ir *IdealRegistry) For(base Value) *Ideal {
	if m := ir.Match(base); m != nil && m.Enabled {
		return m
	}
	return nil
}

// Names returns the registered Ideal names in registration order.
func (ir *IdealRegistry) Names() []string {
	if ir == nil {
		return nil
	}
	names := make([]string, len(ir.ordered))
	for i, id := range ir.ordered {
		names[i] = id.Name
	}
	return names
}
