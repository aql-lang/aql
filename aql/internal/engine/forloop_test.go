package engine

import (
	"bytes"
	"strings"
	"testing"
)

// TestForCount tests for with an integer count: for 3 [print i]
func TestForCount(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	// for 3 [print i] → prints "0\n1\n2\n"
	result := runAQL(t, r, []Value{
		NewWord("for"),
		NewInteger(3),
		NewList([]Value{NewWord("print"), NewWord("i")}),
	})

	got := buf.String()
	if got != "0\n1\n2\n" {
		t.Errorf("for 3 [print i]: got %q, want %q", got, "0\n1\n2\n")
	}

	// for consumes both args and the body produces no stack output (print returns nil)
	if len(result) != 0 {
		t.Errorf("expected empty stack, got %d items: %v", len(result), result)
	}
}

// TestForRange tests for with a [start, end] range.
func TestForRange(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	// for [2,5] [print i] → prints "2\n3\n4\n"
	result := runAQL(t, r, []Value{
		NewWord("for"),
		NewList([]Value{NewInteger(2), NewInteger(5)}),
		NewList([]Value{NewWord("print"), NewWord("i")}),
	})

	got := buf.String()
	if got != "2\n3\n4\n" {
		t.Errorf("for [2,5] [print i]: got %q, want %q", got, "2\n3\n4\n")
	}

	if len(result) != 0 {
		t.Errorf("expected empty stack, got %d items: %v", len(result), result)
	}
}

// TestForRangeStep tests for with a [start, end, step] range.
func TestForRangeStep(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	// for [0,10,3] [print i] → prints "0\n3\n6\n9\n"
	result := runAQL(t, r, []Value{
		NewWord("for"),
		NewList([]Value{NewInteger(0), NewInteger(10), NewInteger(3)}),
		NewList([]Value{NewWord("print"), NewWord("i")}),
	})

	got := buf.String()
	if got != "0\n3\n6\n9\n" {
		t.Errorf("for [0,10,3] [print i]: got %q, want %q", got, "0\n3\n6\n9\n")
	}

	if len(result) != 0 {
		t.Errorf("expected empty stack, got %d items", len(result))
	}
}

// TestForZeroIterations tests that for with 0 count produces nothing.
func TestForZeroIterations(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	result := runAQL(t, r, []Value{
		NewWord("for"),
		NewInteger(0),
		NewList([]Value{NewWord("print"), NewString("x")}),
	})

	if buf.String() != "" {
		t.Errorf("for 0 should produce no output, got %q", buf.String())
	}
	if len(result) != 0 {
		t.Errorf("expected empty stack, got %d items", len(result))
	}
}

// TestForBodyAccumulates tests that body results accumulate on the stack.
func TestForBodyAccumulates(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// for 3 [i] → each iteration pushes i to results → [0, 1, 2]
	result := runAQL(t, r, []Value{
		NewWord("for"),
		NewInteger(3),
		NewList([]Value{NewWord("i")}),
	})

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d: %v", len(result), result)
	}
	for idx, want := range []int64{0, 1, 2} {
		if result[idx].AsInteger() != want {
			t.Errorf("result[%d] = %v, want %d", idx, result[idx], want)
		}
	}
}

// TestForPrintstr tests the example from the spec: for 3 [printstr "x"] → "xxx"
func TestForPrintstr(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	runAQL(t, r, []Value{
		NewWord("for"),
		NewInteger(3),
		NewList([]Value{NewWord("printstr"), NewString("x")}),
	})

	if buf.String() != "xxx" {
		t.Errorf("for 3 [printstr x]: got %q, want %q", buf.String(), "xxx")
	}
}

// TestForBreak tests that break exits the loop early.
func TestForBreak(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	// for 10 [(if [i eq 3] [break]) print i]
	// Use parens to scope the if so print i is not captured as its else branch.
	// Should print 0, 1, 2 then break.
	runAQL(t, r, []Value{
		NewWord("for"),
		NewInteger(10),
		NewList([]Value{
			NewWord("("),
			NewWord("if"),
			NewList([]Value{NewWord("i"), NewWord("eq"), NewInteger(3)}),
			NewList([]Value{NewWord("break")}),
			NewWord(")"),
			NewWord("print"),
			NewWord("i"),
		}),
	})

	got := buf.String()
	if got != "0\n1\n2\n" {
		t.Errorf("for with break: got %q, want %q", got, "0\n1\n2\n")
	}
}

// TestForContinue tests that continue skips to the next iteration.
func TestForContinue(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	// for 5 [(if [i eq 2] [continue]) print i]
	// Should print 0, 1, 3, 4 (skip 2)
	runAQL(t, r, []Value{
		NewWord("for"),
		NewInteger(5),
		NewList([]Value{
			NewWord("("),
			NewWord("if"),
			NewList([]Value{NewWord("i"), NewWord("eq"), NewInteger(2)}),
			NewList([]Value{NewWord("continue")}),
			NewWord(")"),
			NewWord("print"),
			NewWord("i"),
		}),
	})

	got := buf.String()
	if got != "0\n1\n3\n4\n" {
		t.Errorf("for with continue: got %q, want %q", got, "0\n1\n3\n4\n")
	}
}

// TestForStepZero tests that step=0 is an error.
func TestForStepZero(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	err = runAQLError(t, r, []Value{
		NewWord("for"),
		NewList([]Value{NewInteger(0), NewInteger(10), NewInteger(0)}),
		NewList([]Value{NewWord("i")}),
	})
	if err == nil {
		t.Fatal("expected error for step=0")
	}
	if !strings.Contains(err.Error(), "step") {
		t.Errorf("error should mention step: %v", err)
	}
}

// TestForNegativeStep tests counting down with a negative step.
func TestForNegativeStep(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	// for [5,0,-1] [print i] → prints "5\n4\n3\n2\n1\n"
	result := runAQL(t, r, []Value{
		NewWord("for"),
		NewList([]Value{NewInteger(5), NewInteger(0), NewInteger(-1)}),
		NewList([]Value{NewWord("print"), NewWord("i")}),
	})

	got := buf.String()
	if got != "5\n4\n3\n2\n1\n" {
		t.Errorf("for [5,0,-1] [print i]: got %q, want %q", got, "5\n4\n3\n2\n1\n")
	}

	if len(result) != 0 {
		t.Errorf("expected empty stack, got %d items", len(result))
	}
}

// TestForNoStackGrowth verifies the loop doesn't expand the body N times.
// A large iteration count should work without hitting step limits.
func TestForNoStackGrowth(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	// for 1000 [printstr ""] — 1000 iterations, body is tiny
	result := runAQL(t, r, []Value{
		NewWord("for"),
		NewInteger(1000),
		NewList([]Value{NewWord("printstr"), NewString("")}),
	})

	if len(result) != 0 {
		t.Errorf("expected empty stack, got %d items", len(result))
	}
}

// TestForIteratorScoping tests that the iterator doesn't leak after the loop.
func TestForIteratorScoping(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// After "for 3 [i]", the word "i" should resolve to atom (not integer)
	result := runAQL(t, r, []Value{
		NewWord("for"),
		NewInteger(3),
		NewList([]Value{NewWord("i")}),
		NewWord("i"),
	})

	// First 3 results are 0,1,2 from the loop; 4th is atom "i"
	if len(result) != 4 {
		t.Fatalf("expected 4 results, got %d: %v", len(result), result)
	}
	last := result[3]
	if !last.IsAtom() {
		t.Errorf("iterator 'i' should be atom after loop, got type %s", last.VType)
	}
}

// TestParseRange tests the range parser directly.
func TestParseRange(t *testing.T) {
	tests := []struct {
		name        string
		elems       []Value
		start, end  int64
		step        int64
		expectError bool
	}{
		{"single", []Value{NewInteger(5)}, 0, 5, 1, false},
		{"two", []Value{NewInteger(2), NewInteger(8)}, 2, 8, 1, false},
		{"three", []Value{NewInteger(1), NewInteger(10), NewInteger(2)}, 1, 10, 2, false},
		{"empty", []Value{}, 0, 0, 0, true},
		{"four", []Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4)}, 0, 0, 0, true},
		{"non-integer", []Value{NewString("x")}, 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, step, err := parseRange(tt.elems)
			if tt.expectError {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if start != tt.start || end != tt.end || step != tt.step {
				t.Errorf("got (%d,%d,%d), want (%d,%d,%d)", start, end, step, tt.start, tt.end, tt.step)
			}
		})
	}
}
