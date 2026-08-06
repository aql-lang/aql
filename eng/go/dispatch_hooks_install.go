package eng

// The compiler piece's DispatchBraid installation (the S3 seam's
// active half — see dispatch_hooks.go). Runs at init while the
// compiler is linked; a compiler-less build keeps the inactive
// defaults. The plan-returning installers wrap the concrete
// *userPolyPlan so a declined bake yields a NIL INTERFACE, never a
// typed nil (the check side tests `plan != nil`).

func init() { installDispatchBraid() }

func installDispatchBraid() {
	DispatchBraid.RecordOutcome = recordDispatchOutcome
	DispatchBraid.TryFoldScalarConst = tryFoldScalarConst
	DispatchBraid.TryRecordPoly = tryRecordPoly
	DispatchBraid.CompileUserPolyArms = func(r *Registry, es EmitRecorder, word string, args []Value, committedReturns []*Type) UserPolyPlan {
		if p := tryCompileUserPolyArms(r, es, word, args, committedReturns); p != nil {
			return p
		}
		return nil
	}
	DispatchBraid.PlanUserPoly = func(r *Registry, es EmitRecorder, word string, args []Value, declaredReturns []*Type) (UserPolyPlan, bool) {
		p, barred := planUserPolyDispatch(r, es, word, args, declaredReturns)
		if p != nil {
			return p, barred
		}
		return nil, barred
	}
}
