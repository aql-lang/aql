package eng

import (
	"testing"
)

// vm_dynscope_test.go pins the dynamic-scope opcode pair (OpBindDynScope /
// OpLookupDynScope): the registry install + frame-exit cleanup discipline,
// the lookup's read path, and every runtime deferral arm (miss, dispatching
// binding, active token), plus the error-path unwind that keeps a failed run
// from leaking bindings into the registry.

func dsProgram(fns []CompiledFn, main []Instr, consts ...Value) *Program {
	dbg := make([]SrcPos, len(main))
	return &Program{Code: main, Debug: dbg, Consts: consts, Fns: fns}
}

func TestBindDynScopeInstallAndRetCleanup(t *testing.T) {
	r := seam7Reg(t)
	fn := CompiledFn{
		Name: "dsf", Returns: []*Type{TAny},
		Code: []Instr{
			{Op: OpPushConst, Arg: 0},    // 5
			{Op: OpBindDynScope, Arg: 1}, // install dsx=5
			{Op: OpLookupDynScope, Arg: 1},
			{Op: OpRet},
		},
		Debug: make([]SrcPos, 4),
	}
	p := dsProgram([]CompiledFn{fn},
		[]Instr{{Op: OpCallUser, Arg: 0}},
		NewInteger(5), NewString("dsx"))
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("bind+lookup: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("residual %v, want the looked-up value", out)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 5 {
		t.Errorf("lookup read %v, want the frame's binding 5", out[0])
	}
	if _, ok := r.Defs.Top("dsx"); ok {
		t.Error("RET must truncate the frame's dynamic-scope binding")
	}
}

func TestBindDynScopeErrorUnwind(t *testing.T) {
	r := seam7Reg(t)
	fn := CompiledFn{
		Name: "dsboom", Returns: []*Type{TAny},
		Code: []Instr{
			{Op: OpPushConst, Arg: 0},
			{Op: OpBindDynScope, Arg: 1},
			{Op: OpTrap, Arg: 0},
		},
		Debug: make([]SrcPos, 3),
	}
	p := dsProgram([]CompiledFn{fn},
		[]Instr{{Op: OpCallUser, Arg: 0}},
		NewInteger(9), NewString("dserr"))
	p.Traps = []TrapSpec{{Code: "value_error", Detail: "boom", Word: "dsboom"}}
	if _, err := RunProgram(p, r); err == nil {
		t.Fatal("trap must error")
	}
	if _, ok := r.Defs.Top("dserr"); ok {
		t.Error("an error unwind must restore the registry's dynamic-scope bindings")
	}
}

func TestLookupDynScopeDeferralArms(t *testing.T) {
	// Miss: no live binding.
	r := seam7Reg(t)
	p := dsProgram(nil, []Instr{{Op: OpLookupDynScope, Arg: 0}}, NewString("nosuch"))
	_, err := RunProgram(p, r)
	wantInternal(t, err, "dynamic-scope read miss for `nosuch`")

	// A dispatching binding (FnDefInfo): the interpreter would dispatch, not
	// substitute — defer.
	r2 := seam7Reg(t)
	r2.Defs.Push("dsfn", NewFnDef(FnDefInfo{Name: "dsfn", Registry: r2,
		Signatures: []Signature{{Returns: []*Type{TAny}, BarrierPos: -1}}}))
	p2 := dsProgram(nil, []Instr{{Op: OpLookupDynScope, Arg: 0}}, NewString("dsfn"))
	_, err = RunProgram(p2, r2)
	wantInternal(t, err, "dynamic-scope read of a dispatching binding `dsfn`")

	// An active token (a splice marker): steps on the tape, never data — defer.
	r3 := seam7Reg(t)
	r3.Defs.Push("dssp", NewSplice(NewList([]Value{NewInteger(1)})))
	p3 := dsProgram(nil, []Instr{{Op: OpLookupDynScope, Arg: 0}}, NewString("dssp"))
	_, err = RunProgram(p3, r3)
	wantInternal(t, err, "dynamic-scope read of an active token `dssp`")
}

func TestLookupDynScopeReadsTopLevelBinding(t *testing.T) {
	r := seam7Reg(t)
	r.Defs.Push("dsy", NewInteger(7))
	p := dsProgram(nil, []Instr{{Op: OpLookupDynScope, Arg: 0}}, NewString("dsy"))
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("residual %v, want one value", out)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Errorf("lookup read %v, want 7", out[0])
	}
}

// A break escaping a callee to an outer loop (flowSignal) tears down the
// escaped frame's dynamic-scope bindings — the interpreter's
// unwindLiveFrames replays the frame's cleanup tail; without the unwind the
// dead frame's install stays readable in Defs (PR #233 review).
func TestFlowSignalUnwindsEscapedFrameDynBinds(t *testing.T) {
	r := seam7Reg(t)
	fn := CompiledFn{
		Name: "dsbrk", Returns: nil,
		Code: []Instr{
			{Op: OpPushConst, Arg: 0},
			{Op: OpBindDynScope, Arg: 1}, // install dsfl=5 in this frame
			{Op: OpFlowBreak},            // escape to the caller's loop
		},
		Debug: make([]SrcPos, 3),
	}
	main := []Instr{
		{Op: OpPushConst, Arg: 2}, // step 1
		{Op: OpPushConst, Arg: 3}, // end 3
		{Op: OpPushConst, Arg: 4}, // start 0
		{Op: OpForSetup, Arg: 0},  // iterator slot 0
		{Op: OpForNext, Arg: 7},   // exit -> 7
		{Op: OpCallUser, Arg: 0},  // per-iteration call that breaks
		{Op: OpJmp, Arg: 4},       // back-edge
	}
	p := dsProgram([]CompiledFn{fn}, main,
		NewInteger(5), NewString("dsfl"), NewInteger(1), NewInteger(3), NewInteger(0))
	p.NumLocals = 1
	if _, err := RunProgram(p, r); err != nil {
		t.Fatalf("break-out-of-callee run: %v", err)
	}
	if _, ok := r.Defs.Top("dsfl"); ok {
		t.Error("the escaped frame's dynamic-scope binding must tear down with the frame")
	}
}
