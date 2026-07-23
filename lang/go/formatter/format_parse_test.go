package formatter

import "testing"

// TestFormatWithParserSeam pins the parser-as-a-parameter seam: a nil
// parser falls back to DefaultParse, and a supplied parser is honoured —
// the layout rules and emitter run on whatever tree it returns.
func TestFormatWithParserSeam(t *testing.T) {
	// nil parse → DefaultParse.
	if got, want := FormatWith("def  x  1", nil), "def x 1\n"; got != want {
		t.Errorf("FormatWith(nil) = %q, want %q", got, want)
	}

	// A custom front end is a genuine parameter: this one ignores src and
	// returns a fixed tree, which the emitter formats.
	fixed := func(_ string) *Node {
		return &Node{Kind: NdRoot, Children: []*Node{
			{Kind: NdWord, Text: "custom"},
			{Kind: NdWord, Text: "tree"},
		}}
	}
	if got, want := FormatWith("anything at all", fixed), "custom tree\n"; got != want {
		t.Errorf("FormatWith(custom) = %q, want %q", got, want)
	}

	// DefaultParse yields the same result as the Format convenience wrapper.
	if got, want := FormatWith("a  b", DefaultParse), Format("a  b"); got != want {
		t.Errorf("DefaultParse divergence: %q vs %q", got, want)
	}

	// The hand-lexer reference and the (default) tabnas front end agree — the
	// same equivalence TestTabnasParseCorpus pins across the whole corpus.
	if got, want := FormatWith("def  x  1", HandParse), FormatWith("def  x  1", DefaultParse); got != want {
		t.Errorf("HandParse vs DefaultParse divergence: %q vs %q", got, want)
	}
}
