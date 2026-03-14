package native

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

func TestTransformHandlerPassthrough(t *testing.T) {
	// Transform with literal spec values passes them through.
	data := engine.NewMap(func() *engine.OrderedMap {
		m := engine.NewOrderedMap()
		m.Set("a", engine.NewInteger(1))
		return m
	}())
	spec := engine.NewMap(func() *engine.OrderedMap {
		m := engine.NewOrderedMap()
		m.Set("x", engine.NewInteger(99))
		return m
	}())

	result, err := transformHandler([]engine.Value{data, spec}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	v, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x' in result")
	}
	if v.AsInteger() != 99 {
		t.Errorf("expected 99, got %d", v.AsInteger())
	}
}

func TestTransformHandlerInject(t *testing.T) {
	// Transform with backtick path injects value from data.
	data := engine.NewMap(func() *engine.OrderedMap {
		m := engine.NewOrderedMap()
		m.Set("name", engine.NewString("Alice"))
		return m
	}())
	spec := engine.NewMap(func() *engine.OrderedMap {
		m := engine.NewOrderedMap()
		m.Set("greeting", engine.NewString("`name`"))
		return m
	}())

	result, err := transformHandler([]engine.Value{data, spec}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	v, ok := m.Get("greeting")
	if !ok {
		t.Fatal("expected key 'greeting' in result")
	}
	if v.AsString() != "Alice" {
		t.Errorf("expected Alice, got %s", v.AsString())
	}
}

func TestTransformHandlerNestedPath(t *testing.T) {
	// Transform with nested backtick path.
	inner := engine.NewOrderedMap()
	inner.Set("b", engine.NewInteger(42))
	data := engine.NewMap(func() *engine.OrderedMap {
		m := engine.NewOrderedMap()
		m.Set("a", engine.NewMap(inner))
		return m
	}())
	spec := engine.NewMap(func() *engine.OrderedMap {
		m := engine.NewOrderedMap()
		m.Set("val", engine.NewString("`a.b`"))
		return m
	}())

	result, err := transformHandler([]engine.Value{data, spec}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	v, _ := m.Get("val")
	if v.AsInteger() != 42 {
		t.Errorf("expected 42, got %d", v.AsInteger())
	}
}

func TestValueToAnyRoundtrip(t *testing.T) {
	// Test that valueToAny -> anyToValue preserves structure.
	om := engine.NewOrderedMap()
	om.Set("name", engine.NewString("Bob"))
	om.Set("age", engine.NewInteger(25))
	om.Set("active", engine.NewBoolean(true))

	orig := engine.NewMap(om)
	native := valueToAny(orig)
	back, err := anyToValue(native)
	if err != nil {
		t.Fatal(err)
	}
	m := back.AsMap()
	name, _ := m.Get("name")
	if name.AsString() != "Bob" {
		t.Errorf("expected Bob, got %s", name.AsString())
	}
	age, _ := m.Get("age")
	if age.AsInteger() != 25 {
		t.Errorf("expected 25, got %d", age.AsInteger())
	}
	active, _ := m.Get("active")
	if !active.AsBoolean() {
		t.Error("expected true")
	}
}

func TestValueToAnyAtom(t *testing.T) {
	v := engine.NewAtom("hello")
	result := valueToAny(v)
	s, ok := result.(string)
	if !ok || s != "hello" {
		t.Errorf("expected string hello, got %v", result)
	}
}

func TestValueToAnyNone(t *testing.T) {
	v := engine.NewTypeLiteral(engine.TNone)
	result := valueToAny(v)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestValueToAnyDefault(t *testing.T) {
	// A decimal value should fall through to default and return String()
	v := engine.NewDecimal(3.14)
	result := valueToAny(v)
	s, ok := result.(string)
	if !ok {
		t.Errorf("expected string from default, got %T", result)
	}
	if s == "" {
		t.Error("expected non-empty string")
	}
}

func TestAnyToValueNil(t *testing.T) {
	v, err := anyToValue(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.VType.Equal(engine.TNone) {
		t.Errorf("expected none, got %s", v.VType)
	}
}

func TestAnyToValueBool(t *testing.T) {
	v, err := anyToValue(true)
	if err != nil {
		t.Fatal(err)
	}
	if !v.AsBoolean() {
		t.Error("expected true")
	}
}

func TestAnyToValueInt(t *testing.T) {
	v, err := anyToValue(int(42))
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInteger() != 42 {
		t.Errorf("expected 42, got %d", v.AsInteger())
	}
}

func TestAnyToValueInt64(t *testing.T) {
	v, err := anyToValue(int64(99))
	if err != nil {
		t.Fatal(err)
	}
	if v.AsInteger() != 99 {
		t.Errorf("expected 99, got %d", v.AsInteger())
	}
}

func TestAnyToValueUnsupported(t *testing.T) {
	_, err := anyToValue(struct{}{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestValueToAnyList(t *testing.T) {
	elems := []engine.Value{
		engine.NewInteger(1),
		engine.NewString("two"),
		engine.NewBoolean(false),
	}
	orig := engine.NewList(elems)
	native := valueToAny(orig)
	back, err := anyToValue(native)
	if err != nil {
		t.Fatal(err)
	}
	list := back.AsList()
	if len(list) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(list))
	}
	if list[0].AsInteger() != 1 {
		t.Errorf("expected 1, got %d", list[0].AsInteger())
	}
	if list[1].AsString() != "two" {
		t.Errorf("expected two, got %s", list[1].AsString())
	}
	if list[2].AsBoolean() {
		t.Error("expected false")
	}
}
