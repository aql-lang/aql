package serve

import (
	"reflect"
	"testing"
)

func TestSplitSegmentsEmpty(t *testing.T) {
	if got := splitSegments(nil); len(got) != 0 {
		t.Errorf("nil args: got %v, want empty", got)
	}
	if got := splitSegments([]string{}); len(got) != 0 {
		t.Errorf("empty args: got %v, want empty", got)
	}
}

func TestSplitSegmentsSingle(t *testing.T) {
	got := splitSegments([]string{"repl"})
	want := [][]string{{"repl"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitSegmentsMultiple(t *testing.T) {
	got := splitSegments([]string{"registry", "-r", "./mods", "-p", "8080", "+", "lsp", "-p", "9000"})
	want := [][]string{
		{"registry", "-r", "./mods", "-p", "8080"},
		{"lsp", "-p", "9000"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitSegmentsThree(t *testing.T) {
	got := splitSegments([]string{"repl", "+", "registry", "-r", "./mods", "+", "lsp", "-p", "9000"})
	want := [][]string{
		{"repl"},
		{"registry", "-r", "./mods"},
		{"lsp", "-p", "9000"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitSegmentsDropsEmpty(t *testing.T) {
	// Leading, trailing, and doubled "+" tokens should not produce
	// empty segments.
	got := splitSegments([]string{"+", "repl", "+", "+", "lsp", "+"})
	want := [][]string{{"repl"}, {"lsp"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
