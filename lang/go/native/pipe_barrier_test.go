package native

import (
	"testing"
)

// TestPipeBarrierBasic verifies the | syntax in fn signatures.
// def f fn [[Integer | String] [String] [body]] means:
// - Position 0 (Integer) collected forward
// - Position 1 (String) matched from stack (after the barrier)
// So "a" f 1 works (1 forward, "a" from stack) and
// "a" 1 f works (both from stack), but f "a" 1 does NOT match
// because "a" would go to position 0 (Integer) — type mismatch.
func TestPipeBarrierFnDef(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// def f fn [[Integer | String] [String] [add]]
	pairI := NewOrderedMap()
	pairI.Set("n", NewTypeLiteral(TInteger))
	fnBody := NewList([]Value{
		// Signature: [Integer | String] — barrier at position 1
		NewList([]Value{NewTypeLiteral(TInteger), NewWord("|"), NewTypeLiteral(TString)}),
		NewList([]Value{NewTypeLiteral(TString)}),
		// Body: convert the integer to string then concatenate
		NewList([]Value{NewWord("convert"), NewWord("String"), NewWord("add")}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("f"),
		NewWord("fn"), fnBody, NewEnd(),
	})

	// "hello" f 42 → "hello" is on stack, f forward-collects 42.
	// Barrier stops at position 1 → String from stack.
	// f(42, "hello") → "42" add "hello" → "42hello"
	result := runAQL(t, r, []Value{
		NewString("hello"), NewWord("f"), NewInteger(42),
	})
	_as0, _ := AsString(result[0])
	if len(result) != 1 || _as0 != "42hello" {
		t.Errorf(`"hello" f 42 = %v, want "42hello"`, result)
	}

	// "world" 7 f → both on stack, reversed: top=7→sig[0], next="world"→sig[1]
	result = runAQL(t, r, []Value{
		NewString("world"), NewInteger(7), NewWord("f"),
	})
	_as1, _ := AsString(result[0])
	if len(result) != 1 || _as1 != "7world" {
		t.Errorf(`"world" 7 f = %v, want "7world"`, result)
	}
}

// TestPipeBarrierPreventsGreedyForward verifies that the barrier
// prevents get from greedily consuming a second map as forward arg.
func TestPipeBarrierPreventsGreedyForward(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Simulate module access: module_map get key module_map get key
	m1 := NewOrderedMap()
	m1.Set("fn1", NewString("result1"))

	m2 := NewOrderedMap()
	m2.Set("fn2", NewString("result2"))

	// m1 get fn1 m2 get fn2
	// Without barrier: get would forward-collect fn1 AND m2 (both match).
	// With barrier: get collects fn1 forward, gets m1 from stack.
	result := runAQL(t, r, []Value{
		NewMap(m1), NewWord("get"), NewWord("fn1"),
		NewMap(m2), NewWord("get"), NewWord("fn2"),
	})
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(result), result)
	}
	_as2, _ := AsString(result[0])
	if _as2 != "result1" {
		t.Errorf("first get = %v, want 'result1'", result[0])
	}
	_as3, _ := AsString(result[1])
	if _as3 != "result2" {
		t.Errorf("second get = %v, want 'result2'", result[1])
	}
}

// TestPipeBarrierSortOrder verifies that piped signatures sort before
// non-piped signatures of equal arity.
func TestPipeBarrierSortOrder(t *testing.T) {
	piped := Signature{Args: []*Type{TAtom, TNode}, BarrierPos: 1}
	plain := Signature{Args: []*Type{TAtom, TNode}}
	if c := CompareSignatures(&piped, &plain); c >= 0 {
		t.Errorf("piped sig should sort before plain sig, got cmp=%d", c)
	}
}
