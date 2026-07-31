package repl

import (
	"errors"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// TestS7n_DefaultNewRegistryInitFailure drives the DEFAULT newRegistry
// implementation's own error arm (repl.go) by swapping the
// nativeDefaultRegistry seam and calling newRegistry() directly.
// TestStartRegistryError (repl_test.go) replaces newRegistry wholesale
// with a fake, which never exercises this body at all.
func TestS7n_DefaultNewRegistryInitFailure(t *testing.T) {
	orig := nativeDefaultRegistry
	t.Cleanup(func() { nativeDefaultRegistry = orig })
	nativeDefaultRegistry = func(providers ...func(*native.Registry)) (*native.Registry, error) {
		return nil, errors.New("boom-default-registry")
	}

	reg, err := newRegistry()
	if reg != nil {
		t.Errorf("newRegistry() registry = %v, want nil on init failure", reg)
	}
	if err == nil || err.Error() != "boom-default-registry" {
		t.Errorf("newRegistry() err = %v, want boom-default-registry", err)
	}
}
