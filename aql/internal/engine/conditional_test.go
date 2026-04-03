package engine

import "testing"

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
			got := isTruthy(tt.val)
			if got != tt.want {
				t.Errorf("isTruthy(%s) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestIsTruthyMap(t *testing.T) {
	empty := NewOrderedMap()
	if isTruthy(NewMap(empty)) {
		t.Error("empty map should be falsy")
	}
	nonempty := NewOrderedMap()
	nonempty.Set("x", NewInteger(1))
	if !isTruthy(NewMap(nonempty)) {
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
