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

// noteAnalysisFnBinder attributes a body-local def to its enclosing fn
// for the dynamic-scope undefined-word rescue (check mode only; no-op
// at the top level or outside check).
func (r *Registry) noteAnalysisFnBinder(name string) { r.Check.RecordFnBinder(name) }

// analysisInCondBody reports whether the analysed position sits inside
// a conditional body (branch/loop) — the compile-soundness probe for
// overload redefinition.
func (r *Registry) analysisInCondBody() bool { return r.Check.CondBodyDepth > 0 }

// analysisSnapshot captures the checker's per-call state for the
// predicate sandbox; restoreAnalysisSnapshot rolls it back IN PLACE
// (not by swapping the pointer) so a module sub-registry transiently
// sharing the state observes the rollback too
// (design/module-fn-checkstate-ownership.1.md §3.2).
func (r *Registry) analysisSnapshot() *CheckState { return r.Check.Clone() }

func (r *Registry) restoreAnalysisSnapshot(s *CheckState) {
	if s != nil && r.Check != nil {
		*r.Check = *s
	} else {
		r.Check = s
	}
}

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

// analysisFnConstructionPass runs the check piece's construction-time
// static body pass for a newly installed fn (no-op outside check mode).
func (r *Registry) analysisFnConstructionPass(name string, fnDef FnDefInfo) {
	analysisImpl.fnConstructionPass(r, name, fnDef)
}

// analysisReturnsFn builds the check piece's per-signature analysis
// model (the ReturnsFunc baked onto every boru-bodied signature).
func (r *Registry) analysisReturnsFn(name string, s FnSig, fnDef FnDefInfo) ReturnsFunc {
	return analysisImpl.returnsFn(r, name, s, fnDef)
}

// analysisStripToCarriers replaces concrete run inputs with their
// check-mode carriers at the Run boundary (active analysis only —
// callers gate).
func (r *Registry) analysisStripToCarriers(in []Value) []Value {
	return analysisImpl.stripToCarriers(in)
}

// analysisZeroOutResiduals drops phantom 0-output statement residues
// from the top-level residual (recording passes).
func (r *Registry) analysisZeroOutResiduals(stk []Value) []Value {
	return analysisImpl.zeroOutResiduals(r, stk)
}

// analysisCarrierResults models a matched dispatch's results under
// analysis — the carrier-propagation seam of execMatch.
func (r *Registry) analysisCarrierResults(word string, sig *Signature, args []Value, pos SrcPos, ownerReg *Registry, tailConsumed bool) []Value {
	return analysisImpl.carrierResults(r, word, sig, args, pos, ownerReg, tailConsumed)
}

// analysisMixedConform reports whether v is a genuinely MIXED gradual
// carrier conforming to t (matchSignature's gradual arm).
func (r *Registry) analysisMixedConform(v Value, t *Type) bool {
	return analysisImpl.mixedConform(v, t)
}

// analysisValueCarriesCarrier reports whether v transitively contains
// a carrier (the interp-string dynamic-collapse probe).
func (r *Registry) analysisValueCarriesCarrier(v Value) bool {
	return analysisImpl.valueCarriesCarrier(v)
}

// analysisAtUncaughtTopLevel reports unconditional top-level reach
// outside any error-trapping region (the guaranteed-error mirrors'
// reachability gate).
func (r *Registry) analysisAtUncaughtTopLevel() bool { return analysisImpl.atUncaughtTopLevel(r) }

// noteAnalysisUniqueDiagnostic forwards a diagnostic deduped against
// the accumulated list (code+detail+word+position).
func (r *Registry) noteAnalysisUniqueDiagnostic(d CheckDiagnostic) { analysisImpl.addUnique(r, d) }

// analysisImpl is the S1 implementation table: the check piece installs
// the real analysis operations at init (installAnalysisImpl,
// check_recovery.go), and the accessor bodies above dispatch through
// it. The inactive defaults keep a check-less core linkable and are the
// exact no-ops an inactive pass produces; TestInactiveAnalysisImpl pins
// them.
var analysisImpl = struct {
	fnConstructionPass  func(r *Registry, name string, fnDef FnDefInfo)
	returnsFn           func(r *Registry, name string, s FnSig, fnDef FnDefInfo) ReturnsFunc
	stripToCarriers     func(in []Value) []Value
	zeroOutResiduals    func(r *Registry, stk []Value) []Value
	carrierResults      func(r *Registry, word string, sig *Signature, args []Value, pos SrcPos, ownerReg *Registry, tailConsumed bool) []Value
	mixedConform        func(v Value, t *Type) bool
	valueCarriesCarrier func(v Value) bool
	atUncaughtTopLevel  func(r *Registry) bool
	addUnique           func(r *Registry, d CheckDiagnostic)
}{
	fnConstructionPass:  inactiveFnConstructionPass,
	returnsFn:           inactiveReturnsFn,
	stripToCarriers:     inactiveStripToCarriers,
	zeroOutResiduals:    inactiveZeroOutResiduals,
	carrierResults:      inactiveCarrierResults,
	mixedConform:        inactiveMixedConform,
	valueCarriesCarrier: inactiveValueCarriesCarrier,
	atUncaughtTopLevel:  inactiveAtUncaughtTopLevel,
	addUnique:           inactiveAddUnique,
}

// The inactive defaults are NAMED so the seam test pins them directly;
// the check piece's init replaces the table while it is linked, leaving
// these reachable only on a check-less core build.
func inactiveFnConstructionPass(*Registry, string, FnDefInfo)           {}
func inactiveReturnsFn(*Registry, string, FnSig, FnDefInfo) ReturnsFunc { return nil }
func inactiveStripToCarriers(in []Value) []Value                        { return in }
func inactiveZeroOutResiduals(_ *Registry, stk []Value) []Value         { return stk }
func inactiveCarrierResults(*Registry, string, *Signature, []Value, SrcPos, *Registry, bool) []Value {
	return nil
}
func inactiveMixedConform(Value, *Type) bool       { return false }
func inactiveValueCarriesCarrier(Value) bool       { return false }
func inactiveAtUncaughtTopLevel(*Registry) bool    { return false }
func inactiveAddUnique(*Registry, CheckDiagnostic) {}
