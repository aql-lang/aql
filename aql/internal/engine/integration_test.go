package engine

import (
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
)

// helper to run AQL expressions through the engine and return results
func runAQL(t *testing.T, r *Registry, tokens []Value) []Value {
	t.Helper()
	e := NewTop(r)
	result, err := e.Run(tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

func runAQLError(t *testing.T, r *Registry, tokens []Value) error {
	t.Helper()
	e := NewTop(r)
	_, err := e.Run(tokens)
	return err
}

// --- Comparison word integration tests ---

func TestEngineLt(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(1), NewWord("lt"), NewInteger(2)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("1 lt 2 = %v, want true", result)
	}
}

func TestEngineGt(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(3), NewWord("gt"), NewInteger(1)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("3 gt 1 = %v, want true", result)
	}
}

func TestEngineLte(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(1), NewWord("lte"), NewInteger(1)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("1 lte 1 = %v, want true", result)
	}
}

func TestEngineGte(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(2), NewWord("gte"), NewInteger(1)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("2 gte 1 = %v, want true", result)
	}
}

func TestEngineEq(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(5), NewWord("eq"), NewInteger(5)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("5 eq 5 = %v, want true", result)
	}
}

func TestEngineNeq(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewString("a"), NewWord("deq"), NewString("a")})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("'a' deq 'a' = %v, want true", result)
	}
}

func TestEngineLtError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	err = runAQLError(t, r, []Value{NewInteger(1), NewWord("lt"), NewString("a")})
	if err == nil {
		t.Error("expected error for cross-type lt")
	}
}

// --- If word integration tests ---

func TestEngineIf3True(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(true), NewInteger(1), NewInteger(2),
	})
	if len(result) != 1 || result[0].AsInteger() != 1 {
		t.Errorf("if true 1 2 = %v, want 1", result)
	}
}

func TestEngineIf3False(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(false), NewInteger(1), NewInteger(2),
	})
	if len(result) != 1 || result[0].AsInteger() != 2 {
		t.Errorf("if false 1 2 = %v, want 2", result)
	}
}

func TestEngineIf2True(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(true), NewInteger(42),
	})
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("if true 42 = %v, want 42", result)
	}
}

func TestEngineIf2False(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(false), NewInteger(42),
	})
	if len(result) != 0 {
		t.Errorf("if false 42 = %v, want empty", result)
	}
}

func TestEngineIfListCondition(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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

func TestEngineIfOnlyChosenBranchExecutes(t *testing.T) {
	// Register a side-effect word that increments a counter
	callCount := 0
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Register("side-effect",
		Signature{
			Args: []Type{TAny},
			Handler: func(args []Value) ([]Value, error) {
				callCount++
				return args, nil
			},
		},
	)

	// if true [side-effect 1] [side-effect 2] → only then-branch runs
	thenList := NewList([]Value{NewWord("side-effect"), NewInteger(1)})
	elseList := NewList([]Value{NewWord("side-effect"), NewInteger(2)})
	result := runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(true), thenList, elseList,
	})
	if callCount != 1 {
		t.Errorf("expected side-effect called once, got %d", callCount)
	}
	if len(result) != 1 || result[0].AsInteger() != 1 {
		t.Errorf("expected [1], got %v", result)
	}

	// Reset and test false branch
	callCount = 0
	result = runAQL(t, r, []Value{
		NewWord("if"), NewBoolean(false), thenList, elseList,
	})
	if callCount != 1 {
		t.Errorf("expected side-effect called once, got %d", callCount)
	}
	if len(result) != 1 || result[0].AsInteger() != 2 {
		t.Errorf("expected [2], got %v", result)
	}
}

func TestEngineIfFalsy(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	mem.Files["test.txt"] = []byte("hello world")
	r.SetFileOps(mem)

	result := runAQL(t, r, []Value{NewWord("read"), NewString("test.txt")})
	if len(result) != 1 || result[0].AsString() != "hello world" {
		t.Errorf("read 'test.txt' = %v, want 'hello world'", result)
	}
}

func TestEngineReadWithOpts(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	err = runAQLError(t, r, []Value{NewWord("read"), NewString("nope.txt")})
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestEngineReadUnknownFormat(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	mem.Files["test.txt"] = []byte("data")
	r.SetFileOps(mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("yaml"))
	err = runAQLError(t, r, []Value{NewWord("read"), NewString("test.txt"), NewMap(opts)})
	if err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestEngineWriteBasic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	r.SetFileOps(mem)
	if r.FileOps != mem {
		t.Error("SetFileOps did not set the FileOps")
	}
}

func TestRegistryMatchNoFunction(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := r.Match("nonexistent", []Value{}, WordInfo{})
	if result != nil {
		t.Error("expected nil for nonexistent function")
	}
}

// --- Additional engine tests for coverage ---

func TestEngineConvert(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// convert 99 string
	result := runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(99), NewWord("String"),
	})
	if len(result) != 1 || result[0].AsString() != "99" {
		t.Errorf("convert 99 string = %v, want '99'", result)
	}
}

func TestEngineTypeof(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewWord("typeof"), NewInteger(42)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestEngineBase(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewWord("base"), NewTypeLiteral(TInteger)})
	if len(result) != 1 || result[0].AsInteger() != 0 {
		t.Errorf("base integer = %v, want 0", result)
	}
}

func TestEngineDef(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := NewTop(r)
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := NewTop(r)
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(1), NewTypeLiteral(TNumber), NewWord("unify")})
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[1].AsBoolean() != true {
		t.Errorf("1 unify number = %v, want true", result[1])
	}
}

func TestEngineDo(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	list := NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	result := runAQL(t, r, []Value{NewWord("do"), list})
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("do [1 add 2] = %v, want 3", result)
	}
}

func TestEngineDoMap(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := NewOrderedMap()
	m.Set("x", NewList([]Value{NewInteger(3), NewWord("add"), NewInteger(4)}))
	result := runAQL(t, r, []Value{NewWord("do"), NewMap(m)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestEngineOr(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewBoolean(true), NewWord("or"), NewBoolean(false)})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("true or false = %v, want true", result)
	}
}

func TestEngineAnd(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewBoolean(true), NewWord("and"), NewBoolean(false)})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("true and false = %v, want false", result)
	}
}

func TestEngineNot(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewBoolean(true), NewWord("not")})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("true not = %v, want false", result)
	}
}

func TestEngineConvertStringVariants(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// convert 10 string "hex" → 'a'
	result := runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(10), NewWord("String"), NewString("hex"),
	})
	if len(result) != 1 || result[0].AsString() != "a" {
		t.Errorf("convert 10 string hex = %v, want 'a'", result)
	}

	// convert 255 string "HEX" → 'FF'
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(255), NewWord("String"), NewString("HEX"),
	})
	if len(result) != 1 || result[0].AsString() != "FF" {
		t.Errorf("convert 255 string HEX = %v, want 'FF'", result)
	}

	// convert 10 string "bin" → '1010'
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(10), NewWord("String"), NewString("bin"),
	})
	if len(result) != 1 || result[0].AsString() != "1010" {
		t.Errorf("convert 10 string bin = %v, want '1010'", result)
	}

	// convert 8 string "oct" → '10'
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(8), NewWord("String"), NewString("oct"),
	})
	if len(result) != 1 || result[0].AsString() != "10" {
		t.Errorf("convert 8 string oct = %v, want '10'", result)
	}
}

func TestEngineConvertToNumber(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// convert "42" number → 42
	result := runAQL(t, r, []Value{
		NewWord("convert"), NewString("42"), NewWord("Number"),
	})
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("convert '42' number = %v, want 42", result)
	}

	// convert "ff" number "hex" → 255
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewString("ff"), NewWord("Number"), NewString("hex"),
	})
	if len(result) != 1 || result[0].AsInteger() != 255 {
		t.Errorf("convert 'ff' number hex = %v, want 255", result)
	}

	// convert "1010" number "bin" → 10
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewString("1010"), NewWord("Number"), NewString("bin"),
	})
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("convert '1010' number bin = %v, want 10", result)
	}

	// convert "10" number "oct" → 8
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewString("10"), NewWord("Number"), NewString("oct"),
	})
	if len(result) != 1 || result[0].AsInteger() != 8 {
		t.Errorf("convert '10' number oct = %v, want 8", result)
	}
}

func TestEngineConvertToBoolean(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// convert 1 boolean → true
	result := runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(1), NewWord("Boolean"),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("convert 1 boolean = %v, want true", result)
	}

	// convert 0 boolean → false
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(0), NewWord("Boolean"),
	})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("convert 0 boolean = %v, want false", result)
	}

	// convert "true" boolean → true
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewString("true"), NewWord("Boolean"),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("convert 'true' boolean = %v, want true", result)
	}

	// convert "" boolean → false
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewString(""), NewWord("Boolean"),
	})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("convert '' boolean = %v, want false", result)
	}

	// convert true boolean → true (passthrough)
	result = runAQL(t, r, []Value{
		NewWord("convert"), NewBoolean(true), NewWord("Boolean"),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("convert true boolean = %v, want true", result)
	}
}

func TestEngineConvertToAtom(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("convert"), NewInteger(42), NewWord("Atom"),
	})
	if len(result) != 1 || !result[0].IsAtom() {
		t.Errorf("convert 42 atom = %v, want atom", result)
	}
}

func TestEngineBaseTypes(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
		{"none", TNone, "None"},
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def double fn [[number] [number] [dup add]] end 7 double
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def square fn [[x:number] [number] [x mul x]] end 5 square
	xParam := NewOrderedMap()
	xParam.Set("x", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewWord("Number")}),
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def catter fn [[integer string] [string] [add]] end
	// Case: [1 "a"|] -> catter -> all args from prefix
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Integer"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def catter fn [[integer string] [string] [add]] end
	// Case: [2|] -> catter "b" -> string from suffix, integer from prefix
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Integer"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def catter fn [[integer string] [string] [add]] end
	// Case: [|] -> catter 3 "c" -> both args from suffix (positional match)
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Integer"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("add")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("catter"), NewWord("fn"), fnBody, NewWord("end"),
		NewWord("catter"), NewInteger(3), NewString("c"),
	})
	if len(result) != 1 || !result[0].VType.Matches(TString) {
		t.Errorf("catter 'c' 3 = %v, want string result", result)
	}
}

func TestEngineFnConcatArgOrder(t *testing.T) {
	// def joiner fn [[string string string] [string] [args concat]] end
	// Uses args+concat to reveal the exact ordering of 3 args.
	// args returns all fn arguments as a list, concat joins them.
	// The concatenated output string directly reveals argument order.
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("String"), NewWord("String"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("drop"), NewWord("drop"), NewWord("drop"), NewWord("args"), NewWord("concat")}),
	})

	defTokens := []Value{
		NewWord("def"), NewWord("joiner"), NewWord("fn"), fnBody, NewWord("end"),
	}

	// Subtest: all args from prefix (stack)
	// "A" "B" "C" joiner -> args=["A","B","C"] -> concat -> "ABC"
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewString("A"), NewString("B"), NewString("C"), NewWord("joiner"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 1 || result[0].AsString() != "ABC" {
			t.Errorf(`"A" "B" "C" joiner = %v, want ["ABC"]`, result)
		}
	})

	// Subtest: 1 prefix + 2 suffix
	// "A" joiner "B" "C" -> args=["A","B","C"] -> concat -> "ABC"
	t.Run("MixedPrefixSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewString("A"), NewWord("joiner"), NewString("B"), NewString("C"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 1 || result[0].AsString() != "ABC" {
			t.Errorf(`"A" joiner "B" "C" = %v, want ["ABC"]`, result)
		}
	})

	// Subtest: 2 prefix + 1 suffix
	// "A" "B" joiner "C" -> args=["A","B","C"] -> concat -> "ABC"
	t.Run("TwoPrefixOneSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewString("A"), NewString("B"), NewWord("joiner"), NewString("C"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 1 || result[0].AsString() != "ABC" {
			t.Errorf(`"A" "B" joiner "C" = %v, want ["ABC"]`, result)
		}
	})

	// Subtest: all args from suffix
	// joiner "A" "B" "C" -> args=["A","B","C"] -> concat -> "ABC"
	t.Run("AllSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewWord("joiner"), NewString("A"), NewString("B"), NewString("C"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 1 || result[0].AsString() != "ABC" {
			t.Errorf(`joiner "A" "B" "C" = %v, want ["ABC"]`, result)
		}
	})
}

// concatDropBody builds the body [drop..drop args concat] for n unnamed params.
func concatDropBody(n int) []Value {
	var body []Value
	for i := 0; i < n; i++ {
		body = append(body, NewWord("drop"))
	}
	body = append(body, NewWord("args"), NewWord("concat"))
	return body
}

func TestEngineFnConcatArgOrder4Mixed(t *testing.T) {
	// def mix4 fn [[string integer boolean string] [string]
	//              [drop drop drop drop args concat]] end
	// 4 args: string, integer, boolean, string -> concat reveals ordering.
	// valToString: integer->digits, boolean->"true"/"false"
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("String"), NewWord("Integer"), NewWord("Boolean"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(4)),
	})
	defTokens := []Value{
		NewWord("def"), NewWord("mix4"), NewWord("fn"), fnBody, NewWord("end"),
	}

	// "X" 7 true "Z" mix4 -> "X7trueZ"
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewString("X"), NewInteger(7), NewBoolean(true), NewString("Z"), NewWord("mix4"),
		))
		if len(result) != 1 || result[0].AsString() != "X7trueZ" {
			t.Errorf(`all-prefix mix4 = %v, want ["X7trueZ"]`, result)
		}
	})

	// "X" mix4 7 true "Z" -> 1 prefix, 3 suffix
	t.Run("OnePrefixThreeSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewString("X"), NewWord("mix4"), NewInteger(7), NewBoolean(true), NewString("Z"),
		))
		if len(result) != 1 || result[0].AsString() != "X7trueZ" {
			t.Errorf(`1+3 mix4 = %v, want ["X7trueZ"]`, result)
		}
	})

	// "X" 7 mix4 true "Z" -> 2 prefix, 2 suffix
	t.Run("TwoPrefixTwoSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewString("X"), NewInteger(7), NewWord("mix4"), NewBoolean(true), NewString("Z"),
		))
		if len(result) != 1 || result[0].AsString() != "X7trueZ" {
			t.Errorf(`2+2 mix4 = %v, want ["X7trueZ"]`, result)
		}
	})

	// mix4 "X" 7 true "Z" -> all suffix
	t.Run("AllSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewWord("mix4"), NewString("X"), NewInteger(7), NewBoolean(true), NewString("Z"),
		))
		if len(result) != 1 || result[0].AsString() != "X7trueZ" {
			t.Errorf(`all-suffix mix4 = %v, want ["X7trueZ"]`, result)
		}
	})
}

func TestEngineFnConcatArgOrder5Mixed(t *testing.T) {
	// def mix5 fn [[string integer decimal boolean string] [string]
	//              [drop..drop args concat]] end
	// 5 args: string, integer, decimal, boolean, string
	fnBody := NewList([]Value{
		NewList([]Value{
			NewWord("String"), NewWord("Integer"), NewWord("Decimal"),
			NewWord("Boolean"), NewWord("String"),
		}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(5)),
	})
	defTokens := []Value{
		NewWord("def"), NewWord("mix5"), NewWord("fn"), fnBody, NewWord("end"),
	}

	// "a" 3 1.5 false "z" mix5 -> "a31.5falsez"
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewString("a"), NewInteger(3), NewDecimal(1.5), NewBoolean(false), NewString("z"),
			NewWord("mix5"),
		))
		if len(result) != 1 || result[0].AsString() != "a31.5falsez" {
			t.Errorf(`all-prefix mix5 = %v, want ["a31.5falsez"]`, result)
		}
	})

	// "a" 3 mix5 1.5 false "z" -> 2 prefix, 3 suffix
	t.Run("TwoPrefixThreeSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewString("a"), NewInteger(3), NewWord("mix5"),
			NewDecimal(1.5), NewBoolean(false), NewString("z"),
		))
		if len(result) != 1 || result[0].AsString() != "a31.5falsez" {
			t.Errorf(`2+3 mix5 = %v, want ["a31.5falsez"]`, result)
		}
	})

	// mix5 "a" 3 1.5 false "z" -> all suffix
	t.Run("AllSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewWord("mix5"), NewString("a"), NewInteger(3),
			NewDecimal(1.5), NewBoolean(false), NewString("z"),
		))
		if len(result) != 1 || result[0].AsString() != "a31.5falsez" {
			t.Errorf(`all-suffix mix5 = %v, want ["a31.5falsez"]`, result)
		}
	})
}

func TestEngineFnConcatArgOrder7Mixed(t *testing.T) {
	// def mix7 fn [[string integer decimal boolean string integer string]
	//              [string] [drop..drop args concat]] end
	// 7 args covering all scalar types with repeats.
	fnBody := NewList([]Value{
		NewList([]Value{
			NewWord("String"), NewWord("Integer"), NewWord("Decimal"),
			NewWord("Boolean"), NewWord("String"), NewWord("Integer"), NewWord("String"),
		}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(7)),
	})
	defTokens := []Value{
		NewWord("def"), NewWord("mix7"), NewWord("fn"), fnBody, NewWord("end"),
	}
	// Expected: "p1" 2 3.5 true "q4" 56 "r7" -> "p123.5trueq456r7"
	want := "p123.5trueq456r7"
	argVals := []Value{
		NewString("p1"), NewInteger(2), NewDecimal(3.5),
		NewBoolean(true), NewString("q4"), NewInteger(56), NewString("r7"),
	}

	// All prefix
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...), argVals...)
		tokens = append(tokens, NewWord("mix7"))
		result := runAQL(t, r, tokens)
		if len(result) != 1 || result[0].AsString() != want {
			t.Errorf("all-prefix mix7 = %v, want [%q]", result, want)
		}
	})

	// 3 prefix + 4 suffix
	t.Run("ThreePrefixFourSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...), argVals[:3]...)
		tokens = append(tokens, NewWord("mix7"))
		tokens = append(tokens, argVals[3:]...)
		result := runAQL(t, r, tokens)
		if len(result) != 1 || result[0].AsString() != want {
			t.Errorf("3+4 mix7 = %v, want [%q]", result, want)
		}
	})

	// 1 prefix + 6 suffix
	t.Run("OnePrefixSixSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...), argVals[0])
		tokens = append(tokens, NewWord("mix7"))
		tokens = append(tokens, argVals[1:]...)
		result := runAQL(t, r, tokens)
		if len(result) != 1 || result[0].AsString() != want {
			t.Errorf("1+6 mix7 = %v, want [%q]", result, want)
		}
	})

	// All suffix
	t.Run("AllSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...), NewWord("mix7"))
		tokens = append(tokens, argVals...)
		result := runAQL(t, r, tokens)
		if len(result) != 1 || result[0].AsString() != want {
			t.Errorf("all-suffix mix7 = %v, want [%q]", result, want)
		}
	})
}

func TestEngineFnConcatArgOrderEndDisambiguate(t *testing.T) {
	// Tests that the "end" word stops suffix argument collection,
	// preventing the fn from consuming tokens that follow.

	// def cat3 fn [[string string string] [string]
	//              [drop drop drop args concat]] end
	cat3Body := NewList([]Value{
		NewList([]Value{NewWord("String"), NewWord("String"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(3)),
	})
	cat3Def := []Value{
		NewWord("def"), NewWord("cat3"), NewWord("fn"), cat3Body, NewWord("end"),
	}

	// def cat4 fn [[string integer boolean string] [string]
	//              [drop drop drop drop args concat]] end
	cat4Body := NewList([]Value{
		NewList([]Value{NewWord("String"), NewWord("Integer"), NewWord("Boolean"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(4)),
	})
	cat4Def := []Value{
		NewWord("def"), NewWord("cat4"), NewWord("fn"), cat4Body, NewWord("end"),
	}

	// cat3 "A" "B" "C" end "trailing" -> cat3 gets "ABC", "trailing" on stack
	t.Run("EndStopsSuffix3", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat3Def...),
			NewWord("cat3"), NewString("A"), NewString("B"), NewString("C"),
			NewWord("end"), NewString("trailing"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("cat3 A B C end trailing: got %d results, want 2: %v", len(result), result)
		}
		if result[0].AsString() != "ABC" {
			t.Errorf("cat3 result = %q, want %q", result[0].AsString(), "ABC")
		}
		if result[1].AsString() != "trailing" {
			t.Errorf("trailing = %v, want 'trailing'", result[1])
		}
	})

	// "X" cat4 7 true "Z" end "after" -> cat4 gets "X7trueZ", "after" untouched
	t.Run("EndStopsSuffix4Mixed", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat4Def...),
			NewString("X"), NewWord("cat4"), NewInteger(7), NewBoolean(true), NewString("Z"),
			NewWord("end"), NewString("after"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("X cat4 7 true Z end after: got %d results, want 2: %v", len(result), result)
		}
		if result[0].AsString() != "X7trueZ" {
			t.Errorf("cat4 result = %q, want %q", result[0].AsString(), "X7trueZ")
		}
		if result[1].AsString() != "after" {
			t.Errorf("trailing = %v, want 'after'", result[1])
		}
	})

	// Two fn calls using parens and end: (cat3 "A" "B" "C" end) (cat3 "D" "E" "F" end)
	// Parens isolate each call; end stops suffix collection within each group.
	t.Run("EndSeparatesTwoCalls", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat3Def...),
			NewWord("("),
			NewWord("cat3"), NewString("A"), NewString("B"), NewString("C"), NewWord("end"),
			NewWord(")"),
			NewWord("("),
			NewWord("cat3"), NewString("D"), NewString("E"), NewString("F"), NewWord("end"),
			NewWord(")"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("two cat3 calls: got %d results, want 2: %v", len(result), result)
		}
		if result[0].AsString() != "ABC" {
			t.Errorf("first cat3 = %q, want %q", result[0].AsString(), "ABC")
		}
		if result[1].AsString() != "DEF" {
			t.Errorf("second cat3 = %q, want %q", result[1].AsString(), "DEF")
		}
	})

	// Mixed types with end in parens:
	// (cat4 "m" 9 false "n" end) (cat3 "x" "y" "z" end)
	// Verifies end works when switching between fns of different arity/types.
	t.Run("EndSeparatesDifferentFns", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append([]Value{}, cat4Def...)
		tokens = append(tokens, cat3Def...)
		tokens = append(tokens,
			NewWord("("),
			NewWord("cat4"), NewString("m"), NewInteger(9), NewBoolean(false), NewString("n"), NewWord("end"),
			NewWord(")"),
			NewWord("("),
			NewWord("cat3"), NewString("x"), NewString("y"), NewString("z"), NewWord("end"),
			NewWord(")"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("cat4+cat3 with end: got %d results, want 2: %v", len(result), result)
		}
		if result[0].AsString() != "m9falsen" {
			t.Errorf("cat4 = %q, want %q", result[0].AsString(), "m9falsen")
		}
		if result[1].AsString() != "xyz" {
			t.Errorf("cat3 = %q, want %q", result[1].AsString(), "xyz")
		}
	})

	// Prefix-heavy with end: "P" "Q" cat3 "R" end "extra"
	// 2 prefix, 1 suffix, end stops collection, "extra" remains.
	t.Run("EndAfterPartialSuffix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat3Def...),
			NewString("P"), NewString("Q"), NewWord("cat3"), NewString("R"),
			NewWord("end"), NewString("extra"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("P Q cat3 R end extra: got %d results, want 2: %v", len(result), result)
		}
		if result[0].AsString() != "PQR" {
			t.Errorf("cat3 = %q, want %q", result[0].AsString(), "PQR")
		}
		if result[1].AsString() != "extra" {
			t.Errorf("trailing = %v, want 'extra'", result[1])
		}
	})
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def adder fn [[0] [integer] [add 2]] end
	// adder only matches the value 0, adds 2 to it
	fnBody := NewList([]Value{
		NewList([]Value{NewInteger(0)}),
		NewList([]Value{NewWord("Integer")}),
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def adder fn [[0] [integer] [add 2]] end
	// adder should NOT match 5 (only matches 0)
	fnBody := NewList([]Value{
		NewList([]Value{NewInteger(0)}),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("add"), NewInteger(2)}),
	})
	err = runAQLError(t, r, []Value{
		NewWord("def"), NewWord("adder"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(5), NewWord("adder"),
	})
	if err == nil {
		t.Error("expected error: adder should not match 5")
	}
}

func TestEngineFnLiteralTypeMultiSig(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def handler fn [[0] [integer] [add 10] [1] [integer] [add 20]] end
	// handler 0 → 10, handler 1 → 21
	fnBody := NewList([]Value{
		NewList([]Value{NewInteger(0)}),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("add"), NewInteger(10)}),
		NewList([]Value{NewInteger(1)}),
		NewList([]Value{NewWord("Integer")}),
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

func TestEngineFnDefPrefixOnly(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def doubler/p fn [[x:integer] [integer] [x x add]] end
	// doubler/p registers as prefix-only: takes args from the stack only,
	// never collects suffix args via forward.
	fnBody := NewList([]Value{
		func() Value { m := NewOrderedMap(); m.Set("x", NewWord("Integer")); return NewList([]Value{NewImplicitMap(m)}) }(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("x"), NewWord("x"), NewWord("add")}),
	})
	// 5 doubler — 5 is on stack, doubler takes it as prefix arg
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWordModified("doubler", -1, true, false), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(5), NewWord("doubler"),
	})
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("5 doubler = %v, want 10", result)
	}
}

func TestEngineFnDefPrefixOnlyNoSuffixCollection(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def doubler/p fn [[x:integer] [integer] [x x add]] end
	// doubler 5 — prefix-only word should NOT collect 5 as suffix arg.
	// It should fail because there's nothing on the stack for prefix match.
	fnBody := NewList([]Value{
		func() Value { m := NewOrderedMap(); m.Set("x", NewWord("Integer")); return NewList([]Value{NewImplicitMap(m)}) }(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("x"), NewWord("x"), NewWord("add")}),
	})
	// Define using string name (def sig selection changed with new type hierarchy).
	_ = runAQL(t, r, []Value{
		NewWord("def"), NewString("doubler"), NewWord("fn"), fnBody, NewWord("end"),
	})

	// Prefix call with arg on stack should work.
	result := runAQL(t, r, []Value{
		NewInteger(5), NewWord("doubler"),
	})
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("5 doubler = %v, want 10", result)
	}
}

func TestEngineFnAbbreviatedSignature(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// def foo fn [
	//   [string] [string] [add "Q"]    -- full form
	//   integer  string   [add "P"]    -- abbreviated input sig & output sig
	//   99       string   [drop "NN"]  -- abbreviated input sig & output sig
	// ]

	fnBody := NewList([]Value{
		// sig 1: [string] [string] [add "Q"]
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("add"), NewString("Q")}),

		// sig 2: integer string [add "P"]  (abbreviated input & output)
		NewWord("Integer"),
		NewWord("String"),
		NewList([]Value{NewWord("add"), NewString("P")}),

		// sig 3: 99 string [drop "NN"]  (abbreviated input & output)
		NewInteger(99),
		NewWord("String"),
		NewList([]Value{NewWord("drop"), NewString("NN")}),
	})

	// foo "x" → "xQ" (string matches sig 1: "x" add "Q")
	result := runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), fnBody, NewWord("end"),
		NewString("x"), NewWord("foo"),
	})
	if len(result) != 1 || result[0].AsString() != "xQ" {
		t.Errorf("foo \"x\" = %v, want \"xQ\"", result)
	}

	// foo 1 → "1P" (integer matches sig 2: 1 add "P")
	result = runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(1), NewWord("foo"),
	})
	if len(result) != 1 || result[0].AsString() != "1P" {
		t.Errorf("foo 1 = %v, want \"1P\"", result)
	}

	// foo 99 → "NN" (literal 99 matches sig 3: drop "NN")
	result = runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(99), NewWord("foo"),
	})
	if len(result) != 1 || result[0].AsString() != "NN" {
		t.Errorf("foo 99 = %v, want \"NN\"", result)
	}

}

func TestEngineFnAbbreviatedSimple(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def double fn [number number [dup add]] end 7 double
	// All three elements abbreviated (single-valued)
	fnBody := NewList([]Value{
		NewWord("Number"),
		NewWord("Number"),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("double"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(7), NewWord("double"),
	})
	if len(result) != 1 || result[0].AsInteger() != 14 {
		t.Errorf("double 7 = %v, want 14", result)
	}
}

func TestEngineFnFactorial(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def fact fn [0 integer [drop 1] [x:integer] [integer] [x mul fact (x sub 1)]]
	fnBody := NewList([]Value{
		// sig 1 (base case): 0 integer [drop 1]
		NewInteger(0),
		NewWord("Integer"),
		NewList([]Value{NewWord("drop"), NewInteger(1)}),
		// sig 2 (recursive): [x:integer] [integer] [x mul fact (x sub 1)]
		func() Value {
			m := NewOrderedMap()
			m.Set("x", NewWord("Integer"))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{
			NewWord("x"), NewWord("mul"),
			NewWord("fact"),
			NewWord("("), NewWord("x"), NewWord("sub"), NewInteger(1), NewWord(")"),
		}),
	})
	tests := []struct {
		input    int64
		expected int64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{5, 120},
		{7, 5040},
	}
	for _, tc := range tests {
		result := runAQL(t, r, []Value{
			NewWord("def"), NewString("fact"), NewWord("fn"), fnBody, NewWord("end"),
			NewInteger(tc.input), NewWord("fact"),
		})
		if len(result) != 1 || result[0].AsInteger() != tc.expected {
			t.Errorf("fact %d = %v, want %d", tc.input, result, tc.expected)
		}
	}
}

func TestEngineFnFactorialNoVars(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Try several variable-free body forms for the recursive case.
	// Base case is always: 0 integer [drop 1]
	bodies := []struct {
		name string
		body []Value
	}{
		// Approach A: dup sub 1 fact swap mul
		// n dup → n n; n sub 1 → n-1; fact → fact(n-1); swap mul → n*fact(n-1)
		{"dup sub 1 fact swap mul", []Value{
			NewWord("dup"), NewWord("sub"), NewInteger(1),
			NewWord("fact"), NewWord("swap"), NewWord("mul"),
		}},
		// Approach B: dup sub 1 fact mul (rely on mul grabbing n as prefix)
		{"dup sub 1 fact mul", []Value{
			NewWord("dup"), NewWord("sub"), NewInteger(1),
			NewWord("fact"), NewWord("mul"),
		}},
		// Approach C: dup mul fact (dup sub 1)  — same structure as named version
		// but dup in inner parens has no prefix, so this likely fails
		{"dup mul fact (dup sub 1)", []Value{
			NewWord("dup"), NewWord("mul"),
			NewWord("fact"),
			NewWord("("), NewWord("dup"), NewWord("sub"), NewInteger(1), NewWord(")"),
		}},
	}

	tests := []struct {
		input    int64
		expected int64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{5, 120},
		{7, 5040},
	}

	for _, b := range bodies {
		fnBody := NewList([]Value{
			NewInteger(0),
			NewWord("Integer"),
			NewList([]Value{NewWord("drop"), NewInteger(1)}),
			NewWord("Integer"),
			NewWord("Integer"),
			NewList(b.body),
		})
		allPass := true
		for _, tc := range tests {
			result := runAQL(t, r, []Value{
				NewWord("def"), NewString("fact"), NewWord("fn"), fnBody, NewWord("end"),
				NewInteger(tc.input), NewWord("fact"),
			})
			if len(result) != 1 || result[0].AsInteger() != tc.expected {
				t.Logf("FAIL body=%q: fact %d = %v, want %d", b.name, tc.input, result, tc.expected)
				allPass = false
			}
		}
		if allPass {
			t.Logf("PASS body=%q", b.name)
		}
	}
}

func TestEngineFnFactorialNamedZero(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def fact fn [[_:0] integer [1] [x:integer] [integer] [x mul fact (x sub 1)]]
	// Using {_:0} instead of bare 0 in the base case.
	// Named param "_" consumes the 0 from the stack, so the body is just [1].
	fnBody := NewList([]Value{
		// sig 1 (base case): [_:0] integer [1]
		func() Value {
			m := NewOrderedMap()
			m.Set("_", NewInteger(0))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewInteger(1)}),
		// sig 2 (recursive): [x:integer] [integer] [x mul fact (x sub 1)]
		func() Value {
			m := NewOrderedMap()
			m.Set("x", NewWord("Integer"))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{
			NewWord("x"), NewWord("mul"),
			NewWord("fact"),
			NewWord("("), NewWord("x"), NewWord("sub"), NewInteger(1), NewWord(")"),
		}),
	})
	tests := []struct {
		input    int64
		expected int64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{5, 120},
		{7, 5040},
	}
	for _, tc := range tests {
		result := runAQL(t, r, []Value{
			NewWord("def"), NewString("fact"), NewWord("fn"), fnBody, NewWord("end"),
			NewInteger(tc.input), NewWord("fact"),
		})
		if len(result) != 1 || result[0].AsInteger() != tc.expected {
			t.Errorf("fact %d = %v, want %d", tc.input, result, tc.expected)
		}
	}
}

func TestEngineTypeRecord(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	l1 := NewList([]Value{NewInteger(1), NewInteger(2)})
	l2 := NewList([]Value{NewInteger(1), NewInteger(2)})
	result := runAQL(t, r, []Value{l1, l2, NewWord("unify")})
	if len(result) != 2 || !result[1].AsBoolean() {
		t.Errorf("[1,2] unify [1,2] = %v, want true", result)
	}
}

func TestEngineUnifyFail(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(1), NewString("a"), NewWord("unify")})
	if len(result) != 2 || result[1].AsBoolean() {
		t.Errorf("1 unify 'a' = %v, want false", result)
	}
}

func TestEngineUnifyTypedList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tl := NewTypedList(NewTypeLiteral(TNumber))
	cl := NewList([]Value{NewInteger(1), NewInteger(2)})
	result := runAQL(t, r, []Value{tl, cl, NewWord("unify")})
	if len(result) != 2 || !result[1].AsBoolean() {
		t.Errorf("[:number] unify [1,2] = %v, want true", result)
	}
}

func TestEngineUnifyTypedMap(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// string or none
	result := runAQL(t, r, []Value{
		NewTypeLiteral(TString), NewWord("or"), NewTypeLiteral(TNone),
	})
	if len(result) != 1 || !result[0].IsDisjunct() {
		t.Errorf("string or none = %v, want disjunct", result)
	}
}

func TestEngineVar(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
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

// --- Inspect word tests ---

func TestEngineInspectBuiltin(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// inspect add => word_inspection map
	result := runAQL(t, r, []Value{NewWord("inspect"), NewWord("add")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !v.VType.Equal(TWordInspection) {
		t.Fatalf("expected type %s, got %s", TWordInspection, v.VType)
	}
	m := v.AsMap()

	// Check name field.
	name, ok := m.Get("name")
	if !ok || name.AsString() != "add" {
		t.Errorf("name = %v, want 'add'", name)
	}

	// Check kind field.
	kind, ok := m.Get("kind")
	if !ok || kind.AsAtom() != "builtin" {
		t.Errorf("kind = %v, want builtin", kind)
	}

	// Check signatures field is a non-empty list.
	sigs, ok := m.Get("signatures")
	if !ok {
		t.Fatal("missing signatures field")
	}
	sigList := sigs.AsList()
	if len(sigList) == 0 {
		t.Error("expected at least one signature for add")
	}

	// Check first signature has args and precedence.
	sig0 := sigList[0].AsMap()
	args, _ := sig0.Get("args")
	argList := args.AsList()
	if len(argList) != 2 {
		t.Errorf("expected 2 args for add, got %d", len(argList))
	}

	prec, _ := sig0.Get("precedence")
	if prec.AsInteger() == 0 {
		t.Error("expected non-zero precedence for add")
	}
}

func TestEngineInspectUserDefined(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def double [2 mul] ; inspect double
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("double"), NewList([]Value{NewInteger(2), NewWord("mul")}),
		NewWord("inspect"), NewWord("double"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	m := result[0].AsMap()

	kind, _ := m.Get("kind")
	if kind.AsAtom() != "defined" {
		t.Errorf("kind = %v, want defined", kind)
	}

	name, _ := m.Get("name")
	if name.AsString() != "double" {
		t.Errorf("name = %v, want 'double'", name)
	}
}

func TestEngineInspectUnknown(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewWord("inspect"), NewWord("nonexistent")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	m := result[0].AsMap()

	kind, _ := m.Get("kind")
	if kind.AsAtom() != "unknown" {
		t.Errorf("kind = %v, want unknown", kind)
	}

	sigs, _ := m.Get("signatures")
	if len(sigs.AsList()) != 0 {
		t.Errorf("expected empty signatures for unknown word")
	}
}

func TestEngineInspectDotAccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// inspect upper .name => 'upper'
	result := runAQL(t, r, []Value{
		NewWord("inspect"), NewWord("upper"),
		NewWord("."), NewWord("name"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	if result[0].AsString() != "upper" {
		t.Errorf("inspect upper .name = %v, want 'upper'", result[0])
	}
}

func TestEngineInspectTypeLiteral(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// type Qty number ; inspect Qty
	result := runAQL(t, r, []Value{
		NewWord("type"), NewWord("Qty"), NewTypeLiteral(TNumber),
		NewWord("inspect"), NewWord("Qty"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !v.VType.Equal(TTypeInspect) {
		t.Fatalf("expected type %s, got %s", TTypeInspect, v.VType)
	}
	m := v.AsMap()

	name, _ := m.Get("name")
	if name.AsString() != "Qty" {
		t.Errorf("name = %v, want 'Qty'", name)
	}
	kind, _ := m.Get("kind")
	if kind.AsAtom() != "literal" {
		t.Errorf("kind = %v, want literal", kind)
	}
	typ, _ := m.Get("type")
	if typ.AsString() != "Scalar/Number" {
		t.Errorf("type = %v, want 'Scalar/Number'", typ)
	}
}

func TestEngineInspectRecordType(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// type Pos record{x:number,y:number} ; inspect Pos
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TNumber))
	result := runAQL(t, r, []Value{
		NewWord("type"), NewWord("Pos"), NewRecordType(fields),
		NewWord("inspect"), NewWord("Pos"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	m := result[0].AsMap()

	name, _ := m.Get("name")
	if name.AsString() != "Pos" {
		t.Errorf("name = %v, want 'Pos'", name)
	}
	kind, _ := m.Get("kind")
	if kind.AsAtom() != "record" {
		t.Errorf("kind = %v, want record", kind)
	}
	flds, ok := m.Get("fields")
	if !ok {
		t.Fatal("missing fields")
	}
	fm := flds.AsMap()
	xType, _ := fm.Get("x")
	if xType.AsString() != "Scalar/Number" {
		t.Errorf("fields.x = %v, want 'Scalar/Number'", xType)
	}
	yType, _ := fm.Get("y")
	if yType.AsString() != "Scalar/Number" {
		t.Errorf("fields.y = %v, want 'Scalar/Number'", yType)
	}
}

func TestEngineInspectTypeDotAccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// type Qty number ; inspect Qty .kind
	result := runAQL(t, r, []Value{
		NewWord("type"), NewWord("Qty"), NewTypeLiteral(TNumber),
		NewWord("inspect"), NewWord("Qty"),
		NewWord("."), NewWord("kind"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	if result[0].AsAtom() != "literal" {
		t.Errorf("inspect Qty .kind = %v, want literal", result[0])
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

// --- Return type validation tests ---

func TestEngineFnReturnTypeCorrect(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def double fn [[number] [number] [dup add]] end
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("double"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(5), NewWord("double"),
	})
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("5 double = %v, want 10", result)
	}
}

func TestEngineFnReturnTypeWrong(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def bad fn [[number] [string] [dup add]] end
	// Returns a number but declares string return type.
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	err = runAQLError(t, r, []Value{
		NewWord("def"), NewWord("bad"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(5), NewWord("bad"),
	})
	if err == nil {
		t.Fatal("expected return type error, got nil")
	}
	if !strings.Contains(err.Error(), "bad") || !strings.Contains(err.Error(), "expected") {
		t.Errorf("error should mention function name and expected type, got: %v", err)
	}
}

func TestEngineFnReturnCountWrong(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def toomany fn [[number] [number number] [dup]] end
	// Body produces 2 values but signature declares 2 returns, dup produces 2 from 1.
	// Actually let's make it expect 1 but body produces 2.
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup")}), // produces 2 values, signature expects 1
	})
	err = runAQLError(t, r, []Value{
		NewWord("def"), NewWord("toomany"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(5), NewWord("toomany"),
	})
	if err == nil {
		t.Fatal("expected return count error, got nil")
	}
	if !strings.Contains(err.Error(), "toomany") {
		t.Errorf("error should mention function name, got: %v", err)
	}
}

func TestEngineFnReturnTypeAny(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def identity fn [[any] [any] []] end
	// [any] return type should accept any value.
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Any")}),
		NewList([]Value{NewWord("Any")}),
		NewList([]Value{}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("identity"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(42), NewWord("identity"),
	})
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("42 identity = %v, want 42", result)
	}
}

func TestEngineFnReturnTypeUncheckedEmpty(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def dbl fn [[number] [] [dup add]] end
	// Empty return sig means no checking (backwards compat).
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("dbl"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(7), NewWord("dbl"),
	})
	if len(result) != 1 || result[0].AsInteger() != 14 {
		t.Errorf("7 dbl = %v, want 14", result)
	}
}

func TestEngineFnReturnTypeMultipleValues(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def dup2 fn [[number] [number number] [dup]] end
	// Returns 2 numbers.
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number"), NewWord("Number")}),
		NewList([]Value{NewWord("dup")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("dup2"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(3), NewWord("dup2"),
	})
	if len(result) != 2 || result[0].AsInteger() != 3 || result[1].AsInteger() != 3 {
		t.Errorf("3 dup2 = %v, want [3 3]", result)
	}
}

func TestEngineFnReturnTypeNamedParams(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def square fn [[x:number] [number] [x mul x]] end
	xParam := NewOrderedMap()
	xParam.Set("x", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("x"), NewWord("mul"), NewWord("x")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("square"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(6), NewWord("square"),
	})
	if len(result) != 1 || result[0].AsInteger() != 36 {
		t.Errorf("6 square = %v, want 36", result)
	}
}

func TestEngineFnReturnTypeNamedParamsWrongReturn(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def isbig fn [[x:number] [number] [x gt 10]] end
	// Declares number return but body returns boolean via gt.
	xParam := NewOrderedMap()
	xParam.Set("x", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("x"), NewWord("gt"), NewInteger(10)}),
	})
	err = runAQLError(t, r, []Value{
		NewWord("def"), NewWord("isbig"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(5), NewWord("isbig"),
	})
	if err == nil {
		t.Fatal("expected return type error for named param fn, got nil")
	}
	if !strings.Contains(err.Error(), "isbig") {
		t.Errorf("error should mention function name, got: %v", err)
	}
}

func TestEngineFnReturnTypeMultiOverload(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def add1 fn [[number] [number] [1 add] [string] [string] ["1" add]] end
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewInteger(1), NewWord("add")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewString("1"), NewWord("add")}),
	})
	// Test number overload
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("add1"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(10), NewWord("add1"),
	})
	if len(result) != 1 || result[0].AsInteger() != 11 {
		t.Errorf("10 add1 = %v, want 11", result)
	}
	// Test string overload
	result = runAQL(t, r, []Value{
		NewString("hello"), NewWord("add1"),
	})
	if len(result) != 1 || result[0].AsString() != "hello1" {
		t.Errorf("'hello' add1 = %v, want 'hello1'", result)
	}
}

func TestPiecemealDef(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Define foo with number sig, then add string sig
	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	strBody := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})

	// Define both sigs
	_ = runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewWord("end"),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewWord("end"),
	})

	// Test number sig
	result := runAQL(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 || result[0].AsInteger() != 9 {
		t.Errorf("3 foo = %v, want 9", result)
	}

	// Test string sig
	result = runAQL(t, r, []Value{
		NewString("hi"), NewWord("foo"),
	})
	if len(result) != 1 || result[0].AsString() != "hihi" {
		t.Errorf("\"hi\" foo = %v, want \"hihi\"", result)
	}
}

func TestPiecemealUndefPopsRecent(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	strBody := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})

	// def number sig, def string sig, undef (pops string sig), test number sig
	result := runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewWord("end"),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewWord("end"),
		NewWord("undef"), NewWord("foo"), NewWord("end"),
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	if result[0].AsInteger() != 9 {
		t.Errorf("3 foo after undef = %v, want 9", result[0])
	}
}

func TestFnUndefTargeted(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	strBody := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})

	// Targeted removal: def foo fn [[number] [number]] (pairs = remove sig)
	undefSpec := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
	})

	// def both sigs, targeted remove number sig, string sig still works
	_ = runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewWord("end"),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewWord("end"),
		NewWord("def"), NewString("foo"), NewWord("fn"), undefSpec, NewWord("end"),
	})
	result := runAQL(t, r, []Value{
		NewString("hi"), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	if result[0].AsString() != "hihi" {
		t.Errorf("\"hi\" foo after targeted undef = %v, want \"hihi\"", result[0])
	}
}

func TestFnUndefTargetedReverse(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	strBody := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})

	// Remove string sig, keep number sig
	undefSpec := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
	})

	_ = runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewWord("end"),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewWord("end"),
		NewWord("def"), NewString("foo"), NewWord("fn"), undefSpec, NewWord("end"),
	})
	result := runAQL(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	if result[0].AsInteger() != 9 {
		t.Errorf("3 foo after targeted undef string = %v, want 9", result[0])
	}
}

func TestFnUndefNonExistentNoOp(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})

	// Remove a string sig that was never defined — should be a no-op
	undefSpec := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
	})

	_ = runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewWord("end"),
		NewWord("def"), NewString("foo"), NewWord("fn"), undefSpec, NewWord("end"),
	})
	result := runAQL(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	if result[0].AsInteger() != 9 {
		t.Errorf("3 foo after no-op undef = %v, want 9", result[0])
	}
}

func TestFnUndefRemovesAll(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})

	// Remove the only sig — word should become undefined
	undefSpec := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
	})

	result := runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewWord("end"),
		NewWord("def"), NewString("foo"), NewWord("fn"), undefSpec, NewWord("end"),
		NewWord("foo"),
	})
	// foo should fall through to atom (string)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	if result[0].AsString() != "foo" {
		t.Errorf("foo after removing all sigs = %v, want atom \"foo\"", result[0])
	}
}

func TestPiecemealStackUnwind(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// def A (number -> dup mul), def B (string -> dup add), undef B, A still works
	bodyA := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	bodyB := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})

	// Define both
	_ = runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), bodyA, NewWord("end"),
		NewWord("def"), NewString("foo"), NewWord("fn"), bodyB, NewWord("end"),
	})

	// Both sigs work
	result := runAQL(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 || result[0].AsInteger() != 9 {
		t.Fatalf("3 foo = %v, want 9", result)
	}
	result = runAQL(t, r, []Value{
		NewString("hi"), NewWord("foo"),
	})
	if len(result) != 1 || result[0].AsString() != "hihi" {
		t.Fatalf("\"hi\" foo = %v, want \"hihi\"", result)
	}

	// Undef pops B (string sig), A (number sig) remains
	_ = runAQL(t, r, []Value{
		NewWord("undef"), NewWord("foo"), NewWord("end"),
	})
	result = runAQL(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 || result[0].AsInteger() != 9 {
		t.Fatalf("3 foo after undef B = %v, want 9", result)
	}
}
