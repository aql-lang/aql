package formatter

import (
	"strings"

	"github.com/aql-lang/aql/eng/go/parser"
)

// TabnasParse is the tabnas-lexer front end for the formatter: it tokenises
// with the AQL-configured tabnas lexer (parser.LexTokens) and builds the same
// Node tree the built-in hand-lexer produces. It is the concrete realisation
// of the Q2 requirement "Fmt should parse, using a tabnas parser (which is a
// parameter to the formatting)" — inject it via FormatWith(src, TabnasParse).
//
// The tabnas lexer emits FINE tokens (`foo` `.` `bar`, `<`, `>`, `=>` each
// separately) while the formatter's node model wants COARSE words (`foo.bar`,
// `a<b>`, `=>` as one word — the hand-lexer's `isDelimiter` only breaks on
// []{}(),;:?!# and whitespace). tabnasTokenize bridges that by coalescing a
// run of adjacent word-part tokens into one word, so the resulting []Token —
// and hence the layout — matches the hand-lexer. It is wired as DefaultParse;
// TestTabnasParseCorpus pins its byte-for-byte equivalence to HandParse across
// the whole .aql corpus and TestTabnasParseMatchesHandLexer over an edge
// battery. On MALFORMED source (where the tabnas lexer errors and truncates)
// it falls back to HandParse so a formatter never destroys broken code.
func TabnasParse(src string) *Node {
	lts, ok := parser.LexTokens(src)
	if !ok {
		// Malformed source (unterminated string / backtick, bad token): the
		// tabnas token stream is truncated and would erase or mangle the
		// input. Fall back to the hand-lexer, which is best-effort and never
		// destroys a file — a formatter must survive broken code intact.
		return HandParse(src)
	}
	return buildTree(tabnasTokenize(src, lts))
}

// isWordPart reports whether a tabnas token kind contributes to a coarse
// hand-lexer word (a maximal run of non-delimiter characters). Numbers are
// included because a number adjacent to a word is part of that word
// (`foo.5`); a number STANDING ALONE is re-tagged as TokNumber on flush.
// Value literals (#VL — `true`/`false`/`null`) are word content too: the
// hand-lexer has no value token, they are plain words to it, and a value
// glued to surrounding word chars (`a<b>false`) stays one word.
func isWordPart(name string) bool {
	switch name {
	case "#TX", "#DT", "#LA", "#RA", "#AR", "#ML", "#NR", "#BD", "#XML", "#VL":
		return true
	}
	return false
}

func tabnasTokenize(src string, lts []parser.LexToken) []Token {
	var out []Token

	// btEnd marks the byte offset just past the current verbatim template
	// literal: any tabnas token whose SI falls before it was mis-lexed from
	// inside the backtick (`:` split off, `//…` swallowed as a comment) and
	// must be dropped — the literal was already reconstructed from source.
	btEnd := 0

	// word accumulates a contiguous run of word-part tokens; flush emits it as
	// one TokWord (or TokNumber when it is a lone number).
	var word strings.Builder
	var wordKinds []string
	flush := func() {
		if word.Len() == 0 {
			return
		}
		text := word.String()
		if len(wordKinds) == 1 && (wordKinds[0] == "#NR" || wordKinds[0] == "#BD") {
			// The tabnas number matcher swallows a trailing `.` into the
			// literal (`1.`), but the hand-lexer only keeps a `.` in a number
			// when a DIGIT follows (`1.5`); a bare trailing dot is a separate
			// dot operator (`1. x` → `1` `.` `x`). Split it back so the two
			// front ends agree. (A `.` glued to a following word-part never
			// reaches here alone — it coalesces into a multi-kind word.)
			if len(text) > 1 && strings.HasSuffix(text, ".") {
				out = append(out, Token{TokNumber, text[:len(text)-1]})
				out = append(out, Token{TokDot, "."})
			} else {
				out = append(out, Token{TokNumber, text})
			}
		} else {
			out = append(out, Token{TokWord, text})
		}
		word.Reset()
		wordKinds = wordKinds[:0]
	}

	for i := 0; i < len(lts); i++ {
		lt := lts[i]
		// Drop tokens the lexer produced from inside a template literal we
		// already reconstructed verbatim from source.
		if lt.SI < btEnd {
			continue
		}
		// First token past a verbatim backtick: a `//` or `#` INSIDE the
		// literal makes the flat lexer swallow the rest of the physical line
		// into a comment, so real trailing tokens (a closing `)`, a following
		// string) never reach the stream. Recover that gap by re-scanning the
		// swallowed source `[btEnd, lt.SI)` with the correct AQL tokenizer.
		if btEnd > 0 {
			if gap := src[btEnd:lt.SI]; gap != "" {
				flush()
				toks := tokenize(gap)
				// A quote INSIDE an earlier backtick literal makes the flat
				// lexer's string matcher run to a quote inside a LATER literal,
				// swallowing that literal's opening backtick — so lt.SI (the
				// stray closing backtick that survived) falls in the MIDDLE of
				// the later literal, and the re-scanned gap ends with an
				// unterminated backtick. Emit the clean prefix, then the WHOLE
				// literal re-scanned from source, and resync btEnd past it so
				// the stray closing-backtick flat token is dropped.
				if k := len(toks) - 1; k >= 0 && toks[k].Kind == TokString &&
					strings.HasPrefix(toks[k].Text, "`") && !backtickClosed(toks[k].Text) {
					out = append(out, toks[:k]...)
					p := len(gap) - len(toks[k].Text)
					lit, n := scanBacktick(src[btEnd+p:])
					out = append(out, Token{TokString, lit})
					btEnd = btEnd + p + n
					continue
				}
				out = append(out, toks...)
			}
			btEnd = 0
		}
		// A backtick opens a template literal. The flat Next loop lacks the
		// grammar-rule context that drives verbatim #TL lexing, so it mangles
		// the interior; scan it straight from the original source with the
		// SAME scanner the hand-lexer uses, guaranteeing identical output.
		if lt.Name == "#BT" {
			flush()
			s, n := scanBacktick(src[lt.SI:])
			out = append(out, Token{TokString, s})
			btEnd = lt.SI + n
			continue
		}
		// A `.` is part of a word only when ADJACENT to a preceding word char
		// (`foo.bar`); a standalone or leading `.` (` . `, `.foo`) is a TokDot,
		// exactly as the hand-lexer splits it.
		if lt.Name == "#DT" && word.Len() == 0 {
			out = append(out, Token{TokDot, "."})
			continue
		}
		// A quoted string glues into a word when it sits MID-WORD — there was
		// no space to flush the word, so the `"` is not token-initial. The
		// hand-lexer's word scanner treats a non-initial `"` as an ordinary
		// word char, so `x="1"/>` is ONE word (XML attributes). A token-
		// initial string (word.Len()==0) stays its own TokString below.
		if lt.Name == "#ST" && word.Len() > 0 {
			word.WriteString(lt.Src)
			wordKinds = append(wordKinds, lt.Name)
			continue
		}
		if isWordPart(lt.Name) {
			word.WriteString(lt.Src)
			wordKinds = append(wordKinds, lt.Name)
			continue
		}
		flush()
		switch lt.Name {
		case "#SP":
			// whitespace between words separates them; already flushed.
		case "#LN":
			// tabnas COALESCES a run of consecutive blank lines into ONE #LN
			// token (Src == "\n\n\n"); the hand-lexer emits one TokNewline per
			// `\n`. Re-split so blank lines survive identically — count the
			// `\n` bytes (a `\r\n` contributes one), flooring at one so a lone
			// `\r` still yields a break.
			n := strings.Count(lt.Src, "\n")
			if n == 0 {
				n = 1
			}
			for k := 0; k < n; k++ {
				out = append(out, Token{TokNewline, "\n"})
			}
		case "#CM":
			out = append(out, Token{TokComment, lt.Src})
		case "#ST":
			out = append(out, Token{TokString, lt.Src})
		case "#OS":
			out = append(out, Token{TokLBracket, "["})
		case "#CS":
			out = append(out, Token{TokRBracket, "]"})
		case "#OB":
			out = append(out, Token{TokLBrace, "{"})
		case "#CB":
			out = append(out, Token{TokRBrace, "}"})
		case "#OP":
			out = append(out, Token{TokLParen, "("})
		case "#CP":
			out = append(out, Token{TokRParen, ")"})
		case "#CA":
			out = append(out, Token{TokComma, ","})
		case "#CL":
			out = append(out, Token{TokColon, ":"})
		case "#SC":
			out = append(out, Token{TokSemicolon, ";"})
		case "#QM":
			out = append(out, Token{TokQuestion, "?"})
		case "#BG":
			out = append(out, Token{TokBang, "!"})
		case "#PI":
			out = append(out, Token{TokPipe, "|"})
		default: //covergate:allow formatter tabnas front end: unknown tabnas token kinds fall through as words (§formatter)
			out = append(out, Token{TokWord, lt.Src})
		}
	}
	// A backtick whose swallowed tail ran to end-of-source (a `//` inside a
	// literal on the final line, no trailing newline) leaves no token past
	// btEnd to trigger in-loop recovery; sweep it up here.
	if btEnd > 0 && btEnd < len(src) {
		flush()
		out = append(out, tokenize(src[btEnd:])...)
	}
	flush()
	return out
}
