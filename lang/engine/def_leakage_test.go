package engine_test

import (
	"github.com/aql-lang/aql/lang/engine"
	"github.com/aql-lang/aql/lang/native"
	"testing"
)

// TestDefLeakageFromCallAQL verifies that local defs inside fn bodies
// executed via CallAQL do not persist after the fn returns.
// This is the fix for AQL-DX-REPORT Issue 2.
func TestDefLeakageFromCallAQL(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Define a fn that creates a local def inside its body:
	// def myfn fn [[x:Integer] [Integer] [def localvar 99 x add localvar]]
	pairX := engine.NewOrderedMap()
	pairX.Set("x", engine.NewTypeLiteral(engine.TInteger))
	fnBody := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewImplicitMap(pairX)}),
		engine.NewList([]engine.Value{engine.NewTypeLiteral(engine.TInteger)}),
		engine.NewList([]engine.Value{
			engine.NewWord("def"), engine.NewWord("localvar"), engine.NewInteger(99), engine.NewEnd(),
			engine.NewWord("x"), engine.NewWord("add"), engine.NewWord("localvar"),
		}),
	})
	runAQL(t, r, []engine.Value{
		engine.NewWord("def"), engine.NewWord("myfn"),
		engine.NewWord("fn"), fnBody, engine.NewEnd(),
	})

	// Call the fn: 1 myfn → 100
	result := runAQL(t, r, []engine.Value{engine.NewInteger(1), engine.NewWord("myfn")})
	_as0, _ := result[0].AsNumber()
	if len(result) != 1 || _as0 != 100 {
		t.Errorf("1 myfn = %v, want 100", result)
	}

	// Verify 'localvar' does NOT leak into DefStacks after fn returns.
	if r.Defs.Has("localvar") {
		t.Errorf("localvar leaked into DefStacks (depth=%d)", r.Defs.Depth("localvar"))
	}
}

// TestDefLeakageDotNotation verifies that a local def inside a fn body
// does not shadow dot-notation key lookups after the fn returns.
// This was the specific symptom described in the DX report: a fn with
// def op (...) would cause later c.op to resolve 'op' from leaked def
// instead of as a map key.
func TestDefLeakageDotNotation(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Define a fn that creates a local def named 'op':
	// def process fn [[m:Map] [String] [def op (m.op) op]]
	pairM := engine.NewOrderedMap()
	pairM.Set("m", engine.NewTypeLiteral(engine.TMap))
	fnBody := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewImplicitMap(pairM)}),
		engine.NewList([]engine.Value{engine.NewTypeLiteral(engine.TString)}),
		engine.NewList([]engine.Value{
			engine.NewWord("def"), engine.NewWord("op"),
			engine.NewOpenParen(), engine.NewWord("m"), engine.NewWord("get"), engine.NewWord("op"), engine.NewCloseParen(),
			engine.NewEnd(),
			engine.NewWord("op"),
		}),
	})
	runAQL(t, r, []engine.Value{
		engine.NewWord("def"), engine.NewWord("process"),
		engine.NewWord("fn"), fnBody, engine.NewEnd(),
	})

	// Build a map {op:"add"} and call process.
	m := engine.NewOrderedMap()
	m.Set("op", engine.NewString("add"))
	result := runAQL(t, r, []engine.Value{engine.NewMap(m), engine.NewWord("process")})
	_as1, _ := result[0].AsString()
	if len(result) != 1 || _as1 != "add" {
		t.Errorf("{op:'add'} process = %v, want 'add'", result)
	}

	// 'op' must not leak. Verify DefStacks is clean.
	if r.Defs.Has("op") {
		t.Errorf("'op' leaked into DefStacks after process returned (depth=%d)", r.Defs.Depth("op"))
	}

	// A subsequent dot-notation access on a different map should work:
	// {op:"mul"}.op → "mul" (not the leaked "add")
	m2 := engine.NewOrderedMap()
	m2.Set("op", engine.NewString("mul"))
	result2 := runAQL(t, r, []engine.Value{
		engine.NewMap(m2), engine.NewWord("get"), engine.NewWord("op"),
	})
	_as2, _ := result2[0].AsString()
	if len(result2) != 1 || _as2 != "mul" {
		t.Errorf("{op:'mul'} get op = %v, want 'mul'", result2)
	}
}

// TestDefLeakageMultipleCalls verifies that repeated fn calls don't
// accumulate leaked defs.
func TestDefLeakageMultipleCalls(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// def counter fn [[n:Integer] [Integer] [def tmp (n add 1) tmp]]
	pairN := engine.NewOrderedMap()
	pairN.Set("n", engine.NewTypeLiteral(engine.TInteger))
	fnBody := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewImplicitMap(pairN)}),
		engine.NewList([]engine.Value{engine.NewTypeLiteral(engine.TInteger)}),
		engine.NewList([]engine.Value{
			engine.NewWord("def"), engine.NewWord("tmp"),
			engine.NewOpenParen(), engine.NewWord("n"), engine.NewWord("add"), engine.NewInteger(1), engine.NewCloseParen(),
			engine.NewEnd(),
			engine.NewWord("tmp"),
		}),
	})
	runAQL(t, r, []engine.Value{
		engine.NewWord("def"), engine.NewWord("counter"),
		engine.NewWord("fn"), fnBody, engine.NewEnd(),
	})

	// Call multiple times — tmp should never accumulate.
	for i := 0; i < 5; i++ {
		result := runAQL(t, r, []engine.Value{engine.NewInteger(int64(i)), engine.NewWord("counter")})
		expected := int64(i + 1)
		_as3, _ := result[0].AsNumber()
		if len(result) != 1 || _as3 != float64(expected) {
			t.Errorf("call %d: counter(%d) = %v, want %d", i, i, result, expected)
		}
	}

	if d := r.Defs.Depth("tmp"); d > 0 {
		t.Errorf("tmp leaked after %d calls: stack len = %d", 5, d)
	}
}
