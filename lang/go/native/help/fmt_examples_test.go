package help

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/formatter"
)

// exampleSkeleton reduces a snippet to its structural content: whitespace,
// commas, and brackets are dropped and letters are lower-cased. Two snippets
// with the same skeleton differ only by fmt's SEMANTICS-PRESERVING
// canonicalisations. A skeleton change means fmt altered what the code MEANS.
func exampleSkeleton(s string) string {
	return strings.ToLower(strings.NewReplacer(
		" ", "", "\t", "", "\n", "", "\r", "", ",", "", "[", "", "]", "",
	).Replace(s))
}

// TestDescribeExamplesFmtClean enforces "all code examples run through fmt"
// for the describe/help worked examples: every one that fits the width is
// already in fmt's canonical form (running fmt is a no-op). This is the gate
// for the Phase-5 example canonicalisation — the examples were rewritten to
// fmt output, and this keeps them from drifting back out of it.
//
// The few examples that fmt would WRAP (exceed the width) are exempt from
// exact-clean — describe renders one example per line, so they must stay a
// single line — but they must still not be CORRUPTED (skeleton preserved).
// This also stands as a regression guard for the seven fmt-corruption fixes.
func TestDescribeExamplesFmtClean(t *testing.T) {
	clean, wrapped := 0, 0
	for word, e := range registry {
		for _, ex := range e.Examples {
			got := strings.TrimRight(formatter.Format(ex), "\n")
			if strings.Contains(got, "\n") {
				// Too long for one line: exempt from exact-clean, but must not
				// be corrupted.
				wrapped++
				if exampleSkeleton(got) != exampleSkeleton(ex) {
					t.Errorf("fmt corrupts the wrapping %q example:\n  in:  %q\n  out: %q", word, ex, got)
				}
				continue
			}
			clean++
			if got != ex {
				t.Errorf("%q example is not fmt-clean; run it through fmt:\n  have: %q\n  want: %q", word, ex, got)
			}
		}
	}
	if clean == 0 {
		t.Fatal("no examples found in the help registry")
	}
	t.Logf("%d describe examples are fmt-clean; %d exempt (would wrap) and verified non-corrupted", clean, wrapped)
}
