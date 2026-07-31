package formatter

import (
	"strings"
	"testing"
)

// TestScanMinilang exercises every branch of scanMinilang: the closed and
// open forms, each escape, the delimiter varieties, and every decline path
// (not a '+', no lowercase name, a digit name start, no delimiter, and an
// empty open source).
func TestScanMinilang(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // full literal text returned ("" when declined)
		n    int    // bytes consumed (0 when declined)
	}{
		{"closed slash", "+re/[a-z]+/", "+re/[a-z]+/", 11},
		{"closed pipe", "+gex|a*b|", "+gex|a*b|", 9},
		{"kind with digits/dash", "+m2-x/ab/", "+m2-x/ab/", 9},
		{"escaped delim", "+re/a\\/b/", "+re/a\\/b/", 9},
		{"escaped backslash", "+re/a\\\\b/", "+re/a\\\\b/", 9},
		{"escaped space stays in body", "+re/a\\ b/", "+re/a\\ b/", 9},
		{"other escape preserved raw", "+re/\\d+/", "+re/\\d+/", 8},
		{"open form to whitespace", "+m:a@b.com x", "+m:a@b.com", 10},
		{"open form to end", "+m:a@b.com", "+m:a@b.com", 10},
		{"closed then abut", "+re/x/)", "+re/x/", 6},
		{"trailing backslash consumed", "+re/a\\", "+re/a\\", 6},
		// Declines:
		{"not a plus", "re/x/", "", 0},
		{"empty", "", "", 0},
		{"digit name start (signed number)", "+0d5", "", 0},
		{"plus then end", "+", "", 0},
		{"plus then uppercase", "+Re/x/", "", 0},
		{"no delimiter — space after name", "+re x", "", 0},
		{"no delimiter — end after name", "+re", "", 0},
		{"empty open source", "+m: x", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n := scanMinilang(tt.in)
			if got != tt.want || n != tt.n {
				t.Errorf("scanMinilang(%q) = (%q, %d), want (%q, %d)",
					tt.in, got, n, tt.want, tt.n)
			}
		})
	}
}

// TestFormatMinilangVerbatim pins the fix: a minilang literal is one atomic
// token whose interior ([, ], +, *, |, /) survives untouched, while the
// surrounding statement is still normalised. Before the fix, fmt fragmented
// `+re/[a-z]+/` at the bracket tokens and re-spaced it to `+re/ [a-z] +/`.
func TestFormatMinilangVerbatim(t *testing.T) {
	cases := []struct{ in, want string }{
		{"def   f   (+re/[a-z]+/)\n", "def f (+re/[a-z]+/)\n"},
		{"+gex|a*b|\n", "+gex|a*b|\n"},
		{"(+re/x/).fst\n", "(+re/x/).fst\n"},
		{"+hb/de_ad_be_ef/\n", "+hb/de_ad_be_ef/\n"},
		{"+m:alice@example.com\n", "+m:alice@example.com\n"},
	}
	for _, c := range cases {
		if got := Format(c.in); got != c.want {
			t.Errorf("Format(%q)\n  got:  %q\n  want: %q", c.in, got, c.want)
		}
	}

	// Idempotence: formatting the canonical form again is a no-op.
	once := Format("def f (+re/[a-z]+/)\n")
	if twice := Format(once); twice != once {
		t.Errorf("Format not idempotent on a minilang literal:\n  once:  %q\n  twice: %q", once, twice)
	}

	// A minilang literal longer than the width is one token — never split.
	long := "+re/" + strings.Repeat("a", 90) + "/"
	if out := Format("def y " + long + "\n"); !strings.Contains(out, long) {
		t.Errorf("long minilang literal was altered:\n%s", out)
	}
}

// TestFormatCommentNeverSwallowsDelimiter pins the fix that a line comment
// (`# …`, which runs to end of line) never has a closing delimiter or a
// following token placed after it on the same line — otherwise fmt silently
// comments the delimiter out and unbalances the source. Each container with
// an interior line comment therefore breaks multi-line, closing on its own
// line; and every formatted result stays a fixed point.
func TestFormatCommentNeverSwallowsDelimiter(t *testing.T) {
	cases := []struct{ name, in string }{
		{"paren", "def x (\n  a  # c1\n)\n"},
		{"list", "foo [\n  a  # c\n  b\n]\n"},
		{"map", "{a:1\n# note\nb:2}\n"},
		{"nested paren+list", "def f (\n  do [ x ] error [ raise ]  # oops\n)\n"},
		{"comment then close then more", "g [\n  a  # note\n  b c d\n]\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := Format(c.in)
			// No output line may have a token after a '# …' line comment: the
			// comment must be the tail of its line.
			for _, ln := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
				if h := strings.Index(ln, "# "); h >= 0 {
					tail := strings.TrimRight(ln[h:], " ")
					if strings.ContainsAny(tail, ")]}") && !strings.Contains(tail, "#!") {
						// A ')' etc. after the '#' would be commented out.
						// Allow it only if it is part of the comment PROSE, not a
						// real delimiter — here our inputs use plain word comments.
						t.Errorf("%s: delimiter after line comment:\n%s", c.name, out)
					}
				}
			}
			// Idempotence: the corrected layout is a fixed point.
			if twice := Format(out); twice != out {
				t.Errorf("%s: not idempotent\n once:  %q\n twice: %q", c.name, out, twice)
			}
		})
	}
}

// TestFormatDoubleHashIsLineComment pins that `##` is a LINE comment to end
// of line, exactly like `#` — BORU has no bounded block comment. So the whole
// `## … ## …` tail is one comment emitted verbatim; the formatter must never
// treat what follows the second `##` as code and reformat it (which corrupted
// comment text: `## t ## x is integer` → `## t ## x is Integer`).
func TestFormatDoubleHashIsLineComment(t *testing.T) {
	cases := []struct{ in, want string }{
		{"## bc ## foo bar\n", "## bc ## foo bar\n"},
		{"## t ## x is integer\n", "## t ## x is integer\n"}, // NOT `x is Integer`
		{"## t ## {a:1,b:2}\n", "## t ## {a:1,b:2}\n"},       // comma NOT removed
		{"## bc ##\n", "## bc ##\n"},
		{"# line only\n", "# line only\n"},
		{"foo ## bc ## bar\n", "foo ## bc ## bar\n"},
	}
	for _, c := range cases {
		if got := Format(c.in); got != c.want {
			t.Errorf("Format(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFormatDotKeyNotCapitalized pins that the literal dot / dotr accessor
// words quote their following bare word as a FIELD NAME, so a type-named
// field (opts dot table) is never capitalised into a different field.
func TestFormatDotKeyNotCapitalized(t *testing.T) {
	cases := []struct{ in, want string }{
		{"opts dot table\n", "opts dot table\n"},
		{"m dotr string\n", "m dotr string\n"},
		{"opts dot region\n", "opts dot region\n"}, // not a type name anyway
		{"x is integer\n", "x is Integer\n"},       // genuine type still capitalises
	}
	for _, c := range cases {
		if got := Format(c.in); got != c.want {
			t.Errorf("Format(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
