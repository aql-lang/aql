package parser

import (
	jsonic "github.com/tabnas/jsonic/go"
)

// LexToken is one lexical token from the boru-configured tabnas lexer: its
// kind name (e.g. "#TX", "#OP", "#CM"), its raw source text, and SI — the
// byte offset in src where the token starts. It is the trivia-preserving
// token stream a formatter front end consumes. SI lets a front end fall
// back to the ORIGINAL source for spans the flat lexer cannot reproduce
// (template literals need grammar-rule context the bare Next loop lacks).
type LexToken struct {
	Name string
	Src  string
	SI   int
}

// LexTokens tokenises src with the boru-configured tabnas lexer, PRESERVING
// trivia (spaces, newlines, comments). The parser's semantic Parse discards
// those; a formatter needs them, so this drives the same configured lexer
// (base tokens + the template / big-number / minilang / xml matchers) with an
// EMPTY ignore set so `Next` returns #SP / #LN / #CM verbatim.
//
// This is the tabnas half of the "parse using a tabnas parser" seam: a
// formatter front end maps this stream to its node tree. The bool result is
// false when the lexer hit an error (an unterminated string / backtick, a bad
// token) before end-of-input: on a malformed source the token stream is
// truncated and untrustworthy, so a formatter front end should fall back to a
// best-effort path rather than emit a mangled (or empty) result.
func LexTokens(src string) ([]LexToken, bool) {
	j := SafeMake(jsonic.Options{})
	t := setupBaseTokens(j)
	setupTemplateLiteralMatcher(j, t)
	setupBigNumberMatcher(j, t)
	setupMiniLitMatcher(j, t)
	setupXmlMatcher(j, t)

	cfg := j.Config()
	cfg.IgnoreSet = map[jsonic.Tin]bool{} // keep SP / LN / CM
	lex := jsonic.NewLex(src, cfg)

	var out []LexToken
	for {
		tok := lex.Next()
		if tok == nil || tok.Tin == jsonic.TinZZ {
			break
		}
		out = append(out, LexToken{Name: tok.Name, Src: tok.Src, SI: tok.SI})
	}
	// The lexer records the first error in lex.Err and returns an early end
	// token; a clean scan leaves it nil.
	return out, lex.Err == nil
}
