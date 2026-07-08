package eng

import (
	"errors"
	"strings"
	"testing"
)

// vm_seam7_test.go drives the bytecode VM's defensive error arms and a few
// otherwise-uncovered normal branches. Most of these guards are unreachable
// through a correctly compiled program (they exist to turn a compiler bug into
// a clean internal_error → interpreter fallback, never a Go panic), so they are
// exercised here two ways:
//
//   - hand-built malformed Programs fed to RunProgram (the run() dispatch arms),
//   - direct calls to the in-package vmContext helpers (the extracted methods).
//
// Both are legitimate: feeding the VM malformed bytecode simulates exactly the
// compiler-bug shape each guard defends against, and the VM's contract is to
// return an internal_error AqlError (RunCompiled then re-runs the interpreter).

func seam7Reg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// wantInternal asserts err is a non-nil *AqlError carrying the internal_error
// taxonomy whose message contains sub.
func wantInternal(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want internal_error containing %q, got nil", sub)
	}
	var ae *AqlError
	if errors.As(err, &ae) {
		if ae.Code != "internal_error" {
			t.Fatalf("want internal_error, got %q (%v)", ae.Code, err)
		}
	}
	if !strings.Contains(err.Error(), sub) {
		t.Fatalf("error %q does not contain %q", err.Error(), sub)
	}
}

// wantErr asserts err is non-nil and its message contains sub (any taxonomy).
func wantErr(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error containing %q, got nil", sub)
	}
	if !strings.Contains(err.Error(), sub) {
		t.Fatalf("error %q does not contain %q", err.Error(), sub)
	}
}

var seam7Dbg = []SrcPos{{}}

func seam7VC(r *Registry) *vmContext {
	return &vmContext{r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
}

// --- nil program / tapeCoupled / screenResults ---------------------------

func TestSeam7NilProgram(t *testing.T) {
	_, err := RunProgram(nil, seam7Reg(t))
	wantErr(t, err, "nil program")
}

func TestSeam7TapeCoupledDetectsTokens(t *testing.T) {
	// Each tape-coupled token flavour trips tapeCoupled (vm.go:124-131).
	toks := []Value{
		NewWord("w"),
		NewMark("m"),
		NewMove("to", "why"),
		NewForward(ForwardInfo{}),
		NewOpenParen(),
		NewSplice(NewInteger(1)),
	}
	for i := range toks {
		if !tapeCoupled([]Value{toks[i]}) {
			t.Errorf("token %d (%s) not detected as tape-coupled", i, toks[i].String())
		}
	}
	if tapeCoupled([]Value{NewInteger(1), NewString("x")}) {
		t.Error("plain data reported tape-coupled")
	}
}

func TestSeam7ScreenResultsRejectsToken(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	err := vc.screenResults([]Value{NewWord("w")}, "poke", seam7Dbg, 0)
	wantInternal(t, err, "tape-coupled poke")
	if err := vc.screenResults([]Value{NewInteger(1)}, "poke", seam7Dbg, 0); err != nil {
		t.Errorf("clean results screened as tape-coupled: %v", err)
	}
}

// --- run() dispatch error arms via malformed programs --------------------

func runMalformed(t *testing.T, p *Program) error {
	t.Helper()
	if p.Debug == nil {
		p.Debug = make([]SrcPos, len(p.Code))
	}
	_, err := RunProgram(p, seam7Reg(t))
	return err
}

func TestSeam7RunUnderflowArms(t *testing.T) {
	cases := []struct {
		name string
		p    *Program
		sub  string
	}{
		{"store-local", &Program{Code: []Instr{{Op: OpStoreLocal, Arg: 0}}, NumLocals: 1}, "STORE_LOCAL stack underflow"},
		{"drop", &Program{Code: []Instr{{Op: OpDrop}}}, "DROP stack underflow"},
		{"make-list", &Program{Code: []Instr{{Op: OpMakeList, Arg: 2}}}, "MAKE_LIST stack underflow"},
		{"make-map", &Program{Code: []Instr{{Op: OpMakeMap, Arg: 0}}, MakeMaps: []MakeMapSpec{{Keys: []string{"a"}}}}, "MAKE_MAP stack underflow"},
		{"interp", &Program{Code: []Instr{{Op: OpInterp, Arg: 0}}, Interps: []InterpSpec{{NHoles: 1, Segs: []InterpSeg{{Hole: true}}}}}, "INTERP stack underflow"},
		{"push-closure", &Program{Code: []Instr{{Op: OpPushClosure, Arg: 0}}, Fns: []CompiledFn{{Name: "c", NCaptures: 2}}}, "PUSH_CLOSURE capture underflow"},
		{"for-setup", &Program{Code: []Instr{{Op: OpForSetup, Arg: 0}}}, "FOR_SETUP underflow"},
		{"for-next", &Program{Code: []Instr{{Op: OpForNext, Arg: 0}}}, "FOR_NEXT without a loop"},
		{"jmpiffalse", &Program{Code: []Instr{{Op: OpJmpIfFalse, Arg: 5}}}, "JMP_IF_FALSE underflow"},
		{"bind-typed", &Program{Code: []Instr{{Op: OpBindTyped, Arg: 0}}, TypedBinds: []TypedBindSpec{{Kind: TypedBindDepScalar, Name: "x"}}}, "BIND_TYPED stack underflow"},
		{"call-native-poly", &Program{Code: []Instr{{Op: OpCallNativePoly, Arg: 0}}, PolyRefs: []PolyRef{{Word: "p", Arity: 2}}}, "CALL_NATIVE_POLY underflow"},
		{"drop-to-mark", &Program{Code: []Instr{{Op: OpDropToMark}}}, "DROP_TO_MARK with no open mark"},
		{"pop-mark", &Program{Code: []Instr{{Op: OpPopMark}}}, "POP_MARK with no open mark"},
		{"unknown", &Program{Code: []Instr{{Op: Opcode(250)}}}, "unknown opcode"},
		{"flow-break-noloop", &Program{Code: []Instr{{Op: OpFlowBreak}}}, "flow signal with no enclosing loop"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantInternal(t, runMalformed(t, c.p), c.sub)
		})
	}
}

func TestSeam7CallNativeUnderflow(t *testing.T) {
	r := seam7Reg(t)
	sig := Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1,
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewInteger(0)}, nil
		})}
	p := &Program{
		Code:  []Instr{{Op: OpCallNative, Arg: 0}},
		Debug: []SrcPos{{}},
		Sigs:  []SigRef{{Word: "twoarg", Sig: &sig}},
	}
	_, err := RunProgram(p, r)
	wantInternal(t, err, "CALL_NATIVE underflow at twoarg")
}

func TestSeam7CallUserUnderflow(t *testing.T) {
	p := &Program{
		Code:  []Instr{{Op: OpCallUser, Arg: 0}},
		Debug: []SrcPos{{}},
		Fns:   []CompiledFn{{Name: "f", NParams: 2, NLocals: 2}},
	}
	wantInternal(t, runMalformed(t, p), "CALL_USER underflow at f")
}

func TestSeam7BackwardJumpNotToForNext(t *testing.T) {
	// A backward JMP whose target is not a FOR_NEXT is rejected.
	p := &Program{
		Consts: []Value{NewInteger(1)},
		Code:   []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpJmp, Arg: 0}},
		Debug:  []SrcPos{{}, {}},
	}
	wantInternal(t, runMalformed(t, p), "backward jump not to a FOR_NEXT")
}

func TestSeam7BackwardConditionalJump(t *testing.T) {
	// A JMP_IF_FALSE whose (taken) target is <= pc is rejected.
	p := &Program{
		Consts: []Value{NewBoolean(false)},
		Code:   []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpJmpIfFalse, Arg: 0}},
		Debug:  []SrcPos{{}, {}},
	}
	wantInternal(t, runMalformed(t, p), "backward conditional jump")
}

func TestSeam7UnresolvableType(t *testing.T) {
	p := &Program{
		Code:  []Instr{{Op: OpPushType, Arg: 0}},
		Debug: []SrcPos{{}},
		Types: []TypeRef{{Name: "Bogus", ID: "no-such-id"}},
	}
	wantInternal(t, runMalformed(t, p), "unresolvable type operand Bogus")
}

func TestSeam7CodeUnitEndedWithoutRet(t *testing.T) {
	// main CALL_USERs into a unit that runs off the end with a live frame.
	p := &Program{
		Consts: []Value{NewInteger(7)},
		Code:   []Instr{{Op: OpCallUser, Arg: 0}},
		Debug:  []SrcPos{{}},
		Fns: []CompiledFn{{
			Name: "noret", NParams: 0, NLocals: 0,
			Code:  []Instr{{Op: OpPushConst, Arg: 0}},
			Debug: []SrcPos{{}},
		}},
	}
	wantInternal(t, runMalformed(t, p), "code unit ended without RET")
}

// --- opForSetup range errors (direct) ------------------------------------

func TestSeam7ForSetupRangeErrors(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	// Non-integer range triple.
	_, _, err := vc.opForSetup([]Value{NewString("a"), NewString("b"), NewString("c")}, nil, 0, nil, -1, 0, seam7Dbg)
	wantErr(t, err, "range must be concrete Integers")
	// Zero step (stack top→ start, then end, then step).
	_, _, err = vc.opForSetup([]Value{NewInteger(0), NewInteger(5), NewInteger(1)}, nil, 0, nil, -1, 0, seam7Dbg)
	wantErr(t, err, "step cannot be zero")
	// Underflow.
	_, _, err = vc.opForSetup([]Value{NewInteger(1)}, nil, 0, nil, -1, 0, seam7Dbg)
	wantInternal(t, err, "FOR_SETUP underflow")
}

// --- vmMark error arms (direct) ------------------------------------------

func TestSeam7VmMarkErrors(t *testing.T) {
	if _, _, err := vmMark(OpDropToMark, nil, []Value{NewInteger(1)}, seam7Dbg, 0); err == nil {
		t.Error("DROP_TO_MARK with no mark did not error")
	} else {
		wantInternal(t, err, "DROP_TO_MARK with no open mark")
	}
	// mark above current depth.
	if _, _, err := vmMark(OpDropToMark, []int{5}, []Value{NewInteger(1)}, seam7Dbg, 0); err == nil {
		t.Error("DROP_TO_MARK above depth did not error")
	} else {
		wantInternal(t, err, "DROP_TO_MARK above current depth")
	}
	if _, _, err := vmMark(OpPopMark, nil, nil, seam7Dbg, 0); err == nil {
		t.Error("POP_MARK with no mark did not error")
	} else {
		wantInternal(t, err, "POP_MARK with no open mark")
	}
}

// --- checkParamContract / checkNativeParamContract / checkReturnContract --

func TestSeam7CheckParamContractNilParam(t *testing.T) {
	r := seam7Reg(t)
	// A nil param slot is a guaranteed pass (the continue arm).
	fn := &CompiledFn{Name: "f", Params: []*Type{nil}}
	if err := checkParamContract(r, fn, []Value{NewInteger(1)}); err != nil {
		t.Errorf("nil param should pass, got %v", err)
	}
}

func TestSeam7CheckNativeParamContractArms(t *testing.T) {
	r := seam7Reg(t)
	// break arm: more args than the sig's TotalArgs.
	oneArg := Signature{Args: []*Type{TInteger}, BarrierPos: -1}
	if err := checkNativeParamContract(r, &SigRef{Word: "one", Sig: &oneArg}, []Value{NewInteger(1), NewInteger(2)}); err != nil {
		t.Errorf("extra args past sig arity should be ignored, got %v", err)
	}
	// Any slot: guaranteed pass.
	anySig := Signature{Args: []*Type{TAny}, BarrierPos: -1}
	if err := checkNativeParamContract(r, &SigRef{Word: "any", Sig: &anySig}, []Value{NewString("x")}); err != nil {
		t.Errorf("Any slot should pass, got %v", err)
	}
	// QuoteArgs slot: skipped (literal atom key bound as data).
	quoteSig := Signature{Args: []*Type{TInteger}, QuoteArgs: map[int]bool{0: true}, BarrierPos: -1}
	if err := checkNativeParamContract(r, &SigRef{Word: "q", Sig: &quoteSig}, []Value{NewAtom("k")}); err != nil {
		t.Errorf("QuoteArgs slot should be skipped, got %v", err)
	}
	// Mismatch raises the byte-identical signature_error.
	intSig := Signature{Args: []*Type{TInteger}, BarrierPos: -1}
	err := checkNativeParamContract(r, &SigRef{Word: "iw", Sig: &intSig}, []Value{NewString("x")})
	wantErr(t, err, "no matching signature for iw")
}

func TestSeam7CheckReturnContractUnderflow(t *testing.T) {
	r := seam7Reg(t)
	fn := &CompiledFn{Name: "g", Returns: []*Type{TInteger}}
	// Re-entrant (no frame) with too few values on the stack.
	_, err := checkReturnContract(r, fn, nil, 0, false)
	wantErr(t, err, "g")
	if err == nil {
		t.Fatal("underflow return count did not error")
	}
}

// --- vmMakeMap / vmInterp underflow (direct) -----------------------------

func TestSeam7VmMakeMapInterpUnderflow(t *testing.T) {
	p := &Program{
		MakeMaps: []MakeMapSpec{{Keys: []string{"a", "b"}}},
		Interps:  []InterpSpec{{NHoles: 2, Segs: []InterpSeg{{Hole: true}, {Hole: true}}}},
	}
	_, err := vmMakeMap(p, nil, 0, seam7Dbg, 0)
	wantInternal(t, err, "MAKE_MAP stack underflow")
	_, err = vmInterp(p, nil, 0, seam7Dbg, 0)
	wantInternal(t, err, "INTERP stack underflow")
}

// --- callPoly / matchUserPoly error arms (direct) ------------------------

func TestSeam7CallPolyUnderflow(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, err := vc.callPoly(&PolyRef{Word: "p", Arity: 2}, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_NATIVE_POLY underflow at p")
}

func TestSeam7MatchUserPolyArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	// underflow
	_, _, err := vc.matchUserPoly(&UserPolyRef{Word: "u", Arity: 2}, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_USER_POLY underflow at u")
	// unresolved fn (arity 0, so no underflow; Lookup misses)
	_, _, err = vc.matchUserPoly(&UserPolyRef{Word: "nosuchfn", Arity: 0}, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_USER_POLY unresolved fn nosuchfn")
}

// --- callDynamic family error arms (direct) ------------------------------

func TestSeam7CallDynamicArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	// leading underflow
	_, err := vc.callDynamic(2, false, []Value{NewInteger(1)}, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYNAMIC underflow")
	// trailing with arity != 1
	_, err = vc.callDynamic(2, true, []Value{NewInteger(1), NewInteger(2), NewInteger(3)}, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYNAMIC_TRAILING with arity != 1")
	// non-callable leading: value + args stay put.
	got, err := vc.callDynamic(1, false, []Value{NewInteger(9), NewInteger(5)}, seam7Dbg, 0)
	if err != nil {
		t.Fatalf("non-callable leading: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("non-callable leading residual len=%d, want 2", len(got))
	}
}

func TestSeam7CallDynTrailTopArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, err := vc.callDynTrailTop(1, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYN_TRAIL_TOP underflow")
	// non-callable: [args, fn] left untouched.
	got, err := vc.callDynTrailTop(1, []Value{NewInteger(5), NewInteger(9)}, seam7Dbg, 0)
	if err != nil {
		t.Fatalf("trail-top non-callable: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("trail-top non-callable residual len=%d, want 2", len(got))
	}
}

func TestSeam7CallDynApplyTopArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, err := vc.callDynApplyTop(1, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYN_APPLY_TOP underflow")
	// A non-FnDefInfo, non-closure value raises applyHandler's own error.
	_, err = vc.callDynApplyTop(1, []Value{NewInteger(5), NewInteger(9)}, seam7Dbg, 0)
	wantErr(t, err, "carries no FnDefInfo")
}

func TestSeam7CallDynMethodArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, err := vc.callDynMethod(&DynMethodSpec{Word: "m", NArgs: 1, NOut: 1}, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYN_METHOD underflow at m")
	// non-appliable value on top: shape claim failed → defer.
	_, err = vc.callDynMethod(&DynMethodSpec{Word: "m", NArgs: 1, NOut: 1}, []Value{NewInteger(5), NewInteger(9)}, seam7Dbg, 0)
	wantInternal(t, err, "is not an appliable function at run time")
}

func TestSeam7CallDynamicMixedUnderflow(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, err := vc.callDynamicMixed(2, []Value{NewInteger(1)}, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYNAMIC_MIXED underflow")
	_, err = vc.callDynamicMixed(0, nil, seam7Dbg, 0)
	wantInternal(t, err, "CALL_DYNAMIC_MIXED underflow")
}

// --- isDelegationFnDef / tryNativeFnApply (direct) -----------------------

func TestSeam7IsDelegationFnDefEmpty(t *testing.T) {
	if isDelegationFnDef(FnDefInfo{}) {
		t.Error("sig-less FnDefInfo reported as delegation")
	}
}

func TestSeam7TryNativeFnApplyNoSigs(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	// No registered inner and no own signatures → not applied (done=false).
	_, done, err := vc.tryNativeFnApply(FnDefInfo{Name: "unregistered-xyz"}, nil)
	if done || err != nil {
		t.Errorf("no-sig fn: done=%v err=%v, want done=false err=nil", done, err)
	}
	// Own signatures present but no runtime match → still not applied.
	fd := FnDefInfo{Name: "unregistered-xyz", Signatures: []Signature{{Args: []*Type{TInteger}, BarrierPos: -1}}}
	_, done, err = vc.tryNativeFnApply(fd, []Value{NewString("x")})
	if done || err != nil {
		t.Errorf("own-sig no-match: done=%v err=%v, want done=false err=nil", done, err)
	}
}

// --- runFallback error arms (direct) -------------------------------------

func TestSeam7RunFallbackArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, err := vc.runFallback(&FallbackSpan{NIn: 2, Desc: "d"}, nil, seam7Dbg, 0)
	wantInternal(t, err, "FALLBACK underflow at d")
	// NIn > 1 with enough stack: the lowerer never threads >1, so it is refused.
	_, err = vc.runFallback(&FallbackSpan{NIn: 2, Desc: "d"}, []Value{NewInteger(1), NewInteger(2)}, seam7Dbg, 0)
	wantInternal(t, err, "FALLBACK threads >1 input at d")
}

// --- flowSignal no-loop (direct) -----------------------------------------

func TestSeam7FlowSignalNoLoop(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	_, _, _, _, _, _, err := vc.flowSignal(OpFlowBreak, nil, nil, nil, nil, 0, -1, seam7Dbg)
	wantInternal(t, err, "flow signal with no enclosing loop")
}

// --- closure fn-value apply branches -------------------------------------

// seam7IdentityClosureProg builds a program that pushes an argument, an
// identity closure ON TOP of it, then applies the given trailing-fn opcode
// (OpCallDynTrailTop / OpCallDynApplyTop / OpCallDynMethod), driving the
// closure-invoke branch of the corresponding apply method.
func seam7IdentityClosureProg(applyOp Opcode) *Program {
	p := &Program{
		Consts: []Value{NewInteger(5)},
		Code: []Instr{
			{Op: OpPushConst, Arg: 0},   // arg 5
			{Op: OpPushClosure, Arg: 0}, // identity closure on top
			{Op: applyOp, Arg: 1},       // apply the closure to its 1 arg
		},
		Debug: []SrcPos{{}, {}, {}},
		Fns: []CompiledFn{{
			Name: "id", NParams: 1, NLocals: 1, Returns: []*Type{TAny},
			Code:  []Instr{{Op: OpPushLocal, Arg: 0}, {Op: OpRet}},
			Debug: []SrcPos{{}, {}},
		}},
	}
	if applyOp == OpCallDynMethod {
		p.DynMethods = []DynMethodSpec{{Word: "m", NArgs: 1, NOut: 1}}
		p.Code[2] = Instr{Op: OpCallDynMethod, Arg: 0}
	}
	return p
}

func TestSeam7ClosureApplyBranches(t *testing.T) {
	for _, op := range []Opcode{OpCallDynTrailTop, OpCallDynApplyTop, OpCallDynMethod} {
		p := seam7IdentityClosureProg(op)
		got, err := RunProgram(p, seam7Reg(t))
		if err != nil {
			t.Fatalf("%v closure apply: %v", op, err)
		}
		if len(got) != 1 {
			t.Fatalf("%v closure apply residual len=%d, want 1", op, len(got))
		}
		if n, _ := got[0].AsConcreteInteger(); n != 5 {
			t.Errorf("%v identity closure returned %d, want 5", op, n)
		}
	}
}

// --- run()-level CALL_DYN error propagation arms -------------------------

func TestSeam7RunCallDynErrArms(t *testing.T) {
	// OpCallDynTrailTop / OpCallDynApplyTop with an empty stack: the method
	// underflows and run() propagates the error (vm.go run-loop err arms).
	for _, op := range []Opcode{OpCallDynTrailTop, OpCallDynApplyTop} {
		p := &Program{Code: []Instr{{Op: op, Arg: 1}}, Debug: []SrcPos{{}}}
		_, err := RunProgram(p, seam7Reg(t))
		if err == nil {
			t.Fatalf("%v empty-stack apply did not error", op)
		}
		wantInternal(t, err, "underflow")
	}
}

// --- delegation fn-value apply branches ----------------------------------

// seam7DelegReg registers a `cinc` (Integer→Integer, n+1) native and a
// `cfail` (always errors) native, and returns delegation fn VALUES wrapping
// each — a trivial-delegation FnDefInfo whose only sig body is `[Word(name)]`.
func seam7DelegReg(t *testing.T) (r *Registry, inc, fail Value) {
	t.Helper()
	r = seam7Reg(t)
	r.RegisterNativeFunc(NativeFunc{
		Name: "cinc",
		Signatures: []Signature{{
			Args:    []*Type{TInteger},
			Returns: []*Type{TInteger}, BarrierPos: -1,
			Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				n, _ := AsInteger(a[0])
				return []Value{NewInteger(n + 1)}, nil
			}),
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name: "cfail",
		Signatures: []Signature{{
			Args:    []*Type{TInteger},
			Returns: []*Type{TInteger}, BarrierPos: -1,
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				return nil, r.AqlError("value_error", "cfail: boom", "cfail")
			}),
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	deleg := func(name string) Value {
		return NewFnDef(FnDefInfo{
			Name: name, Registry: r,
			Signatures: []Signature{{
				Args: []*Type{TInteger}, Returns: []*Type{TInteger}, BarrierPos: -1,
				Impl: AQL([]Value{NewWord(name)}),
			}},
		})
	}
	return r, deleg("cinc"), deleg("cfail")
}

func TestSeam7DelegationApplySuccess(t *testing.T) {
	r, inc, _ := seam7DelegReg(t)
	if !isDelegationFnDef(inc.Data.(FnDefInfo)) {
		t.Fatal("cinc wrapper is not recognised as a delegation fn")
	}
	vc := seam7VC(r)
	// callDynTrailTop / callDynApplyTop: fn ON TOP of its arg → [arg, fn].
	for _, apply := range []func(int, []Value, []SrcPos, int) ([]Value, error){vc.callDynTrailTop, vc.callDynApplyTop} {
		got, err := apply(1, []Value{NewInteger(5), inc}, seam7Dbg, 0)
		if err != nil {
			t.Fatalf("delegation apply: %v", err)
		}
		if n, _ := got[0].AsConcreteInteger(); n != 6 {
			t.Errorf("delegation cinc(5) = %d, want 6", n)
		}
	}
	// callDynamic: LEADING fn → [fn, arg].
	got, err := vc.callDynamic(1, false, []Value{inc, NewInteger(5)}, seam7Dbg, 0)
	if err != nil {
		t.Fatalf("leading delegation apply: %v", err)
	}
	if n, _ := got[0].AsConcreteInteger(); n != 6 {
		t.Errorf("leading delegation cinc(5) = %d, want 6", n)
	}
	// callDynMethod: fn ON TOP, shape claim {NArgs:1, NOut:1}.
	got, err = vc.callDynMethod(&DynMethodSpec{Word: "cinc", NArgs: 1, NOut: 1}, []Value{NewInteger(5), inc}, seam7Dbg, 0)
	if err != nil {
		t.Fatalf("method delegation apply: %v", err)
	}
	if n, _ := got[0].AsConcreteInteger(); n != 6 {
		t.Errorf("method delegation cinc(5) = %d, want 6", n)
	}
}

func TestSeam7DelegationApplyError(t *testing.T) {
	r, _, fail := seam7DelegReg(t)
	vc := seam7VC(r)
	// Each apply path must surface the inner handler's error (delegation err arm).
	_, err := vc.callDynTrailTop(1, []Value{NewInteger(5), fail}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
	_, err = vc.callDynApplyTop(1, []Value{NewInteger(5), fail}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
	_, err = vc.callDynamic(1, false, []Value{fail, NewInteger(5)}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
	_, err = vc.callDynMethod(&DynMethodSpec{Word: "cfail", NArgs: 1, NOut: 1}, []Value{NewInteger(5), fail}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
}

// --- closure-invoke error arms -------------------------------------------

// seam7ErrClosureProg builds a program whose closure declares a return type
// its body violates, so invokeClosure surfaces the return-type error and each
// apply method takes its closure-invoke ERROR arm.
func seam7ErrClosureProg(applyOp Opcode) *Program {
	p := &Program{
		Consts: []Value{NewInteger(5), NewString("x")},
		Code: []Instr{
			{Op: OpPushConst, Arg: 0},
			{Op: OpPushClosure, Arg: 0},
			{Op: applyOp, Arg: 1},
		},
		Debug: []SrcPos{{}, {}, {}},
		Fns: []CompiledFn{{
			Name: "bad", NParams: 1, NLocals: 1, Returns: []*Type{TInteger},
			Code:  []Instr{{Op: OpPushConst, Arg: 1}, {Op: OpRet}}, // returns a String
			Debug: []SrcPos{{}, {}},
		}},
	}
	if applyOp == OpCallDynMethod {
		p.DynMethods = []DynMethodSpec{{Word: "m", NArgs: 1, NOut: 1}}
		p.Code[2] = Instr{Op: OpCallDynMethod, Arg: 0}
	}
	return p
}

func TestSeam7ClosureApplyErrorArms(t *testing.T) {
	for _, op := range []Opcode{OpCallDynTrailTop, OpCallDynApplyTop, OpCallDynMethod} {
		_, err := RunProgram(seam7ErrClosureProg(op), seam7Reg(t))
		if err == nil {
			t.Fatalf("%v erroring closure did not surface an error", op)
		}
	}
	// callDynamic's leading closure error arm: [closure, arg], apply leading.
	p := &Program{
		Consts: []Value{NewInteger(5), NewString("x")},
		Code: []Instr{
			{Op: OpPushClosure, Arg: 0}, // closure at the base
			{Op: OpPushConst, Arg: 0},   // its arg on top
			{Op: OpCallDynamic, Arg: 1},
		},
		Debug: []SrcPos{{}, {}, {}},
		Fns: []CompiledFn{{
			Name: "bad", NParams: 1, NLocals: 1, Returns: []*Type{TInteger},
			Code:  []Instr{{Op: OpPushConst, Arg: 1}, {Op: OpRet}},
			Debug: []SrcPos{{}, {}},
		}},
	}
	if _, err := RunProgram(p, seam7Reg(t)); err == nil {
		t.Fatal("leading closure error arm did not surface an error")
	}
}

// --- island-run error arms -----------------------------------------------

// seam7UserFail returns an appliable, non-delegation, non-closure user-fn
// VALUE whose body calls the erroring `cfail` native — so applying it forces
// the island sub-engine to error, exercising each apply method's island error
// arm. (A named param disqualifies it from the trivial-delegation fast path.)
func seam7UserFail(r *Registry) Value {
	return NewFnDef(FnDefInfo{
		Name: "cuserfail", Registry: r,
		Signatures: []Signature{{
			Params:  []FnParam{{Name: "n", Type: TInteger}},
			Returns: []*Type{TInteger}, BarrierPos: BarrierAllForward,
			Impl: AQL([]Value{NewWord("cfail"), NewWord("n")}),
		}},
	})
}

func TestSeam7IslandApplyErrorArms(t *testing.T) {
	r, _, _ := seam7DelegReg(t)
	fn := seam7UserFail(r)
	if isDelegationFnDef(fn.Data.(FnDefInfo)) {
		t.Fatal("user fail fn should NOT be a trivial delegation")
	}
	vc := seam7VC(r)
	_, err := vc.callDynTrailTop(1, []Value{NewInteger(5), fn}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
	_, err = vc.callDynApplyTop(1, []Value{NewInteger(5), fn}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
	_, err = vc.callDynMethod(&DynMethodSpec{Word: "cuserfail", NArgs: 1, NOut: 1}, []Value{NewInteger(5), fn}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
	// callDynamicMixed islands its window verbatim — a window that calls cfail
	// errors through the island (the mixed island error arm).
	_, err = vc.callDynamicMixed(2, []Value{NewWord("cfail"), NewInteger(5)}, seam7Dbg, 0)
	wantErr(t, err, "cfail: boom")
}

// TestSeam7MatchUserPolyUnitShapeMismatch drives the unit-shape guard: a
// recorded arm whose Impl/arity still match the live table but whose compiled
// unit index is out of range (a compile/run drift) is refused.
func TestSeam7MatchUserPolyUnitShapeMismatch(t *testing.T) {
	r := seam7Reg(t)
	InstallFnDef(r, "cpoly", FnDefInfo{
		Signatures: []Signature{{
			Params:  []FnParam{{Name: "n", Type: TAny}},
			Returns: []*Type{TAny}, BarrierPos: BarrierAllForward,
			Impl: AQL([]Value{NewWord("n")}),
		}},
	})
	fd := r.Lookup("cpoly")
	if fd == nil || len(fd.Signatures) == 0 {
		t.Fatal("cpoly not installed")
	}
	n := fd.Signatures[0].TotalArgs()
	vc := &vmContext{r: r, p: &Program{}} // no Fns → any unit index is out of range
	pr := &UserPolyRef{
		Word: "cpoly", Arity: n,
		SigIdx: []int{0}, Units: []int{999}, Impls: []SigImpl{fd.Signatures[0].Impl},
	}
	window := make([]Value, n)
	for i := range window {
		window[i] = NewInteger(1)
	}
	_, _, err := vc.matchUserPoly(pr, window, seam7Dbg, 0)
	wantInternal(t, err, "CALL_USER_POLY unit shape mismatch at cpoly")
}

// TestSeam7CallUserPolyParamContract drives OpCallUserPoly where the arm
// matches at the signature level (an Any param) but the compiled fn's concrete
// param type rejects the runtime value — the param-contract guard the checker
// installs for gradual poly dispatch.
func TestSeam7CallUserPolyParamContract(t *testing.T) {
	r := seam7Reg(t)
	InstallFnDef(r, "cpoly2", FnDefInfo{
		Signatures: []Signature{{
			Params:  []FnParam{{Name: "n", Type: TAny}},
			Returns: []*Type{TAny}, BarrierPos: BarrierAllForward,
			Impl: AQL([]Value{NewWord("n")}),
		}},
	})
	fd := r.Lookup("cpoly2")
	n := fd.Signatures[0].TotalArgs()
	p := &Program{
		Consts: []Value{NewString("x")}, // a String flows into a concrete Integer param
		Code:   []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpCallUserPoly, Arg: 0}},
		Debug:  []SrcPos{{}, {}},
		UserPolys: []UserPolyRef{{
			Word: "cpoly2", Arity: n,
			SigIdx: []int{0}, Units: []int{0}, Impls: []SigImpl{fd.Signatures[0].Impl},
		}},
		Fns: []CompiledFn{{
			Name: "cpoly2", NParams: n, NLocals: n, Params: []*Type{TInteger}, Returns: []*Type{TAny},
			Code:  []Instr{{Op: OpPushLocal, Arg: 0}, {Op: OpRet}},
			Debug: []SrcPos{{}, {}},
		}},
	}
	_, err := RunProgram(p, r)
	wantErr(t, err, "no matching signature for cpoly2")
}

func TestSeam7CallNativeHandlerTokenScreened(t *testing.T) {
	r := seam7Reg(t)
	sig := Signature{
		Args: nil, BarrierPos: -1,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewWord("leak")}, nil // a tape-coupled token must never reach the stack
		}),
	}
	p := &Program{
		Code:  []Instr{{Op: OpCallNative, Arg: 0}},
		Debug: []SrcPos{{}},
		Sigs:  []SigRef{{Word: "cretword", Sig: &sig}},
	}
	_, err := RunProgram(p, r)
	wantInternal(t, err, "tape-coupled handler result at cretword")
}
