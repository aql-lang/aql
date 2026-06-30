package lang

import (
	"strings"
	"testing"
)

// TestParseRegisterCheckResolvesInFnBody pins the template.aql check leaf: a parser
// registered with aql:parse's `Parse.register <kind> <built-grammar>` and then used
// by `parse <kind>` INSIDE a fn body must check WITHOUT a false parse_unknown_lang.
//
// parse-register's grammar (a Parse-builder result) is NOT a concrete value under
// static analysis, so the real parser can only install at run time. Its check-mode
// ReturnsFn marks the kind DEFERRED, and the `parse` macro then degrades `parse
// <kind>` to a dynamic value during analysis instead of raising parse_unknown_lang.
// A genuinely-unknown kind (none registered) must STILL error — the leniency is
// scoped to kinds a Parse.register call actually installs.
func TestParseRegisterCheckResolvesInFnBody(t *testing.T) {
	const resolves = `import "aql:parse"
import "aql:parselang"
def g Parse.grammar
Parse.abnf g "op = \"x\"" {start:'op'} end
Parse.register myk g end
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

	// NEGATIVE: a kind that is NEVER registered must still raise parse_unknown_lang
	// (the deferral is keyed on an actual Parse.register call).
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
