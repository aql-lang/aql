package eng

import (
	"strings"
	"testing"
)

func TestUnifyExplainSuccess(t *testing.T) {
	v, err := UnifyExplain(NewInteger(1), NewInteger(1))
	if err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}
	got, _ := AsInteger(v)
	if got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestUnifyExplainScalarMismatch(t *testing.T) {
	_, err := UnifyExplain(NewInteger(1), NewInteger(2))
	if err == nil {
		t.Fatal("expected failure for 1 vs 2")
	}
	if !strings.Contains(err.Error(), "different literal") {
		t.Fatalf("error reason missing 'different literal': %q", err.Error())
	}
}

func TestUnifyExplainCrossType(t *testing.T) {
	_, err := UnifyExplain(NewInteger(1), NewString("hi"))
	if err == nil {
		t.Fatal("expected failure for Integer vs String")
	}
	if !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("error reason missing 'incompatible': %q", err.Error())
	}
}

func TestUnifyExplainListIndexPath(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	b := NewList([]Value{NewInteger(1), NewInteger(99), NewInteger(3)})
	_, err := UnifyExplain(a, b)
	if err == nil {
		t.Fatal("expected element mismatch")
	}
	if len(err.Path) != 1 || err.Path[0] != "[1]" {
		t.Fatalf("path got %v, want [[1]]", err.Path)
	}
	if !strings.Contains(err.Error(), "[1]") {
		t.Fatalf("rendered error missing index: %q", err.Error())
	}
}

func TestUnifyExplainNestedListPath(t *testing.T) {
	a := NewList([]Value{
		NewList([]Value{NewInteger(1), NewInteger(2)}),
		NewList([]Value{NewInteger(3), NewInteger(4)}),
	})
	b := NewList([]Value{
		NewList([]Value{NewInteger(1), NewInteger(2)}),
		NewList([]Value{NewInteger(3), NewInteger(99)}),
	})
	_, err := UnifyExplain(a, b)
	if err == nil {
		t.Fatal("expected nested mismatch")
	}
	want := []string{"[1]", "[1]"}
	if len(err.Path) != 2 || err.Path[0] != want[0] || err.Path[1] != want[1] {
		t.Fatalf("path got %v, want %v", err.Path, want)
	}
}

func TestUnifyExplainMapKeyPath(t *testing.T) {
	aMap := NewOrderedMap()
	aMap.Set("name", NewString("alice"))
	aMap.Set("age", NewInteger(30))

	bMap := NewOrderedMap()
	bMap.Set("name", NewString("alice"))
	bMap.Set("age", NewInteger(99))

	_, err := UnifyExplain(NewMap(aMap), NewMap(bMap))
	if err == nil {
		t.Fatal("expected map mismatch")
	}
	if len(err.Path) == 0 || err.Path[0] != "key:age" {
		t.Fatalf("path got %v, want first element 'key:age'", err.Path)
	}
}

func TestUnifyExplainLengthMismatch(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1)})
	_, err := UnifyExplain(a, b)
	if err == nil {
		t.Fatal("expected length mismatch")
	}
	if !strings.Contains(err.Error(), "length mismatch") {
		t.Fatalf("error missing 'length mismatch': %q", err.Error())
	}
}
