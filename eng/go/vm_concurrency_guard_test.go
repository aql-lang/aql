package eng

import (
	"errors"
	"testing"
)

// RunProgram must refuse to start a compiled run while an INTERPRETER run is
// already in flight on the same registry — the cross-engine race the vmRunning
// CAS alone cannot see (it only guards compiled-vs-compiled). The interpRunDepth
// counter Engine.Run maintains is what makes this detectable. Paired
// positive/negative: refused while a run is active, allowed once it ends.
func TestRunProgramRejectsConcurrentInterpreterRun(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	p := &Program{} // empty program: runs off the end, returns an empty residual

	// NEGATIVE — an interpreter run is in flight (depth > 0): refuse with a
	// concurrency_error, and the vmRunning flag must be released on that return
	// (so the positive case below can still acquire it).
	r.enterInterpRun()
	if _, err := RunProgram(p, r); err == nil {
		r.exitInterpRun()
		t.Fatal("RunProgram succeeded while an interpreter run was active; want concurrency_error")
	} else {
		var ae *AqlError
		if !errors.As(err, &ae) || ae.Code != "concurrency_error" {
			r.exitInterpRun()
			t.Fatalf("wrong error while interpreter active: got %v, want concurrency_error", err)
		}
	}
	r.exitInterpRun()

	// POSITIVE — no interpreter run in flight: the compiled run proceeds (so the
	// negative case is not vacuously "always refuses").
	if _, err := RunProgram(p, r); err != nil {
		t.Fatalf("RunProgram refused on an idle registry: %v", err)
	}
}

// enterInterpRun / exitInterpRun must balance so a normal interpreter Run leaves
// the registry idle for a following compiled run (the standard RunCompiled flow:
// CompileCheck's interpreter check pass completes before RunProgram begins).
func TestInterpRunDepthBalances(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if r.interpRunActive() {
		t.Fatal("fresh registry reports an active interpreter run")
	}
	r.enterInterpRun()
	r.enterInterpRun()
	if !r.interpRunActive() {
		t.Fatal("nested interpreter runs not reported active")
	}
	r.exitInterpRun()
	if !r.interpRunActive() {
		t.Fatal("still one run open, but reported idle")
	}
	r.exitInterpRun()
	if r.interpRunActive() {
		t.Fatal("balanced enter/exit left the registry reporting active")
	}
}
