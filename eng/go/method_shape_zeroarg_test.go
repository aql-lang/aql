package eng

import (
	"strings"
	"testing"
)

// method_shape_zeroarg_test.go pins the shaped 0-arg landing model's decline
// arms and helpers (the guard-owned re-home of the miscompile-E auto-dispatch
// guard): allZeroArgSigs / fnDefName, the 0-arg window path's per-signature
// gates in shapedMethodApplyWindow, NoteMethodShape's non-delegation decline,
// and the NUnnamed arms of the VM's return contract.

// z9Member builds a named trivial-delegation fn VALUE over inner natives
// registered in r (the shaped-instance-method class).
func z9Member(name string, r *Registry, sigs ...Signature) Value {
	return NewFnDef(FnDefInfo{Name: name, Registry: r, Signatures: sigs})
}

func z9Engine(t *testing.T, r *Registry, tape []Value) *Engine {
	t.Helper()
	e := NewTop(r)
	e.tape = NewTape(tape, 8)
	e.pointer = 0
	return e
}

func TestAllZeroArgSigsTable(t *testing.T) {
	zero := Signature{Returns: []*Type{TAny}, BarrierPos: -1, Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewInteger(1)}, nil
	})}
	one := zero
	one.Args = []*Type{TInteger}
	fb := zero
	fb.Fallback = true
	for _, c := range []struct {
		name string
		fn   *FnDefInfo
		want bool
	}{
		{"all-zero", &FnDefInfo{Signatures: []Signature{zero}}, true},
		{"zero-plus-fallback", &FnDefInfo{Signatures: []Signature{zero, fb}}, true},
		{"mixed-arity", &FnDefInfo{Signatures: []Signature{zero, one}}, false},
		{"only-fallback", &FnDefInfo{Signatures: []Signature{fb}}, false},
		{"arg-taking", &FnDefInfo{Signatures: []Signature{one}}, false},
	} {
		if got := allZeroArgSigs(c.fn); got != c.want {
			t.Errorf("%s: allZeroArgSigs = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFnDefNameFallbacks(t *testing.T) {
	r := seam7Reg(t)
	named := z9Member("z9named", r, Signature{Returns: []*Type{TAny}, BarrierPos: -1})
	if got := fnDefName(named); got != "z9named" {
		t.Errorf("fnDefName(named) = %q, want z9named", got)
	}
	if got := fnDefName(NewInteger(3)); got != "fn value" {
		t.Errorf("fnDefName(non-fn) = %q, want the generic label", got)
	}
	if got := fnDefName(NewFnDef(FnDefInfo{})); got != "fn value" {
		t.Errorf("fnDefName(anonymous) = %q, want the generic label", got)
	}
}

// The 0-arg window path: a bare landed member (no forward window) models only
// a GENUINE 0-arg overload whose inner signature is a plain Go handler; a
// gate-failing 0-arg sig, an inner without any 0-arg sig, and a non-0-arg
// member all decline.
func TestShapedMethodApplyWindowZeroArgGates(t *testing.T) {
	impl := Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewInteger(7)}, nil
	})
	memberSig := Signature{Returns: []*Type{TAny}, BarrierPos: -1, Impl: AQL([]Value{NewWord("z9x")})}

	// Inner 0-arg sig behind a NON-0-arg sibling: the loop skips the sibling
	// (continue) and models the 0-arg one.
	r1 := seam7Reg(t)
	r1.RegisterNativeFunc(NativeFunc{Name: "z9x", Signatures: []Signature{
		{Args: []*Type{TInteger}, Returns: []*Type{TAny}, BarrierPos: -1, Impl: impl},
		{Returns: []*Type{TAny}, BarrierPos: -1, Impl: impl},
	}})
	if err := r1.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	m1 := z9Member("z9x", r1, memberSig)
	e1 := z9Engine(t, r1, []Value{m1})
	sg, positions, ok := e1.shapedMethodApplyWindow(0, m1)
	if !ok || sg == nil || sg.TotalArgs() != 0 || positions != nil {
		t.Errorf("mixed-sibling inner: want the 0-arg sig with no positions, got sig=%v pos=%v ok=%v", sg, positions, ok)
	}

	// Inner 0-arg sig failing the plain-Go-handler gate (NoEvalArgs): decline.
	r2 := seam7Reg(t)
	r2.RegisterNativeFunc(NativeFunc{Name: "z9x", Signatures: []Signature{
		{Returns: []*Type{TAny}, BarrierPos: -1, Impl: impl, NoEvalArgs: map[int]bool{0: true}},
	}})
	if err := r2.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	m2 := z9Member("z9x", r2, memberSig)
	e2 := z9Engine(t, r2, []Value{m2})
	if _, _, ok := e2.shapedMethodApplyWindow(0, m2); ok {
		t.Error("NoEvalArgs 0-arg inner sig must not model")
	}

	// Inner with NO 0-arg sig at all (the member's 0-arg claim has no inner
	// twin): the loop finds nothing — terminal decline.
	r3 := seam7Reg(t)
	r3.RegisterNativeFunc(NativeFunc{Name: "z9x", Signatures: []Signature{
		{Args: []*Type{TInteger}, Returns: []*Type{TAny}, BarrierPos: -1, Impl: impl},
	}})
	if err := r3.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	m3 := z9Member("z9x", r3, memberSig)
	e3 := z9Engine(t, r3, []Value{m3})
	if _, _, ok := e3.shapedMethodApplyWindow(0, m3); ok {
		t.Error("inner without a 0-arg sig must not model")
	}

	// A member WITHOUT a genuine 0-arg overload stays data (the
	// !fnValueZeroArg decline).
	r4 := seam7Reg(t)
	r4.RegisterNativeFunc(NativeFunc{Name: "z9y", Signatures: []Signature{
		{Args: []*Type{TInteger}, Returns: []*Type{TAny}, BarrierPos: -1, Impl: impl},
	}})
	if err := r4.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	m4Sig := memberSig
	m4Sig.Args = []*Type{TInteger}
	m4Sig.Impl = AQL([]Value{NewWord("z9y")})
	m4 := z9Member("z9y", r4, m4Sig, m4Sig) // two 1-arg sigs: zeros < 2
	e4 := z9Engine(t, r4, []Value{m4})
	if _, _, ok := e4.shapedMethodApplyWindow(0, m4); ok {
		t.Error("a member without a genuine 0-arg overload must not take the 0-arg path")
	}
}

// NoteMethodShape declines a plain (non-delegation) fn member — a real body
// keeps today's refusal paths.
func TestNoteMethodShapeDeclinesNonDelegation(t *testing.T) {
	r := seam7Reg(t)
	r.Check.Mode = true
	realBody := NewFnDef(FnDefInfo{Name: "z9real", Registry: r, Signatures: []Signature{{
		Params: []FnParam{{Name: "n", Type: TInteger}}, Returns: []*Type{TAny},
		BarrierPos: -1, Impl: AQL([]Value{NewWord("n"), NewWord("n")}),
	}}})
	out := NewDynamicCarrier(TAny)
	r.Check.NoteMethodShape(out, realBody)
	if _, ok := r.Check.methodShapeMember(out.ID); ok {
		t.Error("a non-delegation member must not be annotated")
	}
}

// noteShapedRead self-gates on an inactive recorder.
func TestNoteShapedReadInactive(t *testing.T) {
	es := &EmitState{}
	es.noteShapedRead("some-id")
	if es.shapedReads != nil {
		t.Error("inactive noteShapedRead must record nothing")
	}
	if es.shapedReadOut([]Value{NewDynamicCarrier(TAny)}) {
		t.Error("shapedReadOut over an empty table must be false")
	}
}

// The RET contract's NUnnamed arms (both frame shapes): deficit errors, an
// over-allowance surplus errors after the trim, and the frameless trim caps
// at NUnnamed.
func TestCheckReturnContractNUnnamedArms(t *testing.T) {
	r := seam7Reg(t)
	fn := &CompiledFn{Name: "z9f", Returns: []*Type{TInteger}, NUnnamed: 1}

	// hasFrame deficit: 0 produced above the base, 1 declared.
	if _, err := checkReturnContract(r, fn, []Value{NewInteger(9)}, 1, true); err == nil {
		t.Error("frame deficit must error")
	} else if !strings.Contains(err.Error(), "z9f") {
		t.Errorf("deficit error %q should name the fn", err)
	}

	// hasFrame surplus beyond the unnamed allowance: 3 produced, 1 declared,
	// NUnnamed 1 → 1 extra tolerated, the second errors.
	stack := []Value{NewInteger(1), NewInteger(2), NewInteger(3)}
	if _, err := checkReturnContract(r, fn, stack, 0, true); err == nil {
		t.Error("frame surplus beyond NUnnamed must error")
	}

	// hasFrame surplus within the allowance: trimmed from the frame bottom.
	stack = []Value{NewInteger(1), NewInteger(2)}
	out, err := checkReturnContract(r, fn, stack, 0, true)
	if err != nil {
		t.Fatalf("frame trim: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("frame trim left %v, want the single declared return", out)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 2 {
		t.Errorf("frame trim kept %v, want the TOP value 2", out[0])
	}

	// Frameless trim: extra bottoms discarded up to NUnnamed.
	out, err = checkReturnContract(r, fn, []Value{NewInteger(1), NewInteger(2)}, 0, false)
	if err != nil {
		t.Fatalf("frameless trim: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("frameless trim left %v, want one value", out)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 2 {
		t.Errorf("frameless trim kept %v, want the TOP value 2", out[0])
	}

	// Frameless trim CAP: extra 2 but NUnnamed 1 — only one discards, the
	// remaining surplus stays (the residual absorbs it).
	out, err = checkReturnContract(r, fn, []Value{NewInteger(1), NewInteger(2), NewInteger(3)}, 0, false)
	if err != nil {
		t.Fatalf("frameless capped trim: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("frameless capped trim left %v, want 2 values (cap at NUnnamed)", out)
	}
}

// The guard-owned decline: an annotated genuine-0-arg member whose landing
// the window model cannot claim (a raw Word in the statement window) REFUSES
// the program — the read guard was skipped for the annotated read, so the
// landing owns the miscompile-E refusal.
func TestShapedMethodGuardOwnedDecline(t *testing.T) {
	r := seam7Reg(t)
	impl := Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewInteger(7)}, nil
	})
	r.RegisterNativeFunc(NativeFunc{Name: "z9g", Signatures: []Signature{
		{Returns: []*Type{TAny}, BarrierPos: -1, Impl: impl},
	}})
	if err := r.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	member := z9Member("z9g", r, Signature{
		Returns: []*Type{TAny}, BarrierPos: -1, Impl: AQL([]Value{NewWord("z9g")}),
	})
	done := r.Check.BeginCompilePass()
	defer done()
	carrier := NewDynamicCarrier(TAny)
	r.Check.NoteMethodShape(carrier, member)
	if _, ok := r.Check.methodShapeMember(carrier.ID); !ok {
		t.Fatal("genuine-0-arg delegation member must be annotated")
	}
	e := z9Engine(t, r, []Value{carrier, NewWord("z9unmodelable")})
	if e.tryShapedMethodDispatch(0) {
		t.Fatal("an unclaimable landing must not be consumed")
	}
	es, ok := r.Check.Emit.(*EmitState)
	if !ok {
		t.Fatal("compile pass recorder missing")
	}
	if es.Compilable || !strings.Contains(es.Reason, "shaped 0-arg method landing not modelable at z9g") {
		t.Errorf("want the guard-owned refusal, got compilable=%v reason=%q", es.Compilable, es.Reason)
	}
}

// A runtime user-poly window that matches NO baked arm defers to the
// interpreter (matchUserPoly's no-match, reached through OpCallUserPoly),
// and a poly ref whose recorded impl no longer matches the registered
// signature defers with the drift diagnostic.
func TestSeam7CallUserPolyRuntimeNoMatchAndDrift(t *testing.T) {
	r := seam7Reg(t)
	InstallFnDef(r, "z9poly", FnDefInfo{
		Signatures: []Signature{{
			Params:  []FnParam{{Name: "n", Type: TInteger}},
			Returns: []*Type{TAny}, BarrierPos: BarrierAllForward,
			Impl: AQL([]Value{NewWord("n")}),
		}},
	})
	fd := r.Lookup("z9poly")
	p := &Program{
		Consts: []Value{NewBoolean(true)}, // matches no arm (Integer only)
		Code:   []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpCallUserPoly, Arg: 0}},
		Debug:  []SrcPos{{}, {}},
		UserPolys: []UserPolyRef{{
			Word: "z9poly", Arity: 1,
			SigIdx: []int{0}, Units: []int{0}, Impls: []SigImpl{fd.Signatures[0].Impl},
		}},
		Fns: []CompiledFn{{
			Name: "z9poly", NParams: 1, NLocals: 1, Returns: []*Type{TAny},
			Code:  []Instr{{Op: OpPushLocal, Arg: 0}, {Op: OpRet}},
			Debug: []SrcPos{{}, {}},
		}},
	}
	_, err := RunProgram(p, r)
	wantInternal(t, err, "CALL_USER_POLY no match for z9poly")

	// Drift: the recorded impl is not the registered signature's impl.
	vc := seam7VC(r)
	drift := &UserPolyRef{
		Word: "z9poly", Arity: 1,
		SigIdx: []int{0}, Units: []int{0}, Impls: []SigImpl{AQL([]Value{NewWord("other")})},
	}
	_, _, derr := vc.matchUserPoly(drift, []Value{NewInteger(1)}, seam7Dbg, 0)
	wantInternal(t, derr, "CALL_USER_POLY signature drift at z9poly")
}

// A delegation member whose inner overload is AQL-BODIED in a FOREIGN
// sub-registry declines tryNativeFnApply's fast path (the body's words
// resolve in the module's own scope, which only the interpreter's
// foreign-wrapper branch provides) — the caller then islands the apply.
func TestTryNativeFnApplyForeignAQLBodyDeclines(t *testing.T) {
	main := seam7Reg(t)
	foreign := seam7Reg(t)
	// A module-preamble AQL fn: InstallFnDef registers the body-splicing
	// dispatch handler, so the match succeeds and the AQL-impl probe is what
	// declines.
	InstallFnDef(foreign, "z9aqlbody", FnDefInfo{Signatures: []Signature{
		{Returns: []*Type{TAny}, BarrierPos: -1, Impl: AQL([]Value{NewInteger(42)})},
	}})
	member := z9Member("z9aqlbody", foreign, Signature{
		Returns: []*Type{TAny}, BarrierPos: -1, Impl: AQL([]Value{NewWord("z9aqlbody")}),
	})
	vc := seam7VC(main)
	fd, _ := member.Data.(FnDefInfo)
	res, done, err := vc.tryNativeFnApply(fd, nil)
	if done || err != nil || res != nil {
		t.Errorf("foreign AQL-bodied inner must decline the fast path (island territory): res=%v done=%v err=%v",
			res, done, err)
	}
}
