package test

import (
	"testing"
)

// --- tand distributes over tor ---
//
// Algebraic identity: (A tor B) tand C = (A tand C) tor (B tand C).
// Without distribution, tand on a disjunct fell through to Unify
// which only finds the first matching alternative — losing the rest
// of the union. Distribution rewrites the expression into disjunctive
// normal form before reducing, so tand and tor remain a sound
// algebra. Never-valued cross-product entries drop out via tor's
// identity rule, and structurally identical alternatives are
// deduped.
//
// Test note: `def t expr` captures `expr` as deferred code — t is
// re-evaluated at every access, against the current stack. To pin
// the type to its evaluated form, the body is wrapped in parens:
// `def t (expr)` evaluates eagerly and stores the result.

// Membership-style assertion: define a type via a tand-of-tor
// expression, then check `is t` for representative values. Avoids
// fragile string comparisons against disjunct print form.
func runMembership(t *testing.T, body string, wantTrue, wantFalse []string) {
	t.Helper()
	prog := "def t (" + body + ")\n"
	for _, v := range wantTrue {
		prog += v + " is t\n"
	}
	for _, v := range wantFalse {
		prog += v + " is t\n"
	}
	got := runOne(t, prog)
	if len(got) != len(wantTrue)+len(wantFalse) {
		t.Fatalf("got %d results, want %d", len(got), len(wantTrue)+len(wantFalse))
	}
	for i, v := range wantTrue {
		if got[i] != "true" {
			t.Errorf("%s is t = %v, want true", v, got[i])
		}
	}
	for i, v := range wantFalse {
		j := len(wantTrue) + i
		if got[j] != "false" {
			t.Errorf("%s is t = %v, want false", v, got[j])
		}
	}
}

// (Integer tor String) tand Integer = Integer (the String alt drops
// out — Unify(String, Integer) = Never).
func TestDistribute_LeftDisjunctNarrows(t *testing.T) {
	runMembership(t,
		`(Integer tor String) tand Integer`,
		[]string{`42`},
		[]string{`"hi"`, `true`})
}

// Integer tand (Integer tor String) = Integer (right-side disjunct,
// same outcome).
func TestDistribute_RightDisjunctNarrows(t *testing.T) {
	runMembership(t,
		`Integer tand (Integer tor String)`,
		[]string{`42`},
		[]string{`"hi"`, `true`})
}

// (Integer tor String) tand (Integer tor Boolean) = Integer.
// Both sides are disjuncts; the cross product yields
// {Int∩Int, Int∩Bool, Str∩Int, Str∩Bool} = {Int, Never, Never, Never}
// which filters/dedups to Integer.
func TestDistribute_BothDisjunctsCrossProduct(t *testing.T) {
	runMembership(t,
		`(Integer tor String) tand (Integer tor Boolean)`,
		[]string{`42`},
		[]string{`"hi"`, `true`, `1.5`})
}

// (Integer tor String) tand (String tor Integer) = (Integer tor String).
// Cross product yields {Int, Never, Never, Str} → Integer tor String.
func TestDistribute_BothDisjunctsBothMatch(t *testing.T) {
	runMembership(t,
		`(Integer tor String) tand (String tor Integer)`,
		[]string{`42`, `"hi"`},
		[]string{`true`, `1.5`})
}

// All-disjoint: every cross-product pair fails. Result is Never.
func TestDistribute_AllDisjointToNever(t *testing.T) {
	got := runOne(t, `(Integer tor Boolean) tand (String tor Decimal)`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("(Int tor Bool) tand (Str tor Dec) = %v, want [\"Never\"]", got)
	}
}

// Dedup: (Integer tor Integer) tand Integer = Integer (single alt,
// not a disjunct).
func TestDistribute_Dedups(t *testing.T) {
	runMembership(t,
		`(Integer tor Integer) tand Integer`,
		[]string{`42`},
		[]string{`"hi"`})
}

// Never inside a disjunct is filtered before distribution starts:
// (Integer tor Never) tand (String tor Integer) =
// Integer tand (String tor Integer) = Integer.
func TestDistribute_NeverFiltersBeforeDistribute(t *testing.T) {
	runMembership(t,
		`(Integer tor Never) tand (String tor Integer)`,
		[]string{`42`},
		[]string{`"hi"`, `true`})
}

// tand with Never on either side annihilates regardless of disjunct
// shape: (A tor B) tand Never = Never (the early annihilator check
// fires before distribution).
func TestDistribute_NeverAnnihilatesDisjunct(t *testing.T) {
	got := runOne(t, `(Integer tor String) tand Never`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("(Int tor Str) tand Never = %v, want [\"Never\"]", got)
	}
}

// tand distributes through value-level disjuncts, not just type
// literals. (1 tor 2) tand Integer = (1 tor 2) — both literals are
// already concrete Integers, so each survives the cross-product.
func TestDistribute_LiteralValueDisjunct(t *testing.T) {
	runMembership(t,
		`(1 tor 2) tand Integer`,
		[]string{`1`, `2`},
		[]string{`3`, `"hi"`})
}

// tall folds n-ary intersection using the same machinery, so it
// also distributes: [(Integer tor String) Integer] tall is
// equivalent to (Integer tor String) tand Integer.
func TestDistribute_TallFoldDistributes(t *testing.T) {
	runMembership(t,
		`[(Integer tor String) Integer] tall`,
		[]string{`42`},
		[]string{`"hi"`, `true`})
}

// Three-way fold: each step distributes against the accumulator.
func TestDistribute_TallThreeWay(t *testing.T) {
	runMembership(t,
		`[(Integer tor String) (Integer tor Boolean) Integer] tall`,
		[]string{`42`},
		[]string{`"hi"`, `true`, `1.5`})
}
