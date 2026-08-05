package parser

// The declarative-grammar loader's contract arms — the Go twin of
// src/parser/declgrammar.test.ts: the embedded artifact loads and
// validates, and every unknown-name path (edit op, token, action hook,
// condition hook) fails parser construction loudly.

import (
	"strings"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"
)

func TestDeclGrammarLoads(t *testing.T) {
	g := loadDeclGrammar()
	if g.Schema != 1 {
		t.Fatalf("schema = %d, want 1", g.Schema)
	}
	if len(g.Tokens) < 15 || len(g.Edits) < 2 {
		t.Fatalf("artifact too small: %d tokens, %d edits", len(g.Tokens), len(g.Edits))
	}
}

func declPanics(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("want panic containing %q, got none", want)
		}
		msg, ok := rec.(string)
		if !ok || !strings.Contains(msg, want) {
			t.Fatalf("panic %v does not contain %q", rec, want)
		}
	}()
	fn()
}

func TestDeclGrammarUnknownNames(t *testing.T) {
	j := SafeMake(jsonic.Options{})
	tins := map[string]jsonic.Tin{}
	empty := declHooks{}

	declPanics(t, "unknown edit op", func() {
		applyDeclEdits(j, declGrammar{Schema: 1, Edits: []declEdit{{Rule: "val", Op: "sideways"}}}, tins, empty)
	})
	declPanics(t, "unknown token", func() {
		applyDeclEdits(j, declGrammar{Schema: 1, Edits: []declEdit{{
			Rule: "val", Op: "setOpen", Alts: []declAlt{{S: [][]string{{"ZZTOP"}}}},
		}}}, tins, empty)
	})
	declPanics(t, "unknown action hook", func() {
		applyDeclEdits(j, declGrammar{Schema: 1, Edits: []declEdit{{
			Rule: "val", Op: "setOpen", Alts: []declAlt{{A: "ghost"}},
		}}}, tins, empty)
	})
	declPanics(t, "unknown condition hook", func() {
		applyDeclEdits(j, declGrammar{Schema: 1, Edits: []declEdit{{
			Rule: "val", Op: "setOpen", Alts: []declAlt{{C: "ghost"}},
		}}}, tins, empty)
	})
}
