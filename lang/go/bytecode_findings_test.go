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

// Roadmap item 4 — a STRICT-disjunct straddle (a type-algebra dispatch whose
// operand is a complement/predicate type reaching more than one overload, e.g.
// `is` over `tnot (Integer gt 0)`) lowers to OpCallNativePoly (runtime
// re-match) instead of refusing "polymorphic dispatch". The runtime value is a
// single concrete alternative, so the VM dispatches it faithfully.
func TestStrictDisjunctTypeAlgebraPoly(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(`5 is (tnot (Integer gt 0))`)
	if cerr != nil || prog == nil {
		t.Fatalf("type-algebra poly row did not compile: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("expected a native poly lowering, got an island:\n%s", dis)
	}
	if !strings.Contains(dis, "CALL_NATIVE_POLY") {
		t.Errorf("expected CALL_NATIVE_POLY for the straddling `is`:\n%s", dis)
	}

	// Value + taxonomy parity across both engines, including the truth flip
	// (5 is NOT in the complement of Integer>0; 0 IS) — the negative half.
	for _, c := range []struct {
		src  string
		want string
	}{
		{`5 is (tnot (Integer gt 0))`, "false"},
		{`0 is (tnot (Integer gt 0))`, "true"},
		{`Integer tand (tnot (Integer gt 0))`, "(Integer lte 0)"},
		{`"hi" is (tnot (Integer gt 0))`, "true"},
	} {
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if !compiled || errC != nil {
			t.Fatalf("%q: compiled=%v err=%v", c.src, compiled, errC)
		}
		d, _ := New()
		gotI, _ := d.Run(c.src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: compiled=%v interp=%v", c.src, gotC, gotI)
		}
		if fmt.Sprint(gotC) != "["+c.want+"]" {
			t.Errorf("%q: got %v, want [%s]", c.src, gotC, c.want)
		}
	}
}

// Roadmap item 2 — atom-keyed `set` on an object/class instance (a field write
// like `p set x 7`, whose quoted key previously refused as "quoted-operand
// word set") lowers to CALL_NATIVE. The receiver is a non-const instance so
// the in-place mutation is safe, exactly as integer-keyed `set 1 v arr` already
// relied on.
func TestObjectClassSetLowers(t *testing.T) {
	// Positive: a declared-field write compiles natively and is visible after.
	const ok = `def Point class {x:1} def p (make Point {}) p set x 7 end p.x`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(ok)
	if cerr != nil || prog == nil {
		t.Fatalf("object set did not compile: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") || !strings.Contains(dis, "set") {
		t.Errorf("expected a native CALL_NATIVE set, got:\n%s", dis)
	}
	gotC, compiled, errC := a.RunCompiled(ok)
	if !compiled || errC != nil {
		t.Fatalf("object set run: compiled=%v err=%v", compiled, errC)
	}
	b, _ := New()
	gotI, _ := b.Run(ok)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[7]" {
		t.Errorf("object set: compiled=%v interp=%v (want [7])", gotC, gotI)
	}

	// Negative: an undeclared (sealed) field write raises the SAME sealed_field
	// error in both engines — the compiled mutator must not silently succeed.
	const bad = `def Point class {x:1} def p (make Point {}) p set z 9`
	c, _ := New()
	_, _, errCbad := c.RunCompiled(bad)
	d, _ := New()
	_, errIbad := d.Run(bad)
	if codeOf(errCbad) != "sealed_field" || codeOf(errCbad) != codeOf(errIbad) {
		t.Errorf("sealed-field set: compiled=[%s] interp=[%s] (want sealed_field both)",
			codeOf(errCbad), codeOf(errIbad))
	}
}

// Roadmap item 6 — a computed-else `if cond [then] (expr)` (the else is an
// eagerly-evaluated paren result on the stack, not a literal/local) lowers via
// SWAP (cond to top) + JMP_IF_FALSE + OpDrop (discard the else value on the
// taken path), instead of refusing "computed else value". Both branch
// directions must match the interpreter.
func TestComputedElseIfLowers(t *testing.T) {
	const src = `if (1 eq 1) [99] (add 1 2)`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("computed-else if did not compile: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") || !strings.Contains(dis, "DROP") {
		t.Errorf("expected a native SWAP/JMP_IF_FALSE/DROP lowering:\n%s", dis)
	}

	// Both directions + downstream consumption, value parity with the interpreter.
	for _, c := range []struct {
		src  string
		want string
	}{
		{`if (1 eq 1) [99] (add 1 2)`, "99"},           // taken: then; else value dropped
		{`if (1 eq 2) [99] (add 1 2)`, "3"},            // not taken: computed else is the result
		{`add 10 (if (1 eq 1) [99] (add 1 2))`, "109"}, // consumed downstream
	} {
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if !compiled || errC != nil {
			t.Fatalf("%q: compiled=%v err=%v", c.src, compiled, errC)
		}
		d, _ := New()
		gotI, _ := d.Run(c.src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "["+c.want+"]" {
			t.Errorf("%q: compiled=%v interp=%v (want [%s])", c.src, gotC, gotI, c.want)
		}
	}
}

// Roadmap item 6b — a statement-`if` where exactly one arm nets a value and the
// other nets 0 WITHOUT diverging (`if c [99] []`, `if c [] [99]`,
// `if c [raise] [99]`) lowers as a VARIADIC (0-or-1) branch result instead of
// refusing "branch produces no value". Both directions, the empty arm, and the
// raise-guard (which errors on its taken path) must match the interpreter.
func TestVariadicElseIfLowers(t *testing.T) {
	cases := []struct {
		src       string
		want      string // expected residual when no error
		wantError bool
	}{
		{`if (1 gt 0) [99] []`, "[99]", false},          // then value taken
		{`if (1 eq 2) [99] []`, "[]", false},            // empty else taken → nothing
		{`if (1 eq 2) [] [99]`, "[99]", false},          // else value taken
		{`if (1 eq 1) [] [99]`, "[]", false},            // empty then taken → nothing
		{`if (1 eq 2) [raise "x"] [99]`, "[99]", false}, // guard not taken
		{`if (1 eq 1) [raise "x"] [99]`, "", true},      // guard taken → raises
	}
	for _, c := range cases {
		a, _ := New()
		gotC, compiled, errC := a.RunCompiled(c.src)
		b, _ := New()
		gotI, errI := b.Run(c.src)
		if (errC != nil) != (errI != nil) {
			t.Errorf("%q: error parity compiled=%v interp=%v", c.src, errC, errI)
			continue
		}
		if c.wantError {
			if errC == nil {
				t.Errorf("%q: expected an error (raise on the taken path), got none", c.src)
			}
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", c.src)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v interp=%v (want %s)", c.src, gotC, gotI, c.want)
		}
	}
}
