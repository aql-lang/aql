package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Capture-bearing detached stamps (plan Phase 6.3): StampDetachedFn compiles
// a CAPTURING body — fd.Captured rides compileClosureBody's capture slots
// (the OpPushClosure layout) and the ref carries the captured VALUES so
// RunUnit/runUnitNested bind them at every invoke. The pins: the handler
// stamps (under the "(anonymous fn)" display name), the stamped VM run
// returns the CAPTURED value, and two fn values from the same factory carry
// their OWN captures (per-value refs — StampFnValue clones per value).
func TestCapturingHandlerStampsAndRunsWithCaptures(t *testing.T) {
	const factory = `def mk (fn [[n:Integer] [Any] [ def svc (service {}) add {cmd:"N"} ([req:Map state:Any] => [ n ]) svc svc ]]) `

	a := mustNew(t)
	disarm := a.ArmRuntimeStamping()
	out, err := a.RunInterp(factory + `def s7 (mk 7) def s9 (mk 9) [(call {cmd:"N"} s7) (call {cmd:"N"} s9)]`)
	disarm()
	if err != nil {
		t.Fatalf("armed run: %v", err)
	}
	if fmt.Sprint(out) != "[[7 9]]" {
		t.Errorf("per-value captures: got %v, want [[7 9]] (each service returns ITS n)", out)
	}
	stamped := 0
	for _, ev := range a.StampReport() {
		if ev.Stamped && strings.Contains(ev.Name, "anonymous fn") {
			stamped++
		}
	}
	if stamped < 2 {
		t.Errorf("want both capturing handlers stamped under the anonymous display name, got %d (report %+v)", stamped, a.StampReport())
	}

	// Parity negative: the unarmed interpreter run computes the identical
	// values with zero stamp attempts.
	b := mustNew(t)
	outI, errI := b.RunInterp(factory + `def s7 (mk 7) def s9 (mk 9) [(call {cmd:"N"} s7) (call {cmd:"N"} s9)]`)
	if errI != nil || fmt.Sprint(outI) != fmt.Sprint(out) {
		t.Errorf("parity: armed=%v unarmed=%v (err=%v)", out, outI, errI)
	}
	if got := b.StampReport(); len(got) != 0 {
		t.Errorf("unarmed run must not stamp, got %+v", got)
	}
}
