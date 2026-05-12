package engine

import (
	"strings"
	"testing"
)

// TestIfClauseList exercises the clause-list form of `if`:
//
//	if [c1 b1 c2 b2 … else]
//
// even-index elements are conditions, the following odd-index element is
// that clause's body, and a trailing element (odd-length list) is the
// else. The first truthy condition wins; later conditions are not
// evaluated. Elements may be code-body lists (evaluated / spliced) or
// plain values (used as-is).
func TestIfClauseList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	L := func(vs ...Value) Value { return NewList(vs) }
	S, B, I, W := NewString, NewBoolean, NewInteger, NewWord

	tests := []struct {
		name   string
		tokens []Value
		want   string // space-joined Value.String() of the result stack; "" = empty
	}{
		{"first-cond-truthy", []Value{W("if"), L(B(true), S("yes"))}, "'yes'"},
		{"first-cond-falsy-no-else", []Value{W("if"), L(B(false), S("yes"))}, ""},
		{"second-cond-truthy", []Value{W("if"), L(B(false), S("a"), B(true), S("b"), S("c"))}, "'b'"},
		{"none-match-else", []Value{W("if"), L(B(false), S("a"), B(false), S("b"), S("c"))}, "'c'"},
		{"none-match-no-else", []Value{W("if"), L(B(false), S("a"), B(false), S("b"))}, ""},
		{"lone-else", []Value{W("if"), L(S("only"))}, "'only'"},
		{"empty-list", []Value{W("if"), L()}, ""},
		{"truthy-int-cond", []Value{W("if"), L(I(7), S("nz"), S("z"))}, "'nz'"},
		{"zero-int-cond-falls-to-else", []Value{W("if"), L(I(0), S("nz"), S("z"))}, "'z'"},
		{"code-body-cond-first-matches", []Value{W("if"), L(
			L(I(1), W("gt"), I(0)), S("pos"),
			L(I(1), W("lt"), I(0)), S("neg"),
			S("zero"))}, "'pos'"},
		{"code-body-cond-second-matches", []Value{W("if"), L(
			L(I(1), W("lt"), I(0)), S("pos"),
			L(I(1), W("gt"), I(0)), S("neg"),
			S("zero"))}, "'neg'"},
		{"code-body-cond-none-matches", []Value{W("if"), L(
			L(I(1), W("lt"), I(0)), S("pos"),
			L(I(0), W("lt"), I(0)), S("neg"),
			S("zero"))}, "'zero'"},
		{"code-body-body-then-evaluated", []Value{W("if"), L(B(true),
			L(I(1), W("add"), I(2)), L(I(10), W("add"), I(20)))}, "3"},
		{"code-body-body-else-evaluated", []Value{W("if"), L(B(false),
			L(I(1), W("add"), I(2)), L(I(10), W("add"), I(20)))}, "30"},
		{"stack-form", []Value{L(B(true), S("x")), W("if")}, "'x'"},
		// The second condition would raise undefined_word if evaluated;
		// it isn't, because the first condition matches first.
		{"short-circuit", []Value{W("if"), L(B(true), S("first"),
			L(W("never-evaluated-word")), S("never"))}, "'first'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks := make([]Value, len(tt.tokens))
			copy(toks, tt.tokens)
			got := runAQL(t, r, toks)
			parts := make([]string, len(got))
			for i, v := range got {
				parts[i] = v.String()
			}
			if joined := strings.Join(parts, " "); joined != tt.want {
				t.Errorf("got %q, want %q", joined, tt.want)
			}
		})
	}
}

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want bool
	}{
		{"true", NewBoolean(true), true},
		{"false", NewBoolean(false), false},
		{"int_nonzero", NewInteger(42), true},
		{"int_zero", NewInteger(0), false},
		{"none", NewTypeLiteral(TNone), false},
		{"string_nonempty", NewString("hello"), true},
		{"string_true", NewString("true"), true},
		{"string_false", NewString("false"), false},
		{"string_empty", NewString(""), false},
		{"atom_nonempty", NewAtom("foo"), true},
		{"atom_true", NewAtom("true"), true},
		{"atom_false", NewAtom("false"), false},
		{"list_nonempty", NewList([]Value{NewInteger(1)}), true},
		{"list_empty", NewList([]Value{}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CoerceBoolean(tt.val)
			if got != tt.want {
				t.Errorf("CoerceBoolean(%s) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestIsTruthyMap(t *testing.T) {
	empty := NewOrderedMap()
	if CoerceBoolean(NewMap(empty)) {
		t.Error("empty map should be falsy")
	}
	nonempty := NewOrderedMap()
	nonempty.Set("x", NewInteger(1))
	if !CoerceBoolean(NewMap(nonempty)) {
		t.Error("non-empty map should be truthy")
	}
}

// TestIfListConditionMarkMove verifies that list conditions are evaluated
// via mark/move in the main engine (not a sub-engine).
func TestIfListConditionMarkMove(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// if [1 add 2 gt 2] 10 20 — condition evaluates to true (3>2)
	condList := NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2), NewWord("gt"), NewInteger(2)})
	result := runAQL(t, r, []Value{
		NewWord("if"), condList, NewInteger(10), NewInteger(20),
	})
	_as0, _ := result[0].AsInteger()
	if len(result) != 1 || _as0 != 10 {
		t.Errorf("if [1 add 2 gt 2] 10 20 = %v, want [10]", result)
	}
}

func TestIfListConditionFalseMarkMove(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// if [1 gt 2] 10 20 — condition is false
	condList := NewList([]Value{NewInteger(1), NewWord("gt"), NewInteger(2)})
	result := runAQL(t, r, []Value{
		NewWord("if"), condList, NewInteger(10), NewInteger(20),
	})
	_as1, _ := result[0].AsInteger()
	if len(result) != 1 || _as1 != 20 {
		t.Errorf("if [1 gt 2] 10 20 = %v, want [20]", result)
	}
}

func TestIfScalar2ArgTrue(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// if true 42 — 2-arg, scalar true condition
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(true), NewInteger(42),
	})
	_as2, _ := result[0].AsInteger()
	if len(result) != 1 || _as2 != 42 {
		t.Errorf("if true 42 = %v, want [42]", result)
	}
}

func TestIfScalar2ArgFalse(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// if false 42 — 2-arg, scalar false condition returns nothing
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(false), NewInteger(42),
	})
	if len(result) != 0 {
		t.Errorf("if false 42 = %v, want []", result)
	}
}

// TestIfConditionSharesContext verifies that the condition, evaluated
// via mark/move in the main engine, shares the parent's context.
func TestIfConditionSharesContext(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	condList := NewList([]Value{NewWord("context"), NewWord("get"), NewString("flag")})
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("flag"), NewBoolean(true),
		NewWord("if"), condList, NewString("yes"), NewString("no"),
	})
	_as3, _ := result[0].AsString()
	if len(result) != 1 || _as3 != "yes" {
		t.Errorf("if [context get flag] should see parent context, got %v", result)
	}
}

// TestIfConditionCanSetContext verifies that condition evaluation in the
// main engine can modify the parent's context (unlike the old sub-engine).
func TestIfConditionCanSetContext(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Condition sets a context value, then we read it after if
	condList := NewList([]Value{
		NewWord("context"), NewWord("set"), NewString("seen"), NewBoolean(true),
		NewBoolean(true),
	})
	result := runAQL(t, r, []Value{
		NewWord("if"), condList, NewInteger(1), NewInteger(2),
		NewWord("context"), NewWord("get"), NewString("seen"),
	})
	// Should return [1, true] — the if returned 1 (truthy condition),
	// and context get "seen" returns true (set during condition eval)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(result), result)
	}
	_as4, _ := result[0].AsInteger()
	if _as4 != 1 {
		t.Errorf("if branch result = %v, want 1", result[0])
	}
	_as5, _ := result[1].AsBoolean()
	if !_as5 {
		t.Errorf("context set during condition should persist, got %v", result[1])
	}
}
