package eng

// Capabilities are an opaque plugin slot on the Registry. The engine
// itself doesn't know what capabilities mean — it just stores values
// keyed by name so word handlers can retrieve them at dispatch time.
//
// This is the integration seam between aqleng (the pure execution
// engine) and the host package that supplies real-world services like
// file I/O, format encoders/decoders, or a SQL store. The host
// installs a service:
//
//	r.Capabilities.Set("engine.fileops", myFileOps)
//
// and a word handler retrieves it:
//
//	ops, ok := eng.Cap[fileops.FileOps](r, "engine.fileops")
//	if !ok { return nil, fmt.Errorf("read: no fileops capability") }
//
// Names are convention-only — pick stable strings the host package
// owns. Multiple capabilities (multiple file-ops backends, formats
// keyed by name, etc.) are typically grouped behind one capability
// whose value is a map.
//
// Set INSTALLS or REPLACES; Delete REMOVES. The two-method form
// removes the ambiguity of a single Set-with-nil overload, which made
// `Set("flag", aMaybeNilPointer)` a footgun: a typed-nil interface
// argument would silently become a delete instead of "store the nil
// value". Storing an explicit nil value is a real operation here.

// CapabilityRegistry is the kernel's plugin slot store. See package
// docs above.
type CapabilityRegistry struct {
	store map[string]any
}

// NewCapabilityRegistry returns an empty capability registry.
func NewCapabilityRegistry() *CapabilityRegistry {
	return &CapabilityRegistry{store: make(map[string]any)}
}

// Get returns the value stored under name and true, or (nil, false)
// if no capability is registered under that name.
func (c *CapabilityRegistry) Get(name string) (any, bool) {
	if c == nil {
		return nil, false
	}
	v, ok := c.store[name]
	return v, ok
}

// Set installs (or replaces) the value under name. To remove a
// capability, call Delete — passing a nil value here STORES nil, it
// does not delete.
func (c *CapabilityRegistry) Set(name string, value any) {
	if c == nil {
		return
	}
	if c.store == nil {
		c.store = make(map[string]any)
	}
	c.store[name] = value
}

// Delete removes the entry for name. Returns true if a capability was
// present and removed, false if no capability existed.
func (c *CapabilityRegistry) Delete(name string) bool {
	if c == nil {
		return false
	}
	if _, ok := c.store[name]; !ok {
		return false
	}
	delete(c.store, name)
	return true
}

// Names returns the set of registered capability names in arbitrary
// order. Useful for debugging and snapshot/restore.
func (c *CapabilityRegistry) Names() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.store))
	for k := range c.store {
		names = append(names, k)
	}
	return names
}

// Cap is a typed convenience wrapper around r.Capabilities.Get. It
// returns the capability cast to T plus true on success, or T's zero
// value plus false when the capability is missing or stored under a
// different concrete type.
//
// This is a free function (not a method) because Go does not allow
// generic methods on an existing type.
func Cap[T any](r *Registry, name string) (T, bool) {
	var zero T
	if r == nil {
		return zero, false
	}
	v, ok := r.Capabilities.Get(name)
	if !ok {
		return zero, false
	}
	t, ok := v.(T)
	if !ok {
		return zero, false
	}
	return t, true
}
