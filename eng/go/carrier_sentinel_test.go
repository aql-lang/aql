package eng

import "testing"

func TestValueHasSentinelCoverage(t *testing.T) {
	brk := NewWord("break")
	quotedBrk := brk
	quotedBrk.Quoted = true
	cases := []struct {
		name string
		v    Value
		want bool
	}{
		{"bare break word", brk, true},
		{"continue word", NewWord("continue"), true},
		{"return word", NewWord("return"), true},
		{"non-sentinel word", NewWord("print"), false},
		{"quoted break still detected", quotedBrk, true},
		{"list with break", NewList([]Value{NewInteger(1), brk, NewInteger(2)}), true},
		{"list without break", NewList([]Value{NewInteger(1), NewWord("print")}), false},
		{"list with quoted break", NewList([]Value{quotedBrk}), true},
		{"paren-expr with break", NewParenExpr([]Value{brk}), true},
		{"paren-expr without break", NewParenExpr([]Value{NewWord("print")}), false},
		{"integer (not a body word)", NewInteger(7), false},
		{"fn value skipped (own scope)", NewFunction(FnDefInfo{Name: "f", Signatures: []Signature{{Impl: Boru([]Value{brk})}}}), false},
		{"list containing a fn value", NewList([]Value{NewFunction(FnDefInfo{Signatures: []Signature{{Impl: Boru([]Value{brk})}}})}), false},
	}
	for _, c := range cases {
		if got := bodyHasSentinel(c.v); got != c.want {
			t.Errorf("%s: bodyHasSentinel = %v, want %v", c.name, got, c.want)
		}
	}
}
