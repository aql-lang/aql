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
