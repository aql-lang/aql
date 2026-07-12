package lang

import "testing"

// TestDoCatchMultiValueArity pins the fix for a FALLIBLE multi-value `do` body
// under a catch. `do` catches a body raise into ONE Error value, so a body that
// nets N values with no raise but 1 Error on a raise has a runtime-VARIABLE
// count. The compiler used to seat the static N (via the closure / dyn-body /
// generic RecordCall paths), which UNDERFLOWED on the caught path — a
// STORE_LOCAL underflow (the voxgig-aql/trie `codec-roundtrip` shape:
// `def msg (do [(s.decode) "x"] error […])`). doListReturnsFn now refuses a
// fallible multi-value body so it rides the sound interpreter fallback, while a
// pure / infallible multi-value body still compiles at its exact arity.
//
// The differential corpus is BLIND to this (a fallible body that never raises
// at its test input passes parity while latently unsound), so these are
// hand-pinned off-corpus regressions: engine parity (compiled == interpreter,
// including the raised error) AND the native/fallback expectation per shape.
func TestDoCatchMultiValueArity(t *testing.T) {
	// A module with a value-dependently-raising fn (map-decode-like) and an
	// always-raising one, reached as `M.dec` / `M.boom` (a Reach dispatch).
	const mod = `import module [
  def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]]
  def boom fn [[x:Any] [Any] [ raise bad_input "always" ]]
  export "M" {dec: dec/r, boom: boom/r}
] end `

	// COMPILES natively — pure/infallible multi-value bodies keep their exact N.
	native := []string{
		`do [10 20 30]`,                        // pure literals
		`do [1 add 2 10 mul 4]`,                // infallible core natives
		`do [(1 add 2) (3 mul 4)]`,             // computed, infallible
		`def x 5  do [x 1 add 2]`,              // a binding read + infallible native
		`def b true  do [1 2 (if b [] [9 9])]`, // variadic (rides the backstop)
		`do [for 3 [1]]`,                       // loop residual (variadic backstop)
	}
	for _, src := range native {
		requireEngineParity(t, src, true)
	}

	// FALLS BACK (refuses natively) — fallible multi-value bodies. Parity must
	// still hold: the caught-error path is byte-identical to the interpreter,
	// no STORE_LOCAL underflow. wantCompiled=false (the sound fallback).
	fallback := []string{
		// The trie codec shape: a Reach (module fn) that RAISES, caught, bound.
		mod + `def msg (do [(true 5 M.dec) "no-raise"] error [dot code])  msg`,
		// The same body that does NOT raise at this input still refuses (the
		// static shape is fallible) — and stays correct via fallback.
		mod + `def msg (do [(false 5 M.dec) "no-raise"] error [dot code])  msg`,
		// An always-raising module fn.
		mod + `do [(M.boom 5) "x"] error [dot code]`,
		// A value-diverging native (`div`, CompileValueDiverges) with a NON-static
		// divisor so the body stays multi-value (div does not statically diverge):
		// refuses on the fallible-native path, correct via fallback (y=5, no raise).
		`def y 5  do [(10 div y) 2]`,
		// A user (AQL-body) fn that raises, caught.
		`def f fn [[x:Any] [Any] [raise bad_input "nope"]]  do [(f 5) 2] error [dot code]`,
		// A NATIVE module fn bound to a NAME (Module set, no AQL body) — the
		// fnDefMayRaise Module!="" path. StructUtil.parse can raise on bad input.
		`import "aql:struct-util"  def g StructUtil.parse/r  do [(g "x") 2] error [dot code]`,
		// A bare module-export VALUE in the body — the wordMayRaise TModuleExport path.
		mod + `do [M 3] error [dot code]`,
		// A fallible call nested inside a branch-arm LIST within a paren — the
		// nested-list recursion (`(if … [M.boom] …)`).
		mod + `do [(if true [M.boom 5] [7]) 8] error [dot code]`,
	}
	for _, src := range fallback {
		requireEngineParity(t, src, false)
	}
}
