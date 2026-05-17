package eng

import (
	"reflect"
	"testing"
)

func TestToGoScalars(t *testing.T) {
	cases := []struct {
		name string
		in   Value
		want any
	}{
		{"string", NewString("hi"), "hi"},
		{"integer", NewInteger(42), int64(42)},
		{"decimal", NewDecimal(3.14), float64(3.14)},
		{"boolean-true", NewBoolean(true), true},
		{"boolean-false", NewBoolean(false), false},
		{"atom", NewAtom("book"), "book"},
		{"none", NewNone(), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ToGo(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ToGo(%s) = %#v, want %#v", c.name, got, c.want)
			}
		})
	}
}

func TestToGoList(t *testing.T) {
	v := NewList([]Value{NewInteger(1), NewString("a"), NewBoolean(true)})
	got := ToGo(v)
	want := []any{int64(1), "a", true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToGo(list) = %#v, want %#v", got, want)
	}
}

func TestToGoMap(t *testing.T) {
	om := NewOrderedMap()
	om.Set("name", NewString("Alice"))
	om.Set("age", NewInteger(30))
	v := NewMap(om)
	got := ToGo(v)
	want := map[string]any{"name": "Alice", "age": int64(30)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToGo(map) = %#v, want %#v", got, want)
	}
}

func TestFromGoScalars(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any // expected ToGo result for round-trip
	}{
		{"nil", nil, nil},
		{"string", "hi", "hi"},
		{"bool", true, true},
		{"int", 7, int64(7)},
		{"int64", int64(123), int64(123)},
		{"float-integral", float64(2.0), int64(2)},
		{"float-fractional", float64(2.5), float64(2.5)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := FromGo(c.in)
			got := ToGo(v)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ToGo(FromGo(%v)) = %#v, want %#v", c.in, got, c.want)
			}
		})
	}
}

func TestRoundTripNested(t *testing.T) {
	in := map[string]any{
		"id":     int64(1),
		"name":   "Alice",
		"active": true,
		"tags":   []any{"a", "b", "c"},
		"meta":   map[string]any{"k": int64(9)},
	}
	out := ToGo(FromGo(in))
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round-trip mismatch:\n got: %#v\nwant: %#v", out, in)
	}
}

func TestFromGoFallback(t *testing.T) {
	type custom struct{ X int }
	v := FromGo(custom{X: 5})
	if !v.VType.Matches(TString) {
		t.Fatalf("expected fallback to String for unknown type, got %s", v.VType)
	}
}
