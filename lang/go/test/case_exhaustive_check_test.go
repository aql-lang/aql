package test

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go"
)

// Static exhaustiveness for `case` (design/case-exhaustiveness.0.md): a
// default-less case whose clauses do not provably cover the scrutinee's
// static type is an error-severity check finding (case_not_exhaustive),
// and the trailing default is not required when the type disjunction is
// met. These tests pin both directions for every domain shape — declared
// unions, enums, Boolean, optionals, newtypes, concrete values — plus the
// opt-outs (dynamic scrutinees, per-call-shape concrete narrowing) and
// the two advisory duals.

func checkDiag(t *testing.T, src string) lang.CheckResult {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check %q: %v", src, err)
	}
	return res
}

// diagWithCode returns the first diagnostic with the given code, if any.
func diagWithCode(res lang.CheckResult, code string) (lang.CheckDiagnostic, bool) {
	for _, d := range res.Diagnostics {
		if d.Code == code {
			return d, true
		}
	}
	return lang.CheckDiagnostic{}, false
}

// wantCase asserts the presence (or absence) of one case-coverage code and,
// when present, that its detail mentions every fragment.
func wantCase(t *testing.T, src, code string, want bool, fragments ...string) {
	t.Helper()
	res := checkDiag(t, src)
	d, got := diagWithCode(res, code)
	if got != want {
		t.Fatalf("%q: %s present=%v, want %v (diags: %v)", src, code, got, want, diagCodes(res))
	}
	if !got {
		return
	}
	for _, f := range fragments {
		if !strings.Contains(d.Detail, f) {
			t.Errorf("%q: %s detail %q missing %q", src, code, d.Detail, f)
		}
	}
}

func TestCaseExhaustiveDeclaredUnion(t *testing.T) {
	// A declared union scrutinee with a missing alternative is an error
	// naming the uncovered alternative…
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][Integer][case x [Integer 1]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "String")
	// …full type-clause coverage needs no default…
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" String "s"]]] f 5`,
		"case_not_exhaustive", false)
	// …a trailing default always suffices…
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][Integer][case x [Integer 1 "d"]]] f 5`,
		"case_not_exhaustive", false)
	// …and the union type itself in match position covers all its members.
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [IS "either"]]] f 5`,
		"case_not_exhaustive", false)
	// A nested union expands through its member unions.
	wantCase(t,
		`def AB (Integer tor String) def ABC (AB tor Boolean) `+
			`def f fn [[x:ABC][Integer][case x [Integer 1 String 2 true 3 false 4]]] f 5`,
		"case_not_exhaustive", false)
}

func TestCaseExhaustiveBoolean(t *testing.T) {
	// true+false decompose the opaque Boolean leaf…
	wantCase(t,
		`def f fn [[b:Boolean][Integer][case b [true 1 false 0]]] f true`,
		"case_not_exhaustive", false)
	// …a missing arm is an error naming the uncovered value…
	wantCase(t,
		`def f fn [[b:Boolean][Integer][case b [true 1]]] f true`,
		"case_not_exhaustive", true, "uncovered", "false")
	// …and the Boolean type literal covers both at once.
	wantCase(t,
		`def f fn [[b:Boolean][Integer][case b [Boolean 1]]] f true`,
		"case_not_exhaustive", false)
}

func TestCaseExhaustiveEnum(t *testing.T) {
	// Every member listed → no default needed…
	wantCase(t,
		`def Color (red/q tor green/q tor blue/q) `+
			`def f fn [[c:Color][String][case c [red/q "r" green/q "g" blue/q "b"]]] f red/q`,
		"case_not_exhaustive", false)
	// …a missing member is an error naming it…
	wantCase(t,
		`def Color (red/q tor green/q tor blue/q) `+
			`def f fn [[c:Color][Integer][case c [red/q 1 green/q 2]]] f red/q`,
		"case_not_exhaustive", true, "uncovered", "blue")
	// …and a covering type literal (Atom) covers every member.
	wantCase(t,
		`def Color (red/q tor green/q tor blue/q) `+
			`def f fn [[c:Color][String][case c [Atom "atom"]]] f red/q`,
		"case_not_exhaustive", false)
}

func TestCaseExhaustiveOptionalNone(t *testing.T) {
	wantCase(t,
		`def MaybeInt (Integer tor none) def f fn [[x:MaybeInt][Integer][case x [Integer 1 none 0]]] f 5`,
		"case_not_exhaustive", false)
	wantCase(t,
		`def MaybeInt (Integer tor none) def f fn [[x:MaybeInt][Integer][case x [Integer 1]]] f 5`,
		"case_not_exhaustive", true, "uncovered")
}

func TestCaseExhaustiveConcrete(t *testing.T) {
	// A top-level concrete scrutinee is value-precise: a provably-matching
	// clause list is exhaustive…
	wantCase(t, `case 2 [1 "one" 2 "two"]`, "case_not_exhaustive", false)
	// …and a provably-unmatched value is an error naming it.
	wantCase(t, `case 9 [1 "one" 2 "two"]`, "case_not_exhaustive", true, "uncovered", "9")
	// The stack-value form is checked identically.
	wantCase(t, `9 case [1 "one" 2 "two"]`, "case_not_exhaustive", true, "uncovered", "9")
	wantCase(t, `2 case [1 "one" 2 "two"]`, "case_not_exhaustive", false)
}

func TestCaseExhaustivePlainTypeParam(t *testing.T) {
	// Value clauses can never cover an infinite plain type…
	wantCase(t,
		`def f fn [[x:Integer][String][case x [1 "a" 2 "b"]]] f 1`,
		"case_not_exhaustive", true, "uncovered", "Integer")
	// …a covering type clause or a default resolves it.
	wantCase(t,
		`def f fn [[x:Integer][String][case x [Integer "i"]]] f 1`,
		"case_not_exhaustive", false)
	wantCase(t,
		`def f fn [[x:Integer][String][case x [1 "a" "d"]]] f 1`,
		"case_not_exhaustive", false)
	// A supertype clause covers the subtype scrutinee.
	wantCase(t,
		`def f fn [[x:Integer][String][case x [Number "n"]]] f 1`,
		"case_not_exhaustive", false)
}

func TestCaseExhaustiveNewtype(t *testing.T) {
	// A bare-refine newtype scrutinee is covered by its base type…
	wantCase(t,
		`def Pos refine Integer def f fn [[x:Pos][String][case x [Integer "i"]]] f (def y:Pos 5 y)`,
		"case_not_exhaustive", false)
	// …but a newtype clause does NOT statically cover its base scrutinee
	// (coverage is proven by lattice containment only — the UnifyR
	// base-admission leniency is deliberately not credited).
	wantCase(t,
		`def Pos refine Integer def f fn [[x:Integer][String][case x [Pos "p"]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "Integer")
}

func TestCaseExhaustivePredicatesOpaque(t *testing.T) {
	// Predicate clauses prove no coverage: alone they still demand a default…
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[gt 3] 1 [lte 3] 2]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "Integer")
	// …but they do not defeat full type-clause coverage beside them.
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][Integer][case x [[gt 3] 0 Integer 1 String 2]]] f 5`,
		"case_not_exhaustive", false)
}

func TestCaseExhaustiveSkips(t *testing.T) {
	// A gradual (dynamic) scrutinee skips the check: an :Any param opts out
	// of static typing — including under per-call-shape re-analysis, which
	// binds the concrete call argument (`f 9`).
	wantCase(t,
		`def f fn [[x:Any][][case x [1 "one" 2 "two"]]] f 9`,
		"case_not_exhaustive", false)
	// A code-body scrutinee dispatches on a computed value — skipped.
	wantCase(t, `case [1 add 1] [5 "five"]`, "case_not_exhaustive", false)
	// A lone default clause is trivially exhaustive.
	wantCase(t,
		`def f fn [[x:Integer][String][case x ["only"]]] f 5`,
		"case_not_exhaustive", false)
	// An EMPTY clause list over a typed scrutinee can match nothing.
	wantCase(t,
		`def f fn [[x:Integer][][case x []]] f 5`,
		"case_not_exhaustive", true, "uncovered", "Integer")
}

func TestCaseExhaustiveMatchShapes(t *testing.T) {
	// An unresolvable word in match position is opaque: no coverage
	// credit (a typed default-less case still errors)…
	wantCase(t,
		`def f fn [[x:Integer][String][case x [zzz "a"]]] f 1`,
		"case_not_exhaustive", true, "uncovered", "Integer")
	// …and no false finding when a default is present.
	wantCase(t, `case 1 [zzz "a" "d"]`, "case_not_exhaustive", false)
	// A union match that covers NONE of the scrutinee's alternatives
	// leaves everything uncovered.
	wantCase(t,
		`def IS (Integer tor String) def f fn [[b:Boolean][String][case b [IS "no"]]] f true`,
		"case_not_exhaustive", true, "uncovered")
	// A declared-union fn RETURN feeding case directly is a checked domain.
	wantCase(t,
		`def IS (Integer tor String) def g fn [[][IS][5]] def f fn [[][String][case (g) [Integer "i"]]] f`,
		"case_not_exhaustive", true, "uncovered", "String")
	wantCase(t,
		`def IS (Integer tor String) def g fn [[][IS][5]] def f fn [[][String][case (g) [Integer "i" String "s"]]] f`,
		"case_not_exhaustive", false)
	// A bare type node used as the scrutinee VALUE is not a checkable
	// domain — skipped, never a finding.
	wantCase(t, `case Integer [Integer "i"]`, "case_not_exhaustive", false)
	// A concrete none scrutinee normalizes onto the None node and is
	// covered by a `none` clause.
	wantCase(t, `case none [none 0]`, "case_not_exhaustive", false)
}

func TestCaseUnreachableClauseAdvisory(t *testing.T) {
	// A clause fully covered by an earlier clause is dead…
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [Number 1 Integer 2 3]]] f 1`,
		"case_unreachable_clause", true, "Integer")
	// …including a value clause behind its covering type clause…
	wantCase(t,
		`def f fn [[x:Integer][String][case x [Integer "i" 5 "five" "d"]]] f 1`,
		"case_unreachable_clause", true)
	// …and an exact duplicate value clause.
	wantCase(t, `case 1 [1 "a" 1 "b" "d"]`, "case_unreachable_clause", true)
	// Distinct live clauses are never flagged.
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" String "s"]]] f 5`,
		"case_unreachable_clause", false)
	wantCase(t, `case 2 [1 "a" 2 "b" "d"]`, "case_unreachable_clause", false)
}

func TestCaseRedundantDefaultAdvisory(t *testing.T) {
	// Full coverage of a DECLARED union makes the trailing default dead…
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" String "s" "d"]]] f 5`,
		"case_redundant_default", true)
	// …but partial coverage keeps it live…
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" "d"]]] f 5`,
		"case_redundant_default", false)
	// …and a non-declared domain (a concrete scrutinee) never draws the
	// advisory, even when provably covered: only the author-declared
	// union domain is stable across call shapes.
	wantCase(t, `case 2 [2 "two" "d"]`, "case_redundant_default", false)
}

func TestCaseExhaustiveSeverities(t *testing.T) {
	res := checkDiag(t, `case 9 [1 "one" 2 "two"]`)
	d, ok := diagWithCode(res, "case_not_exhaustive")
	if !ok {
		t.Fatalf("case_not_exhaustive missing: %v", diagCodes(res))
	}
	if d.Severity != lang.SeverityError {
		t.Errorf("case_not_exhaustive severity = %s, want error", d.Severity)
	}
	if d.RuntimeMirror {
		t.Errorf("case_not_exhaustive must NOT be a RuntimeMirror — no-match is not a runtime error, and the compile pipeline must refuse on it")
	}
	if d.Word != "case" {
		t.Errorf("case_not_exhaustive word = %q, want case", d.Word)
	}

	res = checkDiag(t, `def f fn [[x:Integer][Integer][case x [Number 1 Integer 2 3]]] f 1`)
	if d, ok := diagWithCode(res, "case_unreachable_clause"); !ok || d.Severity != lang.SeverityInfo {
		t.Errorf("case_unreachable_clause must be info severity, got %+v (ok=%v)", d, ok)
	}
	res = checkDiag(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" String "s" "d"]]] f 5`)
	if d, ok := diagWithCode(res, "case_redundant_default"); !ok || d.Severity != lang.SeverityInfo {
		t.Errorf("case_redundant_default must be info severity, got %+v (ok=%v)", d, ok)
	}
}

func TestCaseExhaustiveDedupeAcrossCallShapes(t *testing.T) {
	// The fn body is analysed at construction and once per call shape;
	// the finding must appear exactly once per case site.
	res := checkDiag(t,
		`def IS (Integer tor String) def f fn [[x:IS][Integer][case x [Integer 1]]] f 5 f "s"`)
	n := 0
	for _, d := range res.Diagnostics {
		if d.Code == "case_not_exhaustive" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want exactly 1 case_not_exhaustive across call shapes, got %d: %v", n, diagCodes(res))
	}
}

func TestCaseExhaustiveRunRefusal(t *testing.T) {
	// `aql check` gating is severity-based: the finding is an error, so a
	// non-exhaustive program FAILS check while the covered twin passes.
	res := checkDiag(t, `case 9 [1 "one" 2 "two"]`)
	if res.Summary.Errors == 0 {
		t.Fatalf("non-exhaustive case must produce an error-severity finding: %+v", res.Summary)
	}
	res = checkDiag(t, `case 9 [1 "one" 2 "two" "many"]`)
	if res.Summary.Errors != 0 {
		t.Fatalf("defaulted case must check clean, got %d errors: %v", res.Summary.Errors, diagCodes(res))
	}
}
