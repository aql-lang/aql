package basic

import (
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
)

// newTestRegistry mirrors the eng test helper of the same name for the
// micron suites that moved here with the family content: a fresh eng
// registry with the root context initialised. The micron machinery a
// registry needs beyond that (the family Ideal) is installed by the
// tests that construct through it.
func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := eng.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// mapOf mirrors the eng test helper: a concrete Map from alternating
// key/value pairs.
func mapOf(pairs ...any) Value {
	m := NewOrderedMap()
	for i := 0; i < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1].(Value))
	}
	return NewMap(m)
}
