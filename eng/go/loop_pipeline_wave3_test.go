package eng

import (
	"strings"
	"testing"
)

// Coverage for the mark/move interpreter machinery (engine.go: stepMark,
// stepMove, stepMoveCont, stepMoveIf, handleFlowCtrl, handleLoopBreak,
// handleLoopContinue, exitWithFlowCtrl) and the compiled loop pipeline
// (emit.go RecordLoop / ArmLoopCapture / ConsumeLoopArm /
// BeginLoopCarried / EndLoopCarried / Checkpoint, lower.go lowerLoop /
// lowerBreak / lowerContinue / allocLocal, vm.go opForSetup / OpForNext
// / flowSignal). A minimal `cfor` counted loop is registered from
// inside the kernel, mirroring lang's `for` (native_control.go /
// forloop.go): the runtime handler splices mark+body+moveCont tokens,
// the check-mode ReturnsFn analyses the body via AnalyseLoopBody and
// records the loop for the bytecode plan.

// cforRun is the runtime handler: the interpreter's counted loop via
// mark/body/moveCont splicing (the runForLoop shape).
func cforRun(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	n, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, r.AqlError("for_error", "cfor: count must be a concrete Integer", "cfor")
	}
	if n <= 0 {
		return nil, nil
	}
	body := args[1]
	if !IsConcrete(body) {
		return nil, r.AqlError("for_error", "cfor: body must be a concrete list", "cfor")
	}
	lst, _ := AsList(body)
	bodySlice := lst.Slice()

	InstallDef(r, "i", NewInteger(0))
	bodyCopy := make([]Value, len(bodySlice))
	copy(bodyCopy, bodySlice)
	cont := &ForCont{Registry: r, IterName: "i", Current: 0, End: n, Step: 1, Body: bodyCopy}

	id := NextMarkID()
	tokens := make([]Value, 0, len(bodySlice)+2)
	tokens = append(tokens, NewMark(id, bodySlice...))
	iter := make([]Value, len(bodySlice))
	copy(iter, bodySlice)
	tokens = append(tokens, iter...)
	tokens = append(tokens, NewMoveCont(id, "cfor loop", cont))
	return tokens, nil
}

// cforReturns is the check-mode analysis + loop recording (the
// forCarrierAnalyse shape, counted form only).
func cforReturns(args []Value, r *Registry) []Value {
	es := r.Check.Recorder()
	body := args[len(args)-1]
	iter := NewCarrier(TInteger)
	cv := args[0]
	lowerable := IsConcrete(cv) && cv.Parent.ConformsTo(TInteger)
	var startV, endV, stepV Value
	if lowerable {
		startV, endV, stepV = NewInteger(0), cv, NewInteger(1)
		es.ArmLoopCapture()
	}
	stk := AnalyseLoopBody(r, body, []string{"i"}, []Value{iter})
	out := NewCarrier(TList)
	if len(stk) > 0 {
		top := stk[len(stk)-1]
		if IsDisjunct(top) {
			out = NewCarrierTypedListValue(top)
		} else {
			out = NewCarrierTypedList(top.Parent)
		}
	}
	if lowerable {
		frag := es.TakeFragment()
		es.RecordLoop(startV, endV, stepV, frag, stk, iter.ID, out, 0, args[0].Pos())
	}
	return []Value{out}
}

// registerLoopWords adds `cfor`, `break`, and `continue` to a
// covRegistry. break/continue mirror the lang registrations exactly:
// runtime handlers raise the FlowCtrl signal; the recorder's call
// classifier turns the check-mode dispatch into evBreak/evContinue by
// NAME (the words must be called break/continue for that).
func registerLoopWords(r *Registry) {
	registerBranchWords(r)
	r.RegisterNativeFunc(NativeFunc{
		Name: "cfor",
		Signatures: []Signature{{
			Args:       []*Type{TInteger, TList},
			NoEvalArgs: map[int]bool{1: true},
			Impl:       Go(cforRun),
			ReturnsFn:  cforReturns,
			BarrierPos: -1,
		}},
	})
	for name, sig := range map[string]FlowCtrl{"break": FlowBreak, "continue": FlowContinue} {
		ctrl := sig
		r.RegisterNativeFunc(NativeFunc{
			Name: name,
			Signatures: []Signature{{
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					r.FlowCtrl = ctrl
					return nil, nil
				}),
				Returns: []*Type{}, BarrierPos: 0,
			}},
		})
	}
}

// --- interpreter loop paths ------------------------------------------------

func TestInterpretedForLoop(t *testing.T) {
	r := covRegistry(t, registerLoopWords)
	out, err := NewTop(r).Run([]Value{
		NewWord("cfor"), NewInteger(3),
		codeBody(NewWord("cadd"), NewWord("i"), NewInteger(10)),
	})
	if err != nil {
		t.Fatalf("cfor: %v", err)
	}
	if got := renderAll(out); got != "10 | 11 | 12" {
		t.Errorf("cfor results = %q", got)
	}
	// Zero-count loop runs zero times.
	out, err = NewTop(r).Run([]Value{NewWord("cfor"), NewInteger(0), codeBody(NewInteger(1))})
	if err != nil || len(out) != 0 {
		t.Errorf("cfor 0: out=%v err=%v", out, err)
	}
}

func TestInterpretedLoopBreakAndContinue(t *testing.T) {
	r := covRegistry(t, registerLoopWords)
	// break at i>2: only 0 1 2 accumulate.
	out, err := NewTop(r).Run([]Value{
		NewWord("cfor"), NewInteger(5),
		codeBody(
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewWord("i"), NewInteger(2), NewCloseParen(),
			codeBody(NewWord("break")),
			codeBody(NewWord("i")),
		),
	})
	if err != nil {
		t.Fatalf("break loop: %v", err)
	}
	if got := renderAll(out); got != "0 | 1 | 2" {
		t.Errorf("break loop = %q", got)
	}

	// continue at i>2: partial results of later iterations are discarded.
	out, err = NewTop(r).Run([]Value{
		NewWord("cfor"), NewInteger(5),
		codeBody(
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewWord("i"), NewInteger(2), NewCloseParen(),
			codeBody(NewWord("continue")),
			codeBody(NewWord("i")),
		),
	})
	if err != nil {
		t.Fatalf("continue loop: %v", err)
	}
	if got := renderAll(out); got != "0 | 1 | 2" {
		t.Errorf("continue loop = %q", got)
	}
}

func TestBreakOutsideLoopErrors(t *testing.T) {
	r := covRegistry(t, registerLoopWords)
	_, err := NewTop(r).Run([]Value{NewWord("break")})
	if err == nil || !strings.Contains(err.Error(), "outside loop") {
		t.Errorf("bare break: err = %v", err)
	}
	_, err = NewTop(r).Run([]Value{NewWord("continue")})
	if err == nil || !strings.Contains(err.Error(), "outside loop") {
		t.Errorf("bare continue: err = %v", err)
	}
}

func TestMarkMoveReplay(t *testing.T) {
	r := covRegistry(t, nil)
	// A plain move replays the mark's saved body once (both mark and
	// move are consumed by the jump).
	id := NextMarkID()
	out, err := NewTop(r).Run([]Value{
		NewMark(id, NewInteger(7)),
		NewInteger(7),
		NewMove(id, "wave3 replay"),
	})
	if err != nil {
		t.Fatalf("mark/move replay: %v", err)
	}
	if got := renderAll(out); got != "7" {
		t.Errorf("replayed = %q", got)
	}
}

func TestMoveIfSplicesBranches(t *testing.T) {
	r := covRegistry(t, nil)
	run := func(cond Value) string {
		t.Helper()
		id := NextMarkID()
		out, err := NewTop(r).Run([]Value{
			NewMark(id),
			cond,
			NewMoveIf(id, "wave3 if", &IfCont{
				Then: []Value{NewInteger(1)},
				Else: []Value{NewInteger(2)},
			}),
		})
		if err != nil {
			t.Fatalf("moveIf: %v", err)
		}
		return renderAll(out)
	}
	if got := run(NewBoolean(true)); got != "1" {
		t.Errorf("then branch = %q", got)
	}
	if got := run(NewBoolean(false)); got != "2" {
		t.Errorf("else branch = %q", got)
	}

	// Condition produced no value → runtime_error.
	id := NextMarkID()
	_, err := NewTop(r).Run([]Value{
		NewMark(id),
		NewMoveIf(id, "wave3 empty if", &IfCont{Then: []Value{NewInteger(1)}}),
	})
	if err == nil || !strings.Contains(err.Error(), "condition produced no value") {
		t.Errorf("empty condition: err = %v", err)
	}
}

// --- compiled loop pipeline -------------------------------------------------

func TestCompiledCountedLoop(t *testing.T) {
	got := runDifferential(t, registerLoopWords, func() []Value {
		return []Value{
			NewWord("cfor"), NewInteger(3),
			codeBody(NewWord("cadd"), NewWord("i"), NewInteger(10)),
		}
	})
	if got != "10 | 11 | 12" {
		t.Errorf("compiled loop = %q", got)
	}
}

func TestCompiledLoopBreak(t *testing.T) {
	got := runDifferential(t, registerLoopWords, func() []Value {
		return []Value{
			NewWord("cfor"), NewInteger(5),
			codeBody(
				NewWord("cif"),
				NewOpenParen(), NewWord("cgt"), NewWord("i"), NewInteger(2), NewCloseParen(),
				codeBody(NewWord("break")),
				codeBody(NewWord("i")),
			),
		}
	})
	if got != "0 | 1 | 2" {
		t.Errorf("compiled break loop = %q", got)
	}
}

func TestCompiledLoopContinue(t *testing.T) {
	got := runDifferential(t, registerLoopWords, func() []Value {
		return []Value{
			NewWord("cfor"), NewInteger(5),
			codeBody(
				NewWord("cif"),
				NewOpenParen(), NewWord("cgt"), NewWord("i"), NewInteger(2), NewCloseParen(),
				codeBody(NewWord("continue")),
				codeBody(NewWord("i")),
			),
		}
	})
	if got != "0 | 1 | 2" {
		t.Errorf("compiled continue loop = %q", got)
	}
}

func TestCompiledLoopFeedsIterator(t *testing.T) {
	// The loop result is variadic — only the program residual may
	// absorb it; the iterator local drives OpPushLocal per iteration.
	got := runDifferential(t, registerLoopWords, func() []Value {
		return []Value{
			NewWord("cfor"), NewInteger(4),
			codeBody(NewWord("cmul"), NewWord("i"), NewWord("i")),
		}
	})
	if got != "0 | 1 | 4 | 9" {
		t.Errorf("compiled square loop = %q", got)
	}
}

func TestCompiledLoopBranchValueArms(t *testing.T) {
	// A computed condition over the iterator with plain value arms.
	got := runDifferential(t, registerLoopWords, func() []Value {
		return []Value{
			NewWord("cfor"), NewInteger(5),
			codeBody(
				NewWord("cif"),
				NewOpenParen(), NewWord("cgt"), NewWord("i"), NewInteger(2), NewCloseParen(),
				NewInteger(1),
				NewInteger(0),
			),
		}
	})
	if got != "0 | 0 | 0 | 1 | 1" {
		t.Errorf("compiled guard loop = %q", got)
	}
}

func TestCompiledLoopSideEffectBody(t *testing.T) {
	// A body netting zero values per iteration is a side-effect loop
	// (zeroOut); a following literal is the only program result.
	setup := func(r *Registry) {
		registerLoopWords(r)
		r.RegisterNativeFunc(NativeFunc{
			Name: "csink",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return nil, nil
				}),
				Returns: []*Type{}, BarrierPos: -1,
			}},
		})
	}
	got := runDifferential(t, setup, func() []Value {
		return []Value{
			NewWord("cfor"), NewInteger(3),
			codeBody(NewWord("csink"), NewWord("i")),
			NewEnd(),
			NewInteger(42),
		}
	})
	if got != "42" {
		t.Errorf("side-effect loop = %q", got)
	}
}

func TestNestedLoopsInterpretedAndRefused(t *testing.T) {
	// The interpreter runs nested mark/move loops; the Stage-2 compiler
	// refuses a loop whose result is another loop's body result — the
	// refusal reason is pinned so a silent miscompile can't replace it.
	tokens := func() []Value {
		return []Value{
			NewWord("cfor"), NewInteger(2),
			codeBody(
				NewWord("cfor"), NewInteger(2),
				codeBody(NewWord("cadd"), NewWord("i"), NewInteger(100)),
			),
		}
	}
	ri := covRegistry(t, registerLoopWords)
	out, err := NewTop(ri).Run(tokens())
	if err != nil {
		t.Fatalf("nested loops interpreted: %v", err)
	}
	if got := renderAll(out); got != "100 | 101 | 100 | 101" {
		t.Errorf("nested loops = %q", got)
	}
	rc := covRegistry(t, registerLoopWords)
	prog, reason := compileTokens(t, rc, tokens())
	if prog != nil {
		// If a future stage compiles this, it must agree with the interpreter.
		cOut, cErr := RunProgram(prog, rc)
		if cErr != nil || renderAll(cOut) != "100 | 101 | 100 | 101" {
			t.Errorf("compiled nested loops diverge: %v %v", cOut, cErr)
		}
	} else if !strings.Contains(reason, "loop results as a branch/body result") {
		t.Errorf("unexpected refusal reason: %s", reason)
	}
}

func TestLoopSignatureErrorMentionsShape(t *testing.T) {
	// A dispatch that cannot match (type literal in a concrete slot)
	// raises the interpreter's detailed signature_error, including the
	// expected-signature summary and nearby stack types.
	r := covRegistry(t, registerLoopWords)
	_, err := NewTop(r).Run([]Value{NewWord("cfor"), NewTypeLiteral(TInteger), codeBody(NewInteger(1))})
	if err == nil || !strings.Contains(err.Error(), "signature_error") {
		t.Fatalf("literal count: err = %v", err)
	}
	if !strings.Contains(err.Error(), "cfor (Integer, List)") {
		t.Errorf("error does not summarise the expected signature: %v", err)
	}
}

// --- emit checkpoint/rollback ------------------------------------------------

func TestEmitCheckpointRollback(t *testing.T) {
	es := NewEmitState()
	cp := es.Checkpoint()
	// Grow the const/type/interned pools after the checkpoint...
	es.intern(NewInteger(424242))
	es.internType(NewTypeLiteral(TInteger))
	if len(es.consts) != cp.consts+1 || len(es.types) != cp.types+1 {
		t.Fatalf("pools did not grow: consts=%d types=%d", len(es.consts), len(es.types))
	}
	// ...then roll back: the pools trim to the checkpoint.
	es.Rollback(cp)
	if len(es.consts) != cp.consts || len(es.types) != cp.types || es.seq != cp.seq {
		t.Errorf("rollback did not restore: consts=%d types=%d seq=%d", len(es.consts), len(es.types), es.seq)
	}
	// Nil-safety on both.
	var nilES *EmitState
	_ = nilES.Checkpoint()
	nilES.Rollback(cp)
}
