package eng

import "testing"

// ADR-008 coverage for bodyConstructsFn's payload branches: a literal
// FnDefInfo token in the body, and a nested list whose elements construct
// a fn (the recursive ListPayload hit). The word-token branches are
// exercised by the compiled-coverage corpus (the aql:repl refusal tier).
func TestBodyConstructsFnPayloadBranches(t *testing.T) {
	fnVal := NewFunction(FnDefInfo{Name: "inline"})

	cases := []struct {
		name string
		toks []Value
		want bool
	}{
		{"literal fn value token", []Value{fnVal}, true},
		{"nested list with afn word", []Value{NewList([]Value{NewWord("afn")})}, true},
		{"nested list without constructors", []Value{NewList([]Value{NewInteger(1)})}, false},
		{"paren expr with fn word", []Value{NewParenExpr([]Value{NewWord("fn")})}, true},
		{"plain words and scalars", []Value{NewWord("add"), NewInteger(2)}, false},
	}
	for _, c := range cases {
		if got := bodyConstructsFn(c.toks); got != c.want {
			t.Errorf("%s: bodyConstructsFn = %v, want %v", c.name, got, c.want)
		}
	}
}
