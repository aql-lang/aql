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

func TestEvalArgScalar(t *testing.T) {
	r := DefaultRegistry()
	result, err := evalCond(r, NewInteger(42))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("expected [42], got %v", result)
	}
}

func TestEvalArgList(t *testing.T) {
	r := DefaultRegistry()
	// [1 add 2] should evaluate to [3]
	list := NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	result, err := evalCond(r, list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("expected [3], got %v", result)
	}
}
