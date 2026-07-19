package help

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/formatter"
)

// exampleSkeleton reduces a snippet to its structural content: whitespace,
// commas, and brackets are dropped and letters are lower-cased. Two snippets
// with the same skeleton differ only by fmt's SEMANTICS-PRESERVING
// canonicalisations — spacing, optional-comma removal, single-fn bracket
// elision, and type-name capitalisation. A skeleton change means fmt altered
// what the code MEANS.
func exampleSkeleton(s string) string {
	return strings.ToLower(strings.NewReplacer(
		" ", "", "\t", "", "\n", "", "\r", "", ",", "", "[", "", "]", "",
	).Replace(s))
}

// TestDescribeExamplesFmtSafe runs every hand-authored describe/help example
// through the source formatter and pins that fmt never CORRUPTS one — the
// "all code examples run through fmt" requirement enforced as a correctness
// gate, and a regression guard for the seven fmt-corruption fixes (minilang
// literals, none/list/any/node values, fn-def truncation, dot-keys, comment
// absorption, block-comment content, and the `##`-line-comment model).
//
// It asserts structural preservation (skeleton equality), not byte equality:
// the examples are hand-formatted for readability, so fmt legitimately
// re-spaces / drops optional commas / elides single-fn brackets / capitalises
// type names — none of which change the skeleton. Any skeleton change is a
// real corruption and fails here. (Canonicalising the examples to fmt's exact
// output is a separate, visible-doc change reserved for maintainer sign-off;
// this gate is the safe, correctness-only half.)
func TestDescribeExamplesFmtSafe(t *testing.T) {
	checked := 0
	for word, e := range registry {
		for _, ex := range e.Examples {
			checked++
			got := formatter.Format(ex)
			if exampleSkeleton(got) != exampleSkeleton(ex) {
				t.Errorf("fmt corrupts the %q example:\n  in:  %q\n  out: %q", word, ex, strings.TrimRight(got, "\n"))
			}
		}
	}
	if checked == 0 {
		t.Fatal("no examples found in the help registry")
	}
	t.Logf("ran %d describe examples through fmt; none corrupted", checked)
}
