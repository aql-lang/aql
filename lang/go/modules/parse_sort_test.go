package modules

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	tabnas "github.com/tabnas/parser/go"
)

// TestParseMatcherSort pins that applyMatchers leaves Config.CustomMatchers
// priority-sorted even when matchers were added out of order — the tabnas
// lexer interleaves them with the built-in bands by priority, so an
// out-of-order entry would run too late to claim its tokens. White-box: it
// drives the builder's internals directly (the fn is never invoked, only
// wrapped, so a zero Value is fine).
func TestParseMatcherSort(t *testing.T) {
	g := &parseGrammar{j: tabnas.Make()}
	// Added high → low → middle: clearly NOT pre-sorted.
	g.matchers = []pendingMatcher{
		{name: "high", prio: 1000002, fn: native.Value{}},
		{name: "low", prio: 1000000, fn: native.Value{}},
		{name: "mid", prio: 1000001, fn: native.Value{}},
	}

	g.applyMatchers()

	got := g.j.Config().CustomMatchers
	if len(got) < 3 {
		t.Fatalf("expected at least the 3 custom matchers, got %d", len(got))
	}
	prev := got[0].Priority
	for i, m := range got {
		if m.Priority < prev {
			t.Fatalf("CustomMatchers not priority-sorted at index %d: %d after %d", i, m.Priority, prev)
		}
		prev = m.Priority
	}
}
