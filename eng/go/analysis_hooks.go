package eng

// The analysis accessor layer — Stage 2a of the four-piece split
// (design/ENG-FOUR-PIECE.0.md seam S1). Every consultation of the
// checker's state that the PURE INTERPRETER makes routes through the
// small surface below, so the core piece's files carry no direct
// CheckState knowledge: this one file concentrates the coupling, and
// its bodies become AnalysisHooks interface calls (with a cached
// activity bool for the hot loop) when the packages cut. Behavior is
// byte-identical to the direct field access it replaces.

import "fmt"

// analysisActive reports whether an analysis pass (check mode) is on —
// the hot-loop probe (Registry.Lookup, the step loop, dispatch).
func (r *Registry) analysisActive() bool { return r.Check.IsActive() }

// analysisRecorder is the emit-recorder reach; never nil (the inactive
// no-op recorder backs a plain pass).
func (r *Registry) analysisRecorder() EmitRecorder { return r.Check.Recorder() }

// analysisMode reports the plain-check flag (Check.Mode) — the
// carrier-modeling probes distinct from full activity.
func (r *Registry) analysisMode() bool { return r.Check.Mode }

// analysisCompiling reports a compile (recording) pass.
func (r *Registry) analysisCompiling() bool { return r.Check.Compiling }

// noteAnalysisDiagnostic forwards a diagnostic to the checker.
func (r *Registry) noteAnalysisDiagnostic(d CheckDiagnostic) { r.Check.AddDiagnostic(d) }

// noteAnalysisUse marks a name as read (unused-def accounting).
func (r *Registry) noteAnalysisUse(name string) { r.Check.recordUse(name) }

// noteSuppressedRuntimeError latches the checker's runtime-suppression
// flag (a would-be runtime error absorbed by the model).
func (r *Registry) noteSuppressedRuntimeError() { r.Check.SuppressedRuntimeError = true }

// noteAmbiguousGradualSplit latches the gradual-split ambiguity flag.
func (r *Registry) noteAmbiguousGradualSplit() { r.Check.AmbiguousGradualSplit = true }

// analysisStepMeter advances the check-mode step budget. It reports
// EXCEEDED (the run should stop) after emitting the one budget
// diagnostic; a run with analysis off never trips.
func (r *Registry) analysisStepMeter() (exceeded bool) {
	if !r.Check.IsActive() {
		return false
	}
	// -1 is the "unset" sentinel; resolve to the project default. A
	// literal 0 is honored as "abort immediately" rather than treated
	// as a magic "use default."
	budget := r.Check.StepBudget
	if budget == -1 {
		budget = DefaultCheckStepBudget
	}
	r.Check.StepCount++
	if r.Check.StepCount <= budget {
		return false
	}
	if !r.Check.BudgetTripped {
		r.Check.BudgetTripped = true
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:   "step_budget_exceeded",
			Detail: fmt.Sprintf("check mode aborted: step budget of %d exceeded", budget),
		})
	}
	return true
}
