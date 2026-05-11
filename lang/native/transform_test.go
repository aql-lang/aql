package native

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/lang/engine"
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

	result, err := transformHandler([]engine.Value{spec, data}, nil, nil, nil)
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
	vi, _ := v.AsInteger()
	if vi != 99 {
		t.Errorf("expected 99, got %d", vi)
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

	result, err := transformHandler([]engine.Value{spec, data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	v, ok := m.Get("greeting")
	if !ok {
		t.Fatal("expected key 'greeting' in result")
	}
	vs, _ := v.AsString()
	if vs != "Alice" {
		t.Errorf("expected Alice, got %s", vs)
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

	result, err := transformHandler([]engine.Value{spec, data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	v, _ := m.Get("val")
	vi, _ := v.AsInteger()
	if vi != 42 {
		t.Errorf("expected 42, got %d", vi)
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
	ns, _ := name.AsString()
	if ns != "Bob" {
		t.Errorf("expected Bob, got %s", ns)
	}
	age, _ := m.Get("age")
	ai, _ := age.AsInteger()
	if ai != 25 {
		t.Errorf("expected 25, got %d", ai)
	}
	active, _ := m.Get("active")
	ab, _ := active.AsBoolean()
	if !ab {
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
	vb, _ := v.AsBoolean()
	if !vb {
		t.Error("expected true")
	}
}

func TestAnyToValueInt(t *testing.T) {
	v, err := anyToValue(int(42))
	if err != nil {
		t.Fatal(err)
	}
	vi, _ := v.AsInteger()
	if vi != 42 {
		t.Errorf("expected 42, got %d", vi)
	}
}

func TestAnyToValueInt64(t *testing.T) {
	v, err := anyToValue(int64(99))
	if err != nil {
		t.Fatal(err)
	}
	vi, _ := v.AsInteger()
	if vi != 99 {
		t.Errorf("expected 99, got %d", vi)
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
	list := back.AsList().Slice()
	if len(list) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(list))
	}
	li0, _ := list[0].AsInteger()
	if li0 != 1 {
		t.Errorf("expected 1, got %d", li0)
	}
	ls1, _ := list[1].AsString()
	if ls1 != "two" {
		t.Errorf("expected two, got %s", ls1)
	}
	lb2, _ := list[2].AsBoolean()
	if lb2 {
		t.Error("expected false")
	}
}
