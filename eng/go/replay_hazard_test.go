package eng

import "testing"

// bodyHasReplayHazard — the bake-decision screen that fixed the do-unit
// registry-replay miscompile. Every arm pinned: the two hazard families
// (capitalised def/var/undef, import), the sound negatives (value defs,
// non-list values), and the nested-container recursion.
func TestBodyHasReplayHazard(t *testing.T) {
	w := func(name string) Value { return NewWord(name) }
	lst := func(elems ...Value) Value { return NewList(elems) }
	paren := func(toks ...Value) Value { return NewParenExpr(toks) }

	cases := []struct {
		name string
		v    Value
		want bool
	}{
		{"capitalised def", lst(w("def"), w("Big"), NewTypeLiteral(TInteger)), true},
		{"capitalised var", lst(w("var"), w("Big"), NewInteger(1)), true},
		{"capitalised undef", lst(w("undef"), w("Big")), true},
		{"import", lst(w("import"), NewString("boru:minilang")), true},
		{"value def is sound", lst(w("def"), w("b"), NewInteger(5)), false},
		{"value undef is sound", lst(w("undef"), w("b")), false},
		{"def at tail without a name", lst(NewInteger(1), w("def")), false},
		{"quoted-atom capitalised name", lst(w("def"), NewAtom("Big"), NewTypeLiteral(TInteger)), true},
		{"computed name is not static", lst(w("def"), paren(w("mkname")), NewInteger(1)), false},
		{"nested list hazard", lst(w("if"), w("b"), lst(w("def"), w("Big"), NewTypeLiteral(TInteger)), lst()), true},
		{"nested paren hazard", lst(paren(w("import"), NewString("boru:io"))), true},
		{"plain body", lst(NewInteger(1), w("add"), NewInteger(2)), false},
		{"non-container value", NewInteger(7), false},
		{"paren body import", paren(w("import"), NewString("boru:io")), true},
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
	sig := &Signature{NoEvalArgs: map[int]bool{0: true}}
	body := NewList([]Value{NewWord("def"), NewWord("Big"), NewTypeLiteral(TInteger)})
	if noEvalBodiesInert(sig, []Value{body}) {
		t.Fatal("a capitalised-def body must not be inert")
	}
	es := NewEmitState()
	if es.noEvalBodiesInertScoped(sig, []Value{body}) {
		t.Fatal("a capitalised-def body must not be scoped-inert")
	}
	sound := NewList([]Value{NewWord("def"), NewWord("b"), NewInteger(5)})
	if !noEvalBodiesInert(sig, []Value{sound}) {
		t.Fatal("a value-def body must stay inert")
	}
	if !es.noEvalBodiesInertScoped(sig, []Value{sound}) {
		t.Fatal("a value-def body must stay scoped-inert")
	}
}

// bindNameToken — the static-name extraction feeding the capitalised check.
func TestBindNameToken(t *testing.T) {
	if got := bindNameToken(NewWord("Big")); got != "Big" {
		t.Errorf("word: %q", got)
	}
	if got := bindNameToken(NewAtom("Pos")); got != "Pos" {
		t.Errorf("atom: %q", got)
	}
	if got := bindNameToken(NewInteger(3)); got != "" {
		t.Errorf("non-name: %q", got)
	}
}
