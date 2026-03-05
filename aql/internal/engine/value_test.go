package engine

import "testing"

func TestValueStringTypes(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want string
	}{
		{"integer", NewInteger(42), "42"},
		{"string", NewString("hello"), "'hello'"},
		{"empty_string", NewString(""), "''"},
		{"bool_true", NewBoolean(true), "true"},
		{"bool_false", NewBoolean(false), "false"},
		{"atom", NewAtom("foo"), "foo"},
		{"none_literal", NewTypeLiteral(TNone), "none"},
		{"number_literal", NewTypeLiteral(TNumber), "number"},
		{"string_literal", NewTypeLiteral(TString), "string"},
		{"word", NewWord("upper"), "word(upper)"},
		{"open_paren", NewOpenParen(), "("},
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

func TestValueStringList(t *testing.T) {
	v := NewList([]Value{NewInteger(1), NewString("a")})
	got := v.String()
	if got != "[1,'a']" {
		t.Errorf("got %q, want %q", got, "[1,'a']")
	}
}

func TestValueStringMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	m.Set("y", NewString("hi"))
	v := NewMap(m)
	got := v.String()
	if got != "{x:1,y:'hi'}" {
		t.Errorf("got %q, want %q", got, "{x:1,y:'hi'}")
	}
}

func TestValueStringTypedList(t *testing.T) {
	v := NewTypedList(NewTypeLiteral(TString))
	got := v.String()
	if got != "[:string]" {
		t.Errorf("got %q, want %q", got, "[:string]")
	}
}

func TestValueStringTypedMap(t *testing.T) {
	v := NewTypedMap(NewTypeLiteral(TNumber))
	got := v.String()
	if got != "{:number}" {
		t.Errorf("got %q, want %q", got, "{:number}")
	}
}

func TestValueStringRecordType(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	v := NewRecordType(fields)
	got := v.String()
	if got != "record{x:number,y:string}" {
		t.Errorf("got %q, want %q", got, "record{x:number,y:string}")
	}
}

func TestValueStringTableType(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TNumber))
	v := NewTableType(RecordTypeInfo{Fields: fields})
	got := v.String()
	if got != "table{a:number}" {
		t.Errorf("got %q, want %q", got, "table{a:number}")
	}
}

func TestValueStringDisjunct(t *testing.T) {
	v := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	got := v.String()
	if got != "string|none" {
		t.Errorf("got %q, want %q", got, "string|none")
	}
}

func TestValueStringForward(t *testing.T) {
	v := NewForward(ForwardInfo{FuncName: "add", CollectedArgs: 1, ExpectedArgs: 2})
	got := v.String()
	if got != "forward(add,1/2)" {
		t.Errorf("got %q, want %q", got, "forward(add,1/2)")
	}
}

func TestNewFnDef(t *testing.T) {
	v := NewFnDef(FnDefInfo{})
	if !v.VType.Equal(TFnDef) {
		t.Errorf("expected fndef type, got %s", v.VType)
	}
}

func TestNewDisjunct(t *testing.T) {
	v := NewDisjunct([]Value{NewTypeLiteral(TString)})
	if !v.IsDisjunct() {
		t.Error("expected IsDisjunct to be true")
	}
	di := v.AsDisjunct()
	if len(di.Alternatives) != 1 {
		t.Errorf("expected 1 alternative, got %d", len(di.Alternatives))
	}
}

func TestIsBoolean(t *testing.T) {
	if !NewBoolean(true).IsBoolean() {
		t.Error("expected true to be boolean")
	}
	if !NewBoolean(false).IsBoolean() {
		t.Error("expected false to be boolean")
	}
	if NewInteger(1).IsBoolean() {
		t.Error("integer should not be boolean")
	}
}

func TestAsTableType(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	v := NewTableType(RecordTypeInfo{Fields: fields})
	tt := v.AsTableType()
	if tt.Record.Fields.Len() != 1 {
		t.Errorf("expected 1 field, got %d", tt.Record.Fields.Len())
	}
}

func TestAsChildType(t *testing.T) {
	v := NewTypedList(NewTypeLiteral(TString))
	ct := v.AsChildType()
	if !ct.Child.VType.Equal(TString) {
		t.Errorf("expected string child, got %s", ct.Child.VType)
	}
}

func TestOrderedMapSortedKeys(t *testing.T) {
	m := NewOrderedMap()
	m.Set("c", NewInteger(3))
	m.Set("a", NewInteger(1))
	m.Set("b", NewInteger(2))

	keys := m.SortedKeys()
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("SortedKeys = %v, want [a b c]", keys)
	}
}
