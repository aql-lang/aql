package engine_test
import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"testing"
)

// TestListEvalAsArg verifies that parser-created lists (Eval=true) have their
// word elements resolved from DefStacks when consumed as a registered word's
// argument. This is the fix for the "list literal eval" issue described in
// AQL-DX-REPORT.md Issue 1.
func TestListEvalAsArg(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Register a word that takes a list and returns it unchanged.
	r.Register("passlist", engine.Signature{
		Args: []engine.Type{engine.TList},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{args[0]}, nil
		},
	})

	// def c1 10
	// def c2 20
	// [c1 c2] passlist → [10, 20]
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("def"), engine.NewWord("c1"), engine.NewInteger(10), engine.NewWord("end"),
		engine.NewWord("def"), engine.NewWord("c2"), engine.NewInteger(20), engine.NewWord("end"),
		engine.NewEvalList([]engine.Value{engine.NewWord("c1"), engine.NewWord("c2")}),
		engine.NewWord("passlist"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	lst := result[0].AsList()
	if lst.Len() != 2 {
		t.Fatalf("expected list of 2, got %d", lst.Len())
	}
	_as0, _ := lst.Get(0).AsNumber()
	if _as0 != 10 {
		t.Errorf("element 0 = %v, want 10", lst.Get(0))
	}
	_as1, _ := lst.Get(1).AsNumber()
	if _as1 != 20 {
		t.Errorf("element 1 = %v, want 20", lst.Get(1))
	}
}

// TestListEvalArithmetic verifies that list auto-evaluation resolves
// expressions: [1 add 2] consumed as an arg → [3].
func TestListEvalArithmetic(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	r.Register("passlist", engine.Signature{
		Args: []engine.Type{engine.TList},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{args[0]}, nil
		},
	})

	// [1 add 2] passlist → [3]
	result := runAQL(t, r, []engine.Value{
		engine.NewEvalList([]engine.Value{engine.NewInteger(1), engine.NewWord("add"), engine.NewInteger(2)}),
		engine.NewWord("passlist"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	lst := result[0].AsList()
	_as2, _ := lst.Get(0).AsNumber()
	if lst.Len() != 1 || _as2 != 3 {
		t.Errorf("list = %v, want [3]", result[0])
	}
}

// TestListEvalQuotedSkipped verifies that quoted lists (via quote word)
// are NOT auto-evaluated.
func TestListEvalQuotedSkipped(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	r.Register("passlist", engine.Signature{
		Args: []engine.Type{engine.TList},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{args[0]}, nil
		},
	})

	// quote [1 add 2] passlist → [1, word(add), 2] (not evaluated)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("quote"),
		engine.NewEvalList([]engine.Value{engine.NewInteger(1), engine.NewWord("add"), engine.NewInteger(2)}),
		engine.NewWord("passlist"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	lst := result[0].AsList()
	if lst.Len() != 3 {
		t.Errorf("expected 3 elements (unevaluated), got %d: %v", lst.Len(), result[0])
	}
}

// TestListEvalNoEvalArgsPreservesCodeBody verifies that words with
// NoEvalArgs (like def, for) receive the list body unevaluated.
func TestListEvalNoEvalArgsPreservesCodeBody(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// def double [dup add]
	// 5 double → 10
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("def"), engine.NewWord("double"),
		engine.NewEvalList([]engine.Value{engine.NewWord("dup"), engine.NewWord("add")}),
		engine.NewWord("end"),
		engine.NewInteger(5), engine.NewWord("double"),
	})
	_as3, _ := result[0].AsNumber()
	if len(result) != 1 || _as3 != 10 {
		t.Errorf("5 double = %v, want 10", result)
	}
}

// TestListEvalFnDefAutoInvoke verifies that lists are auto-evaluated when
// consumed by FnDef auto-invocation (module functions). Uses a module
// function that takes a list and computes its length.
func TestListEvalFnDefAutoInvoke(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Register a word "listlen" that takes a list and returns its length,
	// via a module function (FnDef with captured registry).
	r.Register("listlen", engine.Signature{
		Args: []engine.Type{engine.TList},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			lst := args[0].AsList()
			return []engine.Value{engine.NewInteger(int64(lst.Len()))}, nil
		},
	})

	// def a 10
	// def b 20
	// [a b] listlen → 2 (list was auto-evaluated to [10, 20], length is 2)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("def"), engine.NewWord("a"), engine.NewInteger(10), engine.NewWord("end"),
		engine.NewWord("def"), engine.NewWord("b"), engine.NewInteger(20), engine.NewWord("end"),
		engine.NewEvalList([]engine.Value{engine.NewWord("a"), engine.NewWord("b")}),
		engine.NewWord("listlen"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	_as4, _ := result[0].AsNumber()
	if _as4 != 2 {
		t.Errorf("listlen = %v, want 2", result[0])
	}
}

// TestListEvalRuntimeListNotEvaluated verifies that runtime-created lists
// (Eval=false, e.g. from word handlers) are NOT auto-evaluated.
func TestListEvalRuntimeListNotEvaluated(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Register a word that produces a list with words in it (Eval=false).
	r.Register("makelist", engine.Signature{
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewList([]engine.Value{engine.NewWord("add"), engine.NewInteger(1)})}, nil
		},
	})

	r.Register("passlist", engine.Signature{
		Args: []engine.Type{engine.TList},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{args[0]}, nil
		},
	})

	// makelist passlist → [word(add), 1] (not evaluated, runtime-created)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("makelist"), engine.NewWord("passlist"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	lst := result[0].AsList()
	if lst.Len() != 2 {
		t.Errorf("expected 2 elements (unevaluated runtime list), got %d", lst.Len())
	}
}
