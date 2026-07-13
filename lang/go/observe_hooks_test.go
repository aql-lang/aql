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

	if _, err := a.Run(`1 add 2`); err != nil {
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
	if _, err := a.Run(`2 add 3`); err != nil {
		t.Fatalf("post-disarm Run: %v", err)
	}
	if len(entries) != before {
		t.Fatalf("disarmed forwarded hook still recorded: %d -> %d", before, len(entries))
	}
}

// The zz-inst shape-claim violation (bytecode_methodshape_test.go) is a real
// compiled program whose OpCallDynMethod defers at run time: the forwarded
// bail hook must see exactly the vm:shaped-method site, and the run must
// still resolve to the interpreter's correct result (the effect-free silent
// fallback pinned by TestRuntimeBailBeforeEffectStillFallsBack).
func TestArmRuntimeBailHookForwarder(t *testing.T) {
	a := zzShapedInstance(t)
	var bails []BailEvent
	defer a.ArmRuntimeBailHook(func(e BailEvent) { bails = append(bails, e) })()

	got, compiled, err := a.RunCompiled(`def i (zz-inst) ; i.m 5 ; 42`)
	if err != nil || compiled {
		t.Fatalf("zz-inst run: compiled=%v err=%v", compiled, err)
	}
	if fmt.Sprint(got) != "[7 42]" {
		t.Fatalf("zz-inst fallback result = %v, want [7 42]", got)
	}
	if len(bails) != 1 || bails[0].Site != "vm:shaped-method" {
		t.Fatalf("bail events = %+v, want exactly one vm:shaped-method", bails)
	}
	if !strings.Contains(bails[0].Reason, "shape claim") {
		t.Fatalf("bail reason = %q, want the shape-claim message", bails[0].Reason)
	}
}
