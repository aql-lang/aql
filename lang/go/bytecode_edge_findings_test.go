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
	// Stage J: RunCompiled returns the refusal as compile_refused (no
	// silent re-run); the program stays fully serviceable via RunInterp.
	b, _ := New()
	_, compiled, errC := b.RunCompiled(src)
	if compiled {
		t.Errorf("%q: RunCompiled reported a compiled run; a refused program must not compile", src)
	}
	if codeOf(errC) != "compile_refused" {
		t.Errorf("%q: RunCompiled err=[%s] %v, want compile_refused (Stage J)", src, codeOf(errC), errC)
	}
	c, _ := New()
	if _, errI := c.RunInterp(src); errI != nil && codeOf(errI) == "compile_refused" {
		t.Errorf("%q: RunInterp must never report compile_refused", src)
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
	gotI, _ := c.RunInterp(src)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: parity compiled=%v interp=%v", src, gotC, gotI)
	}
	if want != "" && fmt.Sprint(gotC) != want {
		t.Errorf("%q: got %v, want %s", src, gotC, want)
	}
}

// §1 — forward collection reaching across an error-handler residual. `add`
// used to match all-stack `[dynamic-error-out, leading-residual]` under the
// dynamic(Any) island output (its String catch-all), stranding the forward
// token the interpreter collects (`5 do … error [drop 9] add 1` → `5 10`
// interp, `14 1` compiled). GRADUATED (L-EACH, plan Phase 5): errorReturnsFn
// narrows the catch result to dynamic(join(pass-through, handler)) — here
// dynamic(Integer) — so the String overload is disjoint, check mode selects
// the interpreter's forward collection, and the rows compile natively. A
// GENUINELY wide join (an Integer|String boundary) keeps the drift refusal —
// pinned below. Words whose forward collection was never blocked (`mul`,
// `sub`, a String forward token) keep compiling as before.
func TestEdgeFindingForwardAcrossErrorResidual(t *testing.T) {
	mustCompileWithParity(t, `5 do [raise aa "x"] error [drop 9] add 1`, "[5 10]")
	mustCompileWithParity(t, `5 do [7] error [drop 9] add 1`, "[5 8]")
	mustCompileWithParity(t, `1 2 3 do [7] error [drop 9] add 1`, "[1 2 3 8]")

	// The genuinely dynamic boundary keeps refusing: the handler nets a
	// String while the pass-through is Integer, so the joined bound stays
	// wide enough that the all-stack String match is viable and the runtime
	// dispatch is value-dependent — no faithful static record exists.
	mustRefuseWithParity(t,
		`5 do [7] error ["x"] add 1`,
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

	// A word with NoEvalArgs (code-body) positions — `if` here — forward-
	// collects its BODY tokens, never a trailing SCALAR after them, so the
	// drift model (a bare-scalar re-collection like `add 1`) does not apply.
	// `if ok [t] [e] 0` with a dynamic `ok` matched all-stack (an analysis-order
	// artifact) and a trailing `0` used to false-positive here; the recorded
	// branch binds the correct operands and the `0` is a separate residual, so
	// it compiles with parity. This is the decision prop_test summary-each shape
	// (`each [ … if ok [print] [print] 0 ]` over dynamic-typed results).
	mustCompileWithParity(t,
		`def xs [{ok:true} {ok:false}] def _ (xs each [ var [[r] def ok (r "ok" get) def res (if ok [1] [2]) def _2 res 0 ] ]) 9`,
		"[9]")
	mustCompileWithParity(t,
		`def m {ok:true} def ok (m get "ok") if ok [ 1 print ] [ 2 print ] 7`,
		"[7]")
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

// §5 (divergence fix) — a `do` body arriving as a QUOTED VALUE (via
// `def b (quote [break])`) runs its tokens AS CODE, so a break/continue inside
// escapes `do` to the enclosing loop exactly as the interpreter's shared tape
// does. Two bugs made the compiled run diverge: (a) bodyHasSentinel honoured
// the .Quoted flag and missed the break, so the const-folded body compiled into
// a closure whose break raised "flow signal with no enclosing loop" and `do`
// caught it as an error VALUE; (b) even routed to a FALLBACK `do`, the VM did
// not translate the escaped FlowCtrl on the fallback seam. Fixed:
// valueHasSentinel ignores .Quoted (routing the body off the closure path), and
// OpFallback now resolves escaped flow like the fn-value-call family. The break
// now unwinds the compiled loop → interpreter parity.
func TestEdgeFindingQuotedDoBodyFlowEscapesLoop(t *testing.T) {
	// The previously-miscompiling shapes now compile with parity: the break
	// terminates the enclosing `for`, so nothing is collected.
	mustCompileWithParity(t, `def b (quote [break]) for 5 [do b i]`, "[]")
	mustCompileWithParity(t, `def b (quote [continue]) for 3 [do b i]`, "[]")
	// A nested loop: the inner break terminates only the INNER `for`, so the
	// outer loop's `i` values survive.
	mustCompileWithParity(t, `def b (quote [break]) for 2 [ for 3 [do b] i ]`, "[0 1]")

	// Negatives — the fix must not over-refuse a sentinel-free quoted do body.
	mustCompileWithParity(t, `def b (quote [7]) for 3 [do b i]`, "[7 0 7 1 7 2]")

	// No ENCLOSING loop: the escaped break has nowhere to unwind to, so the
	// fallback seam's resolveEscapedFlow returns flowSignal's no-loop error and
	// the compiled run raises the interpreter's canonical "break outside loop"
	// — parity on the RAISE, not the value (mustCompileWithParity can't express
	// a raising row). Covers the vm.go OpFallback error arm.
	{
		src := `def b (quote [break]) do b`
		a, _ := New()
		_, iErr := a.RunInterp(src)
		b, _ := New()
		_, _, cErr := b.RunCompiled(src)
		if iErr == nil || cErr == nil || codeOf(iErr) != codeOf(cErr) {
			t.Errorf("%q: raise parity — compiled=[%s]%v interp=[%s]%v",
				src, codeOf(cErr), cErr, codeOf(iErr), iErr)
		}
	}
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

// Conditional fn-shadow divergence — a user fn REDEFINED inside a branch/loop
// body clobbers the enclosing overload in place (installDef's overlap-removal
// drops the outer entry without growing the def depth, so the branch/loop
// rollback cannot restore it). Compiled resolution then statically bakes the
// conditional shadow while the interpreter keeps the outer fn when the branch
// is not taken (or the loop runs zero times), so `if false [def g …] g 1`
// returned the shadow's value compiled but the ORIGINAL interpreted. The fix
// refuses to compile the redefinition (CondBodyDepth-gated) so the interpreter
// owns the shape — slow, not wrong.
func TestEdgeFindingConditionalFnShadowRefuses(t *testing.T) {
	fnA := `fn [[x:Any] [Integer] [x add 100]]`
	fnB := `fn [[x:Any] [Integer] [x add 1]]`
	want := "redefined inside a conditional body"

	// REFUSE: every conditionally-reached redefinition of an outer fn.
	mustRefuseWithParity(t, `def g `+fnA+` if false [def g `+fnB+`] g 1`, want) // branch not taken
	mustRefuseWithParity(t, `def c false def g `+fnA+` if c [def g `+fnB+`] g 1`, want)
	mustRefuseWithParity(t, `def g `+fnA+` if true [def g `+fnB+`] g 1`, want) // taken, still unsound-at-shape
	mustRefuseWithParity(t, `def g `+fnA+` for 2 [def g `+fnB+`] g 1`, want)   // loop body
	mustRefuseWithParity(t, `def g `+fnA+` ([1 2] each [def g `+fnB+`]) g 1`, want)

	// COMPILE (must NOT over-refuse): the redefinition is UNCONDITIONAL.
	mustCompileWithParity(t, `def g `+fnA+` def g `+fnB+` g 1`, "[2]")      // top-level shadow
	mustCompileWithParity(t, `def g `+fnA+` do [def g `+fnB+`] g 1`, "[2]") // do leaks unconditionally
	// COMPILE: no outer overload to clobber — a NEW name defined in a branch.
	mustCompileWithParity(t, `if true [def h `+fnB+` h 1]`, "[2]")
	// COMPILE: the control fn with no shadow at all.
	mustCompileWithParity(t, `def g `+fnA+` g 1`, "[101]")
}
