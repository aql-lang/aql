package native

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// strings.ToLower can change a string's byte length (Turkish İ U+0130
// lowercases to the two-rune "i̇"). The case-insensitive string ops used
// to find a match index in a lowercased copy and then slice the ORIGINAL
// string with it, cutting through a multi-byte rune and producing
// invalid UTF-8. These tests pin offset-safe behaviour: every result
// must be valid UTF-8 and ASCII folding must be unchanged.

func TestFoldIndexOffsetSafe(t *testing.T) {
	tests := []struct {
		name      string
		s, substr string
		from      int
		wantStart int
	}{
		{"ascii hit", "hello", "LL", 0, 2},
		{"ascii miss", "hello", "zz", 0, -1},
		{"from offset", "ababab", "AB", 1, 2},
		{"turkish dotted I matches i", "İx", "i", 0, 0},
		{"empty at from", "abc", "", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, span := foldIndex(tt.s, tt.substr, tt.from)
			if start != tt.wantStart {
				t.Errorf("foldIndex(%q,%q,%d) start = %d, want %d", tt.s, tt.substr, tt.from, start, tt.wantStart)
			}
			if start >= 0 {
				// The reported span must land on a rune boundary.
				slice := tt.s[start : start+span]
				if !utf8.ValidString(slice) {
					t.Errorf("foldIndex span sliced invalid UTF-8: %q", slice)
				}
			}
		})
	}
}

func TestReplaceInsensitiveValidUTF8(t *testing.T) {
	tests := []struct {
		name              string
		input, search, rp string
		all               bool
		want              string
	}{
		{"ascii first", "heLLo", "l", "_", false, "he_Lo"},
		{"ascii all", "heLLo", "l", "_", true, "he__o"},
		{"dotted I first", "İx", "x", "Y", false, "İY"},
		{"dotted I as i", "İstanbul", "i", "Y", true, "Ystanbul"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			if tt.all {
				got = replaceAllInsensitive(tt.input, tt.search, tt.rp, 0, -1)
			} else {
				got = replaceFirstInsensitive(tt.input, tt.search, tt.rp, 0)
			}
			if !utf8.ValidString(got) {
				t.Errorf("result is not valid UTF-8: %q", got)
			}
			if got != tt.want {
				t.Errorf("replace(%q,%q,%q,all=%v) = %q, want %q", tt.input, tt.search, tt.rp, tt.all, got, tt.want)
			}
		})
	}
}

func TestSplitInsensitiveValidUTF8(t *testing.T) {
	parts := splitInsensitive("aXbXc", "x")
	if strings.Join(parts, "|") != "a|b|c" {
		t.Errorf("splitInsensitive ascii = %q, want a|b|c", parts)
	}
	// İ splits as a case-insensitive "i" without corrupting neighbours.
	parts = splitInsensitive("aİb", "i")
	for _, p := range parts {
		if !utf8.ValidString(p) {
			t.Errorf("split produced invalid UTF-8 part: %q", p)
		}
	}
}

func TestFindLiteralMatchesInsensitiveValidUTF8(t *testing.T) {
	matches := findLiteralMatches("İstanbul", "i", true, true)
	for _, m := range matches {
		if !utf8.ValidString(m.m) {
			t.Errorf("match captured invalid UTF-8: %q", m.m)
		}
	}
	// ASCII find-all unchanged: two matches of "l" in "hello".
	matches = findLiteralMatches("hello", "l", false, true)
	if len(matches) != 2 {
		t.Errorf("findLiteralMatches(hello,l,all) = %d matches, want 2", len(matches))
	}
}
