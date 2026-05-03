package test

import (
	"strings"
	"testing"

	aql "github.com/metsitaba/voxgig-exp/aql"
)

// --- Name-confusion guards between `type` and `def` ---
//
// Type names live in r.Types; def names live in DefStacks (and are
// registered as callables when the body is a fn). The two namespaces
// must NOT share a name, otherwise a single source token has two
// meanings depending on context. Both directions of the clash are
// rejected with a clear error before either binding happens.

func expectError(t *testing.T, src string, wantSubstr string) {
	t.Helper()
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(src)
	if err == nil {
		t.Fatalf("expected error matching %q for source:\n%s", wantSubstr, src)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("expected error containing %q, got: %v", wantSubstr, err)
	}
}

// Under the case rule (type names capital, def names not), the
// case-mismatch check fires first and the same name can't legally
// reach both sides. These tests pin the case-rule message; the
// underlying name-clash guard remains in place for the rare future
// case where a capitalised native is added (no such native exists
// today).
func TestNameConfusion_DefThenType_CaseRule(t *testing.T) {
	// `type foo` is rejected outright — type names must capitalise.
	expectError(t, `def foo 1
type foo Integer`, "must start with a capital letter")
}

func TestNameConfusion_TypeThenDef_CaseRule(t *testing.T) {
	// `def Foo` is rejected outright — def names must not capitalise.
	expectError(t, `type Foo Integer
def Foo 1`, "must not start with a capital letter")
}

// Native fn clash: `type` over a registered native — natives are
// lowercase, so the capital-rule message hits first. The
// name-clash guard is still wired up in case capitalised natives
// are ever registered.
func TestNameConfusion_TypeOverNativeFn_CaseRule(t *testing.T) {
	expectError(t, `type add Integer`, "must start with a capital letter")
}

// Re-defining the same TYPE is also rejected (no shadow stack — type
// names are intended to be singletons in a given registry). The
// existing ValidateTypeNameParts check fires first and reports the
// conflict in its own wording.
func TestNameConfusion_TypeRedefinition(t *testing.T) {
	expectError(t, `type Foo Integer
type Foo String`, "conflicts with an existing type name")
}

// --- `is` evaluates predicate types ---
//
// `v is Bbd` runs the Bbd predicate against v. The predicate returns
// None on fail / unified value on success; `is` collapses that to
// Boolean: true iff non-None. Symmetric with the typed-def handler.

const isBbdSource = `type Bbd fn [x:Any Any [if ((x is String) and (x gte "b") and (x lte "d")) [x] [None]]]
`

// `is` is forward-prec and greedy, so multi-line tests wrap each
// `is` expression in parens to scope its forward arg collection.
func TestIsPredicate_True(t *testing.T) {
	got := runOne(t, isBbdSource+`("c" is Bbd)`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("\"c\" is Bbd = %v, want [\"true\"]", got)
	}
}

func TestIsPredicate_FalseOutOfRange(t *testing.T) {
	got := runOne(t, isBbdSource+`("e" is Bbd)`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("\"e\" is Bbd = %v, want [\"false\"]", got)
	}
}

func TestIsPredicate_FalseWrongType(t *testing.T) {
	got := runOne(t, isBbdSource+`(99 is Bbd)`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("99 is Bbd = %v, want [\"false\"]", got)
	}
}

func TestIsPredicate_TransformingPredicate(t *testing.T) {
	// `is` only checks the success/failure flag; the transformed value
	// is discarded by `is` (the Boolean answer is what matters).
	got := runOne(t, `type Up fn [x:Any Any [if (x is String) [x upper] [None]]]
("hello" is Up)`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("\"hello\" is Up = %v, want [\"true\"]", got)
	}
}

// `is` over a non-predicate type still routes through Unify (existing
// behaviour) — guard that the predicate path doesn't break it.
func TestIsPredicate_DepScalarStillWorks(t *testing.T) {
	got := runOne(t, `type G10 (Integer gt 10)
(15 is G10)
(5 is G10)`)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 results", got)
	}
	if got[0] != "true" {
		t.Errorf("15 is G10 = %v, want \"true\"", got[0])
	}
	if got[1] != "false" {
		t.Errorf("5 is G10 = %v, want \"false\"", got[1])
	}
}
