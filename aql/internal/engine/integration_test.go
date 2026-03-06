package engine

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
)

// helper to run AQL expressions through the engine and return results
func runAQL(t *testing.T, r *Registry, tokens []Value) []Value {
	t.Helper()
	e := New(r)
	result, err := e.Run(tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

func runAQLError(t *testing.T, r *Registry, tokens []Value) error {
	t.Helper()
	e := New(r)
	_, err := e.Run(tokens)
	return err
}

// --- Comparison word integration tests ---

func TestEngineLt(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(1), NewWord("lt"), NewInteger(2)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("1 lt 2 = %v, want true", result)
	}
}

func TestEngineGt(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(3), NewWord("gt"), NewInteger(1)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("3 gt 1 = %v, want true", result)
	}
}

func TestEngineLte(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(1), NewWord("lte"), NewInteger(1)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("1 lte 1 = %v, want true", result)
	}
}

func TestEngineGte(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(2), NewWord("gte"), NewInteger(1)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("2 gte 1 = %v, want true", result)
	}
}

func TestEngineEq(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(5), NewWord("eq"), NewInteger(5)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("5 eq 5 = %v, want true", result)
	}
}

func TestEngineNeq(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(5), NewWord("neq"), NewInteger(3)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("5 neq 3 = %v, want true", result)
	}
	result = runAQL(t, r, []Value{NewInteger(5), NewWord("neq"), NewInteger(5)})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("5 neq 5 = %v, want false", result)
	}
}

func TestEngineDeq(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewString("a"), NewWord("deq"), NewString("a")})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("'a' deq 'a' = %v, want true", result)
	}
}

func TestEngineLtError(t *testing.T) {
	r := DefaultRegistry()
	err := runAQLError(t, r, []Value{NewInteger(1), NewWord("lt"), NewString("a")})
	if err == nil {
		t.Error("expected error for cross-type lt")
	}
}

// --- If word integration tests ---

func TestEngineIf3True(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(true), NewInteger(1), NewInteger(2),
	})
	if len(result) != 1 || result[0].AsInteger() != 1 {
		t.Errorf("if true 1 2 = %v, want 1", result)
	}
}

func TestEngineIf3False(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(false), NewInteger(1), NewInteger(2),
	})
	if len(result) != 1 || result[0].AsInteger() != 2 {
		t.Errorf("if false 1 2 = %v, want 2", result)
	}
}

func TestEngineIf2True(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(true), NewInteger(42),
	})
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("if true 42 = %v, want 42", result)
	}
}

func TestEngineIf2False(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(false), NewInteger(42),
	})
	if len(result) != 0 {
		t.Errorf("if false 42 = %v, want empty", result)
	}
}

func TestEngineIfListCondition(t *testing.T) {
	r := DefaultRegistry()
	// if [1 lt 2] 10 20 → should evaluate condition [1 lt 2] → true → return 10
	condList := NewList([]Value{NewInteger(1), NewWord("lt"), NewInteger(2)})
	result := runAQL(t, r, []Value{
		NewWord("if"), condList, NewInteger(10), NewInteger(20),
	})
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("if [1 lt 2] 10 20 = %v, want 10", result)
	}
}

func TestEngineIfListBranch(t *testing.T) {
	r := DefaultRegistry()
	// if true [1 add 2] [3 add 4] → should evaluate [1 add 2] → 3
	thenList := NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	elseList := NewList([]Value{NewInteger(3), NewWord("add"), NewInteger(4)})
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(true), thenList, elseList,
	})
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("if true [1 add 2] [3 add 4] = %v, want 3", result)
	}
}

func TestEngineIfFalsy(t *testing.T) {
	r := DefaultRegistry()
	// if 0 1 2 → 0 is falsy → return 2
	result := runAQL(t, r, []Value{
		NewWord("if"), NewInteger(0), NewInteger(1), NewInteger(2),
	})
	if len(result) != 1 || result[0].AsInteger() != 2 {
		t.Errorf("if 0 1 2 = %v, want 2", result)
	}
}

// --- File I/O integration tests ---

func TestEngineReadBasic(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	mem.Files["test.txt"] = []byte("hello world")
	r.SetFileOps(mem)

	result := runAQL(t, r, []Value{NewWord("read"), NewString("test.txt")})
	if len(result) != 1 || result[0].AsString() != "hello world" {
		t.Errorf("read 'test.txt' = %v, want 'hello world'", result)
	}
}

func TestEngineReadWithOpts(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	mem.Files["data.txt"] = []byte("a\nb\nc")
	r.SetFileOps(mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("lines"))
	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.txt"), NewMap(opts)})
	if len(result) != 1 || !result[0].VType.Equal(TList) {
		t.Errorf("read with lines fmt = %v, want list", result)
	}
	elems := result[0].AsList()
	if len(elems) != 3 {
		t.Errorf("expected 3 lines, got %d", len(elems))
	}
}

func TestEngineReadJSON(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	mem.Files["data.json"] = []byte(`{"x":1}`)
	r.SetFileOps(mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("json"))
	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.json"), NewMap(opts)})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Errorf("read json = %v, want map", result)
	}
}

func TestEngineReadNotFound(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	err := runAQLError(t, r, []Value{NewWord("read"), NewString("nope.txt")})
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestEngineReadUnknownFormat(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	mem.Files["test.txt"] = []byte("data")
	r.SetFileOps(mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("yaml"))
	err := runAQLError(t, r, []Value{NewWord("read"), NewString("test.txt"), NewMap(opts)})
	if err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestEngineWriteBasic(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	result := runAQL(t, r, []Value{NewWord("write"), NewString("out.txt"), NewString("hello")})
	if len(result) != 1 || result[0].AsString() != "out.txt" {
		t.Errorf("write result = %v, want 'out.txt'", result)
	}
	if string(mem.Files["out.txt"]) != "hello" {
		t.Errorf("file content = %q, want %q", mem.Files["out.txt"], "hello")
	}
}

func TestEngineWriteWithOpts(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	opts := NewOrderedMap()
	opts.Set("nl", NewString("crlf"))
	result := runAQL(t, r, []Value{NewWord("write"), NewString("out.txt"), NewString("a\nb"), NewMap(opts)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if string(mem.Files["out.txt"]) != "a\r\nb" {
		t.Errorf("file content = %q, want %q", mem.Files["out.txt"], "a\r\nb")
	}
}

func TestEngineWriteAppend(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	mem.Files["log.txt"] = []byte("first\n")
	r.SetFileOps(mem)

	opts := NewOrderedMap()
	opts.Set("mode", NewString("append"))
	runAQL(t, r, []Value{NewWord("write"), NewString("log.txt"), NewString("second\n"), NewMap(opts)})
	if string(mem.Files["log.txt"]) != "first\nsecond\n" {
		t.Errorf("file content = %q, want %q", mem.Files["log.txt"], "first\nsecond\n")
	}
}

func TestEngineWriteAppendNewFile(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	opts := NewOrderedMap()
	opts.Set("mode", NewString("append"))
	runAQL(t, r, []Value{NewWord("write"), NewString("new.txt"), NewString("data"), NewMap(opts)})
	if string(mem.Files["new.txt"]) != "data" {
		t.Errorf("file content = %q, want %q", mem.Files["new.txt"], "data")
	}
}

func TestEngineWriteAnyOpts(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	// write non-string (map) value with options → auto-serializes with jsonic
	// The write [string, any, map] signature expects path, data, opts
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	runAQL(t, r, []Value{NewString("out.json"), NewMap(m), NewMap(opts), NewWord("write")})
	content := string(mem.Files["out.json"])
	if content == "" {
		t.Errorf("file was not written")
	}
}

func TestEngineReadLineEndings(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	mem.Files["crlf.txt"] = []byte("a\r\nb\r\nc")
	r.SetFileOps(mem)

	// Default nl:"lf" normalizes \r\n to \n
	result := runAQL(t, r, []Value{NewWord("read"), NewString("crlf.txt")})
	if result[0].AsString() != "a\nb\nc" {
		t.Errorf("got %q, want %q", result[0].AsString(), "a\nb\nc")
	}

	// nl:"raw" preserves original
	opts := NewOrderedMap()
	opts.Set("nl", NewString("raw"))
	result = runAQL(t, r, []Value{NewWord("read"), NewString("crlf.txt"), NewMap(opts)})
	if result[0].AsString() != "a\r\nb\r\nc" {
		t.Errorf("raw got %q, want %q", result[0].AsString(), "a\r\nb\r\nc")
	}
}

// --- Registry integration tests ---

func TestRegistrySetFileOps(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)
	if r.FileOps != mem {
		t.Error("SetFileOps did not set the FileOps")
	}
}

func TestRegistryMatchNoFunction(t *testing.T) {
	r := DefaultRegistry()
	result := r.Match("nonexistent", []Value{}, WordInfo{})
	if result != nil {
		t.Error("expected nil for nonexistent function")
	}
}

// --- Additional engine tests for coverage ---

func TestEngineConvert(t *testing.T) {
	r := DefaultRegistry()
	// convert 99 string
	result := runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(99), NewWord("string"),
	})
	if len(result) != 1 || result[0].AsString() != "99" {
		t.Errorf("convert 99 string = %v, want '99'", result)
	}
}

func TestEngineTypeof(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewWord("typeof"), NewInteger(42)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestEngineBase(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewWord("base"), NewTypeLiteral(TInteger)})
	if len(result) != 1 || result[0].AsInteger() != 0 {
		t.Errorf("base integer = %v, want 0", result)
	}
}

func TestEngineDef(t *testing.T) {
	r := DefaultRegistry()
	// def inc [1 add] end 5 inc
	body := NewList([]Value{NewInteger(1), NewWord("add")})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("inc"), body, NewWord("end"),
		NewInteger(5), NewWord("inc"),
	})
	if len(result) != 1 || result[0].AsInteger() != 6 {
		t.Errorf("def inc [1 add]; 5 inc = %v, want 6", result)
	}
}

func TestEngineUndef(t *testing.T) {
	r := DefaultRegistry()
	// def foo 42 end foo undef foo end foo
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("foo"), NewInteger(42), NewWord("end"),
		NewWord("foo"),
		NewWord("undef"), NewWord("foo"), NewWord("end"),
		NewWord("foo"),
	})
	// After undef, foo becomes atom
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(result), result)
	}
	if result[0].AsInteger() != 42 {
		t.Errorf("first foo = %v, want 42", result[0])
	}
}

func TestEngineRecord(t *testing.T) {
	r := DefaultRegistry()
	e := New(r)
	// Parse a pair list manually: jsonic produces maps for x:number syntax
	m1 := NewOrderedMap()
	m1.Set("x", NewTypeLiteral(TNumber))
	m2 := NewOrderedMap()
	m2.Set("y", NewTypeLiteral(TString))
	list := NewList([]Value{NewMap(m1), NewMap(m2)})
	result, err := e.Run([]Value{NewWord("record"), list})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].IsRecordType() {
		t.Errorf("expected record type, got %v", result)
	}
}

func TestEngineTable(t *testing.T) {
	r := DefaultRegistry()
	e := New(r)
	// Create a record type first, then table
	m1 := NewOrderedMap()
	m1.Set("x", NewTypeLiteral(TNumber))
	list := NewList([]Value{NewMap(m1)})
	result, err := e.Run([]Value{NewWord("table"), NewWord("record"), list})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].IsTableType() {
		t.Errorf("expected table type, got %v", result)
	}
}

func TestEngineUnify(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(1), NewTypeLiteral(TNumber), NewWord("unify")})
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[1].AsBoolean() != true {
		t.Errorf("1 unify number = %v, want true", result[1])
	}
}

func TestEngineDo(t *testing.T) {
	r := DefaultRegistry()
	list := NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	result := runAQL(t, r, []Value{NewWord("do"), list})
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("do [1 add 2] = %v, want 3", result)
	}
}

func TestEngineDoMap(t *testing.T) {
	r := DefaultRegistry()
	m := NewOrderedMap()
	m.Set("x", NewList([]Value{NewInteger(3), NewWord("add"), NewInteger(4)}))
	result := runAQL(t, r, []Value{NewWord("do"), NewMap(m)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestEngineOr(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewBoolean(true), NewWord("or"), NewBoolean(false)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("true or false = %v, want true", result)
	}
}

func TestEngineAnd(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewBoolean(true), NewWord("and"), NewBoolean(false)})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("true and false = %v, want false", result)
	}
}

func TestEngineNot(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewBoolean(true), NewWord("not")})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("true not = %v, want false", result)
	}
}

func TestEngineConvertStringVariants(t *testing.T) {
	r := DefaultRegistry()
	// convert 10 string "hex" → 'a'
	result := runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(10), NewWord("string"), NewString("hex"),
	})
	if len(result) != 1 || result[0].AsString() != "a" {
		t.Errorf("convert 10 string hex = %v, want 'a'", result)
	}

	// convert 255 string "HEX" → 'FF'
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(255), NewWord("string"), NewString("HEX"),
	})
	if len(result) != 1 || result[0].AsString() != "FF" {
		t.Errorf("convert 255 string HEX = %v, want 'FF'", result)
	}

	// convert 10 string "bin" → '1010'
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(10), NewWord("string"), NewString("bin"),
	})
	if len(result) != 1 || result[0].AsString() != "1010" {
		t.Errorf("convert 10 string bin = %v, want '1010'", result)
	}

	// convert 8 string "oct" → '10'
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(8), NewWord("string"), NewString("oct"),
	})
	if len(result) != 1 || result[0].AsString() != "10" {
		t.Errorf("convert 8 string oct = %v, want '10'", result)
	}
}

func TestEngineConvertToNumber(t *testing.T) {
	r := DefaultRegistry()
	// convert "42" number → 42
	result := runAQL(t, r, []Value{
		NewWord("convert"), NewString("42"), NewWord("number"),
	})
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("convert '42' number = %v, want 42", result)
	}

	// convert "ff" number "hex" → 255
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewString("ff"), NewWord("number"), NewString("hex"),
	})
	if len(result) != 1 || result[0].AsInteger() != 255 {
		t.Errorf("convert 'ff' number hex = %v, want 255", result)
	}

	// convert "1010" number "bin" → 10
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewString("1010"), NewWord("number"), NewString("bin"),
	})
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("convert '1010' number bin = %v, want 10", result)
	}

	// convert "10" number "oct" → 8
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewString("10"), NewWord("number"), NewString("oct"),
	})
	if len(result) != 1 || result[0].AsInteger() != 8 {
		t.Errorf("convert '10' number oct = %v, want 8", result)
	}
}

func TestEngineConvertToBoolean(t *testing.T) {
	r := DefaultRegistry()
	// convert 1 boolean → true
	result := runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(1), NewWord("boolean"),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("convert 1 boolean = %v, want true", result)
	}

	// convert 0 boolean → false
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(0), NewWord("boolean"),
	})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("convert 0 boolean = %v, want false", result)
	}

	// convert "true" boolean → true
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewString("true"), NewWord("boolean"),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("convert 'true' boolean = %v, want true", result)
	}

	// convert "" boolean → false
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewString(""), NewWord("boolean"),
	})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("convert '' boolean = %v, want false", result)
	}

	// convert true boolean → true (passthrough)
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewBoolean(true), NewWord("boolean"),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("convert true boolean = %v, want true", result)
	}
}

func TestEngineConvertToAtom(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(42), NewWord("atom"),
	})
	if len(result) != 1 || !result[0].IsAtom() {
		t.Errorf("convert 42 atom = %v, want atom", result)
	}
}

func TestEngineBaseTypes(t *testing.T) {
	r := DefaultRegistry()
	tests := []struct {
		name     string
		typeLit  Type
		wantStr  string
	}{
		{"number", TNumber, "0"},
		{"string", TString, "''"},
		{"boolean", TBoolean, "false"},
		{"list", TList, "[]"},
		{"map", TMap, "{}"},
		{"none", TNone, "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runAQL(t, r, []Value{NewWord("base"), NewTypeLiteral(tt.typeLit)})
			if len(result) != 1 || result[0].String() != tt.wantStr {
				t.Errorf("base %s = %v, want %s", tt.name, result, tt.wantStr)
			}
		})
	}
}

func TestEngineFn(t *testing.T) {
	r := DefaultRegistry()
	// def double fn [[number] [number] [dup add]] end 7 double
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("number")}),
		NewList([]Value{NewWord("number")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("double"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(7), NewWord("double"),
	})
	if len(result) != 1 || result[0].AsInteger() != 14 {
		t.Errorf("def double fn; 7 double = %v, want 14", result)
	}
}

func TestEngineFnNamed(t *testing.T) {
	r := DefaultRegistry()
	// def square fn [[x:number] [number] [x mul x]] end 5 square
	xParam := NewOrderedMap()
	xParam.Set("x", NewWord("number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewMap(xParam)}),
		NewList([]Value{NewWord("number")}),
		NewList([]Value{NewWord("x"), NewWord("mul"), NewWord("x")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("square"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(5), NewWord("square"),
	})
	if len(result) != 1 || result[0].AsInteger() != 25 {
		t.Errorf("def square fn; 5 square = %v, want 25", result)
	}
}

func TestEngineFnCatterPrefixOnly(t *testing.T) {
	r := DefaultRegistry()
	// def catter fn [[integer string] [string] [add]] end
	// Case: [1 "a"|] -> catter -> all args from prefix
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("integer"), NewWord("string")}),
		NewList([]Value{NewWord("string")}),
		NewList([]Value{NewWord("add")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("catter"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(1), NewString("a"), NewWord("catter"),
	})
	if len(result) != 1 || !result[0].VType.Matches(TString) {
		t.Errorf("1 'a' catter = %v, want string result", result)
	}
}

func TestEngineFnCatterPartialSuffix(t *testing.T) {
	r := DefaultRegistry()
	// def catter fn [[integer string] [string] [add]] end
	// Case: [2|] -> catter "b" -> string from suffix, integer from prefix
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("integer"), NewWord("string")}),
		NewList([]Value{NewWord("string")}),
		NewList([]Value{NewWord("add")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("catter"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(2), NewWord("catter"), NewString("b"),
	})
	if len(result) != 1 || !result[0].VType.Matches(TString) {
		t.Errorf("2 catter 'b' = %v, want string result", result)
	}
}

func TestEngineFnCatterFullSuffix(t *testing.T) {
	r := DefaultRegistry()
	// def catter fn [[integer string] [string] [add]] end
	// Case: [|] -> catter "c" 3 -> both args from suffix
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("integer"), NewWord("string")}),
		NewList([]Value{NewWord("string")}),
		NewList([]Value{NewWord("add")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("catter"), NewWord("fn"), fnBody, NewWord("end"),
		NewWord("catter"), NewString("c"), NewInteger(3),
	})
	if len(result) != 1 || !result[0].VType.Matches(TString) {
		t.Errorf("catter 'c' 3 = %v, want string result", result)
	}
}

func TestIntegerLiteralType(t *testing.T) {
	// NewInteger encodes literal value in type path: number/integer/5
	v := NewInteger(5)
	if !v.VType.Matches(TInteger) {
		t.Errorf("NewInteger(5).VType = %s, want matches number/integer", v.VType)
	}
	if !v.VType.Matches(TNumber) {
		t.Errorf("NewInteger(5).VType = %s, want matches number", v.VType)
	}
	// Different integers have different types
	v0 := NewInteger(0)
	v1 := NewInteger(1)
	if v0.VType.Equal(v1.VType) {
		t.Errorf("NewInteger(0) and NewInteger(1) should have different types")
	}
	// But both match integer
	if !v0.VType.Matches(TInteger) || !v1.VType.Matches(TInteger) {
		t.Error("both should match integer")
	}
}

func TestEngineFnLiteralType(t *testing.T) {
	r := DefaultRegistry()
	// def adder fn [[0] [integer] [add 2]] end
	// adder only matches the value 0, adds 2 to it
	fnBody := NewList([]Value{
		NewList([]Value{NewInteger(0)}),
		NewList([]Value{NewWord("integer")}),
		NewList([]Value{NewWord("add"), NewInteger(2)}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("adder"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(0), NewWord("adder"),
	})
	if len(result) != 1 || result[0].AsInteger() != 2 {
		t.Errorf("0 adder = %v, want 2", result)
	}
}

func TestEngineFnLiteralTypeNoMatch(t *testing.T) {
	r := DefaultRegistry()
	// def adder fn [[0] [integer] [add 2]] end
	// adder should NOT match 5 (only matches 0)
	fnBody := NewList([]Value{
		NewList([]Value{NewInteger(0)}),
		NewList([]Value{NewWord("integer")}),
		NewList([]Value{NewWord("add"), NewInteger(2)}),
	})
	err := runAQLError(t, r, []Value{
		NewWord("def"), NewWord("adder"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(5), NewWord("adder"),
	})
	if err == nil {
		t.Error("expected error: adder should not match 5")
	}
}

func TestEngineFnLiteralTypeMultiSig(t *testing.T) {
	r := DefaultRegistry()
	// def handler fn [[0] [integer] [add 10] [1] [integer] [add 20]] end
	// handler 0 → 10, handler 1 → 21
	fnBody := NewList([]Value{
		NewList([]Value{NewInteger(0)}),
		NewList([]Value{NewWord("integer")}),
		NewList([]Value{NewWord("add"), NewInteger(10)}),
		NewList([]Value{NewInteger(1)}),
		NewList([]Value{NewWord("integer")}),
		NewList([]Value{NewWord("add"), NewInteger(20)}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("handler"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(0), NewWord("handler"),
	})
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("0 handler = %v, want 10", result)
	}

	result = runAQL(t, r, []Value{
		NewInteger(1), NewWord("handler"),
	})
	if len(result) != 1 || result[0].AsInteger() != 21 {
		t.Errorf("1 handler = %v, want 21", result)
	}
}

func TestEngineTypeRecord(t *testing.T) {
	r := DefaultRegistry()
	// type Point record [x:number y:number] end Point
	xf := NewOrderedMap()
	xf.Set("x", NewTypeLiteral(TNumber))
	yf := NewOrderedMap()
	yf.Set("y", NewTypeLiteral(TNumber))
	fields := NewList([]Value{NewMap(xf), NewMap(yf)})
	result := runAQL(t, r, []Value{
		NewWord("type"), NewWord("Point"), NewWord("record"), fields, NewWord("end"),
		NewWord("Point"),
	})
	if len(result) != 1 || !result[0].IsRecordType() {
		t.Errorf("expected record type, got %v", result)
	}
}

func TestEngineMakeRecord(t *testing.T) {
	r := DefaultRegistry()
	// type P record [x:number y:string] end make P [1 "hi"]
	xf := NewOrderedMap()
	xf.Set("x", NewTypeLiteral(TNumber))
	yf := NewOrderedMap()
	yf.Set("y", NewTypeLiteral(TString))
	fields := NewList([]Value{NewMap(xf), NewMap(yf)})
	vals := NewList([]Value{NewInteger(1), NewString("hi")})
	result := runAQL(t, r, []Value{
		NewWord("type"), NewWord("P"), NewWord("record"), fields, NewWord("end"),
		NewWord("make"), NewWord("P"), vals,
	})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Errorf("expected map, got %v", result)
	}
	m := result[0].AsMap()
	xVal, _ := m.Get("x")
	if xVal.AsInteger() != 1 {
		t.Errorf("x = %v, want 1", xVal)
	}
}

func TestEngineUnifyMaps(t *testing.T) {
	r := DefaultRegistry()
	// {x:1} unify {x:1}
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("x", NewInteger(1))
	result := runAQL(t, r, []Value{NewMap(m1), NewMap(m2), NewWord("unify")})
	if len(result) != 2 || !result[1].AsBoolean() {
		t.Errorf("{x:1} unify {x:1} = %v, want true", result)
	}
}

func TestEngineUnifyLists(t *testing.T) {
	r := DefaultRegistry()
	l1 := NewList([]Value{NewInteger(1), NewInteger(2)})
	l2 := NewList([]Value{NewInteger(1), NewInteger(2)})
	result := runAQL(t, r, []Value{l1, l2, NewWord("unify")})
	if len(result) != 2 || !result[1].AsBoolean() {
		t.Errorf("[1,2] unify [1,2] = %v, want true", result)
	}
}

func TestEngineUnifyFail(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(1), NewString("a"), NewWord("unify")})
	if len(result) != 2 || result[1].AsBoolean() {
		t.Errorf("1 unify 'a' = %v, want false", result)
	}
}

func TestEngineUnifyTypedList(t *testing.T) {
	r := DefaultRegistry()
	tl := NewTypedList(NewTypeLiteral(TNumber))
	cl := NewList([]Value{NewInteger(1), NewInteger(2)})
	result := runAQL(t, r, []Value{tl, cl, NewWord("unify")})
	if len(result) != 2 || !result[1].AsBoolean() {
		t.Errorf("[:number] unify [1,2] = %v, want true", result)
	}
}

func TestEngineUnifyTypedMap(t *testing.T) {
	r := DefaultRegistry()
	tm := NewTypedMap(NewTypeLiteral(TNumber))
	cm := NewOrderedMap()
	cm.Set("a", NewInteger(1))
	cm.Set("b", NewInteger(2))
	result := runAQL(t, r, []Value{tm, NewMap(cm), NewWord("unify")})
	if len(result) != 2 || !result[1].AsBoolean() {
		t.Errorf("{:number} unify {a:1,b:2} = %v, want true", result)
	}
}

func TestEngineDisjunct(t *testing.T) {
	r := DefaultRegistry()
	// string or none
	result := runAQL(t, r, []Value{
		NewTypeLiteral(TString), NewWord("or"), NewTypeLiteral(TNone),
	})
	if len(result) != 1 || !result[0].IsDisjunct() {
		t.Errorf("string or none = %v, want disjunct", result)
	}
}

func TestEngineVar(t *testing.T) {
	r := DefaultRegistry()
	// 5 var [[x] x mul x]
	varBody := NewList([]Value{
		NewList([]Value{NewWord("x")}),
		NewWord("x"), NewWord("mul"), NewWord("x"),
	})
	result := runAQL(t, r, []Value{
		NewInteger(5), NewWord("var"), varBody,
	})
	if len(result) != 1 || result[0].AsInteger() != 25 {
		t.Errorf("5 var [[x] x mul x] = %v, want 25", result)
	}
}

func TestEngineAddStrings(t *testing.T) {
	r := DefaultRegistry()
	result := runAQL(t, r, []Value{NewString("hello"), NewWord("add"), NewString(" world")})
	if len(result) != 1 || result[0].AsString() != "hello world" {
		t.Errorf("'hello' add ' world' = %v, want 'hello world'", result)
	}
}

// --- valToString coverage ---

func TestValToString(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want string
	}{
		{"string", NewString("hello"), "hello"},
		{"integer", NewInteger(42), "42"},
		{"atom", NewAtom("foo"), "foo"},
		{"boolean", NewBoolean(true), "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := valToString(tt.val)
			if got != tt.want {
				t.Errorf("valToString(%s) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func TestEngineReadCSVByExtension(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	mem.Files["data.csv"] = []byte("name,age\nAlice,30\nBob,25")
	r.SetFileOps(mem)

	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.csv")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !v.IsTableType() {
		t.Fatalf("expected table type, got %s", v.VType)
	}
	rows := v.AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	r0 := rows[0].AsMap()
	nameVal, ok := r0.Get("name")
	if !ok {
		t.Fatal("expected 'name' key")
	}
	if nameVal.AsString() != "Alice" {
		t.Errorf("name = %q, want %q", nameVal.AsString(), "Alice")
	}
}

func TestEngineReadTSVByExtension(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	mem.Files["data.tsv"] = []byte("name\tage\nAlice\t30\nBob\t25")
	r.SetFileOps(mem)

	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.tsv")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !v.IsTableType() {
		t.Fatalf("expected table type, got %s", v.VType)
	}
	rows := v.AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestEngineReadCSVExplicitFormat(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	mem.Files["data.txt"] = []byte("a,b\n1,2")
	r.SetFileOps(mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("csv"))
	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.txt"), NewMap(opts)})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !v.IsTableType() {
		t.Fatalf("expected table type, got %s", v.VType)
	}
}

func TestEngineReadOverrideExtension(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	mem.Files["data.csv"] = []byte("hello,world")
	r.SetFileOps(mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.csv"), NewMap(opts)})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	// With text format, we get a plain string, not a table
	if result[0].IsTableType() {
		t.Error("expected non-table type with text format override")
	}
	if result[0].AsString() != "hello,world" {
		t.Errorf("got %q, want %q", result[0].AsString(), "hello,world")
	}
}

func TestEngineReadJSONByExtension(t *testing.T) {
	r := DefaultRegistry()
	mem := fileops.NewMem()
	mem.Files["data.json"] = []byte(`{"key":"value"}`)
	r.SetFileOps(mem)

	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.json")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	if !result[0].VType.Equal(TMap) {
		t.Errorf("expected map type, got %s", result[0].VType)
	}
}

func TestFormatFromExt(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"file.csv", "csv"},
		{"file.tsv", "tsv"},
		{"file.json", "json"},
		{"file.jsonic", "jsonic"},
		{"file.txt", "text"},
		{"file.unknown", ""},
		{"file", ""},
		{"path/to/data.CSV", "csv"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := formatFromExt(tt.path)
			if got != tt.want {
				t.Errorf("formatFromExt(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
