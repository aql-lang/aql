package modules

import "testing"

// TestGexSpecToRegexp pins the glob→regexp translation: the wildcards, the
// literal-escape digraphs, and the ^…$ anchoring.
func TestGexSpecToRegexp(t *testing.T) {
	cases := []struct{ spec, want string }{
		{"a*c", `^a[\s\S]*c$`}, // * → any run (incl. newlines)
		{"a?c", `^a[\s\S]c$`},  // ? → exactly one char
		{"a**b", `^a\*b$`},     // ** → a literal *
		{"a*?b", `^a\?b$`},     // *? → a literal ?
		{"a.c", `^a\.c$`},      // a regexp metachar is quoted, not special
		{"", `^$`},             // empty spec matches only the empty string
		{"*", `^[\s\S]*$`},     // lone wildcard
	}
	for _, c := range cases {
		if got := gexSpecToRegexp(c.spec); got != c.want {
			t.Errorf("gexSpecToRegexp(%q) = %q, want %q", c.spec, got, c.want)
		}
	}
}

// TestGexMatch exercises the compiled matcher: anchoring, the wildcards, the
// literal escapes, newline-crossing (the [\s\S] semantics, NOT line-oriented
// `.`), and multi-byte runes. Each row pairs a match with a non-match.
func TestGexMatch(t *testing.T) {
	cases := []struct {
		spec, subject string
		want          bool
	}{
		// anchored: the WHOLE subject must match, never a substring
		{"a*c", "abc", true},
		{"a*c", "ac", true},
		{"a*c", "xabc", false}, // leading text → no (anchored)
		{"a*c", "abcx", false}, // trailing text → no (anchored)

		// * and ? cross newlines ([\s\S], unlike a line-oriented `.`)
		{"a*c", "a\nb\nc", true},
		{"a?c", "a\nc", true}, // ? matches the newline as one char
		{"a?c", "ac", false},  // ? needs exactly one char
		{"a?c", "abbc", false},

		// literal-escape digraphs
		{"a**b", "a*b", true},
		{"a**b", "axb", false},
		{"a*?b", "a?b", true},
		{"a*?b", "axb", false},

		// multi-byte runes: a glob char and ? both span one whole rune
		{"café*", "café au lait", true},
		{"a?c", "aéc", true}, // ? matches the single rune é
		{"a?c", "a€c", true},
	}
	for _, c := range cases {
		re, err := gexCompile(c.spec)
		if err != nil {
			t.Fatalf("gexCompile(%q): %v", c.spec, err)
		}
		if got := re.MatchString(c.subject); got != c.want {
			t.Errorf("gex %q on %q = %v, want %v", c.spec, c.subject, got, c.want)
		}
	}
}

// TestGexCompileMemoized: the same source returns the same cached *Regexp.
func TestGexCompileMemoized(t *testing.T) {
	a, err := gexCompile("x*y")
	if err != nil {
		t.Fatal(err)
	}
	b, err := gexCompile("x*y")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("gexCompile did not memoize: distinct *Regexp for the same source")
	}
}
