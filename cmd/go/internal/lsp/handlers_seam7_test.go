package lsp

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// TestS7n_EnsureRegistryFailureLogged drives ensureRegistry's
// init-failure arm (handlers.go) by swapping the nativeDefaultRegistry
// seam: the server must log the error to stderr and return a nil
// registry rather than caching a broken one.
func TestS7n_EnsureRegistryFailureLogged(t *testing.T) {
	orig := nativeDefaultRegistry
	t.Cleanup(func() { nativeDefaultRegistry = orig })
	nativeDefaultRegistry = func(providers ...func(*native.Registry)) (*native.Registry, error) {
		return nil, errors.New("boom-registry")
	}

	var stderr bytes.Buffer
	s := newServer(strings.NewReader(""), io.Discard, &stderr)

	reg := s.ensureRegistry()
	if reg != nil {
		t.Errorf("ensureRegistry() = %v, want nil on init failure", reg)
	}
	if !strings.Contains(stderr.String(), "boom-registry") {
		t.Errorf("stderr = %q, want it to contain boom-registry", stderr.String())
	}
	if s.registry != nil {
		t.Error("s.registry should stay nil after a failed build")
	}
}
