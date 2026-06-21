package native

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
)

// TestMembershipGoAqlParity is the convergence proof: a type defined in
// AQL by a predicate body (`def Pos (Integer gt 10)`) and the SAME type
// defined in Go by a predicate func (MintMemberType) must answer `is`
// identically for every value — because, after the convergence, both
// route through the one shared membership contract (matchMembership /
// unifyMembership). If the two paths ever drift, a row here flips.
func TestMembershipGoAqlParity(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// AQL path: install `Pos = Integer gt 10` by running the def.
	toks, perr := parser.Parse(`def Pos (Integer gt 10)`)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	if _, rerr := NewTop(r).Run(toks); rerr != nil {
		t.Fatalf("install AQL predicate type: %v", rerr)
	}
	posAQL := r.LookupTypeName("Pos")
	if posAQL == nil {
		t.Fatal("AQL type Pos did not install")
	}

	// Go path: the same rule as a Go predicate, minted into the same
	// registry via the host helper.
	posGo := r.Types.MintMemberType("PosGo", eng.TInteger, func(v Value) bool {
		n, err := v.AsConcreteInteger()
		return err == nil && n > 10
	})

	// Membership must agree across the whole range, including the wrong
	// family (a String is neither).
	values := []Value{
		NewInteger(-5), NewInteger(0), NewInteger(10), NewInteger(11),
		NewInteger(100), NewString("x"), NewBoolean(true),
	}
	for _, v := range values {
		gotAQL := v.Is(posAQL)
		gotGo := v.Is(posGo)
		if gotAQL != gotGo {
			t.Errorf("membership disagrees for %s: AQL Pos=%v, Go PosGo=%v",
				v.String(), gotAQL, gotGo)
		}
	}

	// And the verdicts are the expected ones (not merely "equal but both
	// wrong"): only integers > 10 are members.
	if !NewInteger(11).Is(posGo) || NewInteger(10).Is(posGo) || NewString("x").Is(posGo) {
		t.Error("Go member type gave an unexpected verdict")
	}
	if !NewInteger(11).Is(posAQL) || NewInteger(10).Is(posAQL) || NewString("x").Is(posAQL) {
		t.Error("AQL predicate type gave an unexpected verdict")
	}
}
