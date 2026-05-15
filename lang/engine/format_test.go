package engine_test

import (
	"github.com/aql-lang/aql/lang/engine"
	"strings"
	"testing"

	multisource "github.com/jsonicjs/multisource/go"

	"github.com/aql-lang/aql/lang/internal/fileops"
)

func TestTextFormatDecode(t *testing.T) {
	f := &engine.TextFormat{}
	result, err := f.Decode("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as0, _ := engine.AsString(result[0])
	if len(result) != 1 || _as0 != "hello world" {
		t.Errorf("got %v, want ['hello world']", result)
	}
}

func TestTextFormatEncode(t *testing.T) {
	f := &engine.TextFormat{}
	s, err := f.Encode(engine.NewString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if s != "hello" {
		t.Errorf("got %q, want %q", s, "hello")
	}

	// Non-string uses String()
	s, err = f.Encode(engine.NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if s != "42" {
		t.Errorf("got %q, want %q", s, "42")
	}
}

func TestJSONFormatDecode(t *testing.T) {
	f := &engine.JSONFormat{}
	result, err := f.Decode(`{"x":1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	if !result[0].VType.Equal(engine.TMap) {
		t.Errorf("expected map, got %s", result[0].VType)
	}
}

func TestJSONFormatDecodeError(t *testing.T) {
	f := &engine.JSONFormat{}
	_, err := f.Decode(`{invalid`)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestJSONFormatEncode(t *testing.T) {
	f := &engine.JSONFormat{}
	s, err := f.Encode(engine.NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if s != "42" {
		t.Errorf("got %q, want %q", s, "42")
	}
}

func TestJsonicFormatDecode(t *testing.T) {
	f := &engine.JsonicFormat{}
	result, err := f.Decode(`{x:1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].VType.Equal(engine.TMap) {
		t.Errorf("expected map, got %v", result)
	}
}

func TestJsonicFormatDecodeNull(t *testing.T) {
	f := &engine.JsonicFormat{}
	result, err := f.Decode(`null`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].VType.Equal(engine.TNone) {
		t.Errorf("expected none, got %v", result)
	}
}

func TestJsonicFormatDecodeError(t *testing.T) {
	f := &engine.JsonicFormat{}
	_, err := f.Decode(`{{{`)
	if err == nil {
		t.Error("expected error for invalid jsonic")
	}
}

func TestJsonicFormatEncode(t *testing.T) {
	f := &engine.JsonicFormat{}
	s, err := f.Encode(engine.NewString("hi"))
	if err != nil {
		t.Fatal(err)
	}
	if s != `"hi"` {
		t.Errorf("got %q, want %q", s, `"hi"`)
	}
}

func TestLinesFormatDecode(t *testing.T) {
	f := &engine.LinesFormat{}
	result, err := f.Decode("a\nb\nc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	elems := engine.AsList(result[0]).Slice()
	if len(elems) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(elems))
	}
	_as3, _ := engine.AsString(elems[0])
	_as2, _ := engine.AsString(elems[1])
	_as1, _ := engine.AsString(elems[2])
	if _as3 != "a" || _as2 != "b" || _as1 != "c" {
		t.Errorf("got %v", elems)
	}
}

func TestLinesFormatEncode(t *testing.T) {
	f := &engine.LinesFormat{}
	list := engine.NewList([]engine.Value{engine.NewString("x"), engine.NewString("y")})
	s, err := f.Encode(list)
	if err != nil {
		t.Fatal(err)
	}
	if s != "x\ny" {
		t.Errorf("got %q, want %q", s, "x\ny")
	}
}

func TestLinesFormatEncodeNonString(t *testing.T) {
	f := &engine.LinesFormat{}
	list := engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2)})
	s, err := f.Encode(list)
	if err != nil {
		t.Fatal(err)
	}
	if s != "1\n2" {
		t.Errorf("got %q, want %q", s, "1\n2")
	}
}

func TestLinesFormatEncodeNonList(t *testing.T) {
	f := &engine.LinesFormat{}
	s, err := f.Encode(engine.NewString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if s != "'hello'" {
		t.Errorf("got %q, want %q", s, "'hello'")
	}
}

func TestDefaultFormats(t *testing.T) {
	fmts := engine.DefaultFormats()
	for _, name := range []string{"text", "json", "jsonic", "lines", "csv", "tsv"} {
		if _, ok := fmts[name]; !ok {
			t.Errorf("missing format: %s", name)
		}
	}
}

// --- CSV format tests ---

func TestCSVFormatDecode(t *testing.T) {
	f := &engine.CSVFormat{}
	result, err := f.Decode("name,age\nAlice,30\nBob,25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	v := result[0]
	if !engine.IsTableType(v) {
		t.Fatalf("expected table type, got %s", v)
	}

	rows := engine.AsList(v).Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Check first row
	r0 := engine.AsMap(rows[0])
	nameVal, ok := r0.Get("name")
	if !ok {
		t.Fatal("expected 'name' key")
	}
	_as4, _ := engine.AsString(nameVal)
	if _as4 != "Alice" {
		_as5, _ := engine.AsString(nameVal)
		t.Errorf("name = %q, want %q", _as5, "Alice")
	}
	ageVal, ok := r0.Get("age")
	if !ok {
		t.Fatal("expected 'age' key")
	}
	_as6, _ := engine.AsString(ageVal)
	if _as6 != "30" {
		_as7, _ := engine.AsString(ageVal)
		t.Errorf("age = %q, want %q", _as7, "30")
	}

	// Check second row
	r1 := engine.AsMap(rows[1])
	nameVal, _ = r1.Get("name")
	_as8, _ := engine.AsString(nameVal)
	if _as8 != "Bob" {
		_as9, _ := engine.AsString(nameVal)
		t.Errorf("name = %q, want %q", _as9, "Bob")
	}
}

func TestCSVFormatDecodeEmpty(t *testing.T) {
	f := &engine.CSVFormat{}
	result, err := f.Decode("name,age\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	rows := engine.AsList(result[0]).Slice()
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestCSVFormatDecodeQuoted(t *testing.T) {
	f := &engine.CSVFormat{}
	result, err := f.Decode("a,b\n\"hello, world\",2\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := engine.AsList(result[0]).Slice()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	m := engine.AsMap(rows[0])
	aVal, _ := m.Get("a")
	_as10, _ := engine.AsString(aVal)
	if _as10 != "hello, world" {
		_as11, _ := engine.AsString(aVal)
		t.Errorf("a = %q, want %q", _as11, "hello, world")
	}
}

func TestCSVFormatDecodeTableSchema(t *testing.T) {
	f := &engine.CSVFormat{}
	result, err := f.Decode("x,y\n1,2")
	if err != nil {
		t.Fatal(err)
	}
	v := result[0]
	tt, _ := engine.AsTableType(v)
	fields := tt.Record.Fields
	if fields.Len() != 2 {
		t.Errorf("expected 2 fields, got %d", fields.Len())
	}
	xType, ok := fields.Get("x")
	if !ok {
		t.Fatal("expected field 'x'")
	}
	if !xType.VType.Equal(engine.TString) {
		t.Errorf("expected string type for x, got %s", xType.VType)
	}
}

func TestCSVFormatEncode(t *testing.T) {
	f := &engine.CSVFormat{}
	// Create a table data value
	fields := engine.NewOrderedMap()
	fields.Set("name", engine.NewTypeLiteral(engine.TString))
	fields.Set("age", engine.NewTypeLiteral(engine.TString))
	rec := engine.RecordTypeInfo{Fields: fields}

	r0 := engine.NewOrderedMap()
	r0.Set("age", engine.NewString("30"))
	r0.Set("name", engine.NewString("Alice"))
	r1 := engine.NewOrderedMap()
	r1.Set("age", engine.NewString("25"))
	r1.Set("name", engine.NewString("Bob"))

	table := engine.Value{VType: engine.TList, Data: engine.TableData{
		Record: rec,
		Rows:   []engine.Value{engine.NewMap(r0), engine.NewMap(r1)},
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
	f := &engine.CSVFormat{}
	fields := engine.NewOrderedMap()
	fields.Set("a", engine.NewTypeLiteral(engine.TString))
	rec := engine.RecordTypeInfo{Fields: fields}

	r0 := engine.NewOrderedMap()
	r0.Set("a", engine.NewString("hello, world"))
	table := engine.Value{VType: engine.TList, Data: engine.TableData{
		Record: rec,
		Rows:   []engine.Value{engine.NewMap(r0)},
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
	f := &engine.TSVFormat{}
	result, err := f.Decode("name\tage\nAlice\t30\nBob\t25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	v := result[0]
	if !engine.IsTableType(v) {
		t.Fatalf("expected table type, got %s", v)
	}

	rows := engine.AsList(v).Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	r0 := engine.AsMap(rows[0])
	nameVal, _ := r0.Get("name")
	_as12, _ := engine.AsString(nameVal)
	if _as12 != "Alice" {
		_as13, _ := engine.AsString(nameVal)
		t.Errorf("name = %q, want %q", _as13, "Alice")
	}
}

func TestTSVFormatEncode(t *testing.T) {
	f := &engine.TSVFormat{}
	fields := engine.NewOrderedMap()
	fields.Set("a", engine.NewTypeLiteral(engine.TString))
	fields.Set("b", engine.NewTypeLiteral(engine.TString))
	rec := engine.RecordTypeInfo{Fields: fields}

	r0 := engine.NewOrderedMap()
	r0.Set("a", engine.NewString("x"))
	r0.Set("b", engine.NewString("y"))
	table := engine.Value{VType: engine.TList, Data: engine.TableData{
		Record: rec,
		Rows:   []engine.Value{engine.NewMap(r0)},
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

	f := &engine.JsonicFormat{
		Resolver: engine.MakeFileOpsResolver(mem),
	}

	// The @"part.jsonic" reference should be resolved and merged.
	result, err := f.Decode(`{@"part.jsonic", y:2}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	m := engine.AsMap(result[0])
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x' from resolved file")
	}
	_as14, _ := engine.AsInteger(xVal)
	if _as14 != 1 {
		t.Errorf("x = %v, want 1", xVal)
	}
	yVal, ok := m.Get("y")
	if !ok {
		t.Fatal("expected key 'y'")
	}
	_as15, _ := engine.AsInteger(yVal)
	if _as15 != 2 {
		t.Errorf("y = %v, want 2", yVal)
	}
}

func TestJsonicFormatMultisourceNested(t *testing.T) {
	// Test nested multisource: a.jsonic references b.jsonic.
	mem := fileops.NewMem()
	mem.Files["b.jsonic"] = []byte(`{nested: true}`)
	mem.Files["a.jsonic"] = []byte(`{@"b.jsonic", top: 1}`)

	f := &engine.JsonicFormat{
		Resolver: engine.MakeFileOpsResolver(mem),
	}

	result, err := f.Decode(`{@"a.jsonic", outer: 99}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := engine.AsMap(result[0])

	outerVal, ok := m.Get("outer")
	if !ok {
		t.Fatal("expected key 'outer'")
	}
	_as16, _ := engine.AsInteger(outerVal)
	if _as16 != 99 {
		t.Errorf("outer = %v, want 99", outerVal)
	}

	topVal, ok := m.Get("top")
	if !ok {
		t.Fatal("expected key 'top' from a.jsonic")
	}
	_as17, _ := engine.AsInteger(topVal)
	if _as17 != 1 {
		t.Errorf("top = %v, want 1", topVal)
	}

	nestedVal, ok := m.Get("nested")
	if !ok {
		t.Fatal("expected key 'nested' from b.jsonic")
	}
	_as18, _ := engine.AsBoolean(nestedVal)
	if !_as18 {
		t.Errorf("nested = %v, want true", nestedVal)
	}
}

func TestJsonicFormatWithoutResolverNoMultisource(t *testing.T) {
	// Without a resolver, the jsonic format should work as before
	// (no multisource, just plain jsonic parsing).
	f := &engine.JsonicFormat{}
	result, err := f.Decode(`{a:1, b:2}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].VType.Equal(engine.TMap) {
		t.Errorf("expected map, got %v", result)
	}
}

func TestJsonicFormatMultisourceNotUsedForJSON(t *testing.T) {
	// JSONFormat must NOT use multisource — it's strict JSON only.
	f := &engine.JSONFormat{}
	// This is valid JSON with an @ in a key — should parse as-is.
	result, err := f.Decode(`{"@ref": "value"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := engine.AsMap(result[0])
	v, ok := m.Get("@ref")
	if !ok {
		t.Fatal("expected key '@ref' in JSON output")
	}
	_as19, _ := engine.AsString(v)
	if _as19 != "value" {
		_as20, _ := engine.AsString(v)
		t.Errorf("got %q, want %q", _as20, "value")
	}
}

func TestJsonicFormatMultisourceNotUsedForText(t *testing.T) {
	// TextFormat must NOT use multisource.
	f := &engine.TextFormat{}
	result, err := f.Decode(`@"somefile.jsonic"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Text format returns raw string, no resolution.
	_as21, _ := engine.AsString(result[0])
	if _as21 != `@"somefile.jsonic"` {
		_as22, _ := engine.AsString(result[0])
		t.Errorf("text format should return raw content, got %q", _as22)
	}
}

func TestMakeFileOpsResolverFindsFile(t *testing.T) {
	mem := fileops.NewMem()
	mem.Files["data.jsonic"] = []byte(`{found:true}`)

	resolver := engine.MakeFileOpsResolver(mem)
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
	resolver := engine.MakeFileOpsResolver(mem)
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

	resolver := engine.MakeFileOpsResolver(mem)
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
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	jf, ok := engine.HostFormats(r)["jsonic"].(*engine.JsonicFormat)
	if !ok {
		t.Fatal("jsonic format should be *JsonicFormat")
	}
	if jf.Resolver == nil {
		t.Error("jsonic format in registry should have a resolver set")
	}
}

func TestRegistryJSONFormatHasNoResolver(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// JSON format should remain unchanged — no multisource.
	_, ok := engine.HostFormats(r)["json"].(*engine.JSONFormat)
	if !ok {
		t.Fatal("json format should be *JSONFormat, not modified")
	}
}

func TestSetFileOpsUpdatesJsonicResolver(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	mem := fileops.NewMem()
	mem.Files["test.jsonic"] = []byte(`{val:42}`)
	engine.SetHostFileOps(r, mem)

	jf := engine.HostFormats(r)["jsonic"].(*engine.JsonicFormat)
	if jf.Resolver == nil {
		t.Fatal("expected resolver to be updated after SetFileOps")
	}

	// Verify the resolver uses the new FileOps.
	result, err := jf.Decode(`{@"test.jsonic"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := engine.AsMap(result[0])
	v, ok := m.Get("val")
	if !ok {
		t.Fatal("expected key 'val' from resolved test.jsonic")
	}
	_as23, _ := engine.AsInteger(v)
	if _as23 != 42 {
		t.Errorf("val = %v, want 42", v)
	}
}
