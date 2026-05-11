package engine_test

import (
	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"testing"
)

func TestValueStringTypes(t *testing.T) {
	tests := []struct {
		name string
		val  engine.Value
		want string
	}{
		{"integer", engine.NewInteger(42), "42"},
		{"string", engine.NewString("hello"), "'hello'"},
		{"empty_string", engine.NewString(""), "''"},
		{"bool_true", engine.NewBoolean(true), "true"},
		{"bool_false", engine.NewBoolean(false), "false"},
		{"atom", engine.NewAtom("foo"), "foo"},
		{"none_literal", engine.NewTypeLiteral(engine.TNone), "None"},
		{"number_literal", engine.NewTypeLiteral(engine.TNumber), "Number"},
		{"string_literal", engine.NewTypeLiteral(engine.TString), "String"},
		{"word", engine.NewWord("upper"), "word(upper)"},
		{"open_paren", engine.NewOpenParen(), "("},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.val.String()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValueStringListOrig(t *testing.T) {
	v := engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewString("a")})
	got := v.String()
	if got != "[1,'a']" {
		t.Errorf("got %q, want %q", got, "[1,'a']")
	}
}

func TestValueStringMapOrig(t *testing.T) {
	m := engine.NewOrderedMap()
	m.Set("x", engine.NewInteger(1))
	m.Set("y", engine.NewString("hi"))
	v := engine.NewMap(m)
	got := v.String()
	if got != "{x:1,y:'hi'}" {
		t.Errorf("got %q, want %q", got, "{x:1,y:'hi'}")
	}
}

func TestValueStringTypedListOrig(t *testing.T) {
	v := engine.NewTypedList(engine.NewTypeLiteral(engine.TString))
	got := v.String()
	if got != "[:String]" {
		t.Errorf("got %q, want %q", got, "[:String]")
	}
}

func TestValueStringTypedMapOrig(t *testing.T) {
	v := engine.NewTypedMap(engine.NewTypeLiteral(engine.TNumber))
	got := v.String()
	if got != "{:Number}" {
		t.Errorf("got %q, want %q", got, "{:Number}")
	}
}

func TestValueStringRecordTypeOrig(t *testing.T) {
	fields := engine.NewOrderedMap()
	fields.Set("x", engine.NewTypeLiteral(engine.TNumber))
	fields.Set("y", engine.NewTypeLiteral(engine.TString))
	v := engine.NewRecordType(fields)
	got := v.String()
	if got != "record{x:Number,y:String}" {
		t.Errorf("got %q, want %q", got, "record{x:Number,y:String}")
	}
}

func TestValueStringTableTypeOrig(t *testing.T) {
	fields := engine.NewOrderedMap()
	fields.Set("a", engine.NewTypeLiteral(engine.TNumber))
	v := engine.NewTableType(engine.RecordTypeInfo{Fields: fields})
	got := v.String()
	if got != "table{a:Number}" {
		t.Errorf("got %q, want %q", got, "table{a:Number}")
	}
}

func TestValueStringDisjunctOrig(t *testing.T) {
	v := engine.NewDisjunct([]engine.Value{engine.NewTypeLiteral(engine.TString), engine.NewTypeLiteral(engine.TNone)})
	got := v.String()
	if got != "String|None" {
		t.Errorf("got %q, want %q", got, "String|None")
	}
}

func TestValueStringForward(t *testing.T) {
	v := engine.NewForward(engine.ForwardInfo{FuncName: "add", CollectedArgs: 1, ExpectedArgs: 2})
	got := v.String()
	if got != "forward(add,1/2)" {
		t.Errorf("got %q, want %q", got, "forward(add,1/2)")
	}
}

func TestNewFnDef(t *testing.T) {
	v := engine.NewFnDef(engine.FnDefInfo{})
	if !v.VType.Equal(engine.TFnDef) {
		t.Errorf("expected fndef type, got %s", v.VType)
	}
}

func TestNewDisjunct(t *testing.T) {
	v := engine.NewDisjunct([]engine.Value{engine.NewTypeLiteral(engine.TString)})
	if !v.IsDisjunct() {
		t.Error("expected IsDisjunct to be true")
	}
	di, _ := v.AsDisjunct()
	if len(di.Alternatives) != 1 {
		t.Errorf("expected 1 alternative, got %d", len(di.Alternatives))
	}
}

func TestIsBoolean(t *testing.T) {
	if !engine.NewBoolean(true).IsBoolean() {
		t.Error("expected true to be boolean")
	}
	if !engine.NewBoolean(false).IsBoolean() {
		t.Error("expected false to be boolean")
	}
	if engine.NewInteger(1).IsBoolean() {
		t.Error("integer should not be boolean")
	}
}

func TestAsTableType(t *testing.T) {
	fields := engine.NewOrderedMap()
	fields.Set("x", engine.NewTypeLiteral(engine.TNumber))
	v := engine.NewTableType(engine.RecordTypeInfo{Fields: fields})
	tt, _ := v.AsTableType()
	if tt.Record.Fields.Len() != 1 {
		t.Errorf("expected 1 field, got %d", tt.Record.Fields.Len())
	}
}

func TestAsChildType(t *testing.T) {
	v := engine.NewTypedList(engine.NewTypeLiteral(engine.TString))
	ct, _ := v.AsChildType()
	if !ct.Child.VType.Equal(engine.TString) {
		t.Errorf("expected string child, got %s", ct.Child.VType)
	}
}

func TestOrderedMapSortedKeys(t *testing.T) {
	m := engine.NewOrderedMap()
	m.Set("c", engine.NewInteger(3))
	m.Set("a", engine.NewInteger(1))
	m.Set("b", engine.NewInteger(2))

	keys := m.SortedKeys()
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("SortedKeys = %v, want [a b c]", keys)
	}
}
