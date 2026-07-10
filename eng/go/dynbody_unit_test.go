package eng

import "testing"

// The DynEnv args-bracket primitives and tryRecordDynBody's decline guards —
// white-box coverage of the branches end-to-end programs cannot reach
// (defensive shapes, tail-swap variants).
func TestArgsStackDepthTruncate(t *testing.T) {
	var nilStack *ArgsStack
	if nilStack.Depth() != 0 {
		t.Errorf("nil Depth: want 0")
	}
	nilStack.Truncate(0) // no-op, no panic
	as := NewArgsStack()
	_ = as.Push(NewInteger(1))
	_ = as.Push(NewInteger(2))
	if as.Depth() != 2 {
		t.Errorf("Depth: want 2, got %d", as.Depth())
	}
	as.Truncate(5) // above depth: no-op
	if as.Depth() != 2 {
		t.Errorf("Truncate above depth must no-op")
	}
	as.Truncate(1)
	if as.Depth() != 1 {
		t.Errorf("Truncate: want 1, got %d", as.Depth())
	}
	as.Truncate(-1) // negative: no-op
	if as.Depth() != 1 {
		t.Errorf("negative Truncate must no-op")
	}
}

// swapTailArgs — the tail-call bracket swap: with a live frame it replaces
// the entry above the frame's base; at activation root it truncates to the
// run floor; outside DynEnv it is a no-op.
func TestSwapTailArgsBranches(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	vc := &vmContext{p: &Program{DynEnv: true}, r: r, argsFloor: 0}
	nl := []Value{NewInteger(7)}
	// Root tail swap: floor-truncate then push.
	_ = r.Args.Push(NewList([]Value{NewInteger(1)}))
	vc.swapTailArgs(nil, nl, 1)
	if r.Args.Depth() != 1 {
		t.Errorf("root tail swap: want depth 1, got %d", r.Args.Depth())
	}
	// Framed tail swap: replace the top entry above the frame's base.
	frames := []vmFrame{{argsBase: 1}}
	_ = r.Args.Push(NewList([]Value{NewInteger(2)}))
	vc.swapTailArgs(frames, nl, 1)
	if r.Args.Depth() != 2 {
		t.Errorf("framed tail swap: want depth 2, got %d", r.Args.Depth())
	}
	top, ok, _ := r.Args.Top()
	if !ok {
		t.Fatalf("framed tail swap: no top")
	}
	if lst, lerr := AsList(top); lerr != nil || lst.Len() != 1 {
		t.Errorf("framed tail swap: top must be the new callee's args [7], got %v", top)
	} else if n, nerr := AsInteger(lst.Slice()[0]); nerr != nil || n != 7 {
		t.Errorf("framed tail swap: want [7], got %v", top)
	}
	// Non-DynEnv: all three helpers no-op.
	off := &vmContext{p: &Program{}, r: r}
	d := r.Args.Depth()
	off.swapTailArgs(frames, nl, 1)
	off.pushFrameArgs(nl, 1)
	off.retFrameArgs(&vmFrame{argsBase: 0})
	if r.Args.Depth() != d {
		t.Errorf("non-DynEnv helpers must not touch the args stack")
	}
}

// tryRecordDynBody decline guards: a Callable whose BodyPos exceeds the args
// (a malformed dispatch shape) and an unresolvable operand both decline,
// leaving the ordinary refusal path.
func TestTryRecordDynBodyDeclines(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	done := r.Check.BeginCompilePass()
	defer done()
	sig := &Signature{Callable: &CallableSpec{BodyPos: 2}, CompileEffect: CompileDynBody}
	outs := []Value{NewCarrier(TAny)}
	if tryRecordDynBody(r, "do", sig, []Value{NewInteger(1)}, outs, SrcPos{}) {
		t.Errorf("BodyPos beyond args must decline")
	}
	// An operand with no compiled home (an unregistered carrier) declines.
	sig2 := &Signature{Callable: &CallableSpec{BodyPos: 0}, CompileEffect: CompileDynBody}
	ghost := NewCarrier(TList)
	ghost.Dynamic = true
	if tryRecordDynBody(r, "do", sig2, []Value{ghost}, outs, SrcPos{}) {
		t.Errorf("unresolvable operand must decline")
	}
}
