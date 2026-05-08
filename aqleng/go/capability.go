package aqleng

// Capabilities are an opaque plugin slot on the Registry. The engine
// itself doesn't know what capabilities mean — it just stores values
// keyed by name so word handlers can retrieve them at dispatch time.
//
// This is the integration seam between aqleng (the pure execution
// engine) and the host package that supplies real-world services like
// file I/O, format encoders/decoders, or a SQL store. The host
// installs a service:
//
//	r.SetCapability("engine.fileops", myFileOps)
//
// and a word handler retrieves it:
//
//	ops, ok := aqleng.Cap[fileops.FileOps](r, "engine.fileops")
//	if !ok { return nil, fmt.Errorf("read: no fileops capability") }
//
// Names are convention-only — pick stable strings the host package
// owns. Multiple capabilities (multiple file-ops backends, formats
// keyed by name, etc.) are typically grouped behind one capability
// whose value is a map.

// Capability returns the value stored under name and true, or nil and
// false if no capability is registered under that name.
func (r *Registry) Capability(name string) (any, bool) {
	if r == nil || r.capabilities == nil {
		return nil, false
	}
	v, ok := r.capabilities[name]
	return v, ok
}

// SetCapability installs (or replaces) the value under name. Pass
// value=nil to remove the entry.
func (r *Registry) SetCapability(name string, value any) {
	if r == nil {
		return
	}
	if value == nil {
		if r.capabilities != nil {
			delete(r.capabilities, name)
		}
		return
	}
	if r.capabilities == nil {
		r.capabilities = make(map[string]any)
	}
	r.capabilities[name] = value
}

// CapabilityNames returns the set of registered capability names in
// arbitrary order. Useful for debugging and snapshot/restore.
func (r *Registry) CapabilityNames() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.capabilities))
	for k := range r.capabilities {
		names = append(names, k)
	}
	return names
}

// Cap is a typed convenience wrapper around Registry.Capability. It
// returns the capability cast to T plus true on success, or T's zero
// value plus false when the capability is missing or stored under a
// different concrete type.
//
// This is a free function (not a method) because Go does not allow
// generic methods on an existing type.
func Cap[T any](r *Registry, name string) (T, bool) {
	var zero T
	v, ok := r.Capability(name)
	if !ok {
		return zero, false
	}
	t, ok := v.(T)
	if !ok {
		return zero, false
	}
	return t, true
}
