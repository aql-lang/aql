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
//	r.SetCapability("engine.fileops", myFileOps)
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
// SetCapability INSTALLS or REPLACES; DeleteCapability REMOVES. The
// previous version overloaded SetCapability(name, nil) to also delete,
// which made `SetCapability("flag", aMaybeNilPointer)` a footgun: a
// typed-nil interface argument silently became a delete instead of a
// "store the nil value" call. The two-method form removes the
// ambiguity. Storing an explicit nil value is now a real operation
// (the capability is present and its value is nil).

// Capability returns the value stored under name and true, or nil and
// false if no capability is registered under that name.
func (r *Registry) Capability(name string) (any, bool) {
	if r == nil || r.capabilities == nil {
		return nil, false
	}
	v, ok := r.capabilities[name]
	return v, ok
}

// SetCapability installs (or replaces) the value under name. To
// remove a capability, call DeleteCapability — passing a nil value
// here STORES nil, it does not delete.
func (r *Registry) SetCapability(name string, value any) {
	if r == nil {
		return
	}
	if r.capabilities == nil {
		r.capabilities = make(map[string]any)
	}
	r.capabilities[name] = value
}

// DeleteCapability removes the entry for name. Returns true if a
// capability was present and removed, false if no capability existed.
func (r *Registry) DeleteCapability(name string) bool {
	if r == nil || r.capabilities == nil {
		return false
	}
	if _, ok := r.capabilities[name]; !ok {
		return false
	}
	delete(r.capabilities, name)
	return true
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
