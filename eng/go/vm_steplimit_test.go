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
	var ae *AqlError
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
