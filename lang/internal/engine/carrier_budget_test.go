package engine_test

import (
	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"testing"
)

// TestCheckStepBudgetTrip exercises the step-budget safeguard by
// setting a very small CheckStepBudget on the registry and running
// a program that would normally take many engine steps. The check
// loop must emit step_budget_exceeded and halt.
func TestCheckStepBudgetTrip(t *testing.T) {
	r, err := engine.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	r.InitRootContext()
	engine.Register(r)

	r.Check.Mode = true
	r.Check.StepBudget = 5 // tiny — any non-trivial program trips

	// A small arithmetic program: in check mode this takes more
	// than 5 engine steps.
	input := []engine.Value{
		engine.NewInteger(1), engine.NewWord("add"), engine.NewInteger(2),
		engine.NewWord("add"), engine.NewInteger(3),
		engine.NewWord("add"), engine.NewInteger(4),
	}
	input = engine.StripToCarriers(input)
	eng := engine.NewTop(r)
	_, err = eng.Run(input)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	found := false
	for _, d := range r.Check.Diagnostics {
		if d.Code == "step_budget_exceeded" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected step_budget_exceeded diagnostic, got: %+v", r.Check.Diagnostics)
	}
}
