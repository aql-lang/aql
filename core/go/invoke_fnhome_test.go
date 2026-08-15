package core

import "testing"

// FnHome answers "which registry resolves this fn value's free words, and which
// captures ride along" for every native callback seam
// (design/FUNCTION-VALUE-SCOPE.0.md). Its three arms are the three shapes a
// callback seam can be handed, and each has a different correct answer — so
// each is pinned here rather than left to the integration suites, where a
// wrong arm would surface as a mysterious value rather than as this function.
func TestFnHome(t *testing.T) {
	caller, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defining, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	caps := []CapturedBinding{{Name: "x", Value: NewInteger(1)}}

	t.Run("no fn value falls back to the caller", func(t *testing.T) {
		// A synthesized carrier sig with no fn behind it: there is no other
		// registry to route to, and no captures to install.
		got, gotCaps := FnHome(caller, nil)
		if got != caller {
			t.Errorf("registry = %p, want the caller %p", got, caller)
		}
		if gotCaps != nil {
			t.Errorf("captures = %v, want nil", gotCaps)
		}
	})

	t.Run("a fn defined in the running scope stays there", func(t *testing.T) {
		// Registry == nil means "defined where it is running"; the caller IS
		// the defining registry, so routing anywhere else would be wrong.
		got, gotCaps := FnHome(caller, &FnDefInfo{Captured: caps})
		if got != caller {
			t.Errorf("registry = %p, want the caller %p", got, caller)
		}
		if len(gotCaps) != 1 || gotCaps[0].Name != "x" {
			t.Errorf("captures = %v, want the fn's own %v", gotCaps, caps)
		}
	})

	t.Run("a fn from another module routes to its definer", func(t *testing.T) {
		// The case the whole fix exists for: the body's free words must resolve
		// where it was WRITTEN, not where a Go word happens to invoke it.
		got, gotCaps := FnHome(caller, &FnDefInfo{Registry: defining, Captured: caps})
		if got != defining {
			t.Errorf("registry = %p, want the DEFINING registry %p (got the caller: %v)",
				got, defining, got == caller)
		}
		if len(gotCaps) != 1 || gotCaps[0].Name != "x" {
			t.Errorf("captures = %v, want the fn's own %v", gotCaps, caps)
		}
	})
}
