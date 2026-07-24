package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestTailCallResultBindsThroughForwardParen pins the interpreter fix for the
// silent def-binding failure (alice dx-report §1): a tail-recursive fn whose
// result is consumed as a FORWARD PAREN-GROUP argument — `def x (f n)`,
// `g (f n)`, `(f n) add …` — must bind/return that result, not raise
// `undefined_word`.
//
// A forward paren group is evaluated eagerly by evalParenGroupAt, which walks
// the group's inner tokens with a LOCAL paren-depth counter to find its
// matching `)`. A tail call dispatched inside the group fires TCO, which
// rewrites the enclosing frame region on the tape (full replacement / shell
// elision) out from under that counter — the group's close paren is deleted or
// index-shifted before the loop decrements on it, so the counter never reaches
// zero, the group never collapses, and its bound result silently vanishes.
// The fix declines TCO while any paren group is being evaluated
// (Engine.parenEvalDepth > 0): the tail call NESTS instead, which the depth
// counter tracks correctly. The bug was interpreter-only — the compiler
// already lowered this correctly — so every case also asserts
// compile == interpret.
func TestTailCallResultBindsThroughForwardParen(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			"minimal: def-bind a tail-recursive result, then use it",
			`def f fn [[n:Integer] [Integer] [ if ((n) lte 0) [ 0 ] [ f ((n) sub 1) ] ]]
			 def x (f 3)
			 (x) add 1`,
			"1",
		},
		{
			"accumulator recursion bound via a forward paren",
			`def f fn [[n:Integer acc:Integer] [Integer] [ if ((n) lte 0) [ (acc) ] [ f ((n) sub 1) ((acc) add (n)) ] ]]
			 def x (f 50 0)
			 (x)`,
			"1275",
		},
		{
			"two recursive results inside one nested paren expression",
			`def f fn [[n:Integer] [Integer] [ if ((n) lte 0) [ 0 ] [ f ((n) sub 1) ] ]]
			 def g fn [[n:Integer] [Integer] [ if ((n) lte 0) [ 7 ] [ g ((n) sub 1) ] ]]
			 ((f 3)) add ((g 4))`,
			"7",
		},
		{
			"recursive result passed straight to another word (no def)",
			`def f fn [[n:Integer] [Integer] [ if ((n) lte 0) [ 42 ] [ f ((n) sub 1) ] ]]
			 (f 5) add 0`,
			"42",
		},
		{
			"outer fn forwards to a recursive inner, result bound via paren",
			`def inner fn [[n:Integer] [Integer] [ if ((n) lte 0) [ 100 ] [ inner ((n) sub 1) ] ]]
			 def outer fn [[n:Integer] [Integer] [ inner (n) ]]
			 def z (outer 5)
			 (z)`,
			"100",
		},
	}
	for _, c := range cases {
		gotI, err := mustNew(t).RunInterp(c.src)
		if err != nil {
			t.Fatalf("%s: interp error (regression — the bug raised undefined_word here): %v\n%s", c.name, err, c.src)
		}
		renderedI := fmt.Sprint(gotI)
		if !strings.Contains(renderedI, c.want) {
			t.Errorf("%s: interp got %v, want it to contain %q", c.name, gotI, c.want)
		}
		gotC, err := mustNew(t).RunCompiledStrict(c.src)
		if err != nil {
			t.Fatalf("%s: strict compiled error: %v\n%s", c.name, err, c.src)
		}
		if fmt.Sprint(gotC) != renderedI {
			t.Errorf("%s: compiled %v != interp %v (MISCOMPILE)", c.name, gotC, gotI)
		}
	}
}
