package eng

import (
	"strings"
	"testing"
)

// OpCallDynMixedFromMark (plan Phase 5, L-DO part 2b): the variadic-region
// verbatim window — island stack[mark:] through the CALL_DYNAMIC_MIXED
// re-step machinery, with the width decided by the RUNTIME region rather
// than a lowered count. These direct-Program pins cover the VM contract
// ahead of the emit/lower weaving.

func markWindowProgram(consts []Value, code []Instr) *Program {
	dbg := make([]SrcPos, len(code))
	return &Program{Consts: consts, Code: code, Debug: dbg}
}

// A data-only window: the island re-steps [1, 2] — no callable present, so
// the residual is the window verbatim (the interpreter leaves plain values).
func TestMarkWindowDataResidual(t *testing.T) {
	r := covRegistry(t, nil)
	p := markWindowProgram(
		[]Value{NewInteger(1), NewInteger(2)},
		[]Instr{
			{Op: OpStackMark},
			{Op: OpPushConst, Arg: 0},
			{Op: OpPushConst, Arg: 1},
			{Op: OpCallDynMixedFromMark},
		})
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("residual = %v, want the verbatim [1 2] window", out)
	}
}

// An EMPTY window: the mark pops, nothing islands, the stack below the mark
// is untouched.
func TestMarkWindowEmptyNoOp(t *testing.T) {
	r := covRegistry(t, nil)
	p := markWindowProgram(
		[]Value{NewInteger(7)},
		[]Instr{
			{Op: OpPushConst, Arg: 0},
			{Op: OpStackMark},
			{Op: OpCallDynMixedFromMark},
		})
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("residual = %v, want the untouched [7]", out)
	}
	if n, _ := AsInteger(out[0]); n != 7 {
		t.Errorf("residual = %v, want 7", out[0])
	}
}

// The window RE-STEPS: a word token in the region dispatches exactly as the
// interpreter's pointer would (1 cadd 2 -> 3). This is the verbatim-window
// contract the do-catch region relies on.
func TestMarkWindowReSteps(t *testing.T) {
	r := covRegistry(t, nil)
	p := markWindowProgram(
		[]Value{NewInteger(1), NewWord("cadd"), NewInteger(2)},
		[]Instr{
			{Op: OpStackMark},
			{Op: OpPushConst, Arg: 0},
			{Op: OpPushConst, Arg: 1},
			{Op: OpPushConst, Arg: 2},
			{Op: OpCallDynMixedFromMark},
		})
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("residual = %v, want one value", out)
	}
	if n, _ := AsInteger(out[0]); n != 3 {
		t.Errorf("residual = %v, want 3 (the island re-stepped 1 cadd 2)", out[0])
	}
}

// Guards: no open mark, and a mark above the stack top (a bytecode-level
// fault) both raise internal errors rather than corrupting the stack.
func TestMarkWindowGuards(t *testing.T) {
	r := covRegistry(t, nil)
	p := markWindowProgram(nil, []Instr{{Op: OpCallDynMixedFromMark}})
	if _, err := RunProgram(p, r); err == nil || !strings.Contains(err.Error(), "no open mark") {
		t.Errorf("no-mark run must raise the guard, got %v", err)
	}
	// A mark stranded above the live top (the value below it dropped) is a
	// bytecode-level fault the guard must catch, not a silent negative slice.
	p2 := markWindowProgram(
		[]Value{NewInteger(7)},
		[]Instr{
			{Op: OpPushConst, Arg: 0},
			{Op: OpStackMark},
			{Op: OpDrop},
			{Op: OpCallDynMixedFromMark},
		})
	if _, err := RunProgram(p2, r); err == nil || !strings.Contains(err.Error(), "mark above stack top") {
		t.Errorf("a stranded mark must raise the guard, got %v", err)
	}
	// An island that RAISES (an unbound word in the window) propagates the
	// error out of the mark-window op.
	p3 := markWindowProgram(
		[]Value{NewWord("zz-no-such-word")},
		[]Instr{
			{Op: OpStackMark},
			{Op: OpPushConst, Arg: 0},
			{Op: OpCallDynMixedFromMark},
		})
	if _, err := RunProgram(p3, r); err == nil || !strings.Contains(err.Error(), "undefined word") {
		t.Errorf("an island raise must propagate, got %v", err)
	}
}
