package langspec

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestVariadicSpreadOracle pins the soundness-oracle contract for a
// variadic-spread bottom carrier (the recursion.tsv:53 void-recursion residual):
// it absorbs ANY number of trailing runtime entries (count flexibility) but each
// must still be a member of the element type (type strictness) — a wrong-typed
// leak is still flagged. Pairs the positive (Integer spread covers N Integers)
// with the negatives (a String among the Integers, and a wrong fixed prefix).
func TestVariadicSpreadOracle(t *testing.T) {
	varInt := core.NewVariadicCarrier(core.NewTypeLiteral(core.TInteger))
	i := core.NewInteger(6)
	s := core.NewString("x")
	strCarrier := core.NewCarrier(core.TString)

	cases := []struct {
		name    string
		checked []core.Value
		actual  []core.Value
		want    bool
	}{
		{"three integers covered", []core.Value{varInt}, []core.Value{i, i, i}, true},
		{"zero trailing covered", []core.Value{varInt}, nil, true},
		{"string leak rejected", []core.Value{varInt}, []core.Value{i, s, i}, false},
		{"fixed prefix + integer spread", []core.Value{varInt, strCarrier}, []core.Value{i, i, s}, true},
		{"fixed prefix wrong type", []core.Value{varInt, strCarrier}, []core.Value{i, i, i}, false},
		{"missing fixed prefix", []core.Value{varInt, strCarrier}, nil, false},
	}
	for _, c := range cases {
		if got := stackTypeCovered(c.checked, c.actual); got != c.want {
			t.Errorf("%s: stackTypeCovered = %v, want %v", c.name, got, c.want)
		}
	}
}
