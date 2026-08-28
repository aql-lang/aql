package eng

import (
	"fmt"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vmInternalError is the ONE text both VM panic guards surface — runVMEntry's
// top-level recover and runForeignUnit's per-callback one. RunCompiled and
// InvokeCompiled both key their degrade-to-interpreter decision on the
// internal_error CLASS, so the two guards must not drift apart.
func vmInternalError(rec any, src string) error {
	return core.MakeBoruError("internal_error",
		fmt.Sprintf("internal bytecode VM error: %v", rec), "", src, "")
}

// Foreign (detached) unit hosting — the half of InvokeCallback's contract that
// was missing.
//
// InvokeCallback promises to run a stamped callback body "on the VM when it
// compiled to a unit (nested in a live run, or fresh on an idle registry)".
// The idle half was real; the NESTED half was not. runUnitNested compared
// ref.Prog against the running program and declined every mismatch, and a
// DETACHED ref (StampDetachedFn) is a mismatch BY CONSTRUCTION — it is compiled
// on an isolated fork into its own standalone one-unit Program. So every
// runtime-stamped callback reached from inside a live compiled run — the
// predicate types being the whole population of them in the corpus — stamped
// successfully, reported Stamped:true in the stamp ledger, and then ran on the
// interpreter anyway. The ledger said compiled; the runtime interpreted. That
// divergence is what the interp-entry census measures and what T2 forbids.
//
// Hosting a foreign program's unit is not the same as starting a run: the
// concurrency guard is already held by the enclosing run, and re-taking it
// (RunUnit) is exactly what would fail. What the foreign unit needs is a
// vmContext of its OWN — bound to ITS program, so unit indices, closure units
// and constants resolve against the right artifact — nested inside the live
// one.
//
// Four pieces of state are deliberately SHARED with the enclosing run rather
// than restarted, because each is a runaway guard that a per-callback reset
// would silently defeat:
//
//   - steps      — copied in and handed back, so a hot callback cannot mint
//     itself a fresh step budget on every invoke.
//   - frameDepth — seeded from the enclosing depth, so recursion THROUGH a
//     detached callback (a predicate whose body triggers another typed bind)
//     is bounded by the same ceiling as recursion inside one program, and
//     fails as tape_exhausted instead of overflowing the Go stack.
//   - ceiling / stepLimit — the registry's, unchanged.
//
// Everything else is per-context and must NOT be shared: dynBinds (the foreign
// run unwinds its own trail), islandEng (the enclosing run's island engine may
// be mid-flight — this is precisely the re-entrancy its contract forbids), and
// foreignInvokers (cleared here exactly as runVMEntry clears its own).

// runForeignUnit runs ref's unit — belonging to a program OTHER than the one
// vc is running — nested inside vc's live run, and reports whether a VM path
// took it. handled=false leaves the callback to the interpreter, byte-
// identically to the behaviour before foreign hosting existed.
//
// The panic guard is local rather than borrowed from the enclosing
// runVMEntry's: a soundness bailout inside ONE callback must degrade THAT
// callback (InvokeCompiled's C1 fence then retries it on CallBoru), not abort
// the whole enclosing program and re-run it on the interpreter.
func (vc *vmContext) runForeignUnit(ref *compiler.CompiledFnRef, args []core.Value) (res []core.Value, handled bool, err error) {
	r := vc.r
	sub := &vmContext{
		p:          ref.Prog,
		r:          r,
		ceiling:    vc.ceiling,
		stepLimit:  vc.stepLimit,
		steps:      vc.steps,
		argsFloor:  r.Args.Depth(),
		frameDepth: vc.frameDepth,
	}
	// Registered FIRST so it runs LAST: the budget is handed back on every
	// path, including the one where the panic guard below has replaced res/err.
	// A bailed callback still spent its steps.
	defer func() { vc.steps = sub.steps }()
	if ref.Prog.DynEnv {
		// The DynEnv args bracket, per runVMEntry: an error unwind returns
		// straight out of sub.run, so rebalance to the entry depth on every
		// path rather than only the clean one.
		defer r.Args.Truncate(sub.argsFloor)
	}
	defer func() {
		if rec := recover(); rec != nil {
			res, handled, err = nil, true, vmInternalError(rec, r.Source)
		}
	}()
	// The foreign program owns body execution for the duration: a closure it
	// pushes, or a further detached ref it reaches, indexes ITS Fns table, so
	// both seams must point at sub while it runs. Restored on exit so the
	// enclosing run resumes with its own.
	prevInvoker := r.Invoker
	prevNested := r.NestedRunner
	r.Invoker = sub.invokeClosureOn
	r.NestedRunner = sub.runUnitNested
	defer func() {
		r.Invoker = prevInvoker
		r.NestedRunner = prevNested
		for _, fr := range sub.foreignInvokers {
			fr.Invoker = nil
		}
	}()
	res, err = sub.enterBodyUnit(r, ref.Unit, bindUnitLocals(&ref.Prog.Fns[ref.Unit], args, ref.Captures))
	return res, true, err
}
