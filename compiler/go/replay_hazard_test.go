package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// bodyHasReplayHazard — the bake-decision screen that fixed the do-unit
// registry-replay miscompile. Every arm pinned: the two hazard families
// (capitalised def/var/undef, import), the sound negatives (value defs,
// non-list values), and the nested-container recursion.
func TestBodyHasReplayHazard(t *testing.T) {
	w := func(name string) core.Value { return core.NewWord(name) }
	lst := func(elems ...core.Value) core.Value { return core.NewList(elems) }
	paren := func(toks ...core.Value) core.Value { return core.NewParenExpr(toks) }

	cases := []struct {
		name string
		v    core.Value
		want bool
	}{
		{"capitalised def", lst(w("def"), w("Big"), core.NewTypeLiteral(core.TInteger)), true},
		{"capitalised var", lst(w("var"), w("Big"), core.NewInteger(1)), true},
		{"capitalised undef", lst(w("undef"), w("Big")), true},
		{"import", lst(w("import"), core.NewString("boru:minilang")), true},
		{"value def is sound", lst(w("def"), w("b"), core.NewInteger(5)), false},
		{"value undef is sound", lst(w("undef"), w("b")), false},
		{"def at tail without a name", lst(core.NewInteger(1), w("def")), false},
		{"quoted-atom capitalised name", lst(w("def"), core.NewAtom("Big"), core.NewTypeLiteral(core.TInteger)), true},
		{"computed name is not static", lst(w("def"), paren(w("mkname")), core.NewInteger(1)), false},
		{"nested list hazard", lst(w("if"), w("b"), lst(w("def"), w("Big"), core.NewTypeLiteral(core.TInteger)), lst()), true},
		{"nested paren hazard", lst(paren(w("import"), core.NewString("boru:io"))), true},
		{"plain body", lst(core.NewInteger(1), w("add"), core.NewInteger(2)), false},
		{"non-container value", core.NewInteger(7), false},
		{"paren body import", paren(w("import"), core.NewString("boru:io")), true},
	}
	for _, c := range cases {
		if got := bodyHasReplayHazard(c.v); got != c.want {
			t.Errorf("%s: bodyHasReplayHazard = %v, want %v", c.name, got, c.want)
		}
	}
}

// Both bake gates decline a hazard-bearing body that is otherwise inert —
// the exact admission the miscompile rode in on (the body of
// `do [def Big Integer 15 is Big]` is a plain word-list that passes
// isInertConst).
func TestNoEvalBodiesInertReplayHazard(t *testing.T) {
	sig := &core.Signature{NoEvalArgs: map[int]bool{0: true}}
	body := core.NewList([]core.Value{core.NewWord("def"), core.NewWord("Big"), core.NewTypeLiteral(core.TInteger)})
	if noEvalBodiesInert(sig, []core.Value{body}) {
		t.Fatal("a capitalised-def body must not be inert")
	}
	es := NewEmitState()
	if es.noEvalBodiesInertScoped(sig, []core.Value{body}) {
		t.Fatal("a capitalised-def body must not be scoped-inert")
	}
	sound := core.NewList([]core.Value{core.NewWord("def"), core.NewWord("b"), core.NewInteger(5)})
	if !noEvalBodiesInert(sig, []core.Value{sound}) {
		t.Fatal("a value-def body must stay inert")
	}
	if !es.noEvalBodiesInertScoped(sig, []core.Value{sound}) {
		t.Fatal("a value-def body must stay scoped-inert")
	}
}

// bindNameToken — the static-name extraction feeding the capitalised check.
func TestBindNameToken(t *testing.T) {
	if got := bindNameToken(core.NewWord("Big")); got != "Big" {
		t.Errorf("word: %q", got)
	}
	if got := bindNameToken(core.NewAtom("Pos")); got != "Pos" {
		t.Errorf("atom: %q", got)
	}
	if got := bindNameToken(core.NewInteger(3)); got != "" {
		t.Errorf("non-name: %q", got)
	}
}
