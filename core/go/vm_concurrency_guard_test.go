package core

import (
	"testing"
)

// EnterInterpRun / ExitInterpRun must balance so a normal interpreter Run leaves
// the registry idle for a following compiled run (the standard RunCompiled flow:
// CompileCheck's interpreter check pass completes before RunProgram begins).
func TestInterpRunDepthBalances(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if r.InterpRunActive() {
		t.Fatal("fresh registry reports an active interpreter run")
	}
	r.EnterInterpRun()
	r.EnterInterpRun()
	if !r.InterpRunActive() {
		t.Fatal("nested interpreter runs not reported active")
	}
	r.ExitInterpRun()
	if !r.InterpRunActive() {
		t.Fatal("still one run open, but reported idle")
	}
	r.ExitInterpRun()
	if r.InterpRunActive() {
		t.Fatal("balanced enter/exit left the registry reporting active")
	}
}
