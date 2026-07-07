package eng

// Seam-8 (cluster W8_eng_rest): in-package unit tests for the nil / empty
// guards on ModuleRegistry (modules.go). All arms are reached by direct
// calls on zero-value and nil receivers. Per design/TEST-SEAMS.10.md.

import "testing"

func TestW8ModuleRegistryNilGuards(t *testing.T) {
	var nilM *ModuleRegistry

	// InheritConfig: nil receiver / nil parent → no-op, no panic.
	nilM.InheritConfig(NewModuleRegistry())
	NewModuleRegistry().InheritConfig(nil)

	// IsLoaded on a nil receiver and a zero (loaded==nil) receiver.
	if nilM.IsLoaded("x") {
		t.Fatal("nil registry reports nothing loaded")
	}
	if (&ModuleRegistry{}).IsLoaded("x") {
		t.Fatal("zero registry reports nothing loaded")
	}

	// LoadedDesc on nil / zero receiver → (zero, false).
	if _, ok := nilM.LoadedDesc("x"); ok {
		t.Fatal("nil registry has no cached desc")
	}
	if _, ok := (&ModuleRegistry{}).LoadedDesc("x"); ok {
		t.Fatal("zero registry has no cached desc")
	}

	// MarkLoaded on nil receiver → no-op.
	nilM.MarkLoaded("x", ModuleDesc{})

	// NextID on nil receiver → the sentinel "mod_0".
	if got := nilM.NextID(); got != "mod_0" {
		t.Fatalf("nil registry NextID = %q, want mod_0", got)
	}
}

func TestW8ModuleRegistryLoadLifecycle(t *testing.T) {
	// MarkLoaded on a zero registry lazily allocates the loaded map.
	m := &ModuleRegistry{}
	m.MarkLoaded("aql:foo", ModuleDesc{ID: "mod_x"})
	if !m.IsLoaded("aql:foo") {
		t.Fatal("module must report as loaded after MarkLoaded")
	}
	if d, ok := m.LoadedDesc("aql:foo"); !ok || d.ID != "mod_x" {
		t.Fatalf("cached desc mismatch: %+v ok=%v", d, ok)
	}
	// NextID increments from a fresh registry.
	m2 := NewModuleRegistry()
	if got := m2.NextID(); got != "mod_1" {
		t.Fatalf("first NextID = %q, want mod_1", got)
	}
}
