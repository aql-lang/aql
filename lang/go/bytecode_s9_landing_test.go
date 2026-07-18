package lang

import "testing"

// REFUSAL-CLOSURE §9 landing battery — written FAILING-FIRST (the goal's
// test-driven discipline): every subtest asserts the TARGET compile-parity
// behavior of one §9 item and fails until its landing flips it. The `want`
// values are the interpreter's (probe-pinned before any implementation).

const s9DocMod = `import module [ def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]] def boom fn [[x:Any] [Any] [ raise bad_input "always" ]] export "M" {dec: dec/r, boom: boom/r} ] end `

func TestS9LoopCarriedVariadicStore(t *testing.T) { // §9.2a — DESIGNED FOLLOW-ON
	// A loop-carried def of a variadic loop region (`for 2 [ def acc (for 2
	// [5]) acc ]`) is the §5 first-value split ONE frame down — but the
	// splice-at-depth bind (OpBindGlobal/OpBindDynScope) uses a stack-TOP-
	// relative depth, and inside a loop-body fragment the region sits at a
	// FRAGMENT-relative depth (the outer iterator locals sit below it), so
	// the naive splice miscompiled [5 0 5 0] vs [5 5 5 5] — the reason
	// SplitLoopRegionBind gates on NestedBodyDepth==0 today. Compiling it
	// needs a fragment-stack-base-relative splice-at-depth (a real VM +
	// lowering extension of §5, threading the fragment base through the
	// bind) plus the spilled N-1 rest flowing as the enclosing loop's
	// variadic body residual. Scoped as its own landing; refuses soundly
	// with parity until then.
	mustRefuseWithParity(t, `for 2 [ def acc (for 2 [5]) acc ]`, "consumes loop results")
	mustRefuseWithParity(t, `for 3 [ def acc (for 2 [1]) ]`, "consumes loop results")
}

func TestS9SpliceComputedPayload(t *testing.T) { // §9.2b
	mustCompileWithParity(t, `def xs (range 1 3) def d word xs d`, "[1 2]")
}

func TestS9XmlInterpComputed(t *testing.T) { // §9.2c
	mustCompileWithParity(t, `def n 7 <p>${n add 1}</p>`, "[<p>8</p>]")
	mustCompileWithParity(t, `def f fn [[x:Integer] [Xml] [<p>${x}</p>]] f 5`, "[<p>5</p>]")
}

func TestS9CurriedFactory(t *testing.T) { // §9.2d
	mustCompileWithParity(t,
		`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] ((mk 1) 2)`, "[3]")
}

func TestS9ParenBoundedLeadingApply(t *testing.T) { // §9.2e
	// The stamp-suite shape: the apply inside an FN BODY (top-level already
	// compiles via RecordDynApply).
	mustCompileWithParity(t,
		`def m {f: ([y:Integer] => [y add 1])} def h fn [[x:Integer] [Integer] [ add 1 ((m get "f") x) ]] h 7`, "[9]")
}

func TestS9SurfaceTypedDispatch(t *testing.T) { // §9.2f
	mustCompileWithParity(t,
		`def Comparable surface {cmp: (fnsig [[Self Self] [Integer]])} Integer exposes Comparable def g gen [(T extends Comparable)] fn [[a:T b:T] [Integer] [cmp a b]] g 3 5`,
		"[1]")
}

func TestS9FnComputedOperand(t *testing.T) { // §9.2g — DESIGNED KEEP
	// A fn constructed over a COMPUTED input sig (`fn (2 add 3) [Integer]
	// [7]`) is runtime-value-dependent: the param PATTERN is the runtime
	// value (5), so a compiled unit would bake the check-time carrier where
	// the live value belongs. No runtime op short of re-constructing the
	// FnDefInfo per evaluation fixes it, and that machinery is unjustified
	// for so exotic a shape — a designed opt-out like §8. Pinned refusing
	// with interpreter parity.
	mustRefuseWithParity(t, `def f fn (2 add 3) [Integer] [7] f 5`,
		"triple-form construction over a computed")
	mustRefuseWithParity(t, `(2 add 3) afn [7]`,
		"construction over a computed")
}

func TestS9FrontierDefOverCatchRegion(t *testing.T) { // §9.1 rows 1-2 — DESIGNED FOLLOW-ON
	// A DEF binding the fallible do-catch's VARIADIC region (`def msg (do
	// [(true 5 M.dec) "no-raise"] error [dot code]) msg`) refuses "residual
	// shape beyond Stage 1 (call result above a literal)" — the seatResults
	// family. Unlike row 3 (a bare region residual the mark-window island
	// owns, fixed by the ExtensionPayload ID mint), these have a call result
	// ABOVE a literal INSIDE a runtime-variable region that a DEF then
	// consumes: the L-DO part-2 region-top variable-arity lowering (a
	// STORE_LOCAL_FROM_MARK sibling seating the region-top under the def
	// while the mark bounds the variable arity). Scoped as its own landing;
	// refuses soundly with interpreter parity until then.
	for _, src := range []string{
		s9DocMod + `def msg (do [(true 5 M.dec) "no-raise"] error [dot code])  msg`,
		s9DocMod + `def msg (do [(false 5 M.dec) "no-raise"] error [dot code])  msg`,
	} {
		mustRefuseWithParity(t, src, "residual shape beyond Stage 1")
	}
}

func TestS9FrontierModuleExportInRegion(t *testing.T) { // §9.1 row 3
	mustCompileWithParity(t, s9DocMod+`do [M 3] error [dot code]`, "")
}
