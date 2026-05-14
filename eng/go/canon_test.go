package eng

import "testing"

func TestCanonValue(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	m.Set("b", NewAtom("foo"))

	quotedList := NewList([]Value{NewInteger(1), NewInteger(2)})
	quotedList.Quoted = true

	cases := []struct {
		name string
		v    Value
		want string
	}{
		{"none", NewNone(), "none"},
		{"integer", NewInteger(42), "42"},
		{"negative integer", NewInteger(-7), "-7"},
		{"decimal", NewDecimal(3.14), "3.14"},
		{"whole decimal", NewDecimal(7), "7.0"},
		{"string", NewString("hello"), "'hello'"},
		{"true", NewBoolean(true), "true"},
		{"false", NewBoolean(false), "false"},
		{"atom", NewAtom("foo"), "(quote foo)"},
		{"empty list", NewList(nil), "[]"},
		{"list of ints", NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}), "[1 2 3]"},
		{"list with atom", NewList([]Value{NewInteger(1), NewAtom("foo")}), "[1 (quote foo)]"},
		{"nested list", NewList([]Value{NewList([]Value{NewInteger(1)}), NewList([]Value{NewInteger(2)})}), "[[1] [2]]"},
		{"quoted list", quotedList, "(quote [1 2])"},
		{"map", NewMap(m), "{a:1 b:(quote foo)}"},
		{"type literal", NewTypeLiteral(TInteger), "Integer"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CanonValue(tc.v)
			if got != tc.want {
				t.Errorf("CanonValue(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestCanonStack(t *testing.T) {
	stack := []Value{
		NewInteger(1),
		NewAtom("foo"),
		NewString("bar"),
		NewBoolean(true),
	}
	got := Canon(stack)
	want := "1 (quote foo) 'bar' true"
	if got != want {
		t.Errorf("Canon = %q, want %q", got, want)
	}
}

func TestCanonEmptyStack(t *testing.T) {
	got := Canon(nil)
	if got != "" {
		t.Errorf("Canon(nil) = %q, want empty string", got)
	}
}
