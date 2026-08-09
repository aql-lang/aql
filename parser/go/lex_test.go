package parser

import (
	"strings"
	"testing"
)

// LexTokens is the formatter-facing seam: the ONLY trivia-preserving view of
// the boru lexer. Its consumer (lang/go/formatter) lives in another module, so
// before the parser was cut out this function was covered incidentally from
// there and had no test of its own. Under the standalone gate that is exactly
// the coverage a module cannot claim — these tests pin it here, where the code
// lives.

// tokenNames flattens a token stream to its kind names, for shape assertions
// that do not care about the source text.
func tokenNames(toks []LexToken) []string {
	out := make([]string, len(toks))
	for i, tk := range toks {
		out[i] = tk.Name
	}
	return out
}

func hasName(toks []LexToken, name string) bool {
	for _, tk := range toks {
		if tk.Name == name {
			return true
		}
	}
	return false
}

// The whole point of LexTokens over Parse: trivia survives. A formatter needs
// the spaces, newlines and comments Parse throws away.
func TestLexTokensPreservesTrivia(t *testing.T) {
	src := "1 add 2\n# a comment\n3"
	toks, ok := LexTokens(src)
	if !ok {
		t.Fatalf("LexTokens(%q) reported a lex error on valid source", src)
	}
	if len(toks) == 0 {
		t.Fatal("LexTokens returned no tokens for non-empty source")
	}
	for _, want := range []string{"#SP", "#LN", "#CM"} {
		if !hasName(toks, want) {
			t.Errorf("trivia token %s missing from stream %v", want, tokenNames(toks))
		}
	}
}

// SI must be a real byte offset into src, because a formatter uses it to fall
// back to the ORIGINAL text for spans the flat lexer cannot reproduce. A
// stream whose offsets do not index src is useless for that.
func TestLexTokensOffsetsIndexTheSource(t *testing.T) {
	src := "'hello' 42 word"
	toks, ok := LexTokens(src)
	if !ok {
		t.Fatalf("LexTokens(%q) reported a lex error on valid source", src)
	}
	for _, tk := range toks {
		if tk.SI < 0 || tk.SI > len(src) {
			t.Fatalf("token %+v: SI out of range for a %d-byte source", tk, len(src))
		}
		// Every token's raw text must appear AT its recorded offset.
		if tk.Src != "" && !strings.HasPrefix(src[tk.SI:], tk.Src) {
			t.Errorf("token %+v: Src is not at SI (src[%d:] = %q)", tk, tk.SI, src[tk.SI:])
		}
	}
}

// The bool result is the contract a formatter front end branches on: false
// means the stream is truncated and untrustworthy, so it should fall back
// rather than emit a mangled result. An unterminated string is the canonical
// way to reach it.
func TestLexTokensReportsLexError(t *testing.T) {
	toks, ok := LexTokens("'unterminated")
	if ok {
		t.Errorf("LexTokens(unterminated string) = ok, want a reported lex error (got %v)",
			tokenNames(toks))
	}
}

// An unterminated BACKTICK does not trip the error flag, despite what
// LexTokens' doc comment used to claim: the backtick is an ordinary operator
// token at the lex level, and the template literal is assembled by a grammar
// rule the bare Next loop never runs. Pinned so the distinction is deliberate
// rather than a latent surprise for a formatter front end, which will see a
// clean stream here and must not assume ok==true implies a well-formed
// template.
func TestLexTokensUnterminatedBacktickIsNotALexError(t *testing.T) {
	toks, ok := LexTokens("`unterminated backtick")
	if !ok {
		t.Fatal("an unterminated backtick unexpectedly became a LEX error; " +
			"if that is now intended, fold it back into TestLexTokensReportsLexError")
	}
	if len(toks) == 0 {
		t.Fatal("expected the backtick to tokenise as ordinary tokens")
	}
}

// Empty input is not an error — it is an empty stream that scanned cleanly.
// Pinned because the loop's first Next() returning the end token is the only
// path that leaves `out` nil, and a formatter must not treat it as a failure.
func TestLexTokensEmptySourceScansClean(t *testing.T) {
	toks, ok := LexTokens("")
	if !ok {
		t.Error("LexTokens(\"\") reported a lex error on empty source")
	}
	if len(toks) != 0 {
		t.Errorf("LexTokens(\"\") = %v, want no tokens", tokenNames(toks))
	}
}

// The configured matchers (template / big-number / minilang / xml) are wired
// into the same lexer LexTokens drives, so their tokens must reach the stream
// rather than being dropped or splitting into character noise.
func TestLexTokensDrivesTheConfiguredMatchers(t *testing.T) {
	for name, src := range map[string]string{
		"big integer": "0d123",
		"xml literal": "<a/>",
		"template":    "`a ${1} b`",
	} {
		t.Run(name, func(t *testing.T) {
			toks, ok := LexTokens(src)
			if !ok {
				t.Fatalf("LexTokens(%q) reported a lex error", src)
			}
			if len(toks) == 0 {
				t.Fatalf("LexTokens(%q) produced no tokens", src)
			}
			// The stream must reconstruct the source: every byte of src is
			// accounted for by some token, which is what "preserving" means.
			var b strings.Builder
			for _, tk := range toks {
				b.WriteString(tk.Src)
			}
			if got := b.String(); got != src {
				t.Errorf("token texts concatenate to %q, want the original %q", got, src)
			}
		})
	}
}

func TestLexTokensNumericBoundaries(t *testing.T) {
	for _, src := range []string{"1.0e400", "1.e2", "0x10000000000000000"} {
		toks, ok := LexTokens(src)
		if !ok || len(toks) != 1 || toks[0].Name != "#NR" || toks[0].Src != src {
			t.Errorf("LexTokens(%q) = %v, ok=%v; want one #NR token", src, toks, ok)
		}
	}
	toks, ok := LexTokens("1.x")
	if !ok || len(toks) != 3 || strings.Join(tokenNames(toks), " ") != "#NR #DT #TX" {
		t.Errorf("LexTokens(1.x) = %v, ok=%v; want #NR #DT #TX", toks, ok)
	}
}
