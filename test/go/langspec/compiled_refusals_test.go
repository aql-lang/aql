// Compilation refusals are test FAILURES. Full native compilation of every AQL
// program is the goal (design/COMPILABLE-SUBSET.md, design/P7-ENDGAME.10.md): a
// whole-program refusal silently runs on the interpreter, which is slower and
// keeps the compiler tied to the tree-walker. So this gate treats every spec-row
// refusal as a failure UNLESS the row is on knownRefusals — the small, documented
// set of correct-by-design soundness refusals where compiling a guess would ship
// a WRONG answer, so the interpreter fallback must own them.
//
// This is the per-ROW companion to TestCompiledCoverage's count/root-cause gate:
// the count gate catches a refusal-count regression, this gate pins the EXACT
// rows, so swapping one refusal for another (count unchanged) still fails, and a
// row that starts compiling is flagged as a stale allowlist entry to ratchet the
// list DOWN toward zero. Both read the single corpus census (compiled_census_test).
package langspec

import "testing"

// knownRefusals is the allowlist of spec rows the bytecode compiler currently
// refuses to lower, keyed by EXACT source. Every entry is a proven correct-by-
// design SOUNDNESS refusal: dispatch does not statically resolve — an "unmatched
// dispatch recovered" best-guess (the checker recovers an unmatched call the
// interpreter resolves or raises at runtime), or a fn value reaching a consuming
// word whose identity/capture state cannot bake — so a compiled guess could
// diverge from the interpreter. These are the ONLY refusals the suite tolerates.
//
// To lower this list (the goal), widen the compilable subset so the row compiles,
// then delete its entry. Do NOT add a row here to silence a real gap — an entry
// is a promise that compiling the row would be UNSOUND, not merely unimplemented.
var knownRefusals = map[string]string{
	// apply.tsv — a bare fn auto-fires on the value before `apply` sees it, so
	// `apply` has no fn: an unmatched dispatch the interpreter raises (ERROR rows).
	`def inc fn [[n:Integer][Integer][n add 1]]  5 inc apply`:       "unmatched dispatch recovered at apply (bare inc fires on 5 first)",
	`def inc fn [[n:Integer][Integer][n add 1]]  5 (ref inc) apply`: "unmatched dispatch recovered at apply ((ref inc) re-steps inc on 5)",

	// generics — a generic-typed param called with the WRONG instantiation is a
	// no-match the interpreter raises; the checker's recovery is a best guess.
	`def Box gen [T] class {v:T} def f fn [[x:(Box of [Number])] [Integer] [1]] end f (make (Box of [Integer]) {v:1})`:                               "unmatched dispatch recovered at f (Box of Number vs Box of Integer)",
	`def Box gen [T] class {v:T} def BoxI (Box of [Integer]) def BoxS (Box of [String]) def g fn [[b:BoxI] [Integer] [1]] end g (make BoxS {v:'s'})`: "unmatched dispatch recovered at g (BoxI vs BoxS)",
	`def Box<T> class {value:T} def f fn [[x:Box<Integer>] [Integer] [x dot value]] end f (make Box<String> {value:'s'})`:                            "unmatched dispatch recovered at f (Box<Integer> vs Box<String>)",
	`def Box gen [T] class {value:T} def f fn [[x:(Box of [Integer])] [Integer] [x dot value]] end f (make (Box of [String]) {value:'s'})`:           "unmatched dispatch recovered at f (Box of Integer vs Box of String)",

	// a variadic-if result feeding a higher-order word (`each`) over a recovered
	// dispatch — the interpreter resolves it at runtime; a static guess diverges.
	`def n 0 if (n eq 0) [99] [1 2] each [dup mul]`: "unmatched dispatch recovered at each (variadic-if operand)",

	// open-words — a locally/module-redefined `add` overload anchored to a user
	// type: an operand outside the overload stays a no-match (negative rows).
	`def f fn [[x:Boolean] [Boolean] [def add fn [[a:Boolean b:Boolean] [Boolean] [a or b]] add x false]]  (f true) add true false`: "unmatched dispatch recovered at add (local add overload)",

	// word-splice — `word` is a live-stack splice, not a spread; `f p` is a
	// no-match the interpreter raises (code splice is NOT spread).
	`def p word [1 add 2] def f fn [[x:Integer][Integer][x mul 10]] f p`: "unmatched dispatch recovered at f (code splice is not spread)",
}

// TestRefusalsAreFailures fails on any spec-row compilation refusal that is not
// a documented correct-by-design refusal, and on any allowlist entry that no
// longer refuses (stale — remove it). See knownRefusals.
func TestRefusalsAreFailures(t *testing.T) {
	c := gatherCensus(t)

	seen := make(map[string]bool, len(knownRefusals))
	for _, r := range c.refusedRows {
		if _, ok := knownRefusals[r.input]; ok {
			seen[r.input] = true
			continue
		}
		t.Errorf("compilation refusal is a failure (%s:%d): %q\n  reason: %s\n  full native compilation is the goal: widen the compilable subset so this row compiles, or — only if compiling it would be UNSOUND — add it to knownRefusals with a soundness justification.",
			r.file, r.line, r.input, r.reason)
	}

	for input, why := range knownRefusals {
		if !seen[input] {
			t.Errorf("stale allowlist entry — this row no longer refuses to compile; delete it from knownRefusals to ratchet the list down:\n  %q\n  (was: %s)", input, why)
		}
	}

	t.Logf("refusals: %d rows, all %d documented in knownRefusals", len(c.refusedRows), len(knownRefusals))
}
