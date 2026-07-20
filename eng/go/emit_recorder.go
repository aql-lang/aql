package eng

// EmitRecorder is the checker-side view of the bytecode recording pass —
// the NARROW seam between static analysis and compilation (G9 / completion
// plan 4.5; comprehensive review Tier-2 item 6). Everything the check side
// (engine.go, carrier.go, core_helpers.go, user_poly.go, check.go, and the
// lang/native handlers) needs from the recorder goes through this interface:
// the checker compiles and runs with NO knowledge of the concrete *EmitState
// beyond it. The emit implementation cluster — emit.go, lower.go,
// callable_words.go — owns the concrete type and may type-assert
// (`rec.(*EmitState)`) to reach recording internals.
//
// Lifecycle: CheckState.Emit holds the recorder. A PLAIN check pass runs
// against the inactive no-op recorder (Begin installs it); the compile entry
// points (lang CompileCheck / Vm.compile) install a real *EmitState for the
// pass. Read the field through CheckState.Recorder(), which never returns
// nil — the recorder methods are then always callable, mirroring the
// nil-receiver-safe discipline the concrete methods already follow.
//
// The method set is exactly the surface the check side calls (enumerated by
// grepping Check.Emit usages outside the emit cluster). Exported methods are
// callable from lang/native; the unexported tail is eng-internal. The
// unexported methods also mean out-of-package types cannot implement the
// interface — the recorder contract is owned here, by design.
type EmitRecorder interface {
	// --- activity / lifecycle ------------------------------------------
	// Active / active report that recording is LIVE (armed, compilable,
	// not suspended). armed reports that a REAL recording state exists at
	// all (a compile pass installed one), live or not — the interface twin
	// of the historical `Check.Emit != nil` probe. Suspend pauses
	// recording, returning the resume func.
	Active() bool
	active() bool
	Armed() bool
	Suspend() func()
	bindRegistry(r *Registry)
	topFrameOnly() bool
	suspendedNow() bool
	bodyAnalysisGuard() func()
	fnBodyGuard() func()

	// --- refusal + site accounting --------------------------------------
	MarkUncompilable(reason string)
	Sites() map[string]int
	Finalize(residual []Value) (*Program, string, bool)

	// SetCatchVariadic latches the NEXT catch-word (CompileFallbackBody)
	// dispatch's recorded result as VARIADIC: `do` catches a body raise into
	// ONE Error value, so a fallible multi-value body's runtime count varies
	// (N no-raise vs 1 caught) and a static N-seat underflows on the caught
	// path. The word's ReturnsFn — the single point that both computes the
	// fallibility (lang doBodyMayRaise) and runs immediately before its own
	// dispatch records — sets/clears it per dispatch; the record paths
	// consume it keyed to the CompileFallbackBody sig, so it can never leak
	// onto an unrelated word's event. Plan Phase 5, L-DO.
	SetCatchVariadic(pending bool)

	// --- dispatch / value recording -------------------------------------
	RecordCall(word string, sig *Signature, args, outs []Value, pos SrcPos, forceDynOut, quoteInertOK bool)
	RecordPoly(word string)
	RecordPolyCall(word string, args, outs []Value, pos SrcPos, ownerReg *Registry, noMatch *PolyNoMatchSpec) bool
	RecordUserCall(unit int, args []Value, outs []Value, pos SrcPos)
	RecordUserPolyCall(word string, ownerReg *Registry, sigIdx, units []int, impls []SigImpl, sigs []Signature, args, outs []Value, pos SrcPos)
	RecordDynApply(args []Value, fn, out Value, pos SrcPos) bool
	RecordDynMethod(fn Value, args, outs []Value, word string, pos SrcPos) bool
	RecordFallback(span FallbackSpan, ins []Value, out Value, pos SrcPos) bool
	RecordTrap(code, detail, word, hint string, pos SrcPos) bool
	RecordTrapErr(ae *AqlError, pos SrcPos) bool
	RecordDispatchRematchValues(word string, vals []Value, writtenOff, nWritten int, pos SrcPos) bool
	RecordTypedBind(spec TypedBindSpec, in, out Value, pos SrcPos) (Value, bool)
	RecordMakeList(r *Registry, ins []Value, out Value, pos SrcPos) bool
	recordMakeListInner(r *Registry, ins []Value, out Value, pos SrcPos) bool
	RecordMakeMap(r *Registry, keys []string, vals []Value, implicit bool, out Value, pos SrcPos) bool
	RecordInterp(parts []InterpPart, holeVals []Value, out Value, pos SrcPos) bool
	RegisterTrailingApply(fnID string, arity int)
	noteMemberFnRead(id string, member Value)
	memberFnRead(id string) bool
	dynInputsProven(sig *Signature, args []Value) bool
	materialise(v Value) (Value, bool)
	resolveOperand(v Value) (emitOperand, bool)
	zeroOutProduced(id string) bool
	alreadyProduced(id string) bool

	// --- defs / locals ---------------------------------------------------
	MarkValueDef(v Value)
	RecordDefRebind(name string, v Value, pos SrcPos)
	RecordDynBind(name string, v Value, pos SrcPos)
	NoteDefRead(id, name string)
	NoteFrozenRead(name string)
	RefuseCarriedUndef(name string)
	NotifyNameRebound(name string)
	RegisterLocal(id string) int
	RememberOriginal(v Value)
	RememberStrippedOriginals(pre, stripped []Value)

	// --- branches / loops ------------------------------------------------
	ArmBranchCapture()
	peekCaptureArm() bool
	ArmLoopCapture()
	ConsumeLoopArm() bool
	TakeFragment() *EmitFragment
	RecordBranch(b BranchRecord)
	RecordLoop(start, end, step Value, body *EmitFragment, bodyStk []Value, iterID string, out Value, regionN int, pos SrcPos)
	SplitLoopRegionBind(name string, v Value) (Value, bool)
	SplitEventRegionBind(name string, v Value) (Value, bool)
	RecordInterpXml(tmpl XmlTmpl, holeVals []Value, out Value, pos SrcPos) bool
	BeginLoopCarried()
	EndLoopCarried()
	NoteLoopCarried(name string, joined, pre Value)
	Checkpoint() emitCheckpoint
	Rollback(cp emitCheckpoint)
	CanSeatAcrossFragment(v Value) bool

	// --- fn-unit compilation ---------------------------------------------
	StartFnCompile(key, name string, fnReg *Registry, args []Value, declared []*Type, paramNames []string, captures []CapturedBinding, generic bool, pos SrcPos) (unit int, finish func([]Value), ok bool)
	SetUnitParamTypes(unit int, paramTypes []*Type, paramPatterns []*Value)
	SetUnitDecl(unit int, decl DeclSite)
	unitVariadic(unit int) bool
	unitNetsZero(unit int) bool
}

// inactiveEmit is the no-op EmitRecorder a NON-compiling pass runs against:
// every method is the inactive/none answer the corresponding nil-receiver
// *EmitState method returns, so swapping it in for the historical nil field
// is behaviour-identical — and a compile-free check pass touches no
// *EmitState code at all.
type inactiveEmit struct{}

// theInactiveEmit is the shared no-op recorder instance (it is stateless).
var theInactiveEmit EmitRecorder = inactiveEmit{}

// Recorder returns the CheckState's emit recorder, never nil: the inactive
// no-op stands in when no recorder is installed (a zero-value CheckState, or
// between passes). All read sites go through this accessor — the field
// itself is written only by the pass entry points and the probe forks.
func (c *CheckState) Recorder() EmitRecorder {
	if c == nil || c.Emit == nil {
		return theInactiveEmit
	}
	return c.Emit
}

func (inactiveEmit) Active() bool              { return false }
func (inactiveEmit) active() bool              { return false }
func (inactiveEmit) Armed() bool               { return false }
func (inactiveEmit) Suspend() func()           { return func() {} }
func (inactiveEmit) bindRegistry(*Registry)    {}
func (inactiveEmit) topFrameOnly() bool        { return true }
func (inactiveEmit) suspendedNow() bool        { return false }
func (inactiveEmit) bodyAnalysisGuard() func() { return func() {} }
func (inactiveEmit) fnBodyGuard() func()       { return func() {} }

func (inactiveEmit) MarkUncompilable(string) {}
func (inactiveEmit) SetCatchVariadic(bool)   {}

func (inactiveEmit) RecordDynBind(string, Value, SrcPos) {}
func (inactiveEmit) NoteDefRead(string, string)          {}
func (inactiveEmit) Sites() map[string]int               { return nil }
func (inactiveEmit) Finalize([]Value) (*Program, string, bool) {
	return nil, "no emit state", false
}

func (inactiveEmit) RecordCall(string, *Signature, []Value, []Value, SrcPos, bool, bool) {}
func (inactiveEmit) RecordPoly(string)                                                   {}
func (inactiveEmit) RecordPolyCall(string, []Value, []Value, SrcPos, *Registry, *PolyNoMatchSpec) bool {
	return false
}
func (inactiveEmit) RecordUserCall(int, []Value, []Value, SrcPos) {}
func (inactiveEmit) RecordUserPolyCall(string, *Registry, []int, []int, []SigImpl, []Signature, []Value, []Value, SrcPos) {
}
func (inactiveEmit) RecordDynApply([]Value, Value, Value, SrcPos) bool { return false }
func (inactiveEmit) RecordDynMethod(Value, []Value, []Value, string, SrcPos) bool {
	return false
}
func (inactiveEmit) RecordFallback(FallbackSpan, []Value, Value, SrcPos) bool { return false }
func (inactiveEmit) RecordTrap(string, string, string, string, SrcPos) bool   { return false }
func (inactiveEmit) RecordTrapErr(*AqlError, SrcPos) bool                     { return false }
func (inactiveEmit) RecordDispatchRematchValues(string, []Value, int, int, SrcPos) bool {
	return false
}
func (inactiveEmit) RecordTypedBind(_ TypedBindSpec, _, out Value, _ SrcPos) (Value, bool) {
	return out, false
}
func (inactiveEmit) RecordMakeList(*Registry, []Value, Value, SrcPos) bool      { return false }
func (inactiveEmit) recordMakeListInner(*Registry, []Value, Value, SrcPos) bool { return false }
func (inactiveEmit) RecordMakeMap(*Registry, []string, []Value, bool, Value, SrcPos) bool {
	return false
}
func (inactiveEmit) RecordInterp([]InterpPart, []Value, Value, SrcPos) bool { return false }
func (inactiveEmit) RegisterTrailingApply(string, int)                      {}
func (inactiveEmit) noteMemberFnRead(string, Value)                         {}
func (inactiveEmit) memberFnRead(string) bool                               { return false }
func (inactiveEmit) dynInputsProven(*Signature, []Value) bool               { return false }
func (inactiveEmit) materialise(v Value) (Value, bool)                      { return v, false }
func (inactiveEmit) resolveOperand(Value) (emitOperand, bool)               { return emitOperand{}, false }
func (inactiveEmit) zeroOutProduced(string) bool                            { return false }
func (inactiveEmit) alreadyProduced(string) bool                            { return false }

func (inactiveEmit) MarkValueDef(Value)                         {}
func (inactiveEmit) RecordDefRebind(string, Value, SrcPos)      {}
func (inactiveEmit) RefuseCarriedUndef(string)                  {}
func (inactiveEmit) NotifyNameRebound(string)                   {}
func (inactiveEmit) NoteFrozenRead(string)                      {}
func (inactiveEmit) RegisterLocal(string) int                   { return -1 }
func (inactiveEmit) RememberOriginal(Value)                     {}
func (inactiveEmit) RememberStrippedOriginals([]Value, []Value) {}

func (inactiveEmit) ArmBranchCapture()                                {}
func (inactiveEmit) peekCaptureArm() bool                             { return false }
func (inactiveEmit) ArmLoopCapture()                                  {}
func (inactiveEmit) ConsumeLoopArm() bool                             { return false }
func (inactiveEmit) TakeFragment() *EmitFragment                      { return nil }
func (inactiveEmit) RecordBranch(BranchRecord)                        {}
func (inactiveEmit) SplitLoopRegionBind(string, Value) (Value, bool)  { return Value{}, false }
func (inactiveEmit) SplitEventRegionBind(string, Value) (Value, bool) { return Value{}, false }

func (inactiveEmit) RecordInterpXml(XmlTmpl, []Value, Value, SrcPos) bool { return false }

func (inactiveEmit) RecordLoop(Value, Value, Value, *EmitFragment, []Value, string, Value, int, SrcPos) {
}
func (inactiveEmit) BeginLoopCarried()                    {}
func (inactiveEmit) EndLoopCarried()                      {}
func (inactiveEmit) NoteLoopCarried(string, Value, Value) {}
func (inactiveEmit) Checkpoint() emitCheckpoint           { return emitCheckpoint{} }
func (inactiveEmit) Rollback(emitCheckpoint)              {}
func (inactiveEmit) CanSeatAcrossFragment(Value) bool     { return false }

func (inactiveEmit) StartFnCompile(string, string, *Registry, []Value, []*Type, []string, []CapturedBinding, bool, SrcPos) (int, func([]Value), bool) {
	return -1, nil, false
}
func (inactiveEmit) SetUnitParamTypes(int, []*Type, []*Value) {}
func (inactiveEmit) SetUnitDecl(int, DeclSite)                {}
func (inactiveEmit) unitVariadic(int) bool                    { return false }
func (inactiveEmit) unitNetsZero(int) bool                    { return false }
