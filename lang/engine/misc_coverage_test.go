package engine

import (
	"bytes"
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/lang/internal/fileops"
)

// ── Format coverage ──────────────────────────────────────────────────

func TestJsonicFormatDecodeNil(t *testing.T) {
	f := &JsonicFormat{}
	vals, err := f.Decode("")
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
}

func TestLinesFormatEncodeNonListCov(t *testing.T) {
	f := &LinesFormat{}
	out, err := f.Encode(NewString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if out != "'hello'" {
		t.Errorf("expected string repr, got %q", out)
	}
}

func TestLinesFormatEncodeList(t *testing.T) {
	f := &LinesFormat{}
	elems := []Value{NewString("line1"), NewInteger(42)}
	out, err := f.Encode(NewList(elems))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "line1") || !strings.Contains(out, "42") {
		t.Errorf("expected lines, got %q", out)
	}
}

func TestLinesFormatDecodeCov(t *testing.T) {
	f := &LinesFormat{}
	vals, err := f.Decode("line1\nline2\nline3")
	if err != nil {
		t.Fatal(err)
	}
	list := vals[0].AsList().Slice()
	if len(list) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(list))
	}
}

func TestEncodeDelimitedQueryBuilder(t *testing.T) {
	// encodeDelimited with a non-table value returns String()
	out, err := encodeDelimited(NewString("test"), ",")
	if err != nil {
		t.Fatal(err)
	}
	if out != "'test'" {
		t.Errorf("expected string repr, got %q", out)
	}
}

func TestEncodeDelimitedEmptyColumns(t *testing.T) {
	td := TableData{
		Record: RecordTypeInfo{Fields: NewOrderedMap()},
		Rows:   []Value{},
	}
	out, err := encodeDelimited(Value{VType: TList, Data: td}, ",")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}

// ── jsonicToValue coverage ───────────────────────────────────────────

func TestJsonicToValueBool(t *testing.T) {
	v, err := jsonicToValue(true)
	if err != nil {
		t.Fatal(err)
	}
	_as0, _ := v.AsBoolean()
	if !_as0 {
		t.Error("expected true")
	}
}

func TestJsonicToValueFloat(t *testing.T) {
	v, err := jsonicToValue(3.14)
	if err != nil {
		t.Fatal(err)
	}
	_as1, _ := v.AsNumber()
	if _as1 != 3.14 {
		_as2, _ := v.AsNumber()
		t.Errorf("expected 3.14, got %f", _as2)
	}
}

func TestJsonicToValueIntegerFloat(t *testing.T) {
	v, err := jsonicToValue(float64(42))
	if err != nil {
		t.Fatal(err)
	}
	_as3, _ := v.AsInteger()
	if _as3 != 42 {
		_as4, _ := v.AsInteger()
		t.Errorf("expected 42, got %d", _as4)
	}
}

func TestJsonicToValueList(t *testing.T) {
	v, err := jsonicToValue([]any{"hello", float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	list := v.AsList().Slice()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
}

func TestJsonicToValueMap(t *testing.T) {
	v, err := jsonicToValue(map[string]any{"a": float64(1), "b": "two"})
	if err != nil {
		t.Fatal(err)
	}
	m := v.AsMap()
	if m.Len() != 2 {
		t.Errorf("expected 2 keys, got %d", m.Len())
	}
}

func TestJsonicToValueUnsupported(t *testing.T) {
	_, err := jsonicToValue(struct{}{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestJsonicToValueNil(t *testing.T) {
	v, err := jsonicToValue(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.VType.Equal(TNone) {
		t.Errorf("expected TNone, got %s", v.VType)
	}
}

// ── valueToJsonic coverage ───────────────────────────────────────────

func TestValueToJsonicNone(t *testing.T) {
	v := NewTypeLiteral(TNone)
	s := valueToJsonic(v)
	if s != "null" {
		t.Errorf("expected null, got %s", s)
	}
}

func TestValueToJsonicAtom(t *testing.T) {
	v := NewAtom("test")
	s := valueToJsonic(v)
	if s != `"test"` {
		t.Errorf("expected quoted test, got %s", s)
	}
}

func TestValueToJsonicListCov(t *testing.T) {
	v := NewList([]Value{NewInteger(1), NewString("two")})
	s := valueToJsonic(v)
	if !strings.Contains(s, "1") || !strings.Contains(s, `"two"`) {
		t.Errorf("expected list encoding, got %s", s)
	}
}

func TestValueToJsonicMapCov(t *testing.T) {
	om := NewOrderedMap()
	om.Set("key", NewInteger(42))
	v := NewMap(om)
	s := valueToJsonic(v)
	if !strings.Contains(s, "key") || !strings.Contains(s, "42") {
		t.Errorf("expected map encoding, got %s", s)
	}
}

func TestValueToJsonicDecimal(t *testing.T) {
	v := NewDecimal(3.14)
	s := valueToJsonic(v)
	if s != "3.14" {
		t.Errorf("expected 3.14, got %s", s)
	}
}

func TestValueToJsonicBoolean(t *testing.T) {
	if valueToJsonic(NewBoolean(true)) != "true" {
		t.Error("expected true")
	}
	if valueToJsonic(NewBoolean(false)) != "false" {
		t.Error("expected false")
	}
}

// ── Trace coverage ───────────────────────────────────────────────────

func TestTraceCoverage(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	// trace [1 2 add]
	result := runAQL(t, r, []Value{
		NewWord("trace"),
		NewList([]Value{NewInteger(1), NewInteger(2), NewWord("add")}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	_as5, _ := result[0].AsNumber()
	if _as5 != 3 {
		t.Errorf("expected 3, got %v", result[0])
	}
	output := buf.String()
	if !strings.Contains(output, "trace") {
		t.Errorf("expected trace output, got %q", output)
	}
}

// ── SQLite REGEXP coverage ───────────────────────────────────────────

func TestSQLiteRegexp(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("name", NewString("alice"))
	row2 := NewOrderedMap()
	row2.Set("name", NewString("bob"))
	row3 := NewOrderedMap()
	row3.Set("name", NewString("charlie"))

	td := TableData{
		Record: rec,
		Rows:   []Value{NewMap(row1), NewMap(row2), NewMap(row3)},
	}
	err = store.StoreTable("people", td)
	if err != nil {
		t.Fatal(err)
	}

	// Test REGEXP function
	result, err := store.Query(`SELECT * FROM "people" WHERE "name" REGEXP '^[ab]'`, &rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows matching regexp, got %d", len(result.Rows))
	}
}

// ── fileio read/write integration ────────────────────────────────────

func TestReadWriteJsonic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	mem.Files["data.jsonic"] = []byte(`{a: 1, b: "hello"}`)
	SetHostFileOps(r, mem)

	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("data.jsonic"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	v, ok := m.Get("a")
	if !ok {
		t.Fatal("expected key 'a'")
	}
	_as6, _ := v.AsInteger()
	if _as6 != 1 {
		_as7, _ := v.AsInteger()
		t.Errorf("expected 1, got %d", _as7)
	}
}

func TestReadWriteJSON(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	mem.Files["data.json"] = []byte(`{"x": 42}`)
	SetHostFileOps(r, mem)

	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("data.json"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestReadWriteLines(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	SetHostFileOps(r, mem)

	// Write lines format
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("lines"))
	// All prefix: nearest→sig[0]=path, next→sig[1]=data, deepest→sig[2]=opts
	result := runAQL(t, r, []Value{
		NewMap(opts), NewString("hello\nworld"), NewString("out.txt"), NewWord("write"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestReadWriteText(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	mem.Files["hello.txt"] = []byte("hello world")
	SetHostFileOps(r, mem)

	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("hello.txt"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	_as8, _ := result[0].AsString()
	if _as8 != "hello world" {
		_as9, _ := result[0].AsString()
		t.Errorf("expected hello world, got %s", _as9)
	}
}

func TestWriteStdout(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	// write to stdout using the explicit "<stdout>" path.
	result := runAQL(t, r, []Value{
		NewWord("write"), NewString("<stdout>"), NewString("hello"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("expected hello in stdout, got %q", buf.String())
	}
}

func TestWriteAppendMode(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	mem.Files["out.txt"] = []byte("first\n")
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("mode", NewString("append"))
	// All prefix: nearest→sig[0]=path, next→sig[1]=data, deepest→sig[2]=opts
	result := runAQL(t, r, []Value{
		NewMap(opts), NewString("second"), NewString("out.txt"), NewWord("write"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	content := string(mem.Files["out.txt"])
	if !strings.Contains(content, "first") || !strings.Contains(content, "second") {
		t.Errorf("expected appended content, got %q", content)
	}
}

// ── print coverage ───────────────────────────────────────────────────

func TestPrintCoverage(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	// print a map
	om := NewOrderedMap()
	om.Set("key", NewString("value"))
	runAQL(t, r, []Value{
		NewMap(om), NewWord("print"),
	})
	output := buf.String()
	if output == "" {
		t.Error("expected print output")
	}
}

func TestPrintListCoverage(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf

	runAQL(t, r, []Value{
		NewList([]Value{NewInteger(1), NewInteger(2)}), NewWord("print"),
	})
	output := buf.String()
	if output == "" {
		t.Error("expected print output")
	}
}

// ── SQLite StoreTable with multiple rows ─────────────────────────────

func TestSQLiteStoreMultipleRows(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	fields.Set("name", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	rows := make([]Value, 10)
	for i := range rows {
		row := NewOrderedMap()
		row.Set("id", NewInteger(int64(i)))
		row.Set("name", NewString("name"))
		rows[i] = NewMap(row)
	}

	td := TableData{Record: rec, Rows: rows}
	err = store.StoreTable("batch", td)
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.Query(`SELECT * FROM "batch" ORDER BY "id"`, &rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 10 {
		t.Errorf("expected 10 rows, got %d", len(result.Rows))
	}
}

// ── registry coverage ────────────────────────────────────────────────

func TestRegistryMatchNoSig(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Try matching with no args against a word that needs args
	result := r.Match("add", []Value{}, WordInfo{})
	if result != nil {
		t.Error("expected nil result for no matching signature")
	}
}

// ── TraceColorize coverage ──────────────────────────────────────────

func TestTraceColorizeCoverage(t *testing.T) {
	// Exercise TraceColorize with different value types
	s := TraceColorize(NewInteger(42))
	if s == "" {
		t.Error("expected non-empty")
	}
	s = TraceColorize(NewString("hello"))
	if s == "" {
		t.Error("expected non-empty")
	}
	s = TraceColorize(NewBoolean(true))
	if s == "" {
		t.Error("expected non-empty")
	}
	s = TraceColorize(NewWord("add"))
	if s == "" {
		t.Error("expected non-empty")
	}
	s = TraceColorize(NewAtom("test"))
	if s == "" {
		t.Error("expected non-empty")
	}
	s = TraceColorize(NewList([]Value{}))
	if s == "" {
		t.Error("expected non-empty")
	}
	s = TraceColorize(NewMap(NewOrderedMap()))
	if s == "" {
		t.Error("expected non-empty")
	}
}

// ── Module file import coverage ──────────────────────────────────────

func TestModuleImportFromFile(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	// Module file that exports "greet" with value "hello"
	mem.Files["mod.aql"] = []byte(`export greet {val: 'world'}`)
	SetHostFileOps(r, mem)
	r.ParseFunc = func(src string) ([]Value, error) {
		// Simple parse: export greet {val: 'world'}
		m := NewOrderedMap()
		m.Set("val", NewString("world"))
		return []Value{
			NewWord("export"), NewAtom("greet"), NewMap(m),
		}, nil
	}

	result := runAQL(t, r, []Value{
		NewWord("import"), NewString("./mod.aql"),
	})
	_ = result

	// Verify the export was installed
	if !r.HasDef("greet") {
		t.Error("expected 'greet' to be defined after import")
	}
}

func TestModuleImportFileWithRename(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	mem.Files["mod2.aql"] = []byte(`export foo {val: 42}`)
	SetHostFileOps(r, mem)
	r.ParseFunc = func(src string) ([]Value, error) {
		m := NewOrderedMap()
		m.Set("val", NewInteger(42))
		return []Value{
			NewWord("export"), NewAtom("foo"), NewMap(m),
		}, nil
	}

	result := runAQL(t, r, []Value{
		NewWord("import"),
		NewList([]Value{NewAtom("foo"), NewAtom("bar")}),
		NewString("./mod2.aql"),
	})
	_ = result

	if !r.HasDef("bar") {
		t.Error("expected 'bar' to be defined after renamed import")
	}
}

func TestModuleExportWithStringName(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Module with string-named export
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	result := runAQL(t, r, []Value{
		NewWord("module"),
		NewList([]Value{
			NewWord("export"), NewString("myexport"), NewMap(m),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestModuleImportSelectedExports(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("y", NewInteger(2))

	// Create module with two exports
	result := runAQL(t, r, []Value{
		NewWord("module"),
		NewList([]Value{
			NewWord("export"), NewAtom("a"), NewMap(m1),
			NewWord("export"), NewAtom("b"), NewMap(m2),
		}),
	})

	// Import only "a" via rename
	runAQL(t, r, []Value{
		NewWord("import"),
		NewList([]Value{NewAtom("a"), NewAtom("alpha")}),
		result[0],
	})

	if !r.HasDef("alpha") {
		t.Error("expected 'alpha' to be defined")
	}
}

func TestModuleImportMultipleRenames(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("y", NewInteger(2))

	result := runAQL(t, r, []Value{
		NewWord("module"),
		NewList([]Value{
			NewWord("export"), NewAtom("p"), NewMap(m1),
			NewWord("export"), NewAtom("q"), NewMap(m2),
		}),
	})

	// Import with multiple renames [[p r] [q s]]
	runAQL(t, r, []Value{
		NewWord("import"),
		NewList([]Value{
			NewList([]Value{NewAtom("p"), NewAtom("r")}),
			NewList([]Value{NewAtom("q"), NewAtom("s")}),
		}),
		result[0],
	})

	if !r.HasDef("r") {
		t.Error("expected 'r' to be defined")
	}
	if !r.HasDef("s") {
		t.Error("expected 's' to be defined")
	}
}

// ── Math binary ops with decimal coverage ────────────────────────────

// TestMathMinMaxDecimal moved to internal/nativemod/ (aql:math module).

// ── Make table ───────────────────────────────────────────────────────

func TestMakeTable(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Define a table type
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TInteger))
	tableType := Value{VType: TList, Data: TableTypeInfo{Record: RecordTypeInfo{Fields: fields}}}

	// make table from positional rows
	rowData := NewList([]Value{
		NewList([]Value{NewString("Alice"), NewInteger(30)}),
		NewList([]Value{NewString("Bob"), NewInteger(25)}),
	})

	result := runAQL(t, r, []Value{
		NewWord("make"), tableType, rowData,
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	list := result[0].AsList().Slice()
	if len(list) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(list))
	}
}

func TestMakeRecordWithBase(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Define a record type
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TInteger))
	recType := Value{VType: TMap, Data: RecordTypeInfo{Fields: fields}}

	// make record with only name, using base:true to fill age with default
	opts := NewOrderedMap()
	opts.Set("base", NewBoolean(true))
	src := NewOrderedMap()
	src.Set("name", NewString("Alice"))
	result := runAQL(t, r, []Value{
		NewWord("make"), recType, NewMap(src), NewMap(opts),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	if m.Len() != 2 {
		t.Errorf("expected 2 fields, got %d", m.Len())
	}
}

func TestMakeRecordWithNamedList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	fields.Set("y", NewTypeLiteral(TString))
	recType := Value{VType: TMap, Data: RecordTypeInfo{Fields: fields}}

	// Named list form: [{x: 1} {y: "hello"}]
	xm := NewOrderedMap()
	xm.Set("x", NewInteger(42))
	ym := NewOrderedMap()
	ym.Set("y", NewString("hi"))

	result := runAQL(t, r, []Value{
		NewWord("make"), recType, NewList([]Value{NewMap(xm), NewMap(ym)}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestMakeConvertDecimalToString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TString), NewDecimal(3.14),
	})
	_as10, _ := result[0].AsString()
	if _as10 != "3.14" {
		_as11, _ := result[0].AsString()
		t.Errorf("expected '3.14', got %s", _as11)
	}
}

func TestMakeConvertBoolFromNumber(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewInteger(1),
	})
	_as12, _ := result[0].AsBoolean()
	if !_as12 {
		t.Error("expected true from 1")
	}
	result = runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewInteger(0),
	})
	_as13, _ := result[0].AsBoolean()
	if _as13 {
		t.Error("expected false from 0")
	}
}

func TestMakeConvertBoolFromNonBoolString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewString("hello"),
	})
	_as14, _ := result[0].AsBoolean()
	if !_as14 {
		t.Error("expected true from non-empty string")
	}
	result = runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewString(""),
	})
	_as15, _ := result[0].AsBoolean()
	if _as15 {
		t.Error("expected false from empty string")
	}
}

func TestMakeConvertToAtomFromString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TAtom), NewString("hello"),
	})
	_as16, _ := result[0].AsAtom()
	if _as16 != "hello" {
		t.Errorf("expected hello atom, got %v", result[0])
	}
}

func TestMakeConvertFloatStringToNumber(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// "3.14" to integer should parse as float then truncate
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TInteger), NewString("3.14"),
	})
	_as17, _ := result[0].AsInteger()
	if _as17 != 3 {
		_as18, _ := result[0].AsInteger()
		t.Errorf("expected 3, got %d", _as18)
	}
}
