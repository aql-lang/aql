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
	// GRADUATED 2026-07-14 (OpDispatchRematch, plan Phase 3): the apply.tsv
	// pair and the four generics rows compile to a terminal runtime rematch —
	// the failed window (a single event-carrier in every one) re-matches over
	// the live values at run time and raises the shared rich diagnostic
	// byte-identical to the interpreter (or defers when it unexpectedly
	// matches). The three rows below stay refused: their windows exceed the
	// written-tuple bound (local add), carry a variadic-if operand (each),
	// or hold a splice.

	// a VARIADIC-IF result feeding `each`: the arms leave DIFFERENT counts
	// (then [99] = one value, else [1 2] = two), so the merged position joins
	// to a None|Integer disjunct and the Any/Disjunct-carrier recovery owns
	// the dispatch — tryRecordPoly rightly declines each's code-body sigs,
	// and a runtime rematch would need a FIXED window arity the variadic
	// residual cannot give it (probe 2026-07-14: the trap is never reached;
	// the old "0-arg courtesy screen" attribution was wrong — each has no
	// 0-arg overload). Graduation rides the Phase 5 variadic-region lowering
	// (OpStackMark/OpDropToMark over the branch merge), which seats the
	// 1-vs-2 residual and lets a terminal rematch own the raise.
	`def n 0 if (n eq 0) [99] [1 2] each [dup mul]`: "unmatched dispatch recovered at each (variadic-if operand)",

	// open-words — a locally/module-redefined `add` overload anchored to a user
	// type: an operand outside the overload stays a no-match (negative rows).
	`def f fn [[x:Boolean] [Boolean] [def add fn [[a:Boolean b:Boolean] [Boolean] [a or b]] add x false]]  (f true) add true false`: "unmatched dispatch recovered at add (local add overload)",

	// GRADUATED 2026-07-14 (word-splice): a PARKED `__SP` marker (def-bound,
	// collected by value — never stepped before the dispatch) is identical at
	// run time, so the definiteness screen no longer lists IsSplice and the
	// row compiles to a serialized terminal trap raising the interpreter's
	// byte-identical no-match. The cascade fn_body_error of the post-trap
	// assumed-dispatch body analysis joined the SuppressBodyErrors codes. A
	// splice AT the pointer fires before any dispatch on both engines, so no
	// window ever holds a would-have-fired marker.
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
