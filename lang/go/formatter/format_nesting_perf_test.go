package formatter

import (
	"strings"
	"testing"
	"time"
)

// deeplyNested builds a statement whose containers nest `depth` levels, with
// enough text at each level that the width strategy rejects the inline form
// and falls through to the wrapping path — the shape that used to trigger
// re-rendering of every subtree at every level.
func deeplyNested(depth int) string {
	var b strings.Builder
	b.WriteString("def probe fn [[x:Integer] [List] [\n")
	for i := 0; i < depth; i++ {
		b.WriteString(strings.Repeat("  ", i+1))
		b.WriteString("(if (x gt ")
		b.WriteString(strings.Repeat("9", 3))
		b.WriteString(") [[\n")
	}
	b.WriteString(strings.Repeat("  ", depth+1))
	b.WriteString("(some-fairly-long-word-here x \"a string literal to consume width\")\n")
	for i := depth - 1; i >= 0; i-- {
		b.WriteString(strings.Repeat("  ", i+1))
		b.WriteString("]] [[]])\n")
	}
	b.WriteString("]]\n")
	return b.String()
}

// Formatting must stay near-linear in nesting depth.
//
// emitNode is memoised because layout is try-then-fall-back: a statement is
// rendered inline before any strategy may reject it, and the wrap path then
// re-renders every child at the continuation indent. Uncached, a subtree
// nested D deep was rendered on the order of 2^D times — kg/validate.boru
// (749 lines, nesting ~10) took 53 s to format against 48 ms to parse.
// A regression here does not fail loudly; it just makes `boru fmt` and the
// LSP's formatting provider unusable, so the budget is asserted.
func TestFormatDeepNestingStaysFast(t *testing.T) {
	// Wall-clock assertion — meaningless under the race detector, which
	// inflates CPU ~5-10x. The race lane runs -short (Makefile test-race).
	if testing.Short() {
		t.Skip("perf timing assertion; skipped under -short (race lane)")
	}
	src := deeplyNested(14)
	start := time.Now()
	out := Format(src)
	elapsed := time.Since(start)

	// Generous: the memoised path formats this in single-digit ms. The
	// pre-fix exponential path took tens of seconds at this depth.
	const budget = 5 * time.Second
	if elapsed > budget {
		t.Errorf("formatting depth-14 nesting took %s, want < %s — "+
			"emitNode memoisation has regressed (see format.go emitNode)", elapsed, budget)
	}
	if out == "" {
		t.Fatal("formatting produced empty output")
	}
}

// Formatting is idempotent: the canonical layout is a fixed point. This is
// the negative half of the memo change — a cache keyed on the wrong thing
// would show up as a second pass that differs from the first.
func TestFormatDeepNestingIdempotent(t *testing.T) {
	for _, depth := range []int{1, 3, 8} {
		once := Format(deeplyNested(depth))
		twice := Format(once)
		if once != twice {
			t.Errorf("depth %d: formatting is not idempotent\nfirst:\n%s\nsecond:\n%s",
				depth, once, twice)
		}
	}
}

// The memo must not collapse renders that differ only by indent: a node laid
// out at one indent and the same node at another are different strings, so
// the key carries the indent. Formatting the same construct at two nesting
// levels must produce correspondingly different indentation.
func TestFormatMemoKeyedOnIndent(t *testing.T) {
	shallow := Format(deeplyNested(1))
	deep := Format(deeplyNested(4))
	if shallow == deep {
		t.Fatal("depth 1 and depth 4 formatted identically — the memo is ignoring indent")
	}
	// The deeper form must actually carry deeper indentation.
	maxIndent := func(s string) int {
		best := 0
		for _, line := range strings.Split(s, "\n") {
			n := len(line) - len(strings.TrimLeft(line, " "))
			if strings.TrimSpace(line) != "" && n > best {
				best = n
			}
		}
		return best
	}
	if maxIndent(deep) <= maxIndent(shallow) {
		t.Errorf("deep max indent %d not greater than shallow %d",
			maxIndent(deep), maxIndent(shallow))
	}
}
