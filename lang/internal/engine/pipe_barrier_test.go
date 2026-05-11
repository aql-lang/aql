package engine_test

import (
	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"github.com/metsitaba/voxgig-exp/lang/internal/native"
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// def f fn [[Integer | String] [String] [add]]
	pairI := engine.NewOrderedMap()
	pairI.Set("n", engine.NewTypeLiteral(engine.TInteger))
	fnBody := engine.NewList([]engine.Value{
		// Signature: [Integer | String] — barrier at position 1
		engine.NewList([]engine.Value{engine.NewTypeLiteral(engine.TInteger), engine.NewWord("|"), engine.NewTypeLiteral(engine.TString)}),
		engine.NewList([]engine.Value{engine.NewTypeLiteral(engine.TString)}),
		// Body: convert the integer to string then concatenate
		engine.NewList([]engine.Value{engine.NewWord("convert"), engine.NewWord("String"), engine.NewWord("add")}),
	})
	runAQL(t, r, []engine.Value{
		engine.NewWord("def"), engine.NewWord("f"),
		engine.NewWord("fn"), fnBody, engine.NewWord("end"),
	})

	// "hello" f 42 → "hello" is on stack, f forward-collects 42.
	// Barrier stops at position 1 → String from stack.
	// f(42, "hello") → "42" add "hello" → "42hello"
	result := runAQL(t, r, []engine.Value{
		engine.NewString("hello"), engine.NewWord("f"), engine.NewInteger(42),
	})
	_as0, _ := result[0].AsString()
	if len(result) != 1 || _as0 != "42hello" {
		t.Errorf(`"hello" f 42 = %v, want "42hello"`, result)
	}

	// "world" 7 f → both on stack, reversed: top=7→sig[0], next="world"→sig[1]
	result = runAQL(t, r, []engine.Value{
		engine.NewString("world"), engine.NewInteger(7), engine.NewWord("f"),
	})
	_as1, _ := result[0].AsString()
	if len(result) != 1 || _as1 != "7world" {
		t.Errorf(`"world" 7 f = %v, want "7world"`, result)
	}
}

// TestPipeBarrierPreventsGreedyForward verifies that the barrier
// prevents get from greedily consuming a second map as forward arg.
func TestPipeBarrierPreventsGreedyForward(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate module access: module_map get key module_map get key
	m1 := engine.NewOrderedMap()
	m1.Set("fn1", engine.NewString("result1"))

	m2 := engine.NewOrderedMap()
	m2.Set("fn2", engine.NewString("result2"))

	// m1 get fn1 m2 get fn2
	// Without barrier: get would forward-collect fn1 AND m2 (both match).
	// With barrier: get collects fn1 forward, gets m1 from stack.
	result := runAQL(t, r, []engine.Value{
		engine.NewMap(m1), engine.NewWord("get"), engine.NewWord("fn1"),
		engine.NewMap(m2), engine.NewWord("get"), engine.NewWord("fn2"),
	})
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(result), result)
	}
	_as2, _ := result[0].AsString()
	if _as2 != "result1" {
		t.Errorf("first get = %v, want 'result1'", result[0])
	}
	_as3, _ := result[1].AsString()
	if _as3 != "result2" {
		t.Errorf("second get = %v, want 'result2'", result[1])
	}
}

// TestPipeBarrierSortOrder verifies that piped signatures sort before
// non-piped signatures of equal arity.
func TestPipeBarrierSortOrder(t *testing.T) {
	piped := engine.Signature{Args: []engine.Type{engine.TAtom, engine.TNode}, BarrierPos: 1}
	plain := engine.Signature{Args: []engine.Type{engine.TAtom, engine.TNode}}
	if engine.SignatureScore(&piped) <= engine.SignatureScore(&plain) {
		t.Errorf("piped score %d should be > plain score %d",
			engine.SignatureScore(&piped), engine.SignatureScore(&plain))
	}
}
