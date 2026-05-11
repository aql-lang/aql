package test

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/lang"
)

// Surface-syntax tests for `def name:Type value`.
//
// At the top level, jsonic parses `name:Type` as a single-pair map,
// so the new [Map, Any] sig on `def` picks it up and uses the map's
// only key as the name and its only value as the type constraint.
// The body must unify with the constraint or the def errors and is
// not installed.

func runOne(t *testing.T, src string) []any {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	result, err := a.Run(src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return result
}

// `def x:Integer 1; x` → 1
func TestTypedDefIntegerLiteralSurface(t *testing.T) {
	got := runOne(t, "def x:Integer 1\nx")
	if len(got) != 1 || got[0] != int64(1) {
		t.Errorf("got %v, want [1]", got)
	}
}

// `def x:Integer "no"` → unify error.
func TestTypedDefIntegerLiteralSurfaceFailure(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`def x:Integer "nope"`)
	if err == nil {
		t.Fatal("expected unify error for `def x:Integer \"nope\"`, got nil")
	}
}

// `def x:String "hi"; x` → "hi"
func TestTypedDefStringSurface(t *testing.T) {
	got := runOne(t, `def x:String "hi"
x`)
	if len(got) != 1 || got[0] != "hi" {
		t.Errorf("got %v, want [\"hi\"]", got)
	}
}

// `type G10 (Integer gt 10); def n:G10 11; n` → 11
func TestTypedDefNamedDepTypeSurface(t *testing.T) {
	got := runOne(t, `type G10 (Integer gt 10)
def n:G10 11
n`)
	if len(got) != 1 || got[0] != int64(11) {
		t.Errorf("got %v, want [11]", got)
	}
}

// `type G10 (Integer gt 10); def n:G10 5` → unify error.
func TestTypedDefNamedDepTypeSurfaceFailure(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = a.Run(`type G10 (Integer gt 10)
def n:G10 5`)
	if err == nil {
		t.Fatal("expected unify error for `def n:G10 5` (5 not gt 10), got nil")
	}
}

// Multiple typed defs in sequence work. Decimal comes back through
// lang.Run's catch-all default branch as a stringified value, so we
// just check non-nil + the integer/string entries.
func TestTypedDefMultipleBindings(t *testing.T) {
	got := runOne(t, `def a:Integer 1
def b:String "two"
def c:Decimal 3.5
a b c`)
	if len(got) != 3 {
		t.Fatalf("got %v, want 3 results", got)
	}
	if got[0] != int64(1) {
		t.Errorf("a = %v, want 1", got[0])
	}
	if got[1] != "two" {
		t.Errorf("b = %v, want \"two\"", got[1])
	}
	if got[2] != "3.5" {
		t.Errorf("c = %v, want \"3.5\"", got[2])
	}
}
