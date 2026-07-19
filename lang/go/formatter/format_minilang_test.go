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
