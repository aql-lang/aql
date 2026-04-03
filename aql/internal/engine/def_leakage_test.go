package engine

import (
	"testing"
)

// TestDefLeakageFromCallAQL verifies that local defs inside fn bodies
// executed via CallAQL do not persist after the fn returns.
// This is the fix for AQL-DX-REPORT Issue 2.
func TestDefLeakageFromCallAQL(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Define a fn that creates a local def inside its body:
	// def myfn fn [[x:Integer] [Integer] [def localvar 99 x add localvar]]
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TInteger))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX)}),
		NewList([]Value{NewTypeLiteral(TInteger)}),
		NewList([]Value{
			NewWord("def"), NewWord("localvar"), NewInteger(99), NewWord("end"),
			NewWord("x"), NewWord("add"), NewWord("localvar"),
		}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("myfn"),
		NewWord("fn"), fnBody, NewWord("end"),
	})

	// Call the fn: 1 myfn → 100
	result := runAQL(t, r, []Value{NewInteger(1), NewWord("myfn")})
	if len(result) != 1 || result[0].AsNumber() != 100 {
		t.Errorf("1 myfn = %v, want 100", result)
	}

	// Verify 'localvar' does NOT leak into DefStacks after fn returns.
	if stack := r.DefStacks["localvar"]; len(stack) > 0 {
		t.Errorf("localvar leaked into DefStacks: %v", stack)
	}
}

// TestDefLeakageDotNotation verifies that a local def inside a fn body
// does not shadow dot-notation key lookups after the fn returns.
// This was the specific symptom described in the DX report: a fn with
// def op (...) would cause later c.op to resolve 'op' from leaked def
// instead of as a map key.
func TestDefLeakageDotNotation(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Define a fn that creates a local def named 'op':
	// def process fn [[m:Map] [String] [def op (m.op) op]]
	pairM := NewOrderedMap()
	pairM.Set("m", NewTypeLiteral(TMap))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairM)}),
		NewList([]Value{NewTypeLiteral(TString)}),
		NewList([]Value{
			NewWord("def"), NewWord("op"),
			NewWord("("), NewWord("m"), NewWord("get"), NewWord("op"), NewWord(")"),
			NewWord("end"),
			NewWord("op"),
		}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("process"),
		NewWord("fn"), fnBody, NewWord("end"),
	})

	// Build a map {op:"add"} and call process.
	m := NewOrderedMap()
	m.Set("op", NewString("add"))
	result := runAQL(t, r, []Value{NewMap(m), NewWord("process")})
	if len(result) != 1 || result[0].AsString() != "add" {
		t.Errorf("{op:'add'} process = %v, want 'add'", result)
	}

	// 'op' must not leak. Verify DefStacks is clean.
	if stack := r.DefStacks["op"]; len(stack) > 0 {
		t.Errorf("'op' leaked into DefStacks after process returned: %v", stack)
	}

	// A subsequent dot-notation access on a different map should work:
	// {op:"mul"}.op → "mul" (not the leaked "add")
	m2 := NewOrderedMap()
	m2.Set("op", NewString("mul"))
	result2 := runAQL(t, r, []Value{
		NewMap(m2), NewWord("get"), NewWord("op"),
	})
	if len(result2) != 1 || result2[0].AsString() != "mul" {
		t.Errorf("{op:'mul'} get op = %v, want 'mul'", result2)
	}
}

// TestDefLeakageMultipleCalls verifies that repeated fn calls don't
// accumulate leaked defs.
func TestDefLeakageMultipleCalls(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// def counter fn [[n:Integer] [Integer] [def tmp (n add 1) tmp]]
	pairN := NewOrderedMap()
	pairN.Set("n", NewTypeLiteral(TInteger))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairN)}),
		NewList([]Value{NewTypeLiteral(TInteger)}),
		NewList([]Value{
			NewWord("def"), NewWord("tmp"),
			NewWord("("), NewWord("n"), NewWord("add"), NewInteger(1), NewWord(")"),
			NewWord("end"),
			NewWord("tmp"),
		}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("counter"),
		NewWord("fn"), fnBody, NewWord("end"),
	})

	// Call multiple times — tmp should never accumulate.
	for i := 0; i < 5; i++ {
		result := runAQL(t, r, []Value{NewInteger(int64(i)), NewWord("counter")})
		expected := int64(i + 1)
		if len(result) != 1 || result[0].AsNumber() != float64(expected) {
			t.Errorf("call %d: counter(%d) = %v, want %d", i, i, result, expected)
		}
	}

	if stack := r.DefStacks["tmp"]; len(stack) > 0 {
		t.Errorf("tmp leaked after %d calls: stack len = %d", 5, len(stack))
	}
}
