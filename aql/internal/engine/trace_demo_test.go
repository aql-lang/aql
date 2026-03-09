package engine

import (
	"os"
	"testing"
)

func TestTraceDemo(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Output = os.Stderr // so it shows with -v

	// trace [1 add 2 mul 3]
	e := NewTop(r)
	result, err := e.Run([]Value{
		NewWord("trace"),
		NewList([]Value{
			NewInteger(1), NewWord("add"), NewInteger(2), NewWord("mul"), NewInteger(3),
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 7 {
		t.Errorf("got %v, want [7]", result)
	}
}

func TestTraceDemoStringOps(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Output = os.Stderr

	// trace ["hello" upper add " WORLD"]
	e := NewTop(r)
	result, err := e.Run([]Value{
		NewWord("trace"),
		NewList([]Value{
			NewString("hello"), NewWord("upper"), NewWord("add"), NewString(" WORLD"),
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "HELLO WORLD" {
		t.Errorf("got %v, want [HELLO WORLD]", result)
	}
}

func TestTraceDemoStackOps(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Output = os.Stderr

	// trace [1 2 3 rot add mul]
	e := NewTop(r)
	result, err := e.Run([]Value{
		NewWord("trace"),
		NewList([]Value{
			NewInteger(1), NewInteger(2), NewInteger(3),
			NewWord("rot"), NewWord("add"), NewWord("mul"),
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 8 {
		t.Errorf("got %v, want [8]", result)
	}
}
