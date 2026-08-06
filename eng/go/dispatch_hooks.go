package eng

// The dispatch-record hook layer — the four-piece split's S3 seam
// (design/ENG-FOUR-PIECE.0.md). Every call the CHECK piece makes into
// the compiler's recording / folding / poly-bake machinery routes
// through the DispatchBraid slot table below, so check files carry no
// named compiler symbols: the compiler piece installs its
// implementations at init (installDispatchBraid,
// dispatch_hooks_install.go), and a compiler-less check build runs the
// named inactive defaults — the exact declines an unarmed recording
// pass produces. Behavior is byte-identical to the direct calls the
// table replaces.

// UserPolyPlan is the check piece's opaque view of one poly user
// call's compiled arm table (the compiler's plan handed back by
// CompileUserPolyArms/PlanUserPoly): the record site substitutes the
// return-join carriers and unpacks the parallel slices into
// EmitRecorder.RecordUserPolyCall without naming the compiler's
// concrete plan type. Installers must return a NIL INTERFACE (not a
// typed nil) when the plan declines.
type UserPolyPlan interface {
	SubstituteJoinedOuts(out []Value)
	SigIdx() []int
	Units() []int
	Impls() []SigImpl
	Sigs() []Signature
}

// DispatchBraid is the check->compiler slot table. The slots are
// inactive only on a build linked without the compiler piece — a
// configuration where nothing records, so every slot declines.
var DispatchBraid = struct {
	// RecordOutcome forwards a matched dispatch's concrete outcome to
	// the recording pipeline (const folds, poly arms, deferred lists).
	RecordOutcome func(r *Registry, word string, sig *Signature, args, out []Value, pos SrcPos, ownerReg *Registry)
	// TryFoldScalarConst asks the compiler for a scalar const-fold of
	// a pure dispatch over inert operands.
	TryFoldScalarConst func(r *Registry, sig *Signature, args []Value) (Value, bool)
	// TryRecordPoly offers a hazardous dispatch to the poly recorder
	// (the VM re-match lowering) and reports whether it was taken.
	TryRecordPoly func(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos, disjunctStraddle bool, ownerReg *Registry, dynamicRecovery bool, noMatch *PolyNoMatchSpec) bool
	// CompileUserPolyArms bakes every same-arity overload's body unit
	// for a user-poly re-match plan (nil = the bake declined an arm).
	CompileUserPolyArms func(r *Registry, es EmitRecorder, word string, args []Value, committedReturns []*Type) UserPolyPlan
	// PlanUserPoly decides whether a multi-overload user-fn call site
	// must ride the user-poly runtime re-match instead of a static arm
	// commit.
	PlanUserPoly func(r *Registry, es EmitRecorder, word string, args []Value, declaredReturns []*Type) (UserPolyPlan, bool)
}{
	RecordOutcome:       inactiveDispatchRecordOutcome,
	TryFoldScalarConst:  inactiveDispatchTryFoldScalarConst,
	TryRecordPoly:       inactiveDispatchTryRecordPoly,
	CompileUserPolyArms: inactiveDispatchCompileUserPolyArms,
	PlanUserPoly:        inactiveDispatchPlanUserPoly,
}

// The inactive defaults are NAMED so the seam test pins them
// (TestInactiveDispatchBraid): each is the exact decline an unarmed
// recording pass produces.
func inactiveDispatchRecordOutcome(*Registry, string, *Signature, []Value, []Value, SrcPos, *Registry) {
}

func inactiveDispatchTryFoldScalarConst(*Registry, *Signature, []Value) (Value, bool) {
	return Value{}, false
}

func inactiveDispatchTryRecordPoly(*Registry, string, *Signature, []Value, []Value, SrcPos, bool, *Registry, bool, *PolyNoMatchSpec) bool {
	return false
}

func inactiveDispatchCompileUserPolyArms(*Registry, EmitRecorder, string, []Value, []*Type) UserPolyPlan {
	return nil
}

func inactiveDispatchPlanUserPoly(*Registry, EmitRecorder, string, []Value, []*Type) (UserPolyPlan, bool) {
	return nil, false
}

// The check-side accessors keep every call site spelled as before; the
// braid is the seam.

func dispatchRecordOutcome(r *Registry, word string, sig *Signature, args, out []Value, pos SrcPos, ownerReg *Registry) {
	DispatchBraid.RecordOutcome(r, word, sig, args, out, pos, ownerReg)
}

func dispatchTryFoldScalarConst(r *Registry, sig *Signature, args []Value) (Value, bool) {
	return DispatchBraid.TryFoldScalarConst(r, sig, args)
}

func dispatchTryRecordPoly(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos, disjunctStraddle bool, ownerReg *Registry, dynamicRecovery bool, noMatch *PolyNoMatchSpec) bool {
	return DispatchBraid.TryRecordPoly(r, word, sig, args, outs, pos, disjunctStraddle, ownerReg, dynamicRecovery, noMatch)
}

func dispatchCompileUserPolyArms(r *Registry, es EmitRecorder, word string, args []Value, committedReturns []*Type) UserPolyPlan {
	return DispatchBraid.CompileUserPolyArms(r, es, word, args, committedReturns)
}

func dispatchPlanUserPoly(r *Registry, es EmitRecorder, word string, args []Value, declaredReturns []*Type) (UserPolyPlan, bool) {
	return DispatchBraid.PlanUserPoly(r, es, word, args, declaredReturns)
}
