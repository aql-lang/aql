package core

// Engine dispatch-hook slots — Stage 3b of the four-piece split
// (design/ENG-FOUR-PIECE.0.md seam S9). A compiler-piece behavior the
// core step loop must be able to OFFER without naming compiler symbols
// registers itself here at init; a nil slot simply declines. At the
// package cut these become the compiler's registrations onto core's
// exported hook points.

// DriftWindowRecorder is the compiler's stack-drift island hook: offered
// a matched dispatch whose forward window drifted, it may record the
// window as a runtime island (drift_window.go) and report true to skip
// the refusal path. Installed by the compiler piece's init; nil declines.
var DriftWindowRecorder func(e *Engine, w WordInfo, sig *Signature, positions []int) bool

// CheckBraid is the S9 dispatch-hook table for the check piece's
// dispatch-recovery braid: the step loop OFFERS each recovery/model
// point through these slots, and the check piece installs its
// implementations at init (installCheckBraid, check_recovery.go). The
// slots are nil only on a core build linked without the check piece —
// a configuration where analysis mode cannot be meaningfully armed.
var CheckBraid = struct {
	CheckMixedFormAdvisories     func(e *Engine, w WordInfo, sig *Signature, positions []int, pos SrcPos, fwdCount, stkCount int)
	CheckModeAssumeSig           func(e *Engine, w WordInfo, fn *FnDefInfo, fallback *Signature, pos SrcPos) error
	CheckModeFallbackPositions   func(e *Engine, n int) []int
	CheckModeParenFnCollapse     func(e *Engine, openIdx, closeIdx int) int
	CheckModeSurfaceShape        func(e *Engine, w WordInfo, pos SrcPos) (bool, error)
	ConcreteEvalOnce             func(e *Engine, items []Value) (Value, bool)
	DrainUndefinedAtoms          func(e *Engine)
	ExprRefsCarrier              func(e *Engine, items []Value) bool
	NoteSpeculativeBarrierCommit func(e *Engine, fwd ForwardInfo)
	RefuseForwardStackDrift      func(e *Engine, sig *Signature, positions []int)
	RefuseStrandedMemberFn       func(e *Engine, positions []int)
	ShareCheckState              func(e *Engine, capturedReg *Registry) func()
	SpliceAnonCheckResult        func(e *Engine, valIdx, nArgs int, sig *FnSig, args []Value, captures []CapturedBinding) error
	SpliceCheckResults           func(e *Engine, positions []int, results []Value)
	SpliceFnValueCheckResult     func(e *Engine, valIdx, nArgs int, fnDef FnDefInfo, sig *FnSig, args []Value) error
	TagCheckModeDefRead          func(e *Engine, top *Value, name string)
	TryDynamicFnValueDispatch    func(e *Engine, valIdx int) bool
	TryMemberFnArrivalDispatch   func(e *Engine, valIdx int) bool
	TryShapedMethodDispatch      func(e *Engine, valIdx int) bool
	UndefinedWordCheckDiag       func(e *Engine, name string, pos SrcPos) CheckDiagnostic
}{
	CheckMixedFormAdvisories:     inactiveCheckMixedFormAdvisories,
	CheckModeAssumeSig:           inactiveCheckModeAssumeSig,
	CheckModeFallbackPositions:   inactiveCheckModeFallbackPositions,
	CheckModeParenFnCollapse:     inactiveCheckModeParenFnCollapse,
	CheckModeSurfaceShape:        inactiveCheckModeSurfaceShape,
	ConcreteEvalOnce:             inactiveConcreteEvalOnce,
	DrainUndefinedAtoms:          inactiveDrainUndefinedAtoms,
	ExprRefsCarrier:              inactiveExprRefsCarrier,
	NoteSpeculativeBarrierCommit: inactiveNoteSpeculativeBarrierCommit,
	RefuseForwardStackDrift:      inactiveRefuseForwardStackDrift,
	RefuseStrandedMemberFn:       inactiveRefuseStrandedMemberFn,
	ShareCheckState:              inactiveShareCheckState,
	SpliceAnonCheckResult:        inactiveSpliceAnonCheckResult,
	SpliceCheckResults:           inactiveSpliceCheckResults,
	SpliceFnValueCheckResult:     inactiveSpliceFnValueCheckResult,
	TagCheckModeDefRead:          inactiveTagCheckModeDefRead,
	TryDynamicFnValueDispatch:    inactiveTryDynamicFnValueDispatch,
	TryMemberFnArrivalDispatch:   inactiveTryMemberFnArrivalDispatch,
	TryShapedMethodDispatch:      inactiveTryShapedMethodDispatch,
	UndefinedWordCheckDiag:       inactiveUndefinedWordCheckDiag,
}

// The inactive defaults are NAMED so the seam test pins them: a core
// build without the check piece runs them as the exact inactive no-ops
// (identity for the paren collapse, empty restores, zero results).
func inactiveCheckMixedFormAdvisories(e *Engine, w WordInfo, sig *Signature, positions []int, pos SrcPos, fwdCount, stkCount int) {
}

func inactiveCheckModeAssumeSig(e *Engine, w WordInfo, fn *FnDefInfo, fallback *Signature, pos SrcPos) error {
	return nil
}

func inactiveCheckModeFallbackPositions(e *Engine, n int) []int { return nil }

func inactiveCheckModeParenFnCollapse(e *Engine, openIdx, closeIdx int) int { return closeIdx }

func inactiveCheckModeSurfaceShape(e *Engine, w WordInfo, pos SrcPos) (bool, error) {
	return false, nil
}

func inactiveConcreteEvalOnce(e *Engine, items []Value) (Value, bool) { return Value{}, false }

func inactiveDrainUndefinedAtoms(e *Engine) {}

func inactiveExprRefsCarrier(e *Engine, items []Value) bool { return false }

func inactiveNoteSpeculativeBarrierCommit(e *Engine, fwd ForwardInfo) {}

func inactiveRefuseForwardStackDrift(e *Engine, sig *Signature, positions []int) {}

func inactiveRefuseStrandedMemberFn(e *Engine, positions []int) {}

func inactiveShareCheckState(e *Engine, capturedReg *Registry) func() { return func() {} }

func inactiveSpliceAnonCheckResult(e *Engine, valIdx, nArgs int, sig *FnSig, args []Value, captures []CapturedBinding) error {
	return nil
}

func inactiveSpliceCheckResults(e *Engine, positions []int, results []Value) {}

func inactiveSpliceFnValueCheckResult(e *Engine, valIdx, nArgs int, fnDef FnDefInfo, sig *FnSig, args []Value) error {
	return nil
}

func inactiveTagCheckModeDefRead(e *Engine, top *Value, name string) {}

func inactiveTryDynamicFnValueDispatch(e *Engine, valIdx int) bool { return false }

func inactiveTryMemberFnArrivalDispatch(e *Engine, valIdx int) bool { return false }

func inactiveTryShapedMethodDispatch(e *Engine, valIdx int) bool { return false }

func inactiveUndefinedWordCheckDiag(e *Engine, name string, pos SrcPos) CheckDiagnostic {
	return CheckDiagnostic{}
}
