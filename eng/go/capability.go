package eng

import "errors"

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
//	ops, ok, err := eng.Cap[capabilities.FileOps](r, "engine.fileops")
//	if err != nil { return nil, err }
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

// errCapabilityNil is returned by every method when the receiver is
// nil. The production path constructs the registry through
// NewRegistry, so a nil receiver indicates a misconfigured
// (zero-initialised or partially built) Registry — a programming
// error that should surface rather than be silently ignored.
var errCapabilityNil = errors.New("capability: nil registry (registry was not initialised via NewRegistry)")

// NewCapabilityRegistry returns an empty capability registry.
func NewCapabilityRegistry() *CapabilityRegistry {
	return &CapabilityRegistry{store: make(map[string]any)}
}

// Get returns (value, true, nil) when name is bound,
// (nil, false, nil) when no capability is registered under that
// name, or (nil, false, error) when the receiver is nil.
func (c *CapabilityRegistry) Get(name string) (any, bool, error) {
	if c == nil {
		return nil, false, errCapabilityNil
	}
	v, ok := c.store[name]
	return v, ok, nil
}

// Set installs (or replaces) the value under name. To remove a
// capability, call Delete — passing a nil value here STORES nil, it
// does not delete. Returns an error only when the receiver is nil.
func (c *CapabilityRegistry) Set(name string, value any) error {
	if c == nil {
		return errCapabilityNil
	}
	if c.store == nil {
		c.store = make(map[string]any)
	}
	c.store[name] = value
	return nil
}

// Delete removes the entry for name. Returns (true, nil) if a
// capability was present and removed, (false, nil) if no capability
// existed, or (false, error) if the receiver is nil.
func (c *CapabilityRegistry) Delete(name string) (bool, error) {
	if c == nil {
		return false, errCapabilityNil
	}
	if _, ok := c.store[name]; !ok {
		return false, nil
	}
	delete(c.store, name)
	return true, nil
}

// Names returns the set of registered capability names in arbitrary
// order. Useful for debugging and snapshot/restore. Returns an error
// only when the receiver is nil; an empty registry returns
// (nil-or-empty, nil).
func (c *CapabilityRegistry) Names() ([]string, error) {
	if c == nil {
		return nil, errCapabilityNil
	}
	names := make([]string, 0, len(c.store))
	for k := range c.store {
		names = append(names, k)
	}
	return names, nil
}

// Cap is a typed convenience wrapper around r.Capabilities.Get. It
// returns (value, true, nil) on success, (zero, false, nil) when the
// capability is missing or stored under a different concrete type,
// or (zero, false, error) when r or r.Capabilities is nil (a
// misconfigured registry).
//
// This is a free function (not a method) because Go does not allow
// generic methods on an existing type.
func Cap[T any](r *Registry, name string) (T, bool, error) {
	var zero T
	if r == nil {
		return zero, false, errCapabilityNil
	}
	v, ok, err := r.Capabilities.Get(name)
	if err != nil {
		return zero, false, err
	}
	if !ok {
		return zero, false, nil
	}
	t, ok := v.(T)
	if !ok {
		return zero, false, nil
	}
	return t, true, nil
}
