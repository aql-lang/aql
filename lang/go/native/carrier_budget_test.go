package native

import (
	"testing"
)

// TestCheckStepBudgetTrip exercises the step-budget safeguard by
// setting a very small CheckStepBudget on the registry and running
// a program that would normally take many engine steps. The check
// loop must emit step_budget_exceeded and halt.
func TestCheckStepBudgetTrip(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	r.InitRootContext()
	Register(r)

	r.Check.Mode = true
	r.Check.StepBudget = 5 // tiny — any non-trivial program trips

	// A small arithmetic program: in check mode this takes more
	// than 5 engine steps.
	input := []Value{
		NewInteger(1), NewWord("add"), NewInteger(2),
		NewWord("add"), NewInteger(3),
		NewWord("add"), NewInteger(4),
	}
	input = StripToCarriers(input)
	eng := NewTop(r)
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
