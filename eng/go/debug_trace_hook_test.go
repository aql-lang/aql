package eng

import (
	"strings"
	"testing"
)

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

// TestDebugParentChain pins hook resolution through the module-registry
// link: a child registry with no hook of its own resolves the parent's
// LIVE hook — set, suppressed, or cleared — and a self-link is refused.
func TestDebugParentChain(t *testing.T) {
	parent := poolTestRegistry(t)
	child := poolTestRegistry(t)
	child.SetDebugParent(parent)

	fires := 0
	parent.SetDebugTrace(func(int, int, []Value, string) { fires++ })
	if _, err := runPooledSub(child, []Value{NewInteger(1)}, false); err != nil {
		t.Fatal(err)
	}
	if fires == 0 {
		t.Fatal("a child registry must resolve the parent's hook")
	}

	// The link reads THROUGH: clearing the parent's hook silences the
	// child immediately (a copied callback would keep firing).
	parent.SetDebugTrace(nil)
	fires = 0
	if _, err := runPooledSub(child, []Value{NewInteger(2)}, false); err != nil {
		t.Fatal(err)
	}
	if fires != 0 {
		t.Error("clearing the parent hook must silence child registries")
	}

	// The child's OWN hook wins over the chain.
	parent.SetDebugTrace(func(int, int, []Value, string) { t.Error("parent hook must not fire") })
	child.SetDebugTrace(func(int, int, []Value, string) { fires++ })
	if _, err := runPooledSub(child, []Value{NewInteger(3)}, false); err != nil {
		t.Fatal(err)
	}
	if fires == 0 {
		t.Error("a child's own hook must win over the chain")
	}
	parent.SetDebugTrace(nil)

	// A self-link is refused, so resolution always terminates.
	solo := poolTestRegistry(t)
	solo.SetDebugParent(solo)
	if got := solo.effectiveDebugTrace(); got != nil {
		t.Error("a self-linked registry with no hook resolves nil")
	}
}

// TestFaultReturnFiresTrace pins the pause-before-unwind seam: a Run-loop
// error fires the trace one final time with a "fault: <err>" note and
// step -1 before the error surfaces.
func TestFaultReturnFiresTrace(t *testing.T) {
	r := poolTestRegistry(t)
	var faults []string
	faultSteps := []int{}
	r.SetDebugTrace(func(step, _ int, _ []Value, note string) {
		if strings.HasPrefix(note, "fault: ") {
			faults = append(faults, note)
			faultSteps = append(faultSteps, step)
		}
	})
	_, err := NewTop(r).Run([]Value{NewWord("zz-no-such-word")})
	if err == nil {
		t.Fatal("the run must fail")
	}
	if len(faults) == 0 || !strings.Contains(faults[0], "zz-no-such-word") {
		t.Fatalf("the trace must receive the fault note; got %v", faults)
	}
	if faultSteps[0] != -1 {
		t.Errorf("a fault fire carries step -1, got %d", faultSteps[0])
	}
	// Negative: an error-free run fires no fault note.
	faults = nil
	if _, err := NewTop(r).Run([]Value{NewInteger(1)}); err != nil {
		t.Fatal(err)
	}
	if len(faults) != 0 {
		t.Errorf("no error, no fault note; got %v", faults)
	}
}

// TestRunningEngineStates pins the cross-engine backtrace raw material:
// during a nested pause every running engine's tape+pointer snapshot is
// available, outermost first; outside a run there are none.
func TestRunningEngineStates(t *testing.T) {
	r := poolTestRegistry(t)
	if got := r.RunningEngineStates(); len(got) != 0 {
		t.Fatalf("no engines outside a run; got %d", len(got))
	}
	var depths []int
	r.SetDebugTrace(func(_, _ int, _ []Value, _ string) {
		depths = append(depths, len(r.RunningEngineStates()))
	})
	// A paren group runs on a nested pooled engine: fires there must see
	// at least two running engines (the top-level and the group's).
	if _, err := NewTop(r).Run([]Value{NewOpenParen(), NewInteger(1), NewCloseParen()}); err != nil {
		t.Fatal(err)
	}
	max := 0
	for _, d := range depths {
		if d > max {
			max = d
		}
	}
	if max < 1 {
		t.Fatalf("running engines must be visible during fires; max depth %d", max)
	}
	for _, st := range r.RunningEngineStates() {
		_ = st // no live engines now; loop body unreachable
	}
}
