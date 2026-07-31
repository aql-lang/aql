package eng

import (
	"errors"
	"testing"
)

// The VM's step budget — the CPU guard mirroring the interpreter's
// evalLimitError. A tail-call spin replaces its frame forever, so
// neither the stack nor the frame ceiling ever trips; the step
// counter must catch it with the same evaluation_limit taxonomy.

// spinProgram is a hand-built endless tail loop: main CALL_USERs
// into a fn whose only instruction tail-calls itself.
func spinProgram() *Program {
	return &Program{
		Code:  []Instr{{Op: OpCallUser, Arg: 0}},
		Debug: []SrcPos{{Row: 1, Col: 1}},
		Fns: []CompiledFn{{
			Name:    "spin",
			NParams: 0,
			NLocals: 0,
			Code:    []Instr{{Op: OpTailCallUser, Arg: 0}},
			Debug:   []SrcPos{{Row: 1, Col: 1}},
		}},
	}
}

func TestVMStepLimitExplicitError(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	_, err = runProgram(spinProgram(), r, 5000) // tiny cap so the test trips quickly
	if err == nil {
		t.Fatal("endless tail spin returned nil error, want evaluation_limit")
	}
	var ae *BoruError
	if !errors.As(err, &ae) || ae.Code != "evaluation_limit" {
		t.Fatalf("error = %v, want code evaluation_limit", err)
	}
}

// A program that finishes within budget must NOT be flagged.
func TestVMStepLimitNotTrippedByNormalProgram(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	p := &Program{
		Code:   []Instr{{Op: OpPushConst, Arg: 0}},
		Debug:  []SrcPos{{Row: 1, Col: 1}},
		Consts: []Value{NewInteger(42)},
	}
	out, err := runProgram(p, r, 5000)
	if err != nil {
		t.Fatalf("trivial program: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
}

// OpFallback drives an interpreter island: a self-contained token span
// re-run through a sub-engine, its residual pushed back onto the shared
// operand stack. Threaded inputs (NIn>0) are popped deepest-first and
// pre-loaded onto the island.
func TestVMFallbackIsland(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	registerAdd(r)
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()

	// nIn=0: a fully-baked island `add 2 3`.
	p := &Program{
		Code:  []Instr{{Op: OpFallback, Arg: 0}},
		Debug: []SrcPos{{Row: 1, Col: 1}},
		Fallbacks: []FallbackSpan{{
			Tokens: []Value{NewWord("add"), NewInteger(2), NewInteger(3)},
			NIn:    0,
			Desc:   "add",
		}},
	}
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("fallback island: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	if n, _ := out[0].AsConcreteInteger(); n != 5 {
		t.Fatalf("island add 2 3 = %d, want 5", n)
	}

	// nIn=1: one threaded input pre-loaded, island `add 10`.
	p2 := &Program{
		Code:   []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpFallback, Arg: 0}},
		Debug:  []SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		Consts: []Value{NewInteger(7)},
		Fallbacks: []FallbackSpan{{
			Tokens: []Value{NewWord("add"), NewInteger(10)},
			NIn:    1,
			Desc:   "add",
		}},
	}
	out2, err := RunProgram(p2, r)
	if err != nil {
		t.Fatalf("threaded island: %v", err)
	}
	if n, _ := out2[0].AsConcreteInteger(); n != 17 {
		t.Fatalf("threaded island 7 + (add 10) = %d, want 17", n)
	}
}
