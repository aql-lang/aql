package eng

// Seam-8 (cluster W8_eng_rest): shared helpers for the W8 seam tests.

import "testing"

// w8reg builds a fresh registry with the root context initialised — the
// standard fixture for kernel unit tests that drive registry-aware paths.
func w8reg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}
