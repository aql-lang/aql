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

// Roadmap item (fn-value introspection) — a type-reading word (typeof / tcmp /
// teq / tand / tor / tnot) over a fn VALUE bakes the immutable fn as a const
// the handler inspects, instead of refusing "function value reaches word". A
// fn-INVOKING use must stay refused (the VM cannot re-step a fn body).
func TestFnValueIntrospectionLowers(t *testing.T) {
	const setup = `def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] `
	for _, c := range []struct {
		src  string
		want string
	}{
		{`typeof (fn [[a:Integer][Integer][a add 1]])`, "[Function]"},
		{setup + `Positive tcmp Positive`, "[0]"}, // equal
		{setup + `Positive tcmp Function`, "[1]"},
		{setup + `Function tcmp Positive`, "[-1]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected native, got island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, _ := b.Run(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// Negative: `is` over a predicate fn INVOKES it (applies the predicate), so
	// it must NOT be exempted — it still falls back (parity preserved).
	const inv = `def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] 5 is Positive`
	c, _ := New()
	_, compiled, _ := c.RunCompiled(inv)
	if compiled {
		t.Errorf("`is` over a predicate fn must not compile as introspection (it invokes the fn)")
	}
}

// Roadmap item 5 (part A) — each/fold/filter over a MAP compiles NATIVE
// instead of islanding: the value-body closure runs per map value through the
// InvokeBody seam (newMapBody routes a compiled closure like a quotation), and
// the closure's input type is the map's common VALUE type. The interpreter's
// token-body path is unchanged.
func TestMapIterationCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`each [mul 10] {a:1 b:2}`, "[{a:10 b:20}]"},
		{`{a:1 b:2} each [mul 10]`, "[{a:10 b:20}]"},
		{`fold [add] {a:1 b:2 c:3} 0`, "[6]"},
		{`filter [gt 2] {a:1 b:5 c:3}`, "[{b:5 c:3}]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native closure lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, _ := b.Run(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// Regression: list iteration still compiles native + correct.
	a, _ := New()
	gotC, compiled, _ := a.RunCompiled(`each [mul 2] [1 2 3]`)
	if !compiled || fmt.Sprint(gotC) != "[[2 4 6]]" {
		t.Errorf("list each regressed: compiled=%v got=%v", compiled, gotC)
	}
}

// Roadmap item 5 part B (filter lambda args) — a `filter ([p] => …) data`
// lambda compiles its afn body to a closure with the word's callback input
// shape ({key,value} pair over a list, KeyVal over a map), driven natively
// through InvokeBody rather than islanding or refusing. Verifies the closure
// path is value- and taxonomy-identical to the interpreter.
func TestFilterLambdaCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		// list filter: the lambda reads the element via the {key,value} pair.
		{`filter ([p:Any] => [(p.value mod 2) eq 0]) [1 2 3 4 5 6]`, "[[2 4 6]]"},
		{`filter ([p:Any] => [p.value gt 3]) [1 2 3 4 5]`, "[[4 5]]"},
		// map filter: the lambda reads the value via a KeyVal, shape preserved.
		{`filter ([kv:KeyVal] => [(kv.v mod 2) eq 0]) {a:1 b:2 c:3 d:4}`, "[{b:2 d:4}]"},
		{`{a:1 b:5 c:3} filter ([kv:KeyVal] => [kv.v gt 2])`, "[{b:5 c:3}]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native closure lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, _ := b.Run(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// A non-Boolean map predicate is a loud filter_error in BOTH engines (the
	// compiled closure runs the same strict check as the interpreter path).
	bad := `{a:1 b:5} filter ([kv:KeyVal] => [kv.v])`
	a, _ := New()
	_, compiled, errC := a.RunCompiled(bad)
	b, _ := New()
	_, errI := b.Run(bad)
	if !compiled || errC == nil || errI == nil {
		t.Errorf("%q: want both engines to error (compiled=%v errC=%v errI=%v)", bad, compiled, errC, errI)
	}
}

// args.N — `args.N` inside a fn body reads the N-th call argument. The params
// ARE the frame's leading locals, so AnalyseFnBody projects them as the args
// list and `get N args` folds to PUSH_LOCAL N (carrier.go tryFoldStaticIndex) —
// no runtime args stack. The compiled body is byte-for-byte the named-param
// form. (Concrete proof that the P7-gate "tier 2" words are reducible, not
// irreducible.) Bare `args` (the whole list) still refuses, by design.
func TestArgsAccessorCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def f fn [[a:Integer b:Integer] [Integer] [args.0 add args.1]] f 3 4`, "[7]"},
		{`def f fn [[a:Integer b:Integer] [Integer] [args.1 sub args.0]] f 10 3`, "[-7]"},
		{`def f fn [[n:Integer] [Integer] [if (n lte 0) [args.0] [f (n sub 1)]]] f 3`, "[0]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native lowering, got an island", c.src)
		}
		// args.N must lower to a frame-local read, not a runtime args call.
		if strings.Contains(prog.Disassemble(), "CALL_NATIVE_POLY") && strings.Contains(c.src, "args.0 add") {
			t.Errorf("%q: args.N did not fold to a local (poly get remains):\n%s", c.src, prog.Disassemble())
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, _ := b.Run(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// The named-param form is byte-identical (args.N is just another spelling).
	a, _ := New()
	p1, _, _, _ := a.CompileCheck(`def f fn [[a:Integer b:Integer] [Integer] [args.0 add args.1]] f 3 4`)
	b, _ := New()
	p2, _, _, _ := b.CompileCheck(`def f fn [[a:Integer b:Integer] [Integer] [a add b]] f 3 4`)
	if p1 == nil || p2 == nil || p1.Disassemble() != p2.Disassemble() {
		t.Errorf("args.N body should compile identically to the named-param body")
	}
}

// word (Forth-style macro splice) — `def x word [body]` binds x to an __SP
// splice marker; at each use site stepLiteral inlines the body and re-steps it
// against the live stack. The bytecode does the SAME thing: carrierResults
// produces the marker as a non-emitting compile-time value (toCarrier preserves
// it), and the use-site splice expands inline — so the body's instructions land
// in place, late binding and all. Proof that the gate's "tier 2" is reducible:
// `word` was the 30-row macro-splice cluster I'd called irreducible meta.
func TestWordSpliceCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def x word [1,2,3] x`, "[1 2 3]"},
		{`word [1 add 2]`, "[3]"}, // splices 1 add 2 -> evaluates inline
		{`def dbl word [dup add] 5 dbl`, "[10]"},
		// Late binding: the splice re-resolves c1 at the USE site (= 20), not at
		// definition (= 10) — the exact case the bytecode "freezes things"
		// objection predicted would break, and does not.
		{`def c1 10 def x word [c1 2] def c1 20 x`, "[20 2]"},
		{`def a word [1,2] def b word [a 3] b`, "[1 2 3]"}, // recursive splice
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected an inline native lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, _ := b.Run(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// A splice of an undefined word errors in BOTH engines (the inline
	// expansion surfaces the undefined_word at check time / run time alike).
	a, _ := New()
	_, _, errC := a.RunCompiled(`def x word [nope 2 3] x`)
	b, _ := New()
	_, errI := b.Run(`def x word [nope 2 3] x`)
	if (errC == nil) != (errI == nil) {
		t.Errorf("word-splice undefined: error divergence compiled=%v interp=%v", errC, errI)
	}
}

// macroexpand — Lisp-style: when the macro and its operands are static the
// expansion is a compile-time computation, so carrierResults runs it and bakes
// the resulting token list as a code-as-data const (a Word is admitted as a
// const MEMBER — isInertConstMember). The compiled result is the same data list
// the interpreter returns. A too-deep / un-bakeable expansion refuses and the
// interpreter surfaces the identical result. Proof that macroexpand is
// reducible compiler work, not irreducible reflection.
func TestMacroexpandCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def twice (macro [[e] [ quote [ unquote e add unquote e ] ]])  macroexpand (twice 5)`, "[[5 word(add) 5]]"},
		{`def innr (macro [[x] [quote [unquote x add 1]]])  def outr (macro [[y] [quote [innr unquote y]]])  macroexpand (outr 5)`, "[[5 word(add) 1]]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a baked code-const, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, _ := b.Run(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// A recursive (too-deep) macro errors in both engines (the compile-time
	// expansion errors, so the row refuses and the interpreter surfaces it).
	deep := `def loopy (macro [[a] [quote [loopy unquote a]]])  macroexpand (loopy 1)`
	a, _ := New()
	_, _, errC := a.RunCompiled(deep)
	b, _ := New()
	_, errI := b.Run(deep)
	if (errC == nil) != (errI == nil) {
		t.Errorf("too-deep macroexpand: error divergence compiled=%v interp=%v", errC, errI)
	}
}

// Nested-variadic branch lowering — a no-default `case` desugars to a nested
// if-chain whose innermost `if guard [block]` has no else, so the chain is
// variadic (0-or-1: a value on a match, nothing on no-match). The lowerer now
// lets a branch arm CARRY a variadic (0-or-1) result and propagates the
// variadic-ness up to the residual, so a 2+-clause no-default case compiles.
func TestNestedVariadicCaseCompiles(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def Big (Integer gt 10) case 50 [Big "big" Integer "small-int"]`, "[big]"},
		{`def Big (Integer gt 10) case 5 [Big "big" Integer "small-int"]`, "[small-int]"},
		{`if true [if false [9]]`, "[]"}, // bare nested-variadic if -> 0 values
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native if-chain, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, _ := b.Run(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// A variadic case result CONSUMED by another word can't compile (the
	// consumer needs a fixed count) — it must still fall back, with parity.
	const consumed = `def Big (Integer gt 10) [99 (case 5 [Big "big"]) 88]`
	a, _ := New()
	_, compiled, _ := a.RunCompiled(consumed)
	b, _ := New()
	gotI, _ := b.Run(consumed)
	if compiled {
		t.Errorf("%q: a consumed variadic case must fall back, not compile", consumed)
	}
	if fmt.Sprint(gotI) != "[[99 88]]" {
		t.Errorf("%q: interp = %v, want [[99 88]]", consumed, gotI)
	}
}

// usurp (and the /u suffix) — a static arg permutation: `usurp f` wraps f so a
// call reverses the arg order (`usurped a b c` ≡ `f c b a`). The wrapper's
// re-dispatch handler now runs in check mode, so the carrier compiler steps the
// re-dispatch and compiles the reversed ORIGINAL call directly — there was never
// any "tape coupling" to model, just an opaque wrapper the compiler stepped past.
func TestUsurpCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def sub2 fn [[a:Integer b:Integer][Integer][a sub b]]  usurp (ref sub2) 10 3`, "[-7]"},
		{`def sub2 fn [[a:Integer b:Integer][Integer][a sub b]]  sub2/u 10 3`, "[-7]"},
		{`def inc fn [[n:Integer][Integer][n add 1]]  inc/u 5`, "[6]"}, // 1-arg usurp is a no-op on order
		{`def cat3 fn [[a:String b:String c:String][String][a add b add c]]  cat3/u 'x' 'y' 'z'`, "[zyx]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native reversed-call lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, _ := b.Run(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}
}

// make computed container defaults — a class field default (or data-map value)
// that is itself a COMPUTED paren-expr ((make Foo 1), (1 add 2)) used to refuse:
// in check mode the inner computation recorded an event the container swallowed
// ("unconsumed call results"). A deterministic computed value is a compile-time
// constant, so it now const-folds (constFoldContainerVal: two non-recording
// concrete evals that must agree), and the container bakes. An INSTANCE default
// bakes only as a class-schema member (make copies it per instance — mutation-
// safe), never as a data-map member. Drives `make`-class-default compilation.
func TestMakeComputedDefaultsCompile(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def Foo refine Integer end def S class {x:(make Foo 1)} end (make S {}) get x`, "[1]"},
		{`def Foo refine Integer end def S class {x:(make Foo 1)} end typeof ((make S {}) get x)`, "[Foo]"},
		{`def Foo class {y:1} end def S class {x:(make Foo {})} end (make S {}) get x`, "[Class/Foo{y:1}]"},
		{`def Foo refine Integer end def S class {x:(make Foo 7)} end (make S {x:(make Foo 9)}) get x`, "[9]"},
		{`def S class {x:(1 add 2)} end (make S {}) get x`, "[3]"},
		{`def m {a:(1 add 2)} m`, "[{a:3}]"}, // data map scalar default also const-folds
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native const-bake, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, _ := b.Run(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// Per-instance copy isolation: make copies the baked schema default, so
	// mutating one instance's field must not affect another's.
	const iso = `def Foo class {n:1} end def S class {x:(make Foo {})} end def a (make S {}) def b (make S {}) (a.x set n 9) end b.x get n`
	a, _ := New()
	gotC, compiled, _ := a.RunCompiled(iso)
	b, _ := New()
	gotI, _ := b.Run(iso)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("per-instance isolation: compiled=%v interp=%v (must match)", gotC, gotI)
	}
	_ = compiled
}

// with-decimal block — a 0-input body run inside a scoped BigDecimal rounding
// context compiles to a closure (like `do`) driven through InvokeBody, so the
// VM-run body's decimal ops read the precision/rounding override exactly as the
// interpreter's doEvalList does. Verifies native lowering + parity, including a
// NESTED context (the inner override shadows the outer).
func TestWithDecimalCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`with-decimal {precision: 5} [0d1.0 div 0d3.0]`, "[0d0.33333]"},
		{`with-decimal {precision: 4 rounding: "down"} [0d2.0 div 0d3.0]`, "[0d0.6666]"},
		{`with-decimal {precision: 6} [with-decimal {precision: 3} [0d1.0 div 0d3.0]]`, "[0d0.333]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native closure lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, _ := b.Run(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}
}

// Roadmap item 5 part B (each/fold/scan map lambda args) — the map-iteration
// words run a KeyVal-shaped lambda closure natively. These share the same
// handler as their token-quotation form, which sees the bare VALUE; the unit's
// recorded ClosureInShape (ClosureWantsKeyVal) keeps the two apart, so a
// KeyVal-destructuring lambda and a value-consuming quotation both compile and
// both stay correct.
func TestMapLambdaCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		// KeyVal-shaped lambda closures (the lambda reads kv.v / kv.i / acc).
		{`{a:1 b:2} each ([kv:KeyVal] => [kv.v add kv.i])`, "[{a:1 b:3}]"},
		{`0 fold ([acc:Integer kv:KeyVal] => [acc add kv.v]) {a:1 b:2 c:3}`, "[6]"},
		{`{a:1 b:2 c:3} scan ([acc:Integer kv:KeyVal] => [acc add kv.v])`, "[{a:1 b:3 c:6}]"},
		// Token-quotation map closures share the handler but take the bare value
		// — the ClosureInShape flag must NOT wrap these in a KeyVal.
		{`each [mul 10] {a:1 b:2}`, "[{a:10 b:20}]"},
		{`fold [add] {a:1 b:2 c:3} 0`, "[6]"},
		{`{a:1 b:2 c:3} scan [add]`, "[{a:1 b:3 c:6}]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native closure lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, _ := b.Run(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}
}
