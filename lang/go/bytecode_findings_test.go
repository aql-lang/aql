package lang

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// Finding A / C — RunCompiled resolves a compiled-mode INTERNAL error (a VM
// lowering assertion or a recovered handler panic) by re-running on the
// interpreter, but surfaces genuine AQL runtime errors (type_error, the
// resource ceilings) as-is. runtimeShouldFallback is the decision point.
func TestRuntimeShouldFallback(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"internal_error falls back", eng.MakeAqlError("internal_error", "boom", "", "", ""), true},
		{"foreign (non-AQL) error falls back", errors.New("some go error"), true},
		{"type_error surfaces", eng.MakeAqlError("type_error", "bad", "", "", ""), false},
		{"evaluation_limit surfaces (fast-fail by design)", eng.MakeAqlError("evaluation_limit", "too long", "", "", ""), false},
		{"tape_exhausted surfaces", eng.MakeAqlError("tape_exhausted", "too big", "", "", ""), false},
		{"signature_error surfaces", eng.MakeAqlError("signature_error", "no sig", "", "", ""), false},
	}
	for _, c := range cases {
		if got := runtimeShouldFallback(c.err); got != c.want {
			t.Errorf("%s: runtimeShouldFallback = %v, want %v", c.name, got, c.want)
		}
	}
}

// Finding A — a genuine compiled-mode runtime error is surfaced WITHOUT a
// silent interpreter fallback (it matches the interpreter by taxonomy, so
// masking it would only hide the result and burn the budget twice).
func TestRunCompiledSurfacesGenuineError(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// An integer overflow is a genuine RUNTIME error the emitter still compiles
	// (concrete args) and the VM raises. It must surface FROM THE COMPILED PATH
	// (not be masked by a fallback) with the AQL taxonomy intact, matching the
	// interpreter.
	const src = `1000000 pow 1000000`
	_, compiled, errC := a.RunCompiled(src)
	if !compiled {
		t.Fatal("overflow program did not run compiled — a genuine runtime error must not trigger fallback")
	}
	b, _ := New()
	_, errI := b.Run(src)
	if codeOf(errC) == "" || codeOf(errC) != codeOf(errI) {
		t.Fatalf("integer_overflow: compiled=[%s] interp=[%s] (want equal, non-empty)", codeOf(errC), codeOf(errI))
	}
}

func codeOf(err error) string {
	var ae *eng.AqlError
	if errors.As(err, &ae) {
		return ae.Code
	}
	if err != nil {
		return "non-aql"
	}
	return ""
}

// Finding D — a loop body that rebinds a def needs multiple fixed-point
// analysis rounds, but only the stabilised round is recorded: the body's
// dispatches must be counted ONCE in SiteCounts (not per-round), and the
// compiled result must still match the interpreter.
func TestLoopFixedPointNoReRecord(t *testing.T) {
	// `def x (add i 1) x` adds exactly one `add` dispatch to the body. With the
	// loop event that is mono=2; a re-recorded second round would make it 3.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, res, cerr := a.CompileCheck(`for 3 [def x (add i 1) x]`)
	if cerr != nil || prog == nil {
		t.Fatalf("rebinding loop did not compile: reason=%q err=%v", reason, cerr)
	}
	if got := res.SiteCounts["mono"]; got != 2 {
		t.Errorf("rebinding-loop mono dispatches = %d, want 2 (the discarded fixed-point round must not re-record)", got)
	}

	// Parity holds regardless of the round count.
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(`for 3 [def x (add i 1) x]`)
	if !compiled || errC != nil {
		t.Fatalf("rebinding loop run: compiled=%v err=%v", compiled, errC)
	}
	c, _ := New()
	gotI, _ := c.Run(`for 3 [def x (add i 1) x]`)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("rebinding loop: compiled=%v interp=%v", gotC, gotI)
	}
}

// Roadmap item 1 — a 3-arg native call whose sig-0 operand is a COMPUTED
// result (a receiver above two const operands) lowers via a push+swap chain
// instead of refusing "operand shape needs reordering". `setpath recv k v`
// with a computed receiver is the driving shape.
func TestThreeArgComputedReceiverLowers(t *testing.T) {
	const src = `"aql:struct-util" import end (StructUtil.setpath (object {a:1}) "b" 2) get b`

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("3-arg computed-receiver row did not compile: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("expected a native lowering, got an island:\n%s", dis)
	}
	if !strings.Contains(dis, "SWAP") || !strings.Contains(dis, "setpath") {
		t.Errorf("expected a push+swap chain into CALL_NATIVE setpath:\n%s", dis)
	}

	// Parity: the compiled result matches the interpreter.
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(src)
	if !compiled || errC != nil {
		t.Fatalf("compiled run: compiled=%v err=%v", compiled, errC)
	}
	c, _ := New()
	gotI, _ := c.Run(src)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("setpath parity: compiled=%v interp=%v", gotC, gotI)
	}
}
