package engine

import (
	"strings"
	"testing"

	multisource "github.com/jsonicjs/multisource/go"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
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

// --- Multisource integration tests ---

func TestJsonicFormatMultisourceResolves(t *testing.T) {
	// Set up in-memory files for the resolver.
	mem := fileops.NewMem()
	mem.Files["part.jsonic"] = []byte(`{x:1}`)

	f := &JsonicFormat{
		Resolver: MakeFileOpsResolver(mem),
	}

	// The @"part.jsonic" reference should be resolved and merged.
	result, err := f.Decode(`{@"part.jsonic", y:2}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	m := result[0].AsMap()
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x' from resolved file")
	}
	if xVal.AsInteger() != 1 {
		t.Errorf("x = %v, want 1", xVal)
	}
	yVal, ok := m.Get("y")
	if !ok {
		t.Fatal("expected key 'y'")
	}
	if yVal.AsInteger() != 2 {
		t.Errorf("y = %v, want 2", yVal)
	}
}

func TestJsonicFormatMultisourceNested(t *testing.T) {
	// Test nested multisource: a.jsonic references b.jsonic.
	mem := fileops.NewMem()
	mem.Files["b.jsonic"] = []byte(`{nested: true}`)
	mem.Files["a.jsonic"] = []byte(`{@"b.jsonic", top: 1}`)

	f := &JsonicFormat{
		Resolver: MakeFileOpsResolver(mem),
	}

	result, err := f.Decode(`{@"a.jsonic", outer: 99}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result[0].AsMap()

	outerVal, ok := m.Get("outer")
	if !ok {
		t.Fatal("expected key 'outer'")
	}
	if outerVal.AsInteger() != 99 {
		t.Errorf("outer = %v, want 99", outerVal)
	}

	topVal, ok := m.Get("top")
	if !ok {
		t.Fatal("expected key 'top' from a.jsonic")
	}
	if topVal.AsInteger() != 1 {
		t.Errorf("top = %v, want 1", topVal)
	}

	nestedVal, ok := m.Get("nested")
	if !ok {
		t.Fatal("expected key 'nested' from b.jsonic")
	}
	if !nestedVal.AsBoolean() {
		t.Errorf("nested = %v, want true", nestedVal)
	}
}

func TestJsonicFormatWithoutResolverNoMultisource(t *testing.T) {
	// Without a resolver, the jsonic format should work as before
	// (no multisource, just plain jsonic parsing).
	f := &JsonicFormat{}
	result, err := f.Decode(`{a:1, b:2}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Errorf("expected map, got %v", result)
	}
}

func TestJsonicFormatMultisourceNotUsedForJSON(t *testing.T) {
	// JSONFormat must NOT use multisource — it's strict JSON only.
	f := &JSONFormat{}
	// This is valid JSON with an @ in a key — should parse as-is.
	result, err := f.Decode(`{"@ref": "value"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result[0].AsMap()
	v, ok := m.Get("@ref")
	if !ok {
		t.Fatal("expected key '@ref' in JSON output")
	}
	if v.AsString() != "value" {
		t.Errorf("got %q, want %q", v.AsString(), "value")
	}
}

func TestJsonicFormatMultisourceNotUsedForText(t *testing.T) {
	// TextFormat must NOT use multisource.
	f := &TextFormat{}
	result, err := f.Decode(`@"somefile.jsonic"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Text format returns raw string, no resolution.
	if result[0].AsString() != `@"somefile.jsonic"` {
		t.Errorf("text format should return raw content, got %q", result[0].AsString())
	}
}

func TestMakeFileOpsResolverFindsFile(t *testing.T) {
	mem := fileops.NewMem()
	mem.Files["data.jsonic"] = []byte(`{found:true}`)

	resolver := MakeFileOpsResolver(mem)
	spec := multisource.PathSpec{
		Full: "data.jsonic",
		Kind: "jsonic",
	}
	res := resolver(spec, nil)
	if !res.Found {
		t.Fatal("expected resolver to find data.jsonic")
	}
	if res.Src != `{found:true}` {
		t.Errorf("got src %q, want {found:true}", res.Src)
	}
}

func TestMakeFileOpsResolverNotFound(t *testing.T) {
	mem := fileops.NewMem()
	resolver := MakeFileOpsResolver(mem)
	spec := multisource.PathSpec{
		Full: "missing.jsonic",
		Kind: "jsonic",
	}
	res := resolver(spec, nil)
	if res.Found {
		t.Fatal("expected resolver to NOT find missing.jsonic")
	}
}

func TestMakeFileOpsResolverImplicitExt(t *testing.T) {
	mem := fileops.NewMem()
	mem.Files["config.jsonic"] = []byte(`{ok:true}`)

	resolver := MakeFileOpsResolver(mem)
	spec := multisource.PathSpec{
		Full: "config",
		Kind: "", // no extension → try implicit
	}
	opts := &multisource.MultiSourceOptions{
		ImplicitExt: []string{"jsonic", "json"},
	}
	res := resolver(spec, opts)
	if !res.Found {
		t.Fatal("expected resolver to find config via implicit .jsonic ext")
	}
	if res.Kind != "jsonic" {
		t.Errorf("got kind %q, want %q", res.Kind, "jsonic")
	}
}

func TestRegistryJsonicFormatHasResolver(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	jf, ok := r.Formats["jsonic"].(*JsonicFormat)
	if !ok {
		t.Fatal("jsonic format should be *JsonicFormat")
	}
	if jf.Resolver == nil {
		t.Error("jsonic format in registry should have a resolver set")
	}
}

func TestRegistryJSONFormatHasNoResolver(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// JSON format should remain unchanged — no multisource.
	_, ok := r.Formats["json"].(*JSONFormat)
	if !ok {
		t.Fatal("json format should be *JSONFormat, not modified")
	}
}

func TestSetFileOpsUpdatesJsonicResolver(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	mem := fileops.NewMem()
	mem.Files["test.jsonic"] = []byte(`{val:42}`)
	r.SetFileOps(mem)

	jf := r.Formats["jsonic"].(*JsonicFormat)
	if jf.Resolver == nil {
		t.Fatal("expected resolver to be updated after SetFileOps")
	}

	// Verify the resolver uses the new FileOps.
	result, err := jf.Decode(`{@"test.jsonic"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result[0].AsMap()
	v, ok := m.Get("val")
	if !ok {
		t.Fatal("expected key 'val' from resolved test.jsonic")
	}
	if v.AsInteger() != 42 {
		t.Errorf("val = %v, want 42", v)
	}
}
