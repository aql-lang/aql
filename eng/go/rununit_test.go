package eng

import (
	"errors"
	"strings"
	"testing"
)

// RunUnit starts a fresh VM run entered at a specific compiled fn unit, binding
// per-call args to the leading param slots and the ref's captures to the
// trailing slots — the durable-callback entry point (serve-raw handlers,
// spawned processes) that fires AFTER the enclosing RunProgram returned. These
// tests drive it with hand-built units, mirroring the vm_steplimit / vm_seam
// style, since eng has no def/fn words to compile a real body.

func runUnitReg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// A unit whose body is [PUSH_LOCAL 0, RET] returns its single param verbatim.
func TestRunUnitBindsParam(t *testing.T) {
	p := &Program{
		Fns: []CompiledFn{{
			Name:    "echo",
			NParams: 1,
			NLocals: 1,
			Code:    []Instr{{Op: OpPushLocal, Arg: 0}, {Op: OpRet, Arg: 0}},
			Debug:   []SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
	ref := &CompiledFnRef{Prog: p, Unit: 0}
	out, err := RunUnit(ref, runUnitReg(t), []Value{NewInteger(7)})
	if err != nil {
		t.Fatalf("RunUnit: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Fatalf("result = %v, want 7", out[0])
	}
}

// A capture fills the trailing local slot: NParams=2, NCaptures=1, body returns
// local 1 (the capture), so the per-call arg lands in slot 0 and the capture in
// slot 1.
func TestRunUnitBindsCapture(t *testing.T) {
	p := &Program{
		Fns: []CompiledFn{{
			Name:      "grabCapture",
			NParams:   2,
			NCaptures: 1,
			NLocals:   2,
			Code:      []Instr{{Op: OpPushLocal, Arg: 1}, {Op: OpRet, Arg: 0}},
			Debug:     []SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
	ref := &CompiledFnRef{Prog: p, Unit: 0, Captures: []Value{NewInteger(99)}}
	out, err := RunUnit(ref, runUnitReg(t), []Value{NewInteger(7)})
	if err != nil {
		t.Fatalf("RunUnit: %v", err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 99 {
		t.Fatalf("result = %v, want the captured 99", out[0])
	}
}

// TestInvokeCallbackInternalErrorFallsBack pins PR #243 comment #3: a callback
// fires AFTER the enclosing RunProgram returned (serve-raw handling a connection,
// a spawned process), so there is no outer RunCompiled to catch a VM bailout and
// re-run on the interpreter. InvokeCallback therefore applies that discipline
// itself — when the stamped unit raises an internal_error (here a CALL_DYNAMIC
// underflow, the vmErrAt soundness-bailout class), it falls back to CallAQL over
// the sig's AQL body instead of leaking the raw internal_error to the peer. The
// body `[42]` returns 42, proving the fallback ran rather than the failing unit.
func TestInvokeCallbackInternalErrorFallsBack(t *testing.T) {
	// A unit that raises internal_error when run: CALL_DYNAMIC over an empty
	// stack underflows → vmErrAt(internal_error).
	p := &Program{Fns: []CompiledFn{{
		Name: "boom", NParams: 0, NLocals: 0,
		Code:  []Instr{{Op: OpCallDynamic, Arg: 0}, {Op: OpRet, Arg: 0}},
		Debug: []SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
	}}}
	ref := &CompiledFnRef{Prog: p, Unit: 0}
	// Confirm the unit really does raise internal_error on its own.
	if _, err := RunUnit(ref, runUnitReg(t), nil); !isInternalErr(err) {
		t.Fatalf("RunUnit err = %v, want an internal_error to drive the fallback", err)
	}
	// The sig carries the stamped ref AND an AQL body CallAQL can run to 42.
	sig := &Signature{Impl: &AQLImpl{Body: []Value{NewInteger(42)}, Compiled: ref}}
	out, err := InvokeCallback(runUnitReg(t), sig, nil, nil)
	if err != nil {
		t.Fatalf("InvokeCallback should have fallen back to CallAQL, got err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1 (the CallAQL body residual)", len(out))
	}
	if n, _ := out[0].AsConcreteInteger(); n != 42 {
		t.Fatalf("result = %v, want 42 from the interpreter fallback", out[0])
	}
}

// TestInvokeCallbackBusyRegistryFallsBack covers the no-VM-path branch of
// invokeCompiledUnit: a stamped ref whose registry is mid-run (canHostVM false)
// with no nestedRunner installed cannot run on the VM, so invokeCompiledUnit
// reports ran=false and InvokeCallback owns the call on the interpreter. The
// CallAQL body `[7]` proves the interpreter ran.
func TestInvokeCallbackBusyRegistryFallsBack(t *testing.T) {
	r := runUnitReg(t)
	r.vmRunning = 1 // canHostVM() false; nestedRunner stays nil
	p := &Program{Fns: []CompiledFn{{
		Name: "x", NParams: 0, NLocals: 0,
		Code: []Instr{{Op: OpRet, Arg: 0}}, Debug: []SrcPos{{Row: 1, Col: 1}},
	}}}
	ref := &CompiledFnRef{Prog: p, Unit: 0}
	// Direct: no VM path applies, so ran=false.
	if _, _, ran := invokeCompiledUnit(r, ref, nil); ran {
		t.Fatal("a busy registry with no nestedRunner must report ran=false")
	}
	// Via InvokeCallback: it falls through to CallAQL over the body.
	sig := &Signature{Impl: &AQLImpl{Body: []Value{NewInteger(7)}, Compiled: ref}}
	out, err := InvokeCallback(r, sig, nil, nil)
	if err != nil {
		t.Fatalf("InvokeCallback should have fallen back to CallAQL, got err: %v", err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Fatalf("result = %v, want 7 from the interpreter fallback", out[0])
	}
}

// isInternalErr classifies exactly the internal_error AqlError (and nothing
// else): a foreign error and a genuine AQL runtime error both report false, so
// InvokeCallback returns them straight through rather than masking with a
// fallback.
func TestIsInternalErr(t *testing.T) {
	if !isInternalErr(makeAqlError("internal_error", "boom", "", "", "")) {
		t.Error("internal_error AqlError must classify true")
	}
	if isInternalErr(makeAqlError("signature_error", "no match", "", "", "")) {
		t.Error("a genuine AQL runtime error must classify false")
	}
	if isInternalErr(errors.New("foreign")) {
		t.Error("a non-AqlError must classify false")
	}
	if isInternalErr(nil) {
		t.Error("nil must classify false")
	}
}

// A nil ref and a ref with a nil program both report "nil unit reference"
// (the invoke seam treats a nil ref as "no runnable unit").
func TestRunUnitNilRef(t *testing.T) {
	for _, ref := range []*CompiledFnRef{nil, {Prog: nil, Unit: 0}} {
		if _, err := RunUnit(ref, runUnitReg(t), nil); err == nil ||
			!strings.Contains(err.Error(), "nil unit reference") {
			t.Fatalf("RunUnit(%v) err = %v, want nil unit reference", ref, err)
		}
	}
}

// A registry already driving a compiled run rejects a second, overlapping run:
// RunUnit shares runProgram's vmRunning CAS guard, so a concurrent callback on
// the SAME registry (rather than a ForkConcurrent clone) is refused rather than
// racing the shared invoker/scopes.
func TestRunUnitRejectsConcurrentRun(t *testing.T) {
	r := runUnitReg(t)
	p := &Program{Fns: []CompiledFn{{Name: "x", NParams: 0, NLocals: 0,
		Code: []Instr{{Op: OpRet, Arg: 0}}, Debug: []SrcPos{{}}}}}
	r.vmRunning = 1 // simulate an in-flight compiled run on this registry
	_, err := RunUnit(&CompiledFnRef{Prog: p, Unit: 0}, r, nil)
	var ae *AqlError
	if err == nil || !errors.As(err, &ae) || ae.Code != "concurrency_error" {
		t.Fatalf("RunUnit during an active run = %v, want concurrency_error", err)
	}
}

// compileStoredBody declines (fails safe) without a live EmitState / registry,
// and for a non-list body operand — the defensive guards on the spawn code-body
// compile path.
func TestCompileStoredBodyGuards(t *testing.T) {
	var nilES *EmitState
	if _, ok := nilES.compileStoredBody(NewInteger(1)); ok {
		t.Fatal("nil EmitState must decline")
	}
	if _, ok := (&EmitState{}).compileStoredBody(NewInteger(1)); ok {
		t.Fatal("reg-less EmitState must decline")
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.InitRootContext()
	if _, ok := (&EmitState{reg: r}).compileStoredBody(NewInteger(1)); ok {
		t.Fatal("a non-list body must decline")
	}
}

// compileStoredFnUnit declines (fails safe) when it has no live EmitState /
// registry to compile against — the defensive guards on the recorder-internal
// path. A nil receiver and a reg-less state both return (0, false).
func TestCompileStoredFnUnitGuards(t *testing.T) {
	var nilES *EmitState
	if _, ok := nilES.compileStoredFnUnit(FnDefInfo{}, SrcPos{}); ok {
		t.Fatal("nil EmitState must decline")
	}
	if _, ok := (&EmitState{}).compileStoredFnUnit(FnDefInfo{}, SrcPos{}); ok {
		t.Fatal("reg-less EmitState must decline")
	}
}

// stampCompiledRef writes onto the first own AQL body sig and reports true; a fn
// value with no own AQL body sig (all Go / all fallback) reports false.
func TestStampCompiledRef(t *testing.T) {
	ref := &CompiledFnRef{Prog: &Program{}, Unit: 0}
	aqlFd := FnDefInfo{Signatures: []Signature{{Impl: &AQLImpl{Body: []Value{NewInteger(1)}}}}}
	if !stampCompiledRef(aqlFd, ref) {
		t.Fatal("an AQL body sig must accept the stamp")
	}
	if aqlFd.Signatures[0].CompiledRef() != ref {
		t.Fatal("stamp did not land on the sig")
	}
	// A fallback-only sig is skipped; a Go sig has no *AQLImpl → no stamp.
	goFd := FnDefInfo{Signatures: []Signature{
		{Fallback: true, Impl: &AQLImpl{}},
		{Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) { return nil, nil })},
	}}
	if stampCompiledRef(goFd, ref) {
		t.Fatal("a fn value with no own AQL body sig must not be stamped")
	}
}

// CompiledRef reads the durable unit reference off an AQL body sig, and returns
// nil for a Go sig (the callback-invocation seam's "no compiled unit" signal).
func TestSignatureCompiledRef(t *testing.T) {
	ref := &CompiledFnRef{Prog: &Program{}, Unit: 3}
	aqlSig := &Signature{Impl: &AQLImpl{Body: []Value{NewInteger(1)}, Compiled: ref}}
	if got := aqlSig.CompiledRef(); got != ref {
		t.Fatalf("AQL-body CompiledRef = %v, want the stamped ref", got)
	}
	// An un-armed AQL body reports no unit.
	bareSig := &Signature{Impl: &AQLImpl{Body: []Value{NewInteger(1)}}}
	if got := bareSig.CompiledRef(); got != nil {
		t.Fatalf("un-armed CompiledRef = %v, want nil", got)
	}
	// A Go sig has no AQL body at all.
	goSig := &Signature{Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return nil, nil
	})}
	if got := goSig.CompiledRef(); got != nil {
		t.Fatalf("Go-sig CompiledRef = %v, want nil", got)
	}
}

// A unit index outside Prog.Fns is rejected (never indexed).
func TestRunUnitUnitOutOfRange(t *testing.T) {
	p := &Program{Fns: []CompiledFn{{Name: "only", NParams: 0, NLocals: 0,
		Code: []Instr{{Op: OpRet, Arg: 0}}, Debug: []SrcPos{{}}}}}
	for _, unit := range []int{-1, 5} {
		ref := &CompiledFnRef{Prog: p, Unit: unit}
		if _, err := RunUnit(ref, runUnitReg(t), nil); err == nil ||
			!strings.Contains(err.Error(), "out of range") {
			t.Fatalf("RunUnit(unit=%d) err = %v, want out of range", unit, err)
		}
	}
}
