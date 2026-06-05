package native

import (
	"fmt"
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
)

// Two user-defined Ideal types registered in NATIVE (Go) code: one with no
// converter (must hit the base Ideal fallback → {} / []) and one whose
// Behavior implements eng.IdealConverter (override → custom Map / List).
var (
	tConvNoOverride = mustRegisterIdeal("Ideal/ConvNoOverride", 10001, nil)
	tConvOverride   = mustRegisterIdeal("Ideal/ConvOverride", 10002, convOverrideBehavior{})
)

func mustRegisterIdeal(path string, id int, b eng.TypeBehavior) *Type {
	t, err := eng.Builtin.RegisterExternalBuiltin(path, id, b)
	if err != nil {
		panic(err)
	}
	return t
}

type convOverrideBehavior struct{}

func (convOverrideBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }
func (convOverrideBehavior) Equal(a, b Value) bool       { return eng.DefaultBehavior.Equal(a, b) }
func (convOverrideBehavior) Format(v Value) string       { return eng.DefaultBehavior.Format(v) }
func (convOverrideBehavior) ToMap(Value) (Value, error) {
	m := NewOrderedMap()
	m.Set("greeting", NewString("hi"))
	return NewMap(m), nil
}
func (convOverrideBehavior) ToList(Value) (Value, error) {
	return NewList([]Value{NewString("a"), NewString("b")}), nil
}

func runConvert(t *testing.T, val Value, src string) string {
	t.Helper()
	r, _ := DefaultRegistry()
	registerIOWords(r)
	InstallDef(r, "thing", val)
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	return eng.Canon(out)
}

// TestConvertIdealNativeFallback: a native Ideal with no IdealConverter
// converts to the base {} / [].
func TestConvertIdealNativeFallback(t *testing.T) {
	v := Value{Parent: tConvNoOverride, Data: ExtensionPayload{Body: "payload"}}
	if got := runConvert(t, v, "convert Map thing"); got != "{}" {
		t.Errorf("convert Map (native Ideal, no override) = %s, want {}", got)
	}
	if got := runConvert(t, v, "convert List thing"); got != "[]" {
		t.Errorf("convert List (native Ideal, no override) = %s, want []", got)
	}
	// And it really is an Ideal that resolves the fallback, not a Map.
	if !v.Parent.ConformsTo(TIdeal) {
		t.Errorf("%s is not under Ideal", v.Parent)
	}
}

// TestConvertIdealNativeOverride: a native Ideal whose Behavior implements
// IdealConverter uses the override.
func TestConvertIdealNativeOverride(t *testing.T) {
	v := Value{Parent: tConvOverride, Data: ExtensionPayload{Body: "payload"}}
	if got := runConvert(t, v, "convert Map thing"); got != "{greeting:'hi'}" {
		t.Errorf("convert Map (native Ideal, override) = %s, want {greeting:'hi'}", got)
	}
	if got := runConvert(t, v, "convert List thing"); got != "['a' 'b']" {
		t.Errorf("convert List (native Ideal, override) = %s, want ['a' 'b']", got)
	}
}

// TestConvertIdealBuiltins covers the concrete IdealConverter overrides on
// the built-in kernel Ideals (constructed in Go, exercised via the engine).
func TestConvertIdealBuiltins(t *testing.T) {
	// Array → its elements (List); index→element (Map).
	arr := NewArray([]Value{NewInteger(10), NewInteger(20)})
	if got := runConvert(t, arr, "convert List thing"); got != "[10 20]" {
		t.Errorf("Array→List = %s, want [10 20]", got)
	}
	if got := runConvert(t, arr, "convert Map thing"); got != "{0:10 1:20}" {
		t.Errorf("Array→Map = %s, want {0:10 1:20}", got)
	}

	// Error → {message:…} / [message].
	erv := NewError(fmt.Errorf("boom"))
	if got := runConvert(t, erv, "convert Map thing"); got != "{message:'boom'}" {
		t.Errorf("Error→Map = %s, want {message:'boom'}", got)
	}
	if got := runConvert(t, erv, "convert List thing"); got != "['boom']" {
		t.Errorf("Error→List = %s, want ['boom']", got)
	}

	// Object instance → its fields / field values.
	fields := NewOrderedMap()
	fields.Set("a", NewInteger(1))
	fields.Set("b", NewString("x"))
	obj := NewObjectInstance(TObject, ObjectInstanceInfo{Fields: fields})
	if got := runConvert(t, obj, "convert Map thing"); got != "{a:1 b:'x'}" {
		t.Errorf("Object→Map = %s, want {a:1 b:'x'}", got)
	}
	if got := runConvert(t, obj, "convert List thing"); got != "[1 'x']" {
		t.Errorf("Object→List = %s, want [1 'x']", got)
	}
}

// TestConvertIdealMoreBuiltins covers Table, Timeout, and Fetch Response.
func TestConvertIdealMoreBuiltins(t *testing.T) {
	// Table → rows (List); columnar (Map).
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	fields.Set("b", NewTypeLiteral(TString))
	row1 := NewOrderedMap()
	row1.Set("a", NewInteger(1))
	row1.Set("b", NewString("x"))
	row2 := NewOrderedMap()
	row2.Set("a", NewInteger(2))
	row2.Set("b", NewString("y"))
	tbl := NewValueRaw(TTable, TableData{
		Record: RecordTypeInfo{Fields: fields},
		Rows:   []Value{NewMap(row1), NewMap(row2)},
	})
	if got := runConvert(t, tbl, "convert List thing"); got != "[{a:1 b:'x'} {a:2 b:'y'}]" {
		t.Errorf("Table→List = %s", got)
	}
	if got := runConvert(t, tbl, "convert Map thing"); got != "{a:[1 2] b:['x' 'y']}" {
		t.Errorf("Table→Map = %s", got)
	}

	// Timeout → {id ms} / [id ms].
	to := Value{Parent: TTimeout, Data: &TimeoutInfo{ID: "t1", Ms: 500}}
	if got := runConvert(t, to, "convert Map thing"); got != "{id:'t1' ms:500}" {
		t.Errorf("Timeout→Map = %s", got)
	}
	if got := runConvert(t, to, "convert List thing"); got != "['t1' 500]" {
		t.Errorf("Timeout→List = %s", got)
	}

	// Fetch Response (map-backed) → its map.
	resp := NewOrderedMap()
	resp.Set("status", NewInteger(200))
	fr := Value{Parent: TFetchResponse, Data: MapPayload{M: resp}}
	if got := runConvert(t, fr, "convert Map thing"); got != "{status:200}" {
		t.Errorf("Fetch→Map = %s", got)
	}
	if got := runConvert(t, fr, "convert List thing"); got != "[200]" {
		t.Errorf("Fetch→List = %s", got)
	}
}
