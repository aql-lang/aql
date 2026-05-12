package test

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang"
)

// --- type shadowing + untype ---
//
// *Type bindings now stack like `def` bindings. `type Foo X; type Foo
// Y` pushes Y on top so subsequent uses see Y; `untype Foo` pops Y
// and X becomes active again. Once the stack empties, the name is
// unbound. Mirrors `def` / `undef` semantics so users have a single
// scoping mental model across the value and type namespaces.

// Shadowing a type binding swaps the active definition without
// error. The most recent binding wins.
func TestTypeShadow_Push(t *testing.T) {
	got := runOne(t, `type Foo Integer
type Foo String
42 is Foo
"hi" is Foo`)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 results", got)
	}
	if got[0] != "false" {
		t.Errorf("42 is Foo (now String) = %v, want false", got[0])
	}
	if got[1] != "true" {
		t.Errorf("\"hi\" is Foo (now String) = %v, want true", got[1])
	}
}

// `untype Foo` pops the most recent binding. The previous binding
// becomes active again.
func TestTypeShadow_Pop(t *testing.T) {
	got := runOne(t, `type Foo Integer
type Foo String
untype Foo
42 is Foo
"hi" is Foo`)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 results", got)
	}
	if got[0] != "true" {
		t.Errorf("42 is Foo (popped to Integer) = %v, want true", got[0])
	}
	if got[1] != "false" {
		t.Errorf("\"hi\" is Foo (popped to Integer) = %v, want false", got[1])
	}
}

// Untype-ing the only binding makes the name unbound. Subsequent
// references error as undefined.
func TestTypeShadow_PopToEmpty(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`type Foo Integer
untype Foo
42 is Foo`)
	if err == nil {
		t.Fatalf("expected error after untype-ing the last binding")
	}
	if !strings.Contains(err.Error(), "Foo") {
		t.Errorf("error %q does not mention Foo", err)
	}
}

// Untype-ing a name with no binding errors with a clear message.
func TestTypeShadow_UntypeUnbound(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`untype Nonexistent`)
	if err == nil {
		t.Fatalf("expected error untyping a nonexistent name")
	}
	if !strings.Contains(err.Error(), "no such type binding") {
		t.Errorf("error %q does not say 'no such type binding'", err)
	}
}

// `untype foo` (lowercase) is rejected — the case rule applies to
// untype the same way it applies to type.
func TestTypeShadow_UntypeRejectsLowercase(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`type Foo Integer
untype foo`)
	if err == nil {
		t.Fatalf("expected error: lowercase untype name")
	}
	if !strings.Contains(err.Error(), "capital letter") {
		t.Errorf("error %q does not mention capital letter", err)
	}
}

// Shadow with a richer type: predicate type over a concrete type
// literal. `is` should consult the active binding.
func TestTypeShadow_PredicateOverLiteral(t *testing.T) {
	got := runOne(t, `type Foo Integer
type Foo fn [x:Any Any [if (x is String) [x] [None]]]
42 is Foo
"hi" is Foo
untype Foo
42 is Foo
"hi" is Foo`)
	if len(got) != 4 {
		t.Fatalf("got %v, want 4 results", got)
	}
	want := []string{"false", "true", "true", "false"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("probe %d: %v, want %v", i, got[i], w)
		}
	}
}

// Shadow with a DepScalar: the type-stack interacts cleanly with the
// dependent-scalar machinery.
func TestTypeShadow_DepScalar(t *testing.T) {
	got := runOne(t, `type Bound (Integer gt 10)
type Bound (Integer gt 100)
50 is Bound
200 is Bound
untype Bound
50 is Bound
200 is Bound`)
	if len(got) != 4 {
		t.Fatalf("got %v, want 4 results", got)
	}
	want := []string{"false", "true", "true", "true"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("probe %d: %v, want %v", i, got[i], w)
		}
	}
}

// Multiple shadows + multiple untypes — verify deep-stack behaviour.
func TestTypeShadow_DeepStack(t *testing.T) {
	got := runOne(t, `type T Integer
type T String
type T Boolean
true is T
untype T
"hi" is T
untype T
42 is T`)
	if len(got) != 3 {
		t.Fatalf("got %v, want 3 results", got)
	}
	want := []string{"true", "true", "true"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("level %d: %v, want %v", i, got[i], w)
		}
	}
}
