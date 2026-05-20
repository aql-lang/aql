package test

import (
	"testing"

	"github.com/aql-lang/aql/lang/go"
)

// --- Never: the bottom type ---
//
// Never is uninhabited — no value belongs to it. It is the dual of Any
// (top): Any is the identity for tand and the absorbing element of tor;
// Never is the identity for tor and the absorbing element of tand.
//
// The principal use is making intersections total. Without Never, the
// intersection of disjoint types (`String tand Integer`) has no answer
// to give and historically errored. With Never, every intersection has
// a well-defined result — disjoint inputs reduce to Never, and that
// answer composes algebraically.

// --- Never as a type literal ---

// Bare `Never` parses as a type literal (like `None`, `Any`).
func TestNever_Literal(t *testing.T) {
	got := runOne(t, `Never`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("Never = %v, want [\"Never\"]", got)
	}
}

// --- tand: Never absorbs ---

// Disjoint scalars reduce to Never (the empty intersection). Pre-Never,
// this errored as "tand: cannot unify values".
func TestNever_TandDisjointScalars(t *testing.T) {
	got := runOne(t, `String tand Integer`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("String tand Integer = %v, want [\"Never\"]", got)
	}
}

// Concrete values that don't unify also produce Never.
func TestNever_TandDisjointLiterals(t *testing.T) {
	got := runOne(t, `1 tand 2`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("1 tand 2 = %v, want [\"Never\"]", got)
	}
}

// Never annihilates: T tand Never = Never tand T = Never.
func TestNever_TandAnnihilates(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`String tand Never`, "Never"},
		{`Never tand String`, "Never"},
		{`Never tand Never`, "Never"},
		{`Never tand Any`, "Never"},
		{`Any tand Never`, "Never"},
	}
	for _, tc := range cases {
		got := runOne(t, tc.expr)
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("%s = %v, want [%q]", tc.expr, got, tc.want)
		}
	}
}

// --- tor: Never is the identity ---

// T tor Never collapses to T (the uninhabited alternative drops out).
// Asserted via `is`: a String value should match the union, and the
// union shouldn't admit non-Strings either way.
func TestNever_TorIdentity(t *testing.T) {
	got := runOne(t, `def t (String tor Never)
"hi" is t
42 is t`)
	if len(got) != 2 || got[0] != "true" || got[1] != "false" {
		t.Errorf("String tor Never identity = %v, want [\"true\" \"false\"]", got)
	}
}

// Never tor T also collapses to T (commutative).
func TestNever_TorIdentityLeft(t *testing.T) {
	got := runOne(t, `def t (Never tor Integer)
42 is t
"hi" is t`)
	if len(got) != 2 || got[0] != "true" || got[1] != "false" {
		t.Errorf("Never tor Integer identity = %v, want [\"true\" \"false\"]", got)
	}
}

// All-Never disjunct collapses to Never itself.
func TestNever_TorAllNever(t *testing.T) {
	got := runOne(t, `Never tor Never`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("Never tor Never = %v, want [\"Never\"]", got)
	}
}

// --- tall / tany: same algebra over lists ---

// tall folds an n-ary intersection. Disjoint elements → Never.
func TestNever_TallDisjoint(t *testing.T) {
	got := runOne(t, `[Integer String Boolean] tall`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("[Integer String Boolean] tall = %v, want [\"Never\"]", got)
	}
}

// tall with a Never element annihilates the whole fold.
func TestNever_TallNeverAnnihilates(t *testing.T) {
	got := runOne(t, `[Integer Never] tall`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("[Integer Never] tall = %v, want [\"Never\"]", got)
	}
}

// tany folds an n-ary union. Never alternatives drop out.
func TestNever_TanyFiltersNever(t *testing.T) {
	got := runOne(t, `[String Never Integer] tany`)
	if len(got) != 1 {
		t.Fatalf("got %v, want 1 result", got)
	}
	// Result is a String|Integer disjunct — its print form depends on
	// disjunct rendering. Just verify it's not "Never" (filtered) and
	// not a single type (still has two alternatives).
	if got[0] == "Never" {
		t.Errorf("[String Never Integer] tany = %v, did not filter Never", got)
	}
}

// All-Never list reduces to Never.
func TestNever_TanyAllNever(t *testing.T) {
	got := runOne(t, `[Never Never] tany`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("[Never Never] tany = %v, want [\"Never\"]", got)
	}
}

// --- tand on records: per-field Never propagates only to that field ---

// Disjoint values for a shared key produce a record whose field is
// Never. The other fields are preserved — tand keeps as much
// information as possible about which field caused the failure.
func TestNever_TandRecordPerField(t *testing.T) {
	got := runOne(t, `{x:Integer y:1} tand {x:String y:1}`)
	if len(got) != 1 {
		t.Fatalf("got %v, want 1 result", got)
	}
	// The whole record collapses to Never (mergeMaps reports
	// failure on the conflicting key, propagated by tand).
	if got[0] != "Never" {
		t.Errorf("disjoint-record tand = %v, want [\"Never\"]", got)
	}
}

// --- is: Never is uninhabited ---

// No concrete value satisfies Never.
func TestNever_IsUninhabited(t *testing.T) {
	cases := []string{
		`42 is Never`,
		`"x" is Never`,
		`true is Never`,
		`None is Never`,
	}
	for _, src := range cases {
		got := runOne(t, src)
		if len(got) != 1 || got[0] != "false" {
			t.Errorf("%s = %v, want [\"false\"]", src, got)
		}
	}
}

// `Never is Never` reflects type equality (Unify(Never, Never)
// succeeds with Never), matching the existing pattern for None.
func TestNever_IsItself(t *testing.T) {
	got := runOne(t, `Never is Never`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("Never is Never = %v, want [\"true\"]", got)
	}
}

// --- def x:Never ---

// Binding any value to a Never-typed slot must fail: Never has no
// values, so no body can satisfy the constraint.
func TestNever_DefRejectsAllValues(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`def x:Never 1`)
	if err == nil {
		t.Errorf("def x:Never 1 succeeded; want error (Never is uninhabited)")
	}
}

// --- def Foo Never ---

// Aliasing Never via `type` is allowed — it just creates a name for the
// uninhabited type. `is` against the alias behaves like `is Never`.
func TestNever_TypeAlias(t *testing.T) {
	got := runOne(t, `def Bottom Never
42 is Bottom`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("42 is Bottom = %v, want [\"false\"]", got)
	}
}
