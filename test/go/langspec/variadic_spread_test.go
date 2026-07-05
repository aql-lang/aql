package langspec

import (
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// TestVariadicSpreadOracle pins the soundness-oracle contract for a
// variadic-spread bottom carrier (the recursion.tsv:53 void-recursion residual):
// it absorbs ANY number of trailing runtime entries (count flexibility) but each
// must still be a member of the element type (type strictness) — a wrong-typed
// leak is still flagged. Pairs the positive (Integer spread covers N Integers)
// with the negatives (a String among the Integers, and a wrong fixed prefix).
func TestVariadicSpreadOracle(t *testing.T) {
	varInt := eng.NewVariadicCarrier(eng.NewTypeLiteral(eng.TInteger))
	i := eng.NewInteger(6)
	s := eng.NewString("x")
	strCarrier := eng.NewCarrier(eng.TString)

	cases := []struct {
		name    string
		checked []eng.Value
		actual  []eng.Value
		want    bool
	}{
		{"three integers covered", []eng.Value{varInt}, []eng.Value{i, i, i}, true},
		{"zero trailing covered", []eng.Value{varInt}, nil, true},
		{"string leak rejected", []eng.Value{varInt}, []eng.Value{i, s, i}, false},
		{"fixed prefix + integer spread", []eng.Value{varInt, strCarrier}, []eng.Value{i, i, s}, true},
		{"fixed prefix wrong type", []eng.Value{varInt, strCarrier}, []eng.Value{i, i, i}, false},
		{"missing fixed prefix", []eng.Value{varInt, strCarrier}, nil, false},
	}
	for _, c := range cases {
		if got := stackTypeCovered(c.checked, c.actual); got != c.want {
			t.Errorf("%s: stackTypeCovered = %v, want %v", c.name, got, c.want)
		}
	}
}
