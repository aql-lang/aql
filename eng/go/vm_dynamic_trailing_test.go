package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestCallDynamicTrailingNonCallable pins the soundness-critical contract of
// OpCallDynamicTrailing that the spec corpus does NOT exercise (every trailing
// fn-value row there is callable): when the value turns out NOT to be callable,
// the op must leave it ON TOP of its arg — the interpreter's trailing residual
// order — and NOT below it the way the leading OpCallDynamic would. A regression
// here silently reorders a non-callable trailing residual.
//
// The recorder lays a trailing apply out as [value, arg] (value at the base, its
// one arg above). For a callable value that is identical to the leading boundary
// (covered by the `5 m.f` differential row); the divergent case is the
// non-callable rotate, exercised here directly via RunProgram.
func TestCallDynamicTrailingNonCallable(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	prog := &compiler.Program{
		Consts: []core.Value{core.NewInteger(99), core.NewInteger(5)},
		Code: []compiler.Instr{
			{Op: compiler.OpPushConst, Arg: 0},           // value 99 (a plain Integer — NOT callable) at the base
			{Op: compiler.OpPushConst, Arg: 1},           // its arg, 5, above
			{Op: compiler.OpCallDynamicTrailing, Arg: 1}, // apply value to 1 arg below — but it is not callable
		},
		Debug: []core.SrcPos{{}, {}, {}},
	}
	got, err := RunProgram(prog, r)
	if err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("non-callable trailing residual has %d values, want 2 ([arg, value])", len(got))
	}
	i0, _ := got[0].AsConcreteInteger()
	i1, _ := got[1].AsConcreteInteger()
	// Non-callable: the value is rotated back ON TOP of its arg → [5, 99],
	// matching the interpreter (a trailing non-Function is left untouched).
	if i0 != 5 || i1 != 99 {
		t.Errorf("non-callable trailing residual = [%d %d], want [5 99] (value rotated on top)", i0, i1)
	}
}
