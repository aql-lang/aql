package engine

import (
	"testing"
)

// --- Typed `def` syntax: def name:*Type value ---
//
// `def x:Integer 1` parses as `def {x:Integer} 1` at the top level
// (jsonic treats `x:Integer` as a single-pair map). The new
// [TMap, TAny] signature on def uses the map's only key as the
// name and its only value as a type constraint, then unifies the
// body with the constraint before installing the def. A
// non-unifying body errors and the def is not installed.

// --- Plain types ---

func TestTypedDefIntegerSuccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Build the program: `def x:Integer 1` where x:Integer becomes
	// a single-key map at the top level.
	m := NewOrderedMap()
	m.Set("x", NewTypeLiteral(TInteger))
	runAQL(t, r, []Value{
		NewWord("def"),
		NewMap(m),
		NewInteger(1),
	})
	// x should resolve to 1.
	result := runAQL(t, r, []Value{NewWord("x")})
	if len(result) != 1 {
		t.Fatalf("expected 1 result for x, got %v", result)
	}
	got, _ := AsInteger(result[0])
	if got != 1 {
		t.Errorf("x = %d, want 1", got)
	}
}

func TestTypedDefIntegerFailure(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("x", NewTypeLiteral(TInteger))
	e := New(r)
	_, err = e.Run([]Value{
		NewWord("def"),
		NewMap(m),
		NewString("not-an-integer"),
	})
	if err == nil {
		t.Fatal("expected type-mismatch error for `def x:Integer \"not-an-integer\"`, got nil")
	}
	// x must NOT have been installed on a failed unify.
	if r.Defs.Has("x") {
		t.Errorf("x was installed despite unify failure (depth=%d)", r.Defs.Depth("x"))
	}
}

// --- Dependent types in def position ---

// `def n:(Integer gt 10) 11` — anonymous dependent type at the colon
// position. Body 11 satisfies the constraint.
func TestTypedDefAnonymousDepIntegerSuccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	dep := NewDepScalar(DepGT, NewInteger(10))
	m := NewOrderedMap()
	m.Set("n", dep)
	runAQL(t, r, []Value{
		NewWord("def"),
		NewMap(m),
		NewInteger(11),
	})
	// n should resolve to 11.
	result := runAQL(t, r, []Value{NewWord("n")})
	got, _ := AsInteger(result[0])
	if got != 11 {
		t.Errorf("n = %d, want 11", got)
	}
}

func TestTypedDefAnonymousDepIntegerFailure(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	dep := NewDepScalar(DepGT, NewInteger(10))
	m := NewOrderedMap()
	m.Set("n", dep)
	e := New(r)
	_, err = e.Run([]Value{
		NewWord("def"),
		NewMap(m),
		NewInteger(5),
	})
	if err == nil {
		t.Fatal("expected unify failure for `def n:(Integer gt 10) 5`, got nil")
	}
}

// --- Named dependent types ---

// Named-type case: a defined DepScalar plugged in as the constraint.
// In real source `def n:G10 11`, the map's `G10` Word is resolved by
// autoEvalMap before def sees it; here we pre-resolve and pass the
// DepScalar directly so the test stays focused on def's own behaviour
// (the parser-driven path is exercised in test/typed_def_test.go).
func TestTypedDefNamedTypeSuccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	g10 := NewDepScalar(DepGT, NewInteger(10))
	m := NewOrderedMap()
	m.Set("n", g10)
	runAQL(t, r, []Value{
		NewWord("def"),
		NewMap(m),
		NewInteger(11),
	})

	result := runAQL(t, r, []Value{NewWord("n")})
	got, _ := AsInteger(result[0])
	if got != 11 {
		t.Errorf("n = %d, want 11", got)
	}
}

func TestTypedDefNamedTypeFailure(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	g10 := NewDepScalar(DepGT, NewInteger(10))
	m := NewOrderedMap()
	m.Set("n", g10)
	e := New(r)
	_, err = e.Run([]Value{
		NewWord("def"),
		NewMap(m),
		NewInteger(5),
	})
	if err == nil {
		t.Fatal("expected unify failure for n:G10 with 5 (G10 = Integer gt 10), got nil")
	}
}

// --- Map shape errors ---

// Multi-key map at the def-name position is rejected.
func TestTypedDefMultiKeyRejected(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("a", NewTypeLiteral(TInteger))
	m.Set("b", NewTypeLiteral(TString))
	e := New(r)
	_, err = e.Run([]Value{
		NewWord("def"),
		NewMap(m),
		NewInteger(1),
	})
	if err == nil {
		t.Fatal("expected error for multi-key def name map, got nil")
	}
}

// Map value that isn't a type constraint is rejected.
func TestTypedDefNonTypeValueRejected(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("x", NewInteger(42)) // 42 is a literal, not a type
	e := New(r)
	_, err = e.Run([]Value{
		NewWord("def"),
		NewMap(m),
		NewInteger(1),
	})
	if err == nil {
		t.Fatal("expected error when def-name map's value isn't a type, got nil")
	}
}

// Surface-syntax tests (parser-driven `def x:Integer 1`) live in
// lang/go/test/typed_def_test.go to avoid an engine ↔ parser import cycle.
