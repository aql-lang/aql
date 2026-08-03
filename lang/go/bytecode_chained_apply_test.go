package lang

import "testing"

// bytecode_chained_apply_test.go pins the chained-forward-apply refusal
// (design/checker-compiler-completeness-review.0.md §2.1). Until 2026-08-02
// the forward spelling `f (g x)` of two chained Function-param applications
// slipped past the pending-apply refusal net (which covered only the
// `apply`-word spelling): noteDynFrameReplay armed the whole-frame replay on
// a window holding TWO applicable values, the flat re-push lost the inner
// group's collapse, and the compiled program raised the RET count error
// (`expected 1 return value(s), got 2`) where the interpreter applies both
// fns (compose → 14). The replay now arms only on a single-applicable
// window; the chained shapes refuse with faithful interpreter fallback.
// Graduation = Stage G proper (fn-unit dynamic apply) — flip these to
// fnValueM2Native and move the frontier rows into the main corpus.
func TestChainedForwardApplyRefuses(t *testing.T) {
	// Legacy refusal+fallback-parity contract, like TestApplyOverParamFnCompiles.
	t.Setenv("BORU_COMPILE_FALLBACK", "1")

	fnValueM2Refusal(t, "compose — outer fn over the inner fn's result",
		`def compose fn [[f:Function g:Function x:Integer] [Integer] [f (g x)]] compose ([a:Integer] => [a mul 2]) ([b:Integer] => [b add 3]) 4`,
		"unapplied fn-value in body residual")
	fnValueM2Refusal(t, "self-composition f (f x)",
		`def twice fn [[f:Function x:Integer] [Integer] [f (f x)]] twice ([n:Integer] => [n add 1]) 5`,
		"unapplied fn-value in body residual")

	// CONTROLS — the single-applicable windows keep compiling natively.
	fnValueM2Native(t, "one applicable of two Function params",
		`def sel1 fn [[f:Function g:Function x:Integer] [Integer] [f x]] sel1 ([a:Integer] => [a mul 2]) ([b:Integer] => [b add 3]) 4`,
		"[8]")
	fnValueM2Native(t, "single param-fn apply at the body tail",
		`def apply1 fn [[fnv:Function] [Integer] [(fnv 100)]] apply1 ([y:Integer] => [y add 1])`,
		"[101]")
}
