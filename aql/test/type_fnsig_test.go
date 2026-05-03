package test

import (
	"testing"

	aql "github.com/metsitaba/voxgig-exp/aql"
)

// --- Function signatures as types ---
//
// `type Mapper fn [[Integer] [Integer]]` installs `Mapper` as a
// function-shape type — a FnUndef value carrying input + output sig
// lists but no body. Mapper can then be used in the typed-def form
// `def n:Mapper somefn` to constrain n to a function value whose
// signatures structurally match Mapper's.

// A function whose sole sig matches Mapper unifies and is bound.
// The `(quote double)` form passes the function as a value rather than
// invoking it — same idiom AQL already uses for higher-order calls.
func TestTypeFnSig_DefBindMatchingFunction(t *testing.T) {
	got := runOne(t, `type Mapper fn [[Integer] [Integer]]
def double fn [[Integer] [Integer] [1 add]]
def m:Mapper (quote double)
double 5`)
	// double 5 → 6 (we just use double directly here; the test point
	// is that `def m:Mapper (quote double)` did not error).
	if len(got) != 1 || got[0] != int64(6) {
		t.Errorf("got %v, want [6]", got)
	}
}

// A non-function value fails the typed binding.
func TestTypeFnSig_DefBindRejectsNonFunction(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`type Mapper fn [[Integer] [Integer]]
def m:Mapper 42`)
	if err == nil {
		t.Fatal("expected unify error for `def m:Mapper 42` (42 is not a function), got nil")
	}
}

// A function whose input types differ from Mapper's fails.
func TestTypeFnSig_DefBindRejectsWrongInputType(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`type Mapper fn [[Integer] [Integer]]
def stringy fn [[String] [Integer] [length]]
def m:Mapper (quote stringy)`)
	if err == nil {
		t.Fatal("expected unify error for `def m:Mapper (quote stringy)` (String != Integer input), got nil")
	}
}

// A function whose return types differ from Mapper's fails.
func TestTypeFnSig_DefBindRejectsWrongReturnType(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`type Mapper fn [[Integer] [Integer]]
def stringer fn [[Integer] [String] [convert String]]
def m:Mapper (quote stringer)`)
	if err == nil {
		t.Fatal("expected unify error for `def m:Mapper (quote stringer)` (returns String != Integer), got nil")
	}
}

// A function with a different arity fails.
func TestTypeFnSig_DefBindRejectsWrongArity(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`type Mapper fn [[Integer] [Integer]]
def two-arg fn [[Integer Integer] [Integer] [add]]
def m:Mapper (quote two-arg)`)
	if err == nil {
		t.Fatal("expected unify error for `def m:Mapper (quote two-arg)` (arity 2 vs 1), got nil")
	}
}

// Different bound names: a second function-shape type and a function
// that satisfies it; ensures the constraint store is per-name.
func TestTypeFnSig_DistinctNamedShapes(t *testing.T) {
	got := runOne(t, `type Mapper fn [[Integer] [Integer]]
type Predicate fn [[Integer] [Boolean]]
def double fn [[Integer] [Integer] [1 add]]
def positive fn [[Integer] [Boolean] [n:Integer 0 gt]]
def m:Mapper (quote double)
def p:Predicate (quote positive)
0`)
	// Just confirm both bindings install without error.
	if len(got) != 1 || got[0] != int64(0) {
		t.Errorf("got %v, want [0]", got)
	}
}

// --- Predicate-as-type: fn with a body returning Boolean ---
//
// `type Bbd fn [x:Any Any [(x is String) and (x gte "b") and (x lte "d")]]`
// installs Bbd as a *predicate* type. `def p:Bbd value` calls the
// predicate with `value`; on true the def installs, on false it errors.

func TestTypeFnPredicate_DefBindWithinRange(t *testing.T) {
	got := runOne(t, `type Bbd fn [x:Any Any [(x is String) and (x gte "b") and (x lte "d")]]
def p:Bbd "c"
p`)
	if len(got) != 1 || got[0] != "c" {
		t.Errorf("got %v, want [\"c\"]", got)
	}
}

func TestTypeFnPredicate_DefBindBoundary(t *testing.T) {
	for _, val := range []string{"b", "c", "d"} {
		got := runOne(t, `type Bbd fn [x:Any Any [(x is String) and (x gte "b") and (x lte "d")]]
def p:Bbd "`+val+`"
p`)
		if len(got) != 1 || got[0] != val {
			t.Errorf("Bbd boundary %q: got %v, want [%q]", val, got, val)
		}
	}
}

func TestTypeFnPredicate_DefBindOutOfRange(t *testing.T) {
	for _, val := range []string{"a", "e", "z"} {
		a, err := aql.New()
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		_, err = a.Run(`type Bbd fn [x:Any Any [(x is String) and (x gte "b") and (x lte "d")]]
def q:Bbd "` + val + `"`)
		if err == nil {
			t.Errorf("Bbd out-of-range %q: expected unify error, got nil", val)
		}
	}
}

func TestTypeFnPredicate_DefBindWrongType(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// Integer 99 is not a String, so (x is String) is false → predicate
	// returns false → def errors.
	_, err = a.Run(`type Bbd fn [x:Any Any [(x is String) and (x gte "b") and (x lte "d")]]
def q:Bbd 99`)
	if err == nil {
		t.Fatal("Bbd with Integer 99: expected unify error, got nil")
	}
}

// The named predicate type should also still be callable as an
// ordinary fn; binding it as a type doesn't preclude direct use.
// aql.Run renders Boolean carriers via `default → v.String()`, so
// the returned []any holds the strings "true" / "false".
func TestTypeFnPredicate_AlsoCallable(t *testing.T) {
	got := runOne(t, `type Bbd fn [x:Any Any [(x is String) and (x gte "b") and (x lte "d")]]
Bbd "c"
Bbd "e"`)
	if len(got) != 2 {
		t.Fatalf("got %v results, want 2", got)
	}
	if got[0] != "true" {
		t.Errorf("Bbd \"c\" = %v, want \"true\"", got[0])
	}
	if got[1] != "false" {
		t.Errorf("Bbd \"e\" = %v, want \"false\"", got[1])
	}
}

// A predicate over Integer values (ranged constraint expressed as a fn).
func TestTypeFnPredicate_IntegerRange(t *testing.T) {
	got := runOne(t, `type Mid fn [n:Any Any [(n is Integer) and (n gte 10) and (n lte 20)]]
def x:Mid 15
x`)
	if len(got) != 1 || got[0] != int64(15) {
		t.Errorf("got %v, want [15]", got)
	}
}

func TestTypeFnPredicate_IntegerRangeFail(t *testing.T) {
	a, err := aql.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`type Mid fn [n:Any Any [(n is Integer) and (n gte 10) and (n lte 20)]]
def x:Mid 25`)
	if err == nil {
		t.Fatal("Mid with 25: expected unify error, got nil")
	}
}
