package lang

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestRegionCaptureFiresOnRealPrograms is the end-to-end pin for Stage 4's
// Phase-A region capture. The unit tests in compiler/go drive
// tryRecordRegion directly over a hand-built window, which proves the
// function but not the WIRING — that core's RegionRecorder seam is installed,
// fires from the real dispatch path, and reaches the live EmitState.
//
// It lives in lang because that is the only layer where all three exist at
// once: the interpreter that fires the seam, the compiler that seats on it,
// and a registry a program actually ran on.
func TestRegionCaptureFiresOnRealPrograms(t *testing.T) {
	run := func(t *testing.T, src string) *compiler.EmitState {
		t.Helper()
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := b.RunCompiled(src); err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		es, _ := b.NativeRegistry().Check.Recorder().(*compiler.EmitState)
		return es
	}

	t.Run("a forward-collecting dispatch is captured", func(t *testing.T) {
		es := run(t, `add 1 2`)
		if es == nil {
			t.Fatal("no EmitState after the run — the seam had nothing to record onto")
		}
		if es.PendingRegionCount() == 0 {
			t.Fatal("`add 1 2` forward-collects, so it must produce a region capture")
		}
		// The join Phase B will use: the dispatching word's own position.
		d, ok := es.TakePendingRegion("add", core.SrcPos{Row: 1, Col: 1})
		if !ok {
			t.Fatal("no capture under (add, 1:1) — the join key Phase B looks up by")
		}
		if len(d.Slots) != 2 {
			t.Fatalf("captured %d slots for `add 1 2`, want 2", len(d.Slots))
		}
		if d.Lead != compiler.LeadWord || d.Word != "add" {
			t.Errorf("lead = %v/%q, want LeadWord/add", d.Lead, d.Word)
		}
	})

	t.Run("a fn body's own dispatch is captured too", func(t *testing.T) {
		es := run(t, `def f fn [[a:Integer b:Integer][Integer][add a b]] end f 1 2`)
		if es == nil || es.PendingRegionCount() < 2 {
			n := 0
			if es != nil {
				n = es.PendingRegionCount()
			}
			t.Fatalf("captured %d regions, want at least 2 (the call and the body's add)", n)
		}
	})
}
