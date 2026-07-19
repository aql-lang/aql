package eng

import (
	"sort"
	"testing"
)

// The line-coverage hook (coverage.go): arm/disarm, cover-id tagging, and the
// noteCoverage emission gates (unarmed, untagged, zero-row, check-mode, fire).
// Each positive path pairs with its negative twin.

func covReg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return r
}

// ArmCoverageHook + noteCoverage fire path: an armed, tagged, non-check-mode
// registry records each executed row; the disarm func stops it.
func TestCoverageHookArmAndFire(t *testing.T) {
	r := covReg(t)
	r.SetCoverID("mod")
	if r.CoverID() != "mod" {
		t.Fatalf("CoverID = %q, want mod", r.CoverID())
	}

	var rows []int
	disarm := r.ArmCoverageHook(func(id string, row int) {
		if id != "mod" {
			t.Errorf("cover id = %q, want mod", id)
		}
		rows = append(rows, row)
	})
	r.noteCoverage(SrcPos{Row: 5})
	r.noteCoverage(SrcPos{Row: 9})
	r.noteCoverage(SrcPos{Row: 5})
	disarm()
	r.noteCoverage(SrcPos{Row: 42}) // after disarm — must not record

	sort.Ints(rows)
	if len(rows) != 3 || rows[0] != 5 || rows[1] != 5 || rows[2] != 9 {
		t.Fatalf("recorded rows = %v, want [5 5 9]", rows)
	}
}

// The emission gates each SUPPRESS a fire: unarmed, untagged (no cover id),
// zero-row position, and check mode.
func TestCoverageHookGates(t *testing.T) {
	fired := 0
	record := func(string, int) { fired++ }

	// Unarmed: no fire (hook holder present but fn nil).
	r := covReg(t)
	r.SetCoverID("mod")
	r.noteCoverage(SrcPos{Row: 1})
	if fired != 0 {
		t.Fatalf("unarmed registry fired %d times", fired)
	}

	// Untagged: armed but no cover id → no fire.
	r2 := covReg(t)
	r2.ArmCoverageHook(record)
	r2.noteCoverage(SrcPos{Row: 1})
	if fired != 0 {
		t.Fatalf("untagged registry fired %d times", fired)
	}

	// Zero-row position: armed + tagged but Row 0 → no fire (unknown position).
	r3 := covReg(t)
	r3.SetCoverID("mod")
	r3.ArmCoverageHook(record)
	r3.noteCoverage(SrcPos{Row: 0})
	if fired != 0 {
		t.Fatalf("zero-row position fired %d times", fired)
	}
	// ... but a real row on the same registry DOES fire.
	r3.noteCoverage(SrcPos{Row: 7})
	if fired != 1 {
		t.Fatalf("armed+tagged real row fired %d times, want 1", fired)
	}

	// Check mode: static-analysis re-run is never counted.
	r4 := covReg(t)
	r4.SetCoverID("mod")
	r4.ArmCoverageHook(record)
	r4.Check.Mode = true
	r4.noteCoverage(SrcPos{Row: 3})
	if fired != 1 {
		t.Fatalf("check-mode step fired (total %d, want still 1)", fired)
	}
	r4.Check.Mode = false
	r4.noteCoverage(SrcPos{Row: 3})
	if fired != 2 {
		t.Fatalf("execute-mode step did not fire (total %d, want 2)", fired)
	}
}

// Nil-receiver and holderless registries stay inert (no panic).
func TestCoverageHookNilSafety(t *testing.T) {
	var nilR *Registry
	nilR.SetCoverID("x")              // no panic
	nilR.noteCoverage(SrcPos{Row: 1}) // no panic
	if nilR.CoverID() != "" {
		t.Fatalf("nil registry cover id = %q, want empty", nilR.CoverID())
	}
	if disarm := nilR.ArmCoverageHook(func(string, int) {}); disarm == nil {
		t.Fatalf("nil registry ArmCoverageHook returned nil disarm")
	} else {
		disarm() // no panic
	}

	// A registry with no coverHook holder (constructed without NewRegistry)
	// arms nothing and never fires.
	bare := &Registry{}
	if d := bare.ArmCoverageHook(func(string, int) {}); d != nil {
		d()
	}
	bare.noteCoverage(SrcPos{Row: 1}) // holder nil → inert
}

// InheritObserveHooks shares the coverage hook holder AND the source registry
// into a sub-registry, so a module body running on the sub-registry reports to
// the parent's armed hook and the parent sees the module's registered source.
func TestCoverageHookInherited(t *testing.T) {
	parent := covReg(t)
	var rows []int
	parent.ArmCoverageHook(func(_ string, row int) { rows = append(rows, row) })

	child := covReg(t)
	child.InheritObserveHooks(parent)
	child.SetCoverID("child-mod")
	child.noteCoverage(SrcPos{Row: 11})

	if len(rows) != 1 || rows[0] != 11 {
		t.Fatalf("inherited hook recorded %v, want [11]", rows)
	}

	// A source registered on the child is visible on the shared parent holder.
	child.RegisterCoverSource("child-mod", "def x 1")
	if src, ok := parent.CoverSource("child-mod"); !ok || src != "def x 1" {
		t.Fatalf("inherited source registry: got (%q,%v), want (\"def x 1\",true)", src, ok)
	}
}

// RegisterCoverSource / CoverSource: register, overwrite (last write wins),
// miss, empty-id skip, and nil/holderless inertness.
func TestCoverSourceRegistry(t *testing.T) {
	r := covReg(t)
	if _, ok := r.CoverSource("m"); ok {
		t.Fatalf("unregistered id must miss")
	}
	r.RegisterCoverSource("m", "src-1")
	r.RegisterCoverSource("m", "src-2") // last write wins
	if src, ok := r.CoverSource("m"); !ok || src != "src-2" {
		t.Fatalf("got (%q,%v), want (src-2,true)", src, ok)
	}
	r.RegisterCoverSource("", "ignored") // empty id → no-op
	if _, ok := r.CoverSource(""); ok {
		t.Fatalf("empty id must not register")
	}

	// nil receiver + holderless registry stay inert (no panic, always miss).
	var nilR *Registry
	nilR.RegisterCoverSource("m", "x")
	if _, ok := nilR.CoverSource("m"); ok {
		t.Fatalf("nil registry reports a source")
	}
	bare := &Registry{}
	bare.RegisterCoverSource("m", "x")
	if _, ok := bare.CoverSource("m"); ok {
		t.Fatalf("holderless registry reports a source")
	}
}
