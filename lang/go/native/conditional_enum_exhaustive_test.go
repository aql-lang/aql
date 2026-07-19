package native

import (
	"strings"
	"testing"
)

// Coverage for the enum/closed-disjunct `case` exhaustiveness advisories in
// conditional.go (checkCaseCoverage / resolveCaseMatch / caseMemberList /
// addCaseDiag). See design/ENUM-EXHAUSTIVENESS.0.md. Every positive assertion
// (a finding IS emitted) is paired with the negative case that must stay
// silent.

// countDiag returns how many diagnostics on r carry the given code.
func countDiag(r *Registry, code string) int {
	n := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == code {
			n++
		}
	}
	return n
}

// firstDiagDetail returns the detail of the first diagnostic with code, or "".
func firstDiagDetail(r *Registry, code string) string {
	for _, d := range r.Check.Diagnostics {
		if d.Code == code {
			return d.Detail
		}
	}
	return ""
}

func TestCaseExhaustivenessMissingMember(t *testing.T) {
	// Non-exhaustive: blue is unhandled and there is no default → advisory.
	r := seam5Reg(t)
	src := `def Color enum [red/q green/q blue/q]
def name fn [[c:Color] [String] [case c [red/q "R" green/q "G"]]]
name red/q`
	if _, err := seam5Check(r, src); err != nil {
		t.Fatal(err)
	}
	if n := countDiag(r, "case_nonexhaustive"); n != 1 {
		t.Fatalf("want 1 case_nonexhaustive diag, got %d", n)
	}
	if d := firstDiagDetail(r, "case_nonexhaustive"); !strings.Contains(d, "blue") {
		t.Fatalf("diag should name the uncovered member blue, got %q", d)
	}

	// Negative: cover all three members → no advisory.
	r2 := seam5Reg(t)
	src2 := `def Color enum [red/q green/q blue/q]
def name fn [[c:Color] [String] [case c [red/q "R" green/q "G" blue/q "B"]]]
name red/q`
	if _, err := seam5Check(r2, src2); err != nil {
		t.Fatal(err)
	}
	if n := countDiag(r2, "case_nonexhaustive"); n != 0 {
		t.Fatalf("exhaustive case must not warn, got %d", n)
	}
}

func TestCaseExhaustivenessDefaultDefeats(t *testing.T) {
	// A trailing default makes the case exhaustive by construction.
	r := seam5Reg(t)
	src := `def Color enum [red/q green/q blue/q]
def name fn [[c:Color] [String] [case c [red/q "R" "other"]]]
name red/q`
	if _, err := seam5Check(r, src); err != nil {
		t.Fatal(err)
	}
	if n := countDiag(r, "case_nonexhaustive"); n != 0 {
		t.Fatalf("case with default must not warn, got %d", n)
	}
}

func TestCaseExhaustivenessUnreachableClause(t *testing.T) {
	// yellow is not a member of Color → the clause can never fire.
	r := seam5Reg(t)
	src := `def Color enum [red/q green/q blue/q]
def name fn [[c:Color] [String] [case c [red/q "R" green/q "G" blue/q "B" yellow/q "Y"]]]
name red/q`
	if _, err := seam5Check(r, src); err != nil {
		t.Fatal(err)
	}
	if n := countDiag(r, "case_unreachable_clause"); n != 1 {
		t.Fatalf("want 1 case_unreachable_clause diag, got %d", n)
	}
	if d := firstDiagDetail(r, "case_unreachable_clause"); !strings.Contains(d, "yellow") {
		t.Fatalf("diag should name the out-of-set clause yellow, got %q", d)
	}
	// All three real members are covered → no exhaustiveness warning.
	if n := countDiag(r, "case_nonexhaustive"); n != 0 {
		t.Fatalf("all members covered must not warn nonexhaustive, got %d", n)
	}

	// Negative: every arm is an in-set member → no unreachable advisory.
	r2 := seam5Reg(t)
	src2 := `def Color enum [red/q green/q blue/q]
def name fn [[c:Color] [String] [case c [red/q "R" green/q "G" blue/q "B"]]]
name red/q`
	if _, err := seam5Check(r2, src2); err != nil {
		t.Fatal(err)
	}
	if n := countDiag(r2, "case_unreachable_clause"); n != 0 {
		t.Fatalf("in-set arms must not warn unreachable, got %d", n)
	}
}

func TestCaseExhaustivenessTorClosedUnion(t *testing.T) {
	// A hand-written closed union `a/q tor b/q tor c/q` is a closed set too —
	// exhaustiveness keys on the shape, not the Enum tag.
	r := seam5Reg(t)
	src := `def Color (red/q tor green/q tor blue/q)
def name fn [[c:Color] [String] [case c [red/q "R" green/q "G"]]]
name red/q`
	if _, err := seam5Check(r, src); err != nil {
		t.Fatal(err)
	}
	if n := countDiag(r, "case_nonexhaustive"); n != 1 {
		t.Fatalf("closed tor-union should warn, got %d", n)
	}
}

func TestCaseExhaustivenessOpenUnionSkipped(t *testing.T) {
	// An open union has a bare type-literal alternative → not a closed set →
	// no coverage obligation.
	r := seam5Reg(t)
	src := `def IorS (Integer tor String)
def name fn [[c:IorS] [String] [case c [Integer "int"]]]
name 5`
	if _, err := seam5Check(r, src); err != nil {
		t.Fatal(err)
	}
	if n := countDiag(r, "case_nonexhaustive"); n != 0 {
		t.Fatalf("open union must not warn, got %d", n)
	}
}

func TestCaseExhaustivenessPredicateArmSuppresses(t *testing.T) {
	// A code-body predicate arm may match any subset of the remaining members,
	// so exhaustiveness is undecidable and must not warn.
	r := seam5Reg(t)
	src := `def Color enum [red/q green/q blue/q]
def name fn [[c:Color] [String] [case c [red/q "R" [teq green/q] "G"]]]
name red/q`
	if _, err := seam5Check(r, src); err != nil {
		t.Fatal(err)
	}
	if n := countDiag(r, "case_nonexhaustive"); n != 0 {
		t.Fatalf("predicate arm must suppress exhaustiveness, got %d", n)
	}
}

// --- Direct unit tests for the guard branches integration cannot reach. ---

func TestCheckCaseCoverageInactiveAndNil(t *testing.T) {
	// Not in check mode → no analysis, no diagnostics.
	r := seam5Reg(t)
	scrut := NewEnum([]Value{NewAtom("red"), NewAtom("green")})
	elems := []Value{NewAtom("red"), NewString("R")}
	checkCaseCoverage(r, scrut, elems)
	if n := countDiag(r, "case_nonexhaustive"); n != 0 {
		t.Fatalf("inactive check must not emit, got %d", n)
	}
	// A nil registry is tolerated (defensive guard).
	checkCaseCoverage(nil, scrut, elems)
}

func TestCheckCaseCoverageNonDisjunct(t *testing.T) {
	// A concrete non-disjunct scrutinee fails AsDisjunct → skipped.
	r := seam5Reg(t)
	defer r.Check.Begin()()
	checkCaseCoverage(r, NewAtom("red"), []Value{NewAtom("red"), NewString("R")})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("non-disjunct scrutinee must not emit, got %d", len(r.Check.Diagnostics))
	}
}

func TestCheckCaseCoverageEmptyDisjunct(t *testing.T) {
	// A disjunct with no alternatives carries no coverage obligation.
	r := seam5Reg(t)
	defer r.Check.Begin()()
	checkCaseCoverage(r, NewEnum(nil), []Value{NewAtom("red"), NewString("R")})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("empty disjunct must not emit, got %d", len(r.Check.Diagnostics))
	}
}

func TestCheckCaseCoverageParenExprArmSuppresses(t *testing.T) {
	// A ParenExpr arm is a computed match → undecidable → no exhaustiveness
	// verdict even though blue is uncovered.
	r := seam5Reg(t)
	defer r.Check.Begin()()
	scrut := NewEnum([]Value{NewAtom("red"), NewAtom("green"), NewAtom("blue")})
	elems := []Value{
		NewParenExpr([]Value{NewWord("id"), NewAtom("red")}), NewString("X"),
		NewAtom("green"), NewString("G"),
	}
	checkCaseCoverage(r, scrut, elems)
	if n := countDiag(r, "case_nonexhaustive"); n != 0 {
		t.Fatalf("ParenExpr arm must suppress exhaustiveness, got %d", n)
	}
}

func TestCheckCaseCoverageDedup(t *testing.T) {
	// The same finding emitted twice (a body re-analysed per call-shape) is
	// de-duplicated by (code, detail).
	r := seam5Reg(t)
	defer r.Check.Begin()()
	scrut := NewEnum([]Value{NewAtom("red"), NewAtom("green"), NewAtom("blue")})
	elems := []Value{NewAtom("red"), NewString("R")} // blue+green uncovered, no default
	checkCaseCoverage(r, scrut, elems)
	checkCaseCoverage(r, scrut, elems)
	if n := countDiag(r, "case_nonexhaustive"); n != 1 {
		t.Fatalf("duplicate findings must dedup to 1, got %d", n)
	}
}

func TestResolveCaseMatch(t *testing.T) {
	r := seam5Reg(t)
	// Bind a type name so ResolveTypedName takes the resolved branch.
	if _, err := seam5Run(r, `def Weekday enum [mon/q tue/q]`); err != nil {
		t.Fatal(err)
	}
	if got := resolveCaseMatch(r, NewWord("Weekday")); IsWord(got) {
		t.Fatalf("bound word should resolve away from a Word, got %v", got)
	}
	// An unbound word falls through to an Atom.
	got := resolveCaseMatch(r, NewWord("zzz-nope"))
	if !got.Parent.Equal(TAtom) {
		t.Fatalf("unbound word should resolve to an Atom, got %v", got.Parent)
	}
	// A non-word match is returned as-is.
	atom := NewAtom("red")
	if got := resolveCaseMatch(r, atom); !got.Parent.Equal(TAtom) || got.String() != "red" {
		t.Fatalf("non-word match should pass through, got %v", got)
	}
}
