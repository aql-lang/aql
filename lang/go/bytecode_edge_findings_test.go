package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Landing tests for design/EDGE-SPEC-FINDINGS.0.md — four compile≠interpret
// divergences the edge-spec expansion surfaced. Each was a shape the compiler
// lowered to a WRONG value; the fix makes the compiler REFUSE (fall back —
// "slow, not wrong") so the interpreter owns the shape. Every finding is pinned
// three ways: the reproducer REFUSES with its reason, the reproducer's compiled
// run falls back to interpreter PARITY, and a sibling that must keep compiling
// natively still does (the negative that proves the refusal is not blanket).

// mustRefuseWithParity asserts src refuses to compile (reason contains want)
// and that RunCompiled falls back to the interpreter's result.
func mustRefuseWithParity(t *testing.T, src, want string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("%q: CompileCheck error %v", src, cerr)
	}
	if prog != nil {
		t.Fatalf("%q: expected a refusal, but it compiled", src)
	}
	if !strings.Contains(reason, want) {
		t.Errorf("%q: refusal reason = %q, want it to contain %q", src, reason, want)
	}
	// RunCompiled falls back (compiled=false) to the interpreter's value.
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(src)
	if compiled {
		t.Errorf("%q: RunCompiled reported a compiled run; a refused program must fall back", src)
	}
	c, _ := New()
	gotI, errI := c.Run(src)
	if fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("%q: fallback error mismatch compiled=%v interp=%v", src, errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: fallback parity: compiled=%v interp=%v", src, gotC, gotI)
	}
}

// mustCompileWithParity asserts src compiles natively (no whole-program
// fallback island in the reason) and RunCompiled matches the interpreter.
func mustCompileWithParity(t *testing.T, src, want string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%q: expected a native compile, refused: reason=%q err=%v", src, reason, cerr)
	}
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(src)
	if !compiled || errC != nil {
		t.Fatalf("%q: compiled run: compiled=%v err=%v", src, compiled, errC)
	}
	c, _ := New()
	gotI, _ := c.Run(src)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: parity compiled=%v interp=%v", src, gotC, gotI)
	}
	if want != "" && fmt.Sprint(gotC) != want {
		t.Errorf("%q: got %v, want %s", src, gotC, want)
	}
}

// §1 — forward collection reaching across an error-handler residual. `add`
// matches all-stack `[dynamic-error-out, leading-residual]` under the dynamic
// island output (its [Scalar Scalar] catch-all), stranding the forward token
// the interpreter collects (`5 do … error [drop 9] add 1` → `5 10` interp,
// `14 1` compiled). Refuse. Words whose forward collection is NOT blocked by
// the dynamic operand (`mul`, `sub`, a String forward token) keep compiling.
func TestEdgeFindingForwardAcrossErrorResidual(t *testing.T) {
	mustRefuseWithParity(t,
		`5 do [raise aa "x"] error [drop 9] add 1`,
		"forward operand accounting across a dynamic/island residual")
	mustRefuseWithParity(t,
		`5 do [7] error [drop 9] add 1`,
		"forward operand accounting across a dynamic/island residual")
	mustRefuseWithParity(t,
		`1 2 3 do [7] error [drop 9] add 1`,
		"forward operand accounting across a dynamic/island residual")

	// Negatives — the drift guard must NOT over-refuse: `mul`/`sub` forward-
	// collect their token (fwdCount>0), a String forward token routes to a
	// different overload, and the no-leading-residual and concrete-`do` forms
	// have no bystander to reach past.
	mustCompileWithParity(t, `5 do [7] error [drop 9] mul 2`, "[5 14]")
	mustCompileWithParity(t, `5 do [7] error [drop 9] sub 1`, "[5 6]")
	mustCompileWithParity(t, `5 do [7] error [drop 9] add "z"`, "[5 7z]")
	mustCompileWithParity(t, `do [7] error [drop 9] add 1`, "[8]")
	mustCompileWithParity(t, `5 do [9] add 1`, "[5 10]")
}

// §2 — an applied member-fn boundary mid-expression. A parked fn read from a
// container (`m.double`) auto-applies the moment a value lands on it; the
// compiler instead lets a downstream word (`eq`) steal that value and applies
// the stranded fn at the residual tail to the wrong operand (`… m.double 21 eq
// 42` → `true` interp, `fn d(Integer) false` compiled). Refuse mid-expression;
// the statement-tail apply and non-fn member reads keep compiling.
func TestEdgeFindingMemberFnApplyMidExpression(t *testing.T) {
	mustRefuseWithParity(t,
		`def d fn [[n:Integer] [Integer] [n mul 2]] def m {double: d/r} m.double 21 eq 42`,
		"member fn value auto-applies mid-expression")

	// Negatives: the bare statement-tail apply lowers to the trailing apply;
	// unapplied member reads stay data; a non-fn member read never auto-applies.
	mustCompileWithParity(t,
		`def d fn [[n:Integer] [Integer] [n mul 2]] def m {double: d/r} m.double 21`, "[42]")
	mustCompileWithParity(t, `def m {x: 5} m.x eq 5`, "[true]")
	mustCompileWithParity(t, `def m {a: [1 2 3]} m.a get 0 eq 1`, "[true]")
}

// §3 — a paren-arrived value run as an else body. `(range 2 4)` reaches the
// compiler as a non-concrete list carrier, so the branch value path pushes the
// LIST while the interpreter's spliceArg EXECUTES it (`if (n eq 0) [99] (range
// 2 4)` → `2 3` interp, `[2 3]` compiled). Refuse the reachable list-value arm;
// a dead arm (a constant condition makes the opposite arm unreachable) and a
// scalar-valued arm keep compiling.
func TestEdgeFindingComputedElseBody(t *testing.T) {
	mustRefuseWithParity(t,
		`def n 5 if (n eq 0) [99] (range 2 4)`,
		"computed branch arm is a spliced list body")
	mustRefuseWithParity(t,
		`def n 0 if (n eq 0) (range 2 4) [99]`,
		"computed branch arm is a spliced list body")
	mustRefuseWithParity(t,
		`def n 5 if (n eq 0) [99] (range 2 3)`,
		"computed branch arm is a spliced list body")

	// Negatives: the spliceable-list arm is the DEAD branch (constant cond →
	// never taken), or the arm is a scalar/paren-scalar the interpreter pushes
	// as a value, or both arms are literal `[…]` bodies.
	mustCompileWithParity(t, `def n 0 if (n eq 0) [99] (range 2 4)`, "[99]")
	mustCompileWithParity(t, `def n 5 if (n eq 0) (range 2 4) [99]`, "[99]")
	mustCompileWithParity(t, `def n 5 if (n eq 0) [99] 42`, "[42]")
	mustCompileWithParity(t, `def n 5 if (n eq 0) [99] (add 1 2)`, "[3]")
	mustCompileWithParity(t, `def n 0 if (n eq 0) [99] [88]`, "[99]")
}

// §4 — `args.N` inside a compiled fn body with UNNAMED params. Unnamed params
// now bind to frame locals exactly like named ones (CompiledFn.NUnnamed: RET
// discards the unconsumed frame-bottom copies the interpreter's body splice
// leaves), so `args.N` folds to PUSH_LOCAL N for EVERY frame shape and these
// previously-refused rows compile with parity.
func TestEdgeFindingArgsOverUnnamedParams(t *testing.T) {
	mustCompileWithParity(t,
		`def f fn [[Integer String] [String] [args.1]] f 1 "hi"`, "[hi]")
	mustCompileWithParity(t,
		`def f fn [[Integer String] [Integer] [args.0]] f 1 "hi"`, "[1]")
	mustCompileWithParity(t,
		`def f fn [[a:Integer Integer] [Integer] [args.0]] f 3 4`, "[3]")

	// Every param NAMED — `args.N` folds to PUSH_LOCAL N.
	mustCompileWithParity(t,
		`def f fn [[a:Integer b:String] [String] [args.1]] f 1 "hi"`, "[hi]")
	mustCompileWithParity(t,
		`def f fn [[a:Integer b:Integer] [Integer] [args.0 add args.1]] f 3 4`, "[7]")
	mustCompileWithParity(t,
		`def f fn [[n:Integer] [Integer] [if (n lte 0) [args.0] [f (n sub 1)]]] f 3`, "[0]")
}
