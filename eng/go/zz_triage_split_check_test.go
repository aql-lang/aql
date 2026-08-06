package eng

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

import (
	"sync"
	"testing"
)

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// vmDefer is the designed-defer choke point: it reports the BailEvent to an
// armed hook and returns the internal_error class runtimeShouldFallback
// resolves; unarmed it just builds the error.
func TestRuntimeBailHookVmDefer(t *testing.T) {
	r := runUnitReg(t)
	var (
		mu     sync.Mutex
		events []BailEvent
	)
	disarm := r.ArmRuntimeBailHook(func(e BailEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	})

	err := vmDefer(r, nil, 0, "vm:test-site", "test defer message")
	if !IsInternalErr(err) {
		t.Fatalf("vmDefer error class = %v, want internal_error", err)
	}
	if len(events) != 1 || events[0].Site != "vm:test-site" || events[0].Reason != "test defer message" {
		t.Fatalf("bail events = %+v, want one vm:test-site event", events)
	}

	disarm()
	if err := vmDefer(r, nil, 0, "vm:test-site", "again"); !IsInternalErr(err) {
		t.Fatalf("disarmed vmDefer error class = %v, want internal_error", err)
	}
	if len(events) != 1 {
		t.Fatalf("disarmed vmDefer still recorded: %d events", len(events))
	}
}

// InheritObserveHooks shares the parent's holders into a module sub-registry:
// entries and bails emitted on the child reach the parent's observers.
func TestInheritObserveHooks(t *testing.T) {
	parent := runUnitReg(t)
	child := runUnitReg(t)
	var c entryCollector
	var bails []BailEvent
	defer parent.ArmInterpEntryHook(c.add)()
	defer parent.ArmRuntimeBailHook(func(e BailEvent) { bails = append(bails, e) })()

	child.InheritObserveHooks(parent)
	if _, err := RunPooledSub(child, []Value{NewInteger(2)}, false); err != nil {
		t.Fatalf("child runPooledSub: %v", err)
	}
	if len(c.entries) == 0 {
		t.Fatal("child interp entries did not reach the parent's hook")
	}
	_ = vmDefer(child, nil, 0, "vm:test-site", "child defer")
	if len(bails) != 1 {
		t.Fatalf("child bail did not reach the parent's hook: %+v", bails)
	}
}
