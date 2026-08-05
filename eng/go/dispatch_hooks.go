package eng

// The dispatch-record hook layer — Stage 3b of the four-piece split
// (design/ENG-FOUR-PIECE.0.md seam S3). Every call the CHECK piece makes
// into the compiler's recording / folding / poly-bake machinery routes
// through the small surface below, so check files carry no named compiler
// symbols: this one file concentrates the check->compiler coupling, and
// its bodies become a DispatchRecorder interface (implemented by the
// compiler, installed at init) when the packages cut. Behavior is
// byte-identical to the direct calls it replaces.

// dispatchRecordOutcome forwards a matched dispatch's concrete outcome to
// the recording pipeline (const folds, poly arms, deferred lists).
func dispatchRecordOutcome(r *Registry, word string, sig *Signature, args, out []Value, pos SrcPos, ownerReg *Registry) {
	recordDispatchOutcome(r, word, sig, args, out, pos, ownerReg)
}

// dispatchTryFoldScalarConst asks the compiler for a scalar const-fold of
// a pure dispatch over inert operands.
func dispatchTryFoldScalarConst(r *Registry, sig *Signature, args []Value) (Value, bool) {
	return tryFoldScalarConst(r, sig, args)
}

// dispatchTryRecordPoly offers a hazardous dispatch to the poly recorder
// (the VM re-match lowering) and reports whether it was taken.
func dispatchTryRecordPoly(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos, disjunctStraddle bool, ownerReg *Registry, dynamicRecovery bool, noMatch *PolyNoMatchSpec) bool {
	return tryRecordPoly(r, word, sig, args, outs, pos, disjunctStraddle, ownerReg, dynamicRecovery, noMatch)
}

// dispatchCompileUserPolyArms bakes every same-arity overload's body unit
// for a user-poly re-match plan (nil = the bake declined an arm).
func dispatchCompileUserPolyArms(r *Registry, es EmitRecorder, word string, args []Value, committedReturns []*Type) *userPolyPlan {
	return tryCompileUserPolyArms(r, es, word, args, committedReturns)
}

// dispatchPlanUserPoly decides whether a multi-overload user-fn call site
// must ride the user-poly runtime re-match instead of a static arm commit.
func dispatchPlanUserPoly(r *Registry, es EmitRecorder, word string, args []Value, declaredReturns []*Type) (*userPolyPlan, bool) {
	return planUserPolyDispatch(r, es, word, args, declaredReturns)
}
