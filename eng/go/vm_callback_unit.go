package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// VM-piece callback unit runner (Stage 2c of the four-piece split; a
// Stage-1 leftover): the RunUnit reach for stamped callback units, called
// only by the CompiledRuntime implementation (compiled_runtime_vm.go).

// invokeCompiledUnit runs a stamped callback unit on the VM and reports whether a
// VM path actually ran it (ran=false → no VM path applied; the caller falls to the
// interpreter). An idle registry (a per-connection / per-process fork) starts a
// fresh RunUnit; a busy one (a service handler invoked synchronously mid-run, a
// predicate type consulted from inside a compiled program) runs the unit NESTED
// via nestedRunner, since a fresh RunUnit would trip the concurrency guard.
//
// Both arms take a DETACHED unit — one belonging to its own standalone program
// rather than the running one. That is worth stating because it was false for
// most of this seam's life: the nested arm declined every cross-program ref, so
// a runtime-stamped body reached mid-run stamped successfully and then ran on
// the interpreter anyway (eng/go/vm_foreign_unit.go). handled=false now means
// only a ref this seam genuinely cannot run.
func invokeCompiledUnit(r *core.Registry, ref *compiler.CompiledFnRef, args []core.Value) (res []core.Value, err error, ran bool) {
	if r.CanHostVM() {
		res, err = RunUnit(ref, r, args)
		return res, err, true
	}
	if r.NestedRunner != nil {
		if res, handled, err := r.NestedRunner(ref, args); handled {
			return res, err, true
		}
	}
	return nil, nil, false
}
