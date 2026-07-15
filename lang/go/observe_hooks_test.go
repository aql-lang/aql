package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The lang-level forwarders for the eng observability seams
// (eng interp_entry.go): the frontier suite arms them through (*AQL).

// A plain interpreted Run reports Engine.Run entries through the forwarder;
// disarm stops recording.
func TestArmInterpEntryHookForwarder(t *testing.T) {
	a := mustNew(t)
	var entries []InterpEntry
	disarm := a.ArmInterpEntryHook(func(e InterpEntry) { entries = append(entries, e) })

	if _, err := a.RunInterp(`1 add 2`); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sawRun bool
	for _, e := range entries {
		if e.Seam == "Engine.Run" {
			sawRun = true
		}
	}
	if !sawRun {
		t.Fatalf("forwarded hook missed Engine.Run: %+v", entries)
	}

	before := len(entries)
	disarm()
	if _, err := a.RunInterp(`2 add 3`); err != nil {
		t.Fatalf("post-disarm Run: %v", err)
	}
	if len(entries) != before {
		t.Fatalf("disarmed forwarded hook still recorded: %d -> %d", before, len(entries))
	}
}

// The refined-return rematch MATCH (TestDispatchRematchMatchDefers' shape) is
// a real compiled program whose OpDispatchRematch defers at run time: the
// forwarded bail hook must see exactly the vm:rematch-matched site, and the
// run must still resolve to the interpreter's correct result via the
// effect-free silent fallback. (The zz-inst shape-claim violation that used
// to sit here was reclassified 2026-07-15: a host handler violating its own
// registered signature is the host-contract internal_error class, not a
// designed model-miss bail, so it no longer feeds the census.)
func TestArmRuntimeBailHookForwarder(t *testing.T) {
	a := mustNew(t)
	var bails []BailEvent
	defer a.ArmRuntimeBailHook(func(e BailEvent) { bails = append(bails, e) })()

	const src = `def Pos (refine Integer) def mk fn [[n:Integer][Integer][def y:Pos n y]] def g fn [[p:Pos][Integer][99]] g (mk 5)`
	got, compiled, err := a.RunCompiled(src)
	if err != nil || compiled {
		t.Fatalf("rematch-match run: compiled=%v err=%v", compiled, err)
	}
	if fmt.Sprint(got) != "[99]" {
		t.Fatalf("rematch-match fallback result = %v, want [99]", got)
	}
	if len(bails) != 1 || bails[0].Site != "vm:rematch-matched" {
		t.Fatalf("bail events = %+v, want exactly one vm:rematch-matched", bails)
	}
	if !strings.Contains(bails[0].Reason, "matched at run time") {
		t.Fatalf("bail reason = %q, want the rematch-matched message", bails[0].Reason)
	}
}
