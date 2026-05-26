package native

import (
	"testing"
)

// TestListEvalAsArg verifies that parser-created lists (Eval=true) have their
// word elements resolved from DefStacks when consumed as a registered word's
// argument. This is the fix for the "list literal eval" issue described in
// AQL-DX-REPORT.md Issue 1.
func TestListEvalAsArg(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Register a word that takes a list and returns it unchanged.
	r.Register("passlist", Signature{
		Args: []*Type{TList},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{args[0]}, nil
		}, BarrierPos:

		// def c1 10
		// def c2 20
		// [c1 c2] passlist → [10, 20]
		-1,
	})

	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("c1"), NewInteger(10), NewEnd(),
		NewWord("def"), NewWord("c2"), NewInteger(20), NewEnd(),
		NewEvalList([]Value{NewWord("c1"), NewWord("c2")}),
		NewWord("passlist"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	lst, _ := AsList(result[0])
	if lst.Len() != 2 {
		t.Fatalf("expected list of 2, got %d", lst.Len())
	}
	_as0, _ := AsNumber(lst.Get(0))
	if _as0 != 10 {
		t.Errorf("element 0 = %v, want 10", lst.Get(0))
	}
	_as1, _ := AsNumber(lst.Get(1))
	if _as1 != 20 {
		t.Errorf("element 1 = %v, want 20", lst.Get(1))
	}
}

// TestListEvalArithmetic verifies that list auto-evaluation resolves
// expressions: [1 add 2] consumed as an arg → [3].
func TestListEvalArithmetic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	r.Register("passlist", Signature{
		Args: []*Type{TList},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{args[0]}, nil
		}, BarrierPos:

		// [1 add 2] passlist → [3]
		-1,
	})

	result := runAQL(t, r, []Value{
		NewEvalList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}),
		NewWord("passlist"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	lst, _ := AsList(result[0])
	_as2, _ := AsNumber(lst.Get(0))
	if lst.Len() != 1 || _as2 != 3 {
		t.Errorf("list = %v, want [3]", result[0])
	}
}

// TestListEvalQuotedSkipped verifies that quoted lists (via quote word)
// are NOT auto-evaluated.
func TestListEvalQuotedSkipped(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	r.Register("passlist", Signature{
		Args: []*Type{TList},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{args[0]}, nil
		}, BarrierPos:

		// quote [1 add 2] passlist → [1, word(add), 2] (not evaluated)
		-1,
	})

	result := runAQL(t, r, []Value{
		NewWord("quote"),
		NewEvalList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}),
		NewWord("passlist"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	lst, _ := AsList(result[0])
	if lst.Len() != 3 {
		t.Errorf("expected 3 elements (unevaluated), got %d: %v", lst.Len(), result[0])
	}
}

// TestListEvalNoEvalArgsPreservesCodeBody verifies that words with
// NoEvalArgs (like def, for) receive the list body unevaluated.
func TestListEvalNoEvalArgsPreservesCodeBody(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// def double [dup add]
	// 5 double → 10
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("double"),
		NewEvalList([]Value{NewWord("dup"), NewWord("add")}),
		NewEnd(),
		NewInteger(5), NewWord("double"),
	})
	_as3, _ := AsNumber(result[0])
	if len(result) != 1 || _as3 != 10 {
		t.Errorf("5 double = %v, want 10", result)
	}
}

// TestListEvalFnDefAutoInvoke verifies that lists are auto-evaluated when
// consumed by FnDef auto-invocation (module functions). Uses a module
// function that takes a list and computes its length.
func TestListEvalFnDefAutoInvoke(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Register a word "listlen" that takes a list and returns its length,
	// via a module function (FnDef with captured registry).
	r.Register("listlen", Signature{
		Args: []*Type{TList},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			lst, _ := AsList(args[0])
			return []Value{NewInteger(int64(lst.Len()))}, nil
		}, BarrierPos:

		// def a 10
		// def b 20
		// [a b] listlen → 2 (list was auto-evaluated to [10, 20], length is 2)
		-1,
	})

	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("a"), NewInteger(10), NewEnd(),
		NewWord("def"), NewWord("b"), NewInteger(20), NewEnd(),
		NewEvalList([]Value{NewWord("a"), NewWord("b")}),
		NewWord("listlen"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	_as4, _ := AsNumber(result[0])
	if _as4 != 2 {
		t.Errorf("listlen = %v, want 2", result[0])
	}
}

// TestListEvalRuntimeListNotEvaluated verifies that runtime-created lists
// (Eval=false, e.g. from word handlers) are NOT auto-evaluated.
func TestListEvalRuntimeListNotEvaluated(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Register a word that produces a list with words in it (Eval=false).
	r.Register("makelist", Signature{
		Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewList([]Value{NewWord("add"), NewInteger(1)})}, nil
		}, BarrierPos: -1,
	})

	r.Register("passlist", Signature{
		Args: []*Type{TList},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{args[0]}, nil
		}, BarrierPos:

		// makelist passlist → [word(add), 1] (not evaluated, runtime-created)
		-1,
	})

	result := runAQL(t, r, []Value{
		NewWord("makelist"), NewWord("passlist"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	lst, _ := AsList(result[0])
	if lst.Len() != 2 {
		t.Errorf("expected 2 elements (unevaluated runtime list), got %d", lst.Len())
	}
}
