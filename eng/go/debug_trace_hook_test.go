package eng

import "testing"

// TestRegistryDebugTraceHook pins the registry-level debug step hook
// (SetDebugTrace): engines constructed on the registry — and pooled
// sub-engines as they are re-taken — start with it as their trace, an
// engine's own SetTrace overrides it, and clearing the hook must not
// leak a stale callback on a pooled engine's reuse.
func TestRegistryDebugTraceHook(t *testing.T) {
	r := poolTestRegistry(t)
	fires := 0
	r.SetDebugTrace(func(int, int, []Value, string) { fires++ })

	// The pooled sub-evaluation seam fires the hook.
	if _, err := runPooledSub(r, []Value{NewInteger(1)}, false); err != nil {
		t.Fatal(err)
	}
	if fires == 0 {
		t.Fatal("the hook must fire on a pooled sub-run")
	}

	// NewTop (the CallAQL / top-level constructor) inherits it too.
	fires = 0
	if _, err := NewTop(r).Run([]Value{NewInteger(2)}); err != nil {
		t.Fatal(err)
	}
	if fires == 0 {
		t.Error("NewTop must inherit the registry hook")
	}

	// An engine's own SetTrace overrides the inherited hook (dedicated
	// tracers like IO.trace / Debug.step keep their private callbacks).
	fires = 0
	e := New(r)
	e.SetTrace(nil)
	if _, err := e.Run([]Value{NewInteger(3)}); err != nil {
		t.Fatal(err)
	}
	if fires != 0 {
		t.Error("SetTrace must override the registry hook for that engine")
	}

	// Negative (pool-reuse leak): clearing the hook must clear POOLED
	// engines as they are re-taken — a stale callback must never replay.
	r.SetDebugTrace(nil)
	fires = 0
	if _, err := runPooledSub(r, []Value{NewInteger(4)}, false); err != nil {
		t.Fatal(err)
	}
	if fires != 0 {
		t.Error("a cleared hook must not leak on a pooled engine's reuse")
	}

	// New(nil) stays constructible (the nil-registry guard).
	if New(nil) == nil || NewTop(nil) == nil {
		t.Error("constructors must tolerate a nil registry")
	}
}
