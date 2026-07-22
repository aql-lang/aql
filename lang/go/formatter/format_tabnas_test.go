package formatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTabnasParseMatchesHandLexer pins the Phase-4 tabnas front end: for every
// construct AQL has, FormatWith(src, TabnasParse) must produce byte-identical
// output to FormatWith(src, HandParse). TabnasParse drives the AQL-configured
// tabnas lexer (parser.LexTokens) and coalesces its FINE tokens back into the
// COARSE words the node model wants; this table is the differential contract
// that the two front ends agree. Cases are chosen to exercise every branch of
// tabnasTokenize (each token kind, the standalone-dot split, the newline
// re-split, verbatim-backtick scanning, and swallowed-tail recovery).
func TestTabnasParseMatchesHandLexer(t *testing.T) {
	cases := []string{
		// words, dotted access, standalone dot
		"foo.bar baz",
		"m . k",
		"a.b.c",
		// numbers (lone → TokNumber) and number-adjacent words
		"1 2.5 -3 0xff",
		"foo.5",
		// a number's trailing `.` with no following digit is a dot operator,
		// not part of the literal (the tabnas matcher swallows it into `1.`)
		"1. x\n",
		"1 .5\n",
		"1.\n",
		// XML literals with quoted attributes: the `\"1\"` is a MID-WORD string
		// that must glue into the surrounding word, not split out
		`<a x="1"/>`,
		`xml-attr 'x' <a x="1"/>`,
		`x="1"/>`,
		`"1"/>`,
		// malformed source must round-trip through the hand-lexer fallback,
		// never be erased or mangled
		`"unterminated`,
		"`unterminated",
		"ok then `bad//`\nmore",
		// strings and escapes
		`"hi" 'there' "a\"b"`,
		// every bracket / punct kind
		"[1 2 3]",
		"{a:1 b:2}",
		"(add 1 2)",
		"a , b ; c",
		"x ? y ! z | w",
		// pipes as barriers in a fn sig
		"fn [[a:Integer | b:String] [Any] [a]]",
		// comments (both # and //) and blank-line runs
		"# header\n\n\ncode here\n",
		"1 // trailing\n2\n",
		// a comment with NO trailing newline (runs to EOF) — exercises the
		// hand-lexer's IndexByte-miss branch through HandParse too
		"# tail comment",
		"code ## trailing to eof",
		// consecutive newlines / CRLF / lone CR
		"a\n\n\nb",
		"a\r\nb",
		"a\rb",
		// minilang literals scanned verbatim
		"+re/[a-z]+/ x",
		"xs filter +gex|*.go|",
		// clean backtick (empty swallowed gap)
		"`hello ${name}` done",
		// a backtick literal whose ENTIRE content is a quote char (`"` / `'`):
		// the flat lexer starts a string at the inner quote that runs to a quote
		// inside a LATER literal, swallowing that literal's opening backtick. The
		// gap recovery must re-scan the split literal whole, not leave it
		// unterminated (sift.aql's char-quote check; regression for PR #298).
		"a eq `\"`",
		"(a eq `\"`) or (b eq `\"`)",
		"if (((a eq `\"`) and (b eq `\"`)) or ((a eq `'`) and (b eq `'`))) [x] [y]",
		"x `'` y `'` z",
		// backtick containing `//` → mis-lexer swallows the line tail; the
		// closing paren after it must be recovered
		"def u ( `http://h/${p}/x` )\n",
		// backtick with `//`, swallowed tail runs to EOF (trailing recovery)
		"call `a//b`)",
		// a realistic multi-line fn def
		"def f fn [[x:Integer] [Integer] [\n  x mul 2\n]]\n",
		// value literals (#VL) are word content: standalone, after a colon,
		// and glued mid-word
		"true false null",
		"{a:false b:true}",
		"a<b>false",
		// lambda arrow
		"x:Integer => [x add 1]",
		// empty and whitespace-only
		"",
		"   \n  \n",
	}
	for _, src := range cases {
		hand := FormatWith(src, HandParse)
		tab := FormatWith(src, TabnasParse)
		if hand != tab {
			t.Errorf("TabnasParse diverges from hand-lexer for %q:\n  hand: %q\n  tab:  %q", src, hand, tab)
		}
	}
}

// TestTabnasParseAbsolute guards against a shared-bug false pass: a few cases
// are pinned to their EXACT expected output, so the differential test above
// can't be satisfied by both front ends being wrong the same way.
func TestTabnasParseAbsolute(t *testing.T) {
	cases := []struct{ src, want string }{
		// blank line between the header comment and the imports survives.
		{"# hdr\n\nimport \"aql:net\"\n", "# hdr\n\nimport \"aql:net\"\n"},
		// a URL with `://` inside a backtick is emitted verbatim, and the
		// closing paren is not lost.
		{"def u ( `http://h/x` )\n", "def u (`http://h/x`)\n"},
		// standalone dot stays a dot operator, not glued into a word.
		{"m . k", "m.k\n"},
		// XML attribute string stays glued (verbatim), no injected spaces.
		{`<a x="1"/>`, "<a x=\"1\"/>\n"},
		// number with a bare trailing dot splits into number + dot operator.
		{"1. x\n", "1.x\n"},
		// an unterminated string is preserved verbatim (fallback), not erased.
		{`"abc`, "\"abc\n"},
	}
	for _, c := range cases {
		if got := FormatWith(c.src, TabnasParse); got != c.want {
			t.Errorf("FormatWith(%q, TabnasParse) = %q, want %q", c.src, got, c.want)
		}
	}
}

// TestTabnasParseCorpus is the whole-repo differential: every checked-in .aql
// file must format identically through both front ends. It is skipped when the
// corpus is not reachable from the test's working directory (e.g. an isolated
// package checkout) so it never fails spuriously.
func TestTabnasParseCorpus(t *testing.T) {
	root := "../../.."
	if _, err := os.Stat(filepath.Join(root, "go.work")); err != nil {
		if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
			t.Skip("repo root not reachable; skipping corpus differential")
		}
	}
	var files []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".aql") {
			files = append(files, p)
		}
		return nil
	})
	if len(files) == 0 {
		t.Skip("no .aql corpus found")
	}
	checked := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		src := string(b)
		if hand, tab := FormatWith(src, HandParse), FormatWith(src, TabnasParse); hand != tab {
			t.Errorf("%s: TabnasParse diverges from hand-lexer", f)
		}
		checked++
	}
	t.Logf("tabnas/hand differential clean across %d .aql files", checked)
}
