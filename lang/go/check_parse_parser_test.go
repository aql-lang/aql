package lang

import (
	"strings"
	"testing"
)

// TestParseParserCheckResolvesInFnBody pins the template.aql check leaf: a
// parser finalized with `def myk (Parse.parser g)` and then used by
// `parse myk` INSIDE a fn body must check WITHOUT a false parse_unknown_lang.
//
// Parse.parser's grammar is NOT a concrete value under static analysis, so
// the binding is an abstract Function carrier; the `parse` macro's
// bound-name carrier branch degrades the call to a dynamic value during
// analysis instead of raising parse_unknown_lang. A genuinely-unknown name
// (no binding, no kind) must STILL error — the leniency is scoped to
// Function-family bindings.
func TestParseParserCheckResolvesInFnBody(t *testing.T) {
	const resolves = `import "aql:parse"
import "aql:parselang"
def g Parse.grammar
Parse.abnf g "op = \"x\"" {start:'op'} end
def myk (Parse.parser g) end
def lex fn [ [s:String] [List] [ def _ (parse myk s) [1] ] ]
(lex "x") print`

	a, _ := New()
	res, _ := a.Check(resolves)
	for _, d := range res.Diagnostics {
		if strings.Contains(d.Code, "parse_unknown_lang") ||
			(strings.Contains(d.Detail, "no parser") && strings.Contains(d.Detail, "myk")) {
			t.Errorf("runtime-registered kind myk must resolve in check; got %s: %s", d.Code, d.Detail)
		}
	}

	// NEGATIVE: a name that is neither a kind nor a binding must still raise
	// parse_unknown_lang (the leniency is keyed on an actual Function binding).
	const unknown = `import "aql:parse"
import "aql:parselang"
def lex fn [ [s:String] [List] [ def _ (parse nosuchkind s) [1] ] ]
(lex "x") print`

	b, _ := New()
	res2, _ := b.Check(unknown)
	found := false
	for _, d := range res2.Diagnostics {
		if strings.Contains(d.Code, "parse_unknown_lang") ||
			strings.Contains(d.Detail, "no parser") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("a genuinely-unknown parser kind must still raise parse_unknown_lang")
	}
}
