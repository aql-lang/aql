package engine

import (
	"strings"
	"testing"
)

func TestTextFormatDecode(t *testing.T) {
	f := &TextFormat{}
	result, err := f.Decode("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].AsString() != "hello world" {
		t.Errorf("got %v, want ['hello world']", result)
	}
}

func TestTextFormatEncode(t *testing.T) {
	f := &TextFormat{}
	s, err := f.Encode(NewString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if s != "hello" {
		t.Errorf("got %q, want %q", s, "hello")
	}

	// Non-string uses String()
	s, err = f.Encode(NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if s != "42" {
		t.Errorf("got %q, want %q", s, "42")
	}
}

func TestJSONFormatDecode(t *testing.T) {
	f := &JSONFormat{}
	result, err := f.Decode(`{"x":1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	if !result[0].VType.Equal(TMap) {
		t.Errorf("expected map, got %s", result[0].VType)
	}
}

func TestJSONFormatDecodeError(t *testing.T) {
	f := &JSONFormat{}
	_, err := f.Decode(`{invalid`)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestJSONFormatEncode(t *testing.T) {
	f := &JSONFormat{}
	s, err := f.Encode(NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if s != "42" {
		t.Errorf("got %q, want %q", s, "42")
	}
}

func TestJsonicFormatDecode(t *testing.T) {
	f := &JsonicFormat{}
	result, err := f.Decode(`{x:1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Errorf("expected map, got %v", result)
	}
}

func TestJsonicFormatDecodeNull(t *testing.T) {
	f := &JsonicFormat{}
	result, err := f.Decode(`null`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].VType.Equal(TNone) {
		t.Errorf("expected none, got %v", result)
	}
}

func TestJsonicFormatDecodeError(t *testing.T) {
	f := &JsonicFormat{}
	_, err := f.Decode(`{{{`)
	if err == nil {
		t.Error("expected error for invalid jsonic")
	}
}

func TestJsonicFormatEncode(t *testing.T) {
	f := &JsonicFormat{}
	s, err := f.Encode(NewString("hi"))
	if err != nil {
		t.Fatal(err)
	}
	if s != `"hi"` {
		t.Errorf("got %q, want %q", s, `"hi"`)
	}
}

func TestLinesFormatDecode(t *testing.T) {
	f := &LinesFormat{}
	result, err := f.Decode("a\nb\nc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	elems := result[0].AsList()
	if len(elems) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(elems))
	}
	if elems[0].AsString() != "a" || elems[1].AsString() != "b" || elems[2].AsString() != "c" {
		t.Errorf("got %v", elems)
	}
}

func TestLinesFormatEncode(t *testing.T) {
	f := &LinesFormat{}
	list := NewList([]Value{NewString("x"), NewString("y")})
	s, err := f.Encode(list)
	if err != nil {
		t.Fatal(err)
	}
	if s != "x\ny" {
		t.Errorf("got %q, want %q", s, "x\ny")
	}
}

func TestLinesFormatEncodeNonString(t *testing.T) {
	f := &LinesFormat{}
	list := NewList([]Value{NewInteger(1), NewInteger(2)})
	s, err := f.Encode(list)
	if err != nil {
		t.Fatal(err)
	}
	if s != "1\n2" {
		t.Errorf("got %q, want %q", s, "1\n2")
	}
}

func TestLinesFormatEncodeNonList(t *testing.T) {
	f := &LinesFormat{}
	s, err := f.Encode(NewString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if s != "'hello'" {
		t.Errorf("got %q, want %q", s, "'hello'")
	}
}

func TestDefaultFormats(t *testing.T) {
	fmts := DefaultFormats()
	for _, name := range []string{"text", "json", "jsonic", "lines", "csv", "tsv"} {
		if _, ok := fmts[name]; !ok {
			t.Errorf("missing format: %s", name)
		}
	}
}

// --- CSV format tests ---

func TestCSVFormatDecode(t *testing.T) {
	f := &CSVFormat{}
	result, err := f.Decode("name,age\nAlice,30\nBob,25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	v := result[0]
	if !v.IsTableType() {
		t.Fatalf("expected table type, got %s", v)
	}

	rows := v.AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Check first row
	r0 := rows[0].AsMap()
	nameVal, ok := r0.Get("name")
	if !ok {
		t.Fatal("expected 'name' key")
	}
	if nameVal.AsString() != "Alice" {
		t.Errorf("name = %q, want %q", nameVal.AsString(), "Alice")
	}
	ageVal, ok := r0.Get("age")
	if !ok {
		t.Fatal("expected 'age' key")
	}
	if ageVal.AsString() != "30" {
		t.Errorf("age = %q, want %q", ageVal.AsString(), "30")
	}

	// Check second row
	r1 := rows[1].AsMap()
	nameVal, _ = r1.Get("name")
	if nameVal.AsString() != "Bob" {
		t.Errorf("name = %q, want %q", nameVal.AsString(), "Bob")
	}
}

func TestCSVFormatDecodeEmpty(t *testing.T) {
	f := &CSVFormat{}
	result, err := f.Decode("name,age\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	rows := result[0].AsList()
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestCSVFormatDecodeQuoted(t *testing.T) {
	f := &CSVFormat{}
	result, err := f.Decode("a,b\n\"hello, world\",2\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	m := rows[0].AsMap()
	aVal, _ := m.Get("a")
	if aVal.AsString() != "hello, world" {
		t.Errorf("a = %q, want %q", aVal.AsString(), "hello, world")
	}
}

func TestCSVFormatDecodeTableSchema(t *testing.T) {
	f := &CSVFormat{}
	result, err := f.Decode("x,y\n1,2")
	if err != nil {
		t.Fatal(err)
	}
	v := result[0]
	tt := v.AsTableType()
	fields := tt.Record.Fields
	if fields.Len() != 2 {
		t.Errorf("expected 2 fields, got %d", fields.Len())
	}
	xType, ok := fields.Get("x")
	if !ok {
		t.Fatal("expected field 'x'")
	}
	if !xType.VType.Equal(TString) {
		t.Errorf("expected string type for x, got %s", xType.VType)
	}
}

func TestCSVFormatEncode(t *testing.T) {
	f := &CSVFormat{}
	// Create a table data value
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	r0 := NewOrderedMap()
	r0.Set("age", NewString("30"))
	r0.Set("name", NewString("Alice"))
	r1 := NewOrderedMap()
	r1.Set("age", NewString("25"))
	r1.Set("name", NewString("Bob"))

	table := Value{VType: TList, Data: TableData{
		Record: rec,
		Rows:   []Value{NewMap(r0), NewMap(r1)},
	}}

	s, err := f.Encode(table)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "name") || !strings.Contains(s, "age") {
		t.Errorf("encoded CSV missing headers: %q", s)
	}
	if !strings.Contains(s, "Alice") || !strings.Contains(s, "Bob") {
		t.Errorf("encoded CSV missing data: %q", s)
	}
}

func TestCSVFormatEncodeQuoted(t *testing.T) {
	f := &CSVFormat{}
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	r0 := NewOrderedMap()
	r0.Set("a", NewString("hello, world"))
	table := Value{VType: TList, Data: TableData{
		Record: rec,
		Rows:   []Value{NewMap(r0)},
	}}
	s, err := f.Encode(table)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"hello, world"`) {
		t.Errorf("expected quoted field in: %q", s)
	}
}

// --- TSV format tests ---

func TestTSVFormatDecode(t *testing.T) {
	f := &TSVFormat{}
	result, err := f.Decode("name\tage\nAlice\t30\nBob\t25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	v := result[0]
	if !v.IsTableType() {
		t.Fatalf("expected table type, got %s", v)
	}

	rows := v.AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	r0 := rows[0].AsMap()
	nameVal, _ := r0.Get("name")
	if nameVal.AsString() != "Alice" {
		t.Errorf("name = %q, want %q", nameVal.AsString(), "Alice")
	}
}

func TestTSVFormatEncode(t *testing.T) {
	f := &TSVFormat{}
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TString))
	fields.Set("b", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	r0 := NewOrderedMap()
	r0.Set("a", NewString("x"))
	r0.Set("b", NewString("y"))
	table := Value{VType: TList, Data: TableData{
		Record: rec,
		Rows:   []Value{NewMap(r0)},
	}}
	s, err := f.Encode(table)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "a\tb") {
		t.Errorf("expected tab-separated headers in: %q", s)
	}
	if !strings.Contains(s, "x\ty") {
		t.Errorf("expected tab-separated data in: %q", s)
	}
}
