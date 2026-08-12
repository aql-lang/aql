package eng

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestUnifyExplainSuccess(t *testing.T) {
	v, err := core.UnifyExplain(core.NewInteger(1), core.NewInteger(1))
	if err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}
	got, _ := core.AsInteger(v)
	if got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestUnifyExplainScalarMismatch(t *testing.T) {
	_, err := core.UnifyExplain(core.NewInteger(1), core.NewInteger(2))
	if err == nil {
		t.Fatal("expected failure for 1 vs 2")
	}
	if !strings.Contains(err.Error(), "different literal") {
		t.Fatalf("error reason missing 'different literal': %q", err.Error())
	}
}

func TestUnifyExplainCrossType(t *testing.T) {
	_, err := core.UnifyExplain(core.NewInteger(1), core.NewString("hi"))
	if err == nil {
		t.Fatal("expected failure for Integer vs String")
	}
	if !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("error reason missing 'incompatible': %q", err.Error())
	}
}

func TestUnifyExplainListIndexPath(t *testing.T) {
	a := core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)})
	b := core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(99), core.NewInteger(3)})
	_, err := core.UnifyExplain(a, b)
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
	a := core.NewList([]core.Value{
		core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)}),
		core.NewList([]core.Value{core.NewInteger(3), core.NewInteger(4)}),
	})
	b := core.NewList([]core.Value{
		core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)}),
		core.NewList([]core.Value{core.NewInteger(3), core.NewInteger(99)}),
	})
	_, err := core.UnifyExplain(a, b)
	if err == nil {
		t.Fatal("expected nested mismatch")
	}
	want := []string{"[1]", "[1]"}
	if len(err.Path) != 2 || err.Path[0] != want[0] || err.Path[1] != want[1] {
		t.Fatalf("path got %v, want %v", err.Path, want)
	}
}

func TestUnifyExplainMapKeyPath(t *testing.T) {
	aMap := core.NewOrderedMap()
	aMap.Set("name", core.NewString("alice"))
	aMap.Set("age", core.NewInteger(30))

	bMap := core.NewOrderedMap()
	bMap.Set("name", core.NewString("alice"))
	bMap.Set("age", core.NewInteger(99))

	_, err := core.UnifyExplain(core.NewMap(aMap), core.NewMap(bMap))
	if err == nil {
		t.Fatal("expected map mismatch")
	}
	if len(err.Path) == 0 || err.Path[0] != "key:age" {
		t.Fatalf("path got %v, want first element 'key:age'", err.Path)
	}
}

func TestUnifyExplainLengthMismatch(t *testing.T) {
	a := core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	b := core.NewList([]core.Value{core.NewInteger(1)})
	_, err := core.UnifyExplain(a, b)
	if err == nil {
		t.Fatal("expected length mismatch")
	}
	if !strings.Contains(err.Error(), "length mismatch") {
		t.Fatalf("error missing 'length mismatch': %q", err.Error())
	}
}
