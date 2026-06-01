package native

import (
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
	if !v.Parent.Matches(TIdeal) {
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
