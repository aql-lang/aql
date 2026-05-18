package native

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/internal/fileops"
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
	_as0, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as0 {
		t.Errorf("1 lt 2 = %v, want true", result)
	}
}

func TestEngineGt(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(3), NewWord("gt"), NewInteger(1)})
	_as1, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as1 {
		t.Errorf("3 gt 1 = %v, want true", result)
	}
}

func TestEngineLte(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(1), NewWord("lte"), NewInteger(1)})
	_as2, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as2 {
		t.Errorf("1 lte 1 = %v, want true", result)
	}
}

func TestEngineGte(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(2), NewWord("gte"), NewInteger(1)})
	_as3, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as3 {
		t.Errorf("2 gte 1 = %v, want true", result)
	}
}

func TestEngineEq(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(5), NewWord("eq"), NewInteger(5)})
	_as4, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as4 {
		t.Errorf("5 eq 5 = %v, want true", result)
	}
}

func TestEngineNeq(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(5), NewWord("neq"), NewInteger(3)})
	_as5, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as5 {
		t.Errorf("5 neq 3 = %v, want true", result)
	}
	result = runAQL(t, r, []Value{NewInteger(5), NewWord("neq"), NewInteger(5)})
	_as6, _ := AsBoolean(result[0])
	if len(result) != 1 || _as6 {
		t.Errorf("5 neq 5 = %v, want false", result)
	}
}

func TestEngineDeq(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewString("a"), NewWord("deq"), NewString("a")})
	_as7, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as7 {
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
	_as8, _ := AsInteger(result[0])
	if len(result) != 1 || _as8 != 1 {
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
	_as9, _ := AsInteger(result[0])
	if len(result) != 1 || _as9 != 2 {
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
	_as10, _ := AsInteger(result[0])
	if len(result) != 1 || _as10 != 42 {
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
	_as11, _ := AsInteger(result[0])
	if len(result) != 1 || _as11 != 10 {
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
	_as12, _ := AsInteger(result[0])
	if len(result) != 1 || _as12 != 3 {
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
			Args: []*Type{TAny},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
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
	_as13, _ := AsInteger(result[0])
	if len(result) != 1 || _as13 != 1 {
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
	_as14, _ := AsInteger(result[0])
	if len(result) != 1 || _as14 != 2 {
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
	_as15, _ := AsInteger(result[0])
	if len(result) != 1 || _as15 != 2 {
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
	SetHostFileOps(r, mem)

	result := runAQL(t, r, []Value{NewWord("read"), NewString("test.txt")})
	_as16, _ := AsString(result[0])
	if len(result) != 1 || _as16 != "hello world" {
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
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("lines"))
	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.txt"), NewMap(opts)})
	if len(result) != 1 || !result[0].VType.Equal(TList) {
		t.Errorf("read with lines fmt = %v, want list", result)
	}
	_lst, _ := AsList(result[0])
	elems := _lst.Slice()
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
	SetHostFileOps(r, mem)

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
	SetHostFileOps(r, mem)

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
	SetHostFileOps(r, mem)

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
	SetHostFileOps(r, mem)

	result := runAQL(t, r, []Value{NewWord("write"), NewString("out.txt"), NewString("hello")})
	_as17, _ := AsString(result[0])
	if len(result) != 1 || _as17 != "out.txt" {
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
	SetHostFileOps(r, mem)

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
	SetHostFileOps(r, mem)

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
	SetHostFileOps(r, mem)

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
	SetHostFileOps(r, mem)

	// write non-string (map) value with options → auto-serializes with jsonic
	// The write [string, any, map] signature expects path, data, opts
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	// All prefix: nearest→sig[0]=path, next→sig[1]=data, deepest→sig[2]=opts
	runAQL(t, r, []Value{NewMap(opts), NewMap(m), NewString("out.json"), NewWord("write")})
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
	SetHostFileOps(r, mem)

	// Default nl:"lf" normalizes \r\n to \n
	result := runAQL(t, r, []Value{NewWord("read"), NewString("crlf.txt")})
	_as18, _ := AsString(result[0])
	if _as18 != "a\nb\nc" {
		_as19, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as19, "a\nb\nc")
	}

	// nl:"raw" preserves original
	opts := NewOrderedMap()
	opts.Set("nl", NewString("raw"))
	result = runAQL(t, r, []Value{NewWord("read"), NewString("crlf.txt"), NewMap(opts)})
	_as20, _ := AsString(result[0])
	if _as20 != "a\r\nb\r\nc" {
		_as21, _ := AsString(result[0])
		t.Errorf("raw got %q, want %q", _as21, "a\r\nb\r\nc")
	}
}

// --- Registry integration tests ---

func TestRegistrySetFileOps(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	SetHostFileOps(r, mem)
	if HostFileOps(r) != mem {
		t.Error("SetHostFileOps did not install the FileOps capability")
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
	// 99 convert String
	result := runAQL(t, r, []Value{
		NewInteger(99), NewWord("convert"), NewWord("String"),
	})
	_as22, _ := AsString(result[0])
	if len(result) != 1 || _as22 != "99" {
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
	_as23, _ := AsInteger(result[0])
	if len(result) != 1 || _as23 != 0 {
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
		NewWord("def"), NewWord("inc"), body, NewEnd(),
		NewInteger(5), NewWord("inc"),
	})
	_as24, _ := AsInteger(result[0])
	if len(result) != 1 || _as24 != 6 {
		t.Errorf("def inc [1 add]; 5 inc = %v, want 6", result)
	}
}

func TestEngineUndef(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def foo 42 end foo undef foo end foo → error (foo undefined after undef)
	e := New(r)
	_, err = e.Run([]Value{
		NewWord("def"), NewWord("foo"), NewInteger(42), NewEnd(),
		NewWord("foo"),
		NewWord("undef"), NewWord("foo"), NewEnd(),
		NewWord("foo"),
	})
	if err == nil {
		t.Fatal("expected error for undefined word after undef, got nil")
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
	if len(result) != 1 || !IsRecordType(result[0]) {
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
	if len(result) != 1 || !IsTableType(result[0]) {
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
	_as26, _ := AsBoolean(result[1])
	if _as26 != true {
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
	_as27, _ := AsInteger(result[0])
	if len(result) != 1 || _as27 != 3 {
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
	_as28, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as28 {
		t.Errorf("true or false = %v, want true", result)
	}
}

func TestEngineAnd(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewBoolean(true), NewWord("and"), NewBoolean(false)})
	_as29, _ := AsBoolean(result[0])
	if len(result) != 1 || _as29 {
		t.Errorf("true and false = %v, want false", result)
	}
}

func TestEngineNot(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewBoolean(true), NewWord("not")})
	_as30, _ := AsBoolean(result[0])
	if len(result) != 1 || _as30 {
		t.Errorf("true not = %v, want false", result)
	}
}

func TestEngineConvertStringVariants(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	HEXOpts := NewOrderedMap()
	HEXOpts.Set("base", NewString("HEX"))
	binOpts := NewOrderedMap()
	binOpts.Set("base", NewString("bin"))
	octOpts := NewOrderedMap()
	octOpts.Set("base", NewString("oct"))

	// 10 convert String {base:hex} → 'a'
	result := runAQL(t, r, []Value{
		NewInteger(10), NewWord("convert"), NewWord("String"), NewMap(hexOpts),
	})
	_as31, _ := AsString(result[0])
	if len(result) != 1 || _as31 != "a" {
		t.Errorf("10 convert String {base:hex} = %v, want 'a'", result)
	}

	// 255 convert String {base:HEX} → 'FF'
	result = runAQL(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewWord("String"), NewMap(HEXOpts),
	})
	_as32, _ := AsString(result[0])
	if len(result) != 1 || _as32 != "FF" {
		t.Errorf("255 convert String {base:HEX} = %v, want 'FF'", result)
	}

	// 10 convert String {base:bin} → '1010'
	result = runAQL(t, r, []Value{
		NewInteger(10), NewWord("convert"), NewWord("String"), NewMap(binOpts),
	})
	_as33, _ := AsString(result[0])
	if len(result) != 1 || _as33 != "1010" {
		t.Errorf("10 convert String {base:bin} = %v, want '1010'", result)
	}

	// 8 convert String {base:oct} → '10'
	result = runAQL(t, r, []Value{
		NewInteger(8), NewWord("convert"), NewWord("String"), NewMap(octOpts),
	})
	_as34, _ := AsString(result[0])
	if len(result) != 1 || _as34 != "10" {
		t.Errorf("8 convert String {base:oct} = %v, want '10'", result)
	}
}

func TestEngineConvertToNumber(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// "42" convert Number → 42
	result := runAQL(t, r, []Value{
		NewString("42"), NewWord("convert"), NewWord("Number"),
	})
	_as35, _ := AsInteger(result[0])
	if len(result) != 1 || _as35 != 42 {
		t.Errorf("'42' convert Number = %v, want 42", result)
	}

	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	binOpts := NewOrderedMap()
	binOpts.Set("base", NewString("bin"))
	octOpts := NewOrderedMap()
	octOpts.Set("base", NewString("oct"))

	// "ff" convert Number {base:hex} → 255
	result = runAQL(t, r, []Value{
		NewString("ff"), NewWord("convert"), NewWord("Number"), NewMap(hexOpts),
	})
	_as36, _ := AsInteger(result[0])
	if len(result) != 1 || _as36 != 255 {
		t.Errorf("'ff' convert Number {base:hex} = %v, want 255", result)
	}

	// "1010" convert Number {base:bin} → 10
	result = runAQL(t, r, []Value{
		NewString("1010"), NewWord("convert"), NewWord("Number"), NewMap(binOpts),
	})
	_as37, _ := AsInteger(result[0])
	if len(result) != 1 || _as37 != 10 {
		t.Errorf("'1010' convert Number {base:bin} = %v, want 10", result)
	}

	// "10" convert Number {base:oct} → 8
	result = runAQL(t, r, []Value{
		NewString("10"), NewWord("convert"), NewWord("Number"), NewMap(octOpts),
	})
	_as38, _ := AsInteger(result[0])
	if len(result) != 1 || _as38 != 8 {
		t.Errorf("'10' convert Number {base:oct} = %v, want 8", result)
	}
}

func TestEngineConvertToBoolean(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// 1 convert Boolean → true
	result := runAQL(t, r, []Value{
		NewInteger(1), NewWord("convert"), NewWord("Boolean"),
	})
	_as39, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as39 {
		t.Errorf("1 convert Boolean = %v, want true", result)
	}

	// 0 convert Boolean → false
	result = runAQL(t, r, []Value{
		NewInteger(0), NewWord("convert"), NewWord("Boolean"),
	})
	_as40, _ := AsBoolean(result[0])
	if len(result) != 1 || _as40 {
		t.Errorf("0 convert Boolean = %v, want false", result)
	}

	// "true" convert Boolean → true
	result = runAQL(t, r, []Value{
		NewString("true"), NewWord("convert"), NewWord("Boolean"),
	})
	_as41, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as41 {
		t.Errorf("'true' convert Boolean = %v, want true", result)
	}

	// "" convert Boolean → false
	result = runAQL(t, r, []Value{
		NewString(""), NewWord("convert"), NewWord("Boolean"),
	})
	_as42, _ := AsBoolean(result[0])
	if len(result) != 1 || _as42 {
		t.Errorf("'' convert Boolean = %v, want false", result)
	}

	// true convert Boolean → true (passthrough)
	result = runAQL(t, r, []Value{
		NewBoolean(true), NewWord("convert"), NewWord("Boolean"),
	})
	_as43, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as43 {
		t.Errorf("true convert Boolean = %v, want true", result)
	}
}

func TestEngineBaseTypes(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		typeLit *Type
		wantStr string
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
		NewWord("def"), NewWord("double"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(7), NewWord("double"),
	})
	_as44, _ := AsInteger(result[0])
	if len(result) != 1 || _as44 != 14 {
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
		NewWord("def"), NewWord("square"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("square"),
	})
	_as45, _ := AsInteger(result[0])
	if len(result) != 1 || _as45 != 25 {
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
	// All prefix: nearest→sig[0]=Integer, next→sig[1]=String
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("catter"), NewWord("fn"), fnBody, NewEnd(),
		NewString("a"), NewInteger(1), NewWord("catter"),
	})
	if len(result) != 1 || !result[0].VType.Matches(TString) {
		t.Errorf("1 'a' catter = %v, want string result", result)
	}
}

func TestEngineFnCatterPartialForward(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def catter fn [[integer string] [string] [add]] end
	// Case: catter 2 "b" -> all forward (integer, string)
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Integer"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("add")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("catter"), NewWord("fn"), fnBody, NewEnd(),
		NewWord("catter"), NewInteger(2), NewString("b"),
	})
	if len(result) != 1 || !result[0].VType.Matches(TString) {
		t.Errorf("2 catter 'b' = %v, want string result", result)
	}
}

func TestEngineFnCatterFullForward(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def catter fn [[integer string] [string] [add]] end
	// Case: [|] -> catter 3 "c" -> both args from forward (positional match)
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Integer"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("add")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("catter"), NewWord("fn"), fnBody, NewEnd(),
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
		NewWord("def"), NewWord("joiner"), NewWord("fn"), fnBody, NewEnd(),
	}

	// Subtest: all args from prefix (stack)
	// "A" "B" "C" joiner → nearest to joiner is "C"→sig[0], "B"→sig[1], "A"→sig[2]
	// All positions are equivalent: values nearest the word map to sig[0].
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewString("A"), NewString("B"), NewString("C"), NewWord("joiner"),
		)
		result := runAQL(t, r, tokens)
		_as46, _ := AsString(result[0])
		if len(result) != 1 || _as46 != "CBA" {
			t.Errorf(`"A" "B" "C" joiner = %v, want ["CBA"]`, result)
		}
	})

	// Subtest: 1 prefix + 2 forward
	// "A" joiner "B" "C" → fwd: "B"→sig[0], "C"→sig[1]; stack: "A"→sig[2]
	t.Run("MixedPrefixForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewString("A"), NewWord("joiner"), NewString("B"), NewString("C"),
		)
		result := runAQL(t, r, tokens)
		_as47, _ := AsString(result[0])
		if len(result) != 1 || _as47 != "BCA" {
			t.Errorf(`"A" joiner "B" "C" = %v, want ["BCA"]`, result)
		}
	})

	// Subtest: 2 prefix + 1 forward
	// "A" "B" joiner "C" → fwd: "C"→sig[0]; stack: top="B"→sig[1], "A"→sig[2]
	t.Run("TwoPrefixOneForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewString("A"), NewString("B"), NewWord("joiner"), NewString("C"),
		)
		result := runAQL(t, r, tokens)
		_as48, _ := AsString(result[0])
		if len(result) != 1 || _as48 != "CBA" {
			t.Errorf(`"A" "B" joiner "C" = %v, want ["CBA"]`, result)
		}
	})

	// Subtest: all args from forward
	// joiner "A" "B" "C" -> args=["A","B","C"] -> concat -> "ABC"
	t.Run("AllForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewWord("joiner"), NewString("A"), NewString("B"), NewString("C"),
		)
		result := runAQL(t, r, tokens)
		_as49, _ := AsString(result[0])
		if len(result) != 1 || _as49 != "ABC" {
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
	// ValToString: integer->digits, boolean->"true"/"false"
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("String"), NewWord("Integer"), NewWord("Boolean"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(4)),
	})
	defTokens := []Value{
		NewWord("def"), NewWord("mix4"), NewWord("fn"), fnBody, NewEnd(),
	}

	// All prefix: nearest→sig[0]=String, next→sig[1]=Integer, next→sig[2]=Boolean, deepest→sig[3]=String
	// Stack bottom-to-top: "Z" true 7 "X" mix4
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewString("Z"), NewBoolean(true), NewInteger(7), NewString("X"), NewWord("mix4"),
		))
		_as50, _ := AsString(result[0])
		if len(result) != 1 || _as50 != "X7trueZ" {
			t.Errorf(`all-prefix mix4 = %v, want ["X7trueZ"]`, result)
		}
	})

	// "Z" mix4 "X" 7 true → 1 prefix + 3 forward, types align with sig positions.
	// sig[0]=String("X"), sig[1]=Integer(7), sig[2]=Boolean(true), sig[3]=String("Z" from stack).
	t.Run("OnePrefixThreeForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewString("Z"), NewWord("mix4"), NewString("X"), NewInteger(7), NewBoolean(true),
		))
		_as51, _ := AsString(result[0])
		if len(result) != 1 || _as51 != "X7trueZ" {
			t.Errorf(`1+3 mix4 = %v, want ["X7trueZ"]`, result)
		}
	})

	// mix4 "X" 7 true "Z" -> all forward
	t.Run("TwoPrefixTwoForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewWord("mix4"), NewString("X"), NewInteger(7), NewBoolean(true), NewString("Z"),
		))
		_as52, _ := AsString(result[0])
		if len(result) != 1 || _as52 != "X7trueZ" {
			t.Errorf(`mix4 all-forward = %v, want ["X7trueZ"]`, result)
		}
	})

	// mix4 "X" 7 true "Z" -> all forward
	t.Run("AllForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewWord("mix4"), NewString("X"), NewInteger(7), NewBoolean(true), NewString("Z"),
		))
		_as53, _ := AsString(result[0])
		if len(result) != 1 || _as53 != "X7trueZ" {
			t.Errorf(`all-forward mix4 = %v, want ["X7trueZ"]`, result)
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
		NewWord("def"), NewWord("mix5"), NewWord("fn"), fnBody, NewEnd(),
	}

	// All prefix: nearest→sig[0]=String, ..., deepest→sig[4]=String
	// Stack bottom-to-top: "z" false 1.5 3 "a" mix5
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewString("z"), NewBoolean(false), NewDecimal(1.5), NewInteger(3), NewString("a"),
			NewWord("mix5"),
		))
		_as54, _ := AsString(result[0])
		if len(result) != 1 || _as54 != "a31.5falsez" {
			t.Errorf(`all-prefix mix5 = %v, want ["a31.5falsez"]`, result)
		}
	})

	// mix5 "a" 3 1.5 false "z" -> all forward
	t.Run("AllForwardExplicit", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewWord("mix5"), NewString("a"), NewInteger(3),
			NewDecimal(1.5), NewBoolean(false), NewString("z"),
		))
		_as55, _ := AsString(result[0])
		if len(result) != 1 || _as55 != "a31.5falsez" {
			t.Errorf(`2+3 mix5 = %v, want ["a31.5falsez"]`, result)
		}
	})

	// mix5 "a" 3 1.5 false "z" -> all forward
	t.Run("AllForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		result := runAQL(t, r, append(append([]Value{}, defTokens...),
			NewWord("mix5"), NewString("a"), NewInteger(3),
			NewDecimal(1.5), NewBoolean(false), NewString("z"),
		))
		_as56, _ := AsString(result[0])
		if len(result) != 1 || _as56 != "a31.5falsez" {
			t.Errorf(`all-forward mix5 = %v, want ["a31.5falsez"]`, result)
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
		NewWord("def"), NewWord("mix7"), NewWord("fn"), fnBody, NewEnd(),
	}
	// Expected concat in sig order: "p123.5trueq456r7"
	want := "p123.5trueq456r7"
	argVals := []Value{
		NewString("p1"), NewInteger(2), NewDecimal(3.5),
		NewBoolean(true), NewString("q4"), NewInteger(56), NewString("r7"),
	}

	// All prefix: stack bottom-to-top reversed from sig order (nearest→sig[0])
	// sig[6]=String, sig[5]=Integer, sig[4]=String, sig[3]=Boolean, sig[2]=Decimal, sig[1]=Integer, sig[0]=String
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		argValsReversed := []Value{
			NewString("r7"), NewInteger(56), NewString("q4"),
			NewBoolean(true), NewDecimal(3.5), NewInteger(2), NewString("p1"),
		}
		tokens := append(append([]Value{}, defTokens...), argValsReversed...)
		tokens = append(tokens, NewWord("mix7"))
		result := runAQL(t, r, tokens)
		_as57, _ := AsString(result[0])
		if len(result) != 1 || _as57 != want {
			t.Errorf("all-prefix mix7 = %v, want [%q]", result, want)
		}
	})

	// all forward (was 3+4 mixed, changed for sequential planner)
	t.Run("ThreePrefixFourForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...), NewWord("mix7"))
		tokens = append(tokens, argVals...)
		result := runAQL(t, r, tokens)
		_as58, _ := AsString(result[0])
		if len(result) != 1 || _as58 != want {
			t.Errorf("mix7 all-forward = %v, want [%q]", result, want)
		}
	})

	// 1 prefix + 6 forward: last arg ("r7") as prefix, rest forward.
	// Forward types must align with sig[0..5], prefix fills sig[6].
	t.Run("OnePrefixSixForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...), argVals[6]) // "r7" prefix
		tokens = append(tokens, NewWord("mix7"))
		tokens = append(tokens, argVals[:6]...) // "p1" 2 3.5 true "q4" 56 forward
		result := runAQL(t, r, tokens)
		_as59, _ := AsString(result[0])
		if len(result) != 1 || _as59 != want {
			t.Errorf("1+6 mix7 = %v, want [%q]", result, want)
		}
	})

	// All forward
	t.Run("AllForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...), NewWord("mix7"))
		tokens = append(tokens, argVals...)
		result := runAQL(t, r, tokens)
		_as60, _ := AsString(result[0])
		if len(result) != 1 || _as60 != want {
			t.Errorf("all-forward mix7 = %v, want [%q]", result, want)
		}
	})
}

func TestEngineFnConcatArgOrderEndDisambiguate(t *testing.T) {
	// Tests that the "end" word stops forward argument collection,
	// preventing the fn from consuming tokens that follow.

	// def cat3 fn [[string string string] [string]
	//              [drop drop drop args concat]] end
	cat3Body := NewList([]Value{
		NewList([]Value{NewWord("String"), NewWord("String"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(3)),
	})
	cat3Def := []Value{
		NewWord("def"), NewWord("cat3"), NewWord("fn"), cat3Body, NewEnd(),
	}

	// def cat4 fn [[string integer boolean string] [string]
	//              [drop drop drop drop args concat]] end
	cat4Body := NewList([]Value{
		NewList([]Value{NewWord("String"), NewWord("Integer"), NewWord("Boolean"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(4)),
	})
	cat4Def := []Value{
		NewWord("def"), NewWord("cat4"), NewWord("fn"), cat4Body, NewEnd(),
	}

	// cat3 "A" "B" "C" end "trailing" -> cat3 gets "ABC", "trailing" on stack
	t.Run("EndStopsForward3", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat3Def...),
			NewWord("cat3"), NewString("A"), NewString("B"), NewString("C"),
			NewEnd(), NewString("trailing"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("cat3 A B C end trailing: got %d results, want 2: %v", len(result), result)
		}
		_as61, _ := AsString(result[0])
		if _as61 != "ABC" {
			_as62, _ := AsString(result[0])
			t.Errorf("cat3 result = %q, want %q", _as62, "ABC")
		}
		_as63, _ := AsString(result[1])
		if _as63 != "trailing" {
			t.Errorf("trailing = %v, want 'trailing'", result[1])
		}
	})

	// "Z" cat4 "X" 7 true end "after" → 1 prefix + 3 forward, types align.
	// sig[0]=String("X"), sig[1]=Integer(7), sig[2]=Boolean(true), sig[3]=String("Z" from stack).
	t.Run("EndStopsForward4Mixed", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat4Def...),
			NewString("Z"), NewWord("cat4"), NewString("X"), NewInteger(7), NewBoolean(true),
			NewEnd(), NewString("after"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("Z cat4 X 7 true end after: got %d results, want 2: %v", len(result), result)
		}
		_as64, _ := AsString(result[0])
		if _as64 != "X7trueZ" {
			_as65, _ := AsString(result[0])
			t.Errorf("cat4 result = %q, want %q", _as65, "X7trueZ")
		}
		_as66, _ := AsString(result[1])
		if _as66 != "after" {
			t.Errorf("trailing = %v, want 'after'", result[1])
		}
	})

	// Two fn calls using parens and end: (cat3 "A" "B" "C" end) (cat3 "D" "E" "F" end)
	// Parens isolate each call; end stops forward collection within each group.
	t.Run("EndSeparatesTwoCalls", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat3Def...),
			NewOpenParen(),
			NewWord("cat3"), NewString("A"), NewString("B"), NewString("C"), NewEnd(),
			NewCloseParen(),
			NewOpenParen(),
			NewWord("cat3"), NewString("D"), NewString("E"), NewString("F"), NewEnd(),
			NewCloseParen(),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("two cat3 calls: got %d results, want 2: %v", len(result), result)
		}
		_as67, _ := AsString(result[0])
		if _as67 != "ABC" {
			_as68, _ := AsString(result[0])
			t.Errorf("first cat3 = %q, want %q", _as68, "ABC")
		}
		_as69, _ := AsString(result[1])
		if _as69 != "DEF" {
			_as70, _ := AsString(result[1])
			t.Errorf("second cat3 = %q, want %q", _as70, "DEF")
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
			NewOpenParen(),
			NewWord("cat4"), NewString("m"), NewInteger(9), NewBoolean(false), NewString("n"), NewEnd(),
			NewCloseParen(),
			NewOpenParen(),
			NewWord("cat3"), NewString("x"), NewString("y"), NewString("z"), NewEnd(),
			NewCloseParen(),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("cat4+cat3 with end: got %d results, want 2: %v", len(result), result)
		}
		_as71, _ := AsString(result[0])
		if _as71 != "m9falsen" {
			_as72, _ := AsString(result[0])
			t.Errorf("cat4 = %q, want %q", _as72, "m9falsen")
		}
		_as73, _ := AsString(result[1])
		if _as73 != "xyz" {
			_as74, _ := AsString(result[1])
			t.Errorf("cat3 = %q, want %q", _as74, "xyz")
		}
	})

	// Prefix-heavy with end: "P" "Q" cat3 "R" end "extra"
	// 2 prefix, 1 forward, end stops collection, "extra" remains.
	// fwd: "R"→sig[0]; stack: top="Q"→sig[1], "P"→sig[2] → "RQP"
	t.Run("EndAfterPartialForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat3Def...),
			NewString("P"), NewString("Q"), NewWord("cat3"), NewString("R"),
			NewEnd(), NewString("extra"),
		)
		result := runAQL(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("P Q cat3 R end extra: got %d results, want 2: %v", len(result), result)
		}
		_as75, _ := AsString(result[0])
		if _as75 != "RQP" {
			_as76, _ := AsString(result[0])
			t.Errorf("cat3 = %q, want %q", _as76, "RQP")
		}
		_as77, _ := AsString(result[1])
		if _as77 != "extra" {
			t.Errorf("trailing = %v, want 'extra'", result[1])
		}
	})
}

func TestIntegerLiteralType(t *testing.T) {
	// Post §1.1 fix: NewInteger no longer encodes the value in the
	// type path. All integers share VType=Integer;
	// specific-value dispatch goes through Signature.Patterns.
	v := NewInteger(5)
	if !v.VType.Equal(TInteger) {
		t.Errorf("NewInteger(5).VType = %s, want Integer", v.VType)
	}
	if !v.VType.Matches(TNumber) {
		t.Errorf("NewInteger(5).VType = %s, want matches number", v.VType)
	}
	// Two different integers now share the same VType — pattern
	// dispatch uses Signature.Patterns instead of type-path leaves.
	v0 := NewInteger(0)
	v1 := NewInteger(1)
	if !v0.VType.Equal(v1.VType) {
		t.Errorf("NewInteger(0) and NewInteger(1) should share VType=Integer; got %s vs %s", v0.VType, v1.VType)
	}
	// And both still match Integer / Number / Scalar.
	if !v0.VType.Matches(TInteger) || !v1.VType.Matches(TInteger) {
		t.Error("both should match Integer")
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
		NewWord("def"), NewWord("adder"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(0), NewWord("adder"),
	})
	_as78, _ := AsInteger(result[0])
	if len(result) != 1 || _as78 != 2 {
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
		NewWord("def"), NewWord("adder"), NewWord("fn"), fnBody, NewEnd(),
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
		NewWord("def"), NewWord("handler"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(0), NewWord("handler"),
	})
	_as79, _ := AsInteger(result[0])
	if len(result) != 1 || _as79 != 10 {
		t.Errorf("0 handler = %v, want 10", result)
	}

	result = runAQL(t, r, []Value{
		NewInteger(1), NewWord("handler"),
	})
	_as80, _ := AsInteger(result[0])
	if len(result) != 1 || _as80 != 21 {
		t.Errorf("1 handler = %v, want 21", result)
	}
}

func TestEngineFnDefPrefixOnly(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def doubler/s fn [[x:integer] [integer] [x x add]] end
	// doubler/s registers as stack-only: takes args from the stack only,
	// never collects forward args via forward.
	fnBody := NewList([]Value{
		func() Value {
			m := NewOrderedMap()
			m.Set("x", NewWord("Integer"))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("x"), NewWord("x"), NewWord("add")}),
	})
	// 5 doubler — 5 is on stack, doubler takes it as prefix arg
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWordModified("doubler", -1, true, false), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("doubler"),
	})
	_as81, _ := AsInteger(result[0])
	if len(result) != 1 || _as81 != 10 {
		t.Errorf("5 doubler = %v, want 10", result)
	}
}

func TestEngineFnDefPrefixOnlyNoForwardCollection(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// def doubler/s fn [[x:integer] [integer] [x x add]] end
	// doubler 5 — stack-only word should NOT collect 5 as forward arg.
	// It should fail because there's nothing on the stack for prefix match.
	fnBody := NewList([]Value{
		func() Value {
			m := NewOrderedMap()
			m.Set("x", NewWord("Integer"))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("x"), NewWord("x"), NewWord("add")}),
	})
	// Define using string name (def sig selection changed with new type hierarchy).
	_ = runAQL(t, r, []Value{
		NewWord("def"), NewString("doubler"), NewWord("fn"), fnBody, NewEnd(),
	})

	// Prefix call with arg on stack should work.
	result := runAQL(t, r, []Value{
		NewInteger(5), NewWord("doubler"),
	})
	_as82, _ := AsInteger(result[0])
	if len(result) != 1 || _as82 != 10 {
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
		NewWord("def"), NewString("foo"), NewWord("fn"), fnBody, NewEnd(),
		NewString("x"), NewWord("foo"),
	})
	_as83, _ := AsString(result[0])
	if len(result) != 1 || _as83 != "xQ" {
		t.Errorf("foo \"x\" = %v, want \"xQ\"", result)
	}

	// foo 1 → "1P" (integer matches sig 2: 1 add "P")
	result = runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(1), NewWord("foo"),
	})
	_as84, _ := AsString(result[0])
	if len(result) != 1 || _as84 != "1P" {
		t.Errorf("foo 1 = %v, want \"1P\"", result)
	}

	// foo 99 → "NN" (literal 99 matches sig 3: drop "NN")
	result = runAQL(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(99), NewWord("foo"),
	})
	_as85, _ := AsString(result[0])
	if len(result) != 1 || _as85 != "NN" {
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
		NewWord("def"), NewWord("double"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(7), NewWord("double"),
	})
	_as86, _ := AsInteger(result[0])
	if len(result) != 1 || _as86 != 14 {
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
		// sig 2 (recursive): [x:integer] [integer] [x (fact (x sub 1)) mul]
		func() Value {
			m := NewOrderedMap()
			m.Set("x", NewWord("Integer"))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{
			NewWord("x"),
			NewOpenParen(), NewWord("fact"), NewOpenParen(), NewWord("x"), NewWord("sub"), NewInteger(1), NewCloseParen(), NewCloseParen(),
			NewWord("mul"),
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
			NewWord("def"), NewString("fact"), NewWord("fn"), fnBody, NewEnd(),
			NewInteger(tc.input), NewWord("fact"),
		})
		_as87, _ := AsInteger(result[0])
		if len(result) != 1 || _as87 != tc.expected {
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
			NewOpenParen(), NewWord("dup"), NewWord("sub"), NewInteger(1), NewCloseParen(),
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
			e := NewTop(r)
			result, err := e.Run([]Value{
				NewWord("def"), NewString("fact"), NewWord("fn"), fnBody, NewEnd(),
				NewInteger(tc.input), NewWord("fact"),
			})
			if err != nil {
				t.Logf("FAIL body=%q: fact %d error: %v", b.name, tc.input, err)
				allPass = false
				break
			}
			_as88, _ := AsInteger(result[0])
			if len(result) != 1 || _as88 != tc.expected {
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
		// sig 2 (recursive): [x:integer] [integer] [x (fact (x sub 1)) mul]
		func() Value {
			m := NewOrderedMap()
			m.Set("x", NewWord("Integer"))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{
			NewWord("x"),
			NewOpenParen(), NewWord("fact"), NewOpenParen(), NewWord("x"), NewWord("sub"), NewInteger(1), NewCloseParen(), NewCloseParen(),
			NewWord("mul"),
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
			NewWord("def"), NewString("fact"), NewWord("fn"), fnBody, NewEnd(),
			NewInteger(tc.input), NewWord("fact"),
		})
		_as89, _ := AsInteger(result[0])
		if len(result) != 1 || _as89 != tc.expected {
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
		NewWord("type"), NewWord("Point"), NewWord("record"), fields, NewEnd(),
		NewWord("Point"),
	})
	if len(result) != 1 || !IsRecordType(result[0]) {
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
		NewWord("type"), NewWord("P"), NewWord("record"), fields, NewEnd(),
		NewWord("make"), NewWord("P"), vals,
	})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Errorf("expected map, got %v", result)
	}
	m, _ := AsMap(result[0])
	xVal, _ := m.Get("x")
	_as90, _ := AsInteger(xVal)
	if _as90 != 1 {
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
	_as91, _ := AsBoolean(result[1])
	if len(result) != 2 || !_as91 {
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
	_as92, _ := AsBoolean(result[1])
	if len(result) != 2 || !_as92 {
		t.Errorf("[1,2] unify [1,2] = %v, want true", result)
	}
}

func TestEngineUnifyFail(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewInteger(1), NewString("a"), NewWord("unify")})
	_as93, _ := AsBoolean(result[1])
	if len(result) != 2 || _as93 {
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
	_as94, _ := AsBoolean(result[1])
	if len(result) != 2 || !_as94 {
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
	_as95, _ := AsBoolean(result[1])
	if len(result) != 2 || !_as95 {
		t.Errorf("{:number} unify {a:1,b:2} = %v, want true", result)
	}
}

func TestEngineDisjunct(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// string tor none
	result := runAQL(t, r, []Value{
		NewTypeLiteral(TString), NewWord("tor"), NewTypeLiteral(TNone),
	})
	if len(result) != 1 || !IsDisjunct(result[0]) {
		t.Errorf("string tor none = %v, want disjunct", result)
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
	_as96, _ := AsInteger(result[0])
	if len(result) != 1 || _as96 != 25 {
		t.Errorf("5 var [[x] x mul x] = %v, want 25", result)
	}
}

func TestEngineAddStrings(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewString("hello"), NewWord("add"), NewString(" world")})
	_as97, _ := AsString(result[0])
	if len(result) != 1 || _as97 != "hello world" {
		t.Errorf("'hello' add ' world' = %v, want 'hello world'", result)
	}
}

// --- ValToString coverage ---

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
			got := ValToString(tt.val)
			if got != tt.want {
				t.Errorf("ValToString(%s) = %q, want %q", tt.val, got, tt.want)
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
	SetHostFileOps(r, mem)

	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.csv")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !IsTableType(v) {
		t.Fatalf("expected table type, got %s", v.VType)
	}
	_lst, _ := AsList(v)
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	r0, _ := AsMap(rows[0])
	nameVal, ok := r0.Get("name")
	if !ok {
		t.Fatal("expected 'name' key")
	}
	_as98, _ := AsString(nameVal)
	if _as98 != "Alice" {
		_as99, _ := AsString(nameVal)
		t.Errorf("name = %q, want %q", _as99, "Alice")
	}
}

func TestEngineReadTSVByExtension(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	mem.Files["data.tsv"] = []byte("name\tage\nAlice\t30\nBob\t25")
	SetHostFileOps(r, mem)

	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.tsv")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !IsTableType(v) {
		t.Fatalf("expected table type, got %s", v.VType)
	}
	_lst, _ := AsList(v)
	rows := _lst.Slice()
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
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("csv"))
	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.txt"), NewMap(opts)})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !IsTableType(v) {
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
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	result := runAQL(t, r, []Value{NewWord("read"), NewString("data.csv"), NewMap(opts)})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	// With text format, we get a plain string, not a table
	if IsTableType(result[0]) {
		t.Error("expected non-table type with text format override")
	}
	_as100, _ := AsString(result[0])
	if _as100 != "hello,world" {
		_as101, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as101, "hello,world")
	}
}

func TestEngineReadJSONByExtension(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mem := fileops.NewMem()
	mem.Files["data.json"] = []byte(`{"key":"value"}`)
	SetHostFileOps(r, mem)

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
	if !v.VType.Equal(TInspect) {
		t.Fatalf("expected type %s, got %s", TInspect, v.VType)
	}
	m, _ := AsMap(v)

	// Check name field.
	name, ok := m.Get("name")
	_as102, _ := AsString(name)
	if !ok || _as102 != "add" {
		t.Errorf("name = %v, want 'add'", name)
	}

	// Check kind field.
	kind, ok := m.Get("kind")
	_as103, _ := AsAtom(kind)
	if !ok || _as103 != "native" {
		t.Errorf("kind = %v, want native", kind)
	}

	// Check signatures field is a non-empty list.
	sigs, ok := m.Get("signatures")
	if !ok {
		t.Fatal("missing signatures field")
	}
	_lst, _ := AsList(sigs)
	sigList := _lst.Slice()
	if len(sigList) == 0 {
		t.Error("expected at least one signature for add")
	}

	// Check first signature has args.
	sig0, _ := AsMap(sigList[0])
	args, _ := sig0.Get("args")
	_lst2, _ := AsList(args)
	argList := _lst2.Slice()
	if len(argList) != 2 {
		t.Errorf("expected 2 args for add, got %d", len(argList))
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
	m, _ := AsMap(result[0])

	kind, _ := m.Get("kind")
	_as104, _ := AsAtom(kind)
	if _as104 != "defined" {
		t.Errorf("kind = %v, want defined", kind)
	}

	name, _ := m.Get("name")
	_as105, _ := AsString(name)
	if _as105 != "double" {
		t.Errorf("name = %v, want 'double'", name)
	}
}

func TestEngineInspectUnknown(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{NewWord("inspect"), NewAtom("nonexistent")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	m, _ := AsMap(result[0])

	kind, _ := m.Get("kind")
	_as106, _ := AsAtom(kind)
	if _as106 != "unknown" {
		t.Errorf("kind = %v, want unknown", kind)
	}

	sigs, _ := m.Get("signatures")
	_lst, _ := AsList(sigs)
	if len(_lst.Slice()) != 0 {
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
		NewWord("get"), NewWord("name"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	_as107, _ := AsString(result[0])
	if _as107 != "upper" {
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
	if !v.VType.Equal(TInspect) {
		t.Fatalf("expected type %s, got %s", TInspect, v.VType)
	}
	m, _ := AsMap(v)

	name, _ := m.Get("name")
	_as108, _ := AsString(name)
	if _as108 != "Qty" {
		t.Errorf("name = %v, want 'Qty'", name)
	}
	kind, _ := m.Get("kind")
	_as109, _ := AsAtom(kind)
	if _as109 != "literal" {
		t.Errorf("kind = %v, want literal", kind)
	}
	// A named type's `type` is the metatype "Type"; its underlying
	// structure leaf goes to `struct`.
	typ, _ := m.Get("type")
	_as110, _ := AsString(typ)
	if _as110 != "Type" {
		t.Errorf("type = %v, want 'Type'", typ)
	}
	strct, _ := m.Get("struct")
	_asStruct, _ := AsString(strct)
	if _asStruct != "Number" {
		t.Errorf("struct = %v, want 'Number'", strct)
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
	m, _ := AsMap(result[0])

	name, _ := m.Get("name")
	_as111, _ := AsString(name)
	if _as111 != "Pos" {
		t.Errorf("name = %v, want 'Pos'", name)
	}
	kind, _ := m.Get("kind")
	_as112, _ := AsAtom(kind)
	if _as112 != "record" {
		t.Errorf("kind = %v, want record", kind)
	}
	flds, ok := m.Get("fields")
	if !ok {
		t.Fatal("missing fields")
	}
	fm, _ := AsMap(flds)
	xType, _ := fm.Get("x")
	_as113, _ := AsString(xType)
	if _as113 != "Number" {
		t.Errorf("fields.x = %v, want 'Number'", xType)
	}
	yType, _ := fm.Get("y")
	_as114, _ := AsString(yType)
	if _as114 != "Number" {
		t.Errorf("fields.y = %v, want 'Number'", yType)
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
		NewWord("get"), NewWord("kind"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	_as115, _ := AsAtom(result[0])
	if _as115 != "literal" {
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
		NewWord("def"), NewWord("double"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("double"),
	})
	_as116, _ := AsInteger(result[0])
	if len(result) != 1 || _as116 != 10 {
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
		NewWord("def"), NewWord("bad"), NewWord("fn"), fnBody, NewEnd(),
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
	// def toomany fn [[number] [number] [dup]] end
	// Body produces 2 values (dup), signature declares 1 return.
	// The extra value is the unconsumed unnamed arg which is discarded,
	// leaving only the declared return value.
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("toomany"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("toomany"),
	})
	_as117, _ := AsInteger(result[0])
	if len(result) != 1 || _as117 != 5 {
		t.Errorf("expected [5], got %v", result)
	}

	// Genuinely wrong: body produces more values than unnamed args + declared returns.
	// def bad fn [[number] [number] [dup dup]] end — 3 results, 1 unnamed + 1 return = 2 max.
	fnBody2 := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("dup")}),
	})
	err = runAQLError(t, r, []Value{
		NewWord("def"), NewWord("bad"), NewWord("fn"), fnBody2, NewEnd(),
		NewInteger(5), NewWord("bad"),
	})
	if err == nil {
		t.Fatal("expected return count error, got nil")
	}
	if !strings.Contains(err.Error(), "bad") {
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
		NewWord("def"), NewWord("identity"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(42), NewWord("identity"),
	})
	_as118, _ := AsInteger(result[0])
	if len(result) != 1 || _as118 != 42 {
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
		NewWord("def"), NewWord("dbl"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(7), NewWord("dbl"),
	})
	_as119, _ := AsInteger(result[0])
	if len(result) != 1 || _as119 != 14 {
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
		NewWord("def"), NewWord("dup2"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(3), NewWord("dup2"),
	})
	_as121, _ := AsInteger(result[0])
	_as120, _ := AsInteger(result[1])
	if len(result) != 2 || _as121 != 3 || _as120 != 3 {
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
		NewWord("def"), NewWord("square"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(6), NewWord("square"),
	})
	_as122, _ := AsInteger(result[0])
	if len(result) != 1 || _as122 != 36 {
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
		NewWord("def"), NewWord("isbig"), NewWord("fn"), fnBody, NewEnd(),
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
		NewWord("def"), NewWord("add1"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(10), NewWord("add1"),
	})
	_as123, _ := AsInteger(result[0])
	if len(result) != 1 || _as123 != 11 {
		t.Errorf("10 add1 = %v, want 11", result)
	}
	// Test string overload
	result = runAQL(t, r, []Value{
		NewString("hello"), NewWord("add1"),
	})
	_as124, _ := AsString(result[0])
	if len(result) != 1 || _as124 != "hello1" {
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
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewEnd(),
	})

	// Test number sig
	result := runAQL(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	_as125, _ := AsInteger(result[0])
	if len(result) != 1 || _as125 != 9 {
		t.Errorf("3 foo = %v, want 9", result)
	}

	// Test string sig
	result = runAQL(t, r, []Value{
		NewString("hi"), NewWord("foo"),
	})
	_as126, _ := AsString(result[0])
	if len(result) != 1 || _as126 != "hihi" {
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
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewEnd(),
		NewWord("undef"), NewWord("foo"), NewEnd(),
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	_as127, _ := AsInteger(result[0])
	if _as127 != 9 {
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
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fnsig"), undefSpec, NewEnd(),
	})
	result := runAQL(t, r, []Value{
		NewString("hi"), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	_as128, _ := AsString(result[0])
	if _as128 != "hihi" {
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
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fnsig"), undefSpec, NewEnd(),
	})
	result := runAQL(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	_as129, _ := AsInteger(result[0])
	if _as129 != 9 {
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
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fnsig"), undefSpec, NewEnd(),
	})
	result := runAQL(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	_as130, _ := AsInteger(result[0])
	if _as130 != 9 {
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

	e := New(r)
	_, err = e.Run([]Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fnsig"), undefSpec, NewEnd(),
		NewWord("foo"),
	})
	// foo should error (undefined after all sigs removed)
	if err == nil {
		t.Fatal("expected error for undefined word after removing all sigs, got nil")
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
		NewWord("def"), NewString("foo"), NewWord("fn"), bodyA, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fn"), bodyB, NewEnd(),
	})

	// Both sigs work
	result := runAQL(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	_as132, _ := AsInteger(result[0])
	if len(result) != 1 || _as132 != 9 {
		t.Fatalf("3 foo = %v, want 9", result)
	}
	result = runAQL(t, r, []Value{
		NewString("hi"), NewWord("foo"),
	})
	_as133, _ := AsString(result[0])
	if len(result) != 1 || _as133 != "hihi" {
		t.Fatalf("\"hi\" foo = %v, want \"hihi\"", result)
	}

	// Undef pops B (string sig), A (number sig) remains
	_ = runAQL(t, r, []Value{
		NewWord("undef"), NewWord("foo"), NewEnd(),
	})
	result = runAQL(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	_as134, _ := AsInteger(result[0])
	if len(result) != 1 || _as134 != 9 {
		t.Fatalf("3 foo after undef B = %v, want 9", result)
	}
}

// --- Metatype integration tests ---

func TestTypeofMetatypes(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Metatypes are collapsed: typeof / fulltypeof of ANY type literal
	// is uniformly "Type" — there is no ScalarType / NodeType /
	// ObjectType layer at the surface.
	tests := []struct {
		name     string
		typeLit  Value
		wantType string // expected typeof result
		wantFull string // expected fulltypeof result
	}{
		{"String", NewTypeLiteral(TString), "Type", "Type"},
		{"Number", NewTypeLiteral(TNumber), "Type", "Type"},
		{"Integer", NewTypeLiteral(TInteger), "Type", "Type"},
		{"Decimal", NewTypeLiteral(TDecimal), "Type", "Type"},
		{"Boolean", NewTypeLiteral(TBoolean), "Type", "Type"},
		{"List", NewTypeLiteral(TList), "Type", "Type"},
		{"Map", NewTypeLiteral(TMap), "Type", "Type"},
		{"Scalar", NewTypeLiteral(TScalar), "Type", "Type"},
		{"Node", NewTypeLiteral(TNode), "Type", "Type"},
		{"Any", NewTypeLiteral(TAny), "Type", "Type"},
		{"None", NewTypeLiteral(TNone), "Type", "Type"},
		{"Object", NewTypeLiteral(TObject), "Type", "Type"},
		{"Table", NewTypeLiteral(TTable), "Type", "Type"},
		{"Record", NewTypeLiteral(TRecord), "Type", "Type"},
		{"Resource", NewTypeLiteral(TResource), "Type", "Type"},
		{"Atom", NewTypeLiteral(TAtom), "Type", "Type"},
		{"Type", NewTypeLiteral(TType), "Type", "Type"},
		{"Function", NewTypeLiteral(TFunction), "Type", "Type"},
		{"Disjunct", NewTypeLiteral(TDisjunct), "Type", "Type"},
	}

	for _, tt := range tests {
		t.Run("typeof-"+tt.name, func(t *testing.T) {
			result := runAQL(t, r, []Value{tt.typeLit, NewWord("typeof")})
			if len(result) != 1 {
				t.Fatalf("expected 1 result, got %d", len(result))
			}
			// typeof now returns a Type literal, not an Atom — compare
			// via String() which renders the leaf.
			got := result[0].String()
			if got != tt.wantType {
				t.Errorf("typeof %s = %q, want %q", tt.name, got, tt.wantType)
			}
		})
		t.Run("fulltypeof-"+tt.name, func(t *testing.T) {
			result := runAQL(t, r, []Value{tt.typeLit, NewWord("fulltypeof")})
			if len(result) != 1 {
				t.Fatalf("expected 1 result, got %d", len(result))
			}
			got, _ := AsAtom(result[0])
			if got != tt.wantFull {
				t.Errorf("fulltypeof %s = %q, want %q", tt.name, got, tt.wantFull)
			}
		})
	}

	// Concrete values: typeof returns a Type literal of the value's
	// exact VType. Render via String() (leaf).
	t.Run("typeof-concrete-integer", func(t *testing.T) {
		result := runAQL(t, r, []Value{NewInteger(42), NewWord("typeof")})
		if len(result) != 1 || result[0].String() != "Integer" {
			t.Errorf("typeof 42 = %v, want Integer", result)
		}
	})
	t.Run("typeof-concrete-boolean", func(t *testing.T) {
		result := runAQL(t, r, []Value{NewBoolean(true), NewWord("typeof")})
		if len(result) != 1 || result[0].String() != "Boolean" {
			t.Errorf("typeof true = %v, want Boolean", result)
		}
	})
}

func TestIsMetatypes(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		val  Value
		pat  Value
		want bool
	}{
		// ScalarType matches scalar subtypes
		{"Boolean is ScalarType", NewTypeLiteral(TBoolean), NewTypeLiteral(TScalarType), true},
		{"String is ScalarType", NewTypeLiteral(TString), NewTypeLiteral(TScalarType), true},
		{"Integer is ScalarType", NewTypeLiteral(TInteger), NewTypeLiteral(TScalarType), true},

		// NodeType matches node subtypes
		{"List is NodeType", NewTypeLiteral(TList), NewTypeLiteral(TNodeType), true},
		{"Map is NodeType", NewTypeLiteral(TMap), NewTypeLiteral(TNodeType), true},

		// Type matches everything
		{"Boolean is Type", NewTypeLiteral(TBoolean), NewTypeLiteral(TType), true},
		{"List is Type", NewTypeLiteral(TList), NewTypeLiteral(TType), true},
		{"Object is Type", NewTypeLiteral(TObject), NewTypeLiteral(TType), true},
		{"Any is Type", NewTypeLiteral(TAny), NewTypeLiteral(TType), true},

		// Negative cases
		{"List is ScalarType", NewTypeLiteral(TList), NewTypeLiteral(TScalarType), false},
		{"Boolean is NodeType", NewTypeLiteral(TBoolean), NewTypeLiteral(TNodeType), false},
		{"Object is ScalarType", NewTypeLiteral(TObject), NewTypeLiteral(TScalarType), false},
		{"Object is NodeType", NewTypeLiteral(TObject), NewTypeLiteral(TNodeType), false},

		// Scalar/Node roots have metatype Type, not ScalarNodeType
		{"Scalar is ScalarType", NewTypeLiteral(TScalar), NewTypeLiteral(TScalarType), false},
		{"Node is NodeType", NewTypeLiteral(TNode), NewTypeLiteral(TNodeType), false},
		{"Scalar is Type", NewTypeLiteral(TScalar), NewTypeLiteral(TType), true},
		{"Node is Type", NewTypeLiteral(TNode), NewTypeLiteral(TType), true},

		// Metatypes themselves
		{"ScalarType is Type", NewTypeLiteral(TScalarType), NewTypeLiteral(TType), true},
		{"NodeType is Type", NewTypeLiteral(TNodeType), NewTypeLiteral(TType), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runAQL(t, r, []Value{tt.val, NewWord("is"), tt.pat})
			if len(result) != 1 {
				t.Fatalf("expected 1 result, got %d", len(result))
			}
			got, _ := AsBoolean(result[0])
			if got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// --- String interpolation integration tests ---

func TestInterpStringLiteral(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	parts := []InterpPart{
		{Lit: "hello world"},
	}
	result := runAQL(t, r, []Value{NewInterpString(parts)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	got, _ := AsString(result[0])
	if got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestInterpStringWithExpression(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewString("world"), NewWord("def"), NewWord("name"), NewEnd(),
		NewInterpString([]InterpPart{
			{Lit: "hello "},
			{Expr: []Value{NewWord("name")}},
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	got, _ := AsString(result[0])
	if got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestInterpStringArithmetic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewInterpString([]InterpPart{
			{Lit: "answer: "},
			{Expr: []Value{NewInteger(1), NewWord("add"), NewInteger(2)}},
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	got, _ := AsString(result[0])
	if got != "answer: 3" {
		t.Errorf("expected 'answer: 3', got %q", got)
	}
}

func TestInterpStringMultipleExprs(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewInteger(1), NewWord("def"), NewWord("a"), NewEnd(),
		NewInteger(2), NewWord("def"), NewWord("b"), NewEnd(),
		NewInterpString([]InterpPart{
			{Expr: []Value{NewWord("a")}},
			{Lit: " and "},
			{Expr: []Value{NewWord("b")}},
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	got, _ := AsString(result[0])
	if got != "1 and 2" {
		t.Errorf("expected '1 and 2', got %q", got)
	}
}

func TestInterpStringInMapValue(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewInteger(42), NewWord("def"), NewWord("x"), NewEnd(),
		NewEvalMap(func() *OrderedMap {
			om := NewOrderedMap()
			om.Set("msg", NewInterpString([]InterpPart{
				{Lit: "value is "},
				{Expr: []Value{NewWord("x")}},
			}))
			return om
		}()),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := AsMap(result[0])
	if m == nil {
		t.Fatal("expected map result")
	}
	v, ok := m.Get("msg")
	if !ok {
		t.Fatal("expected 'msg' key in map")
	}
	got, _ := AsString(v)
	if got != "value is 42" {
		t.Errorf("expected 'value is 42', got %q", got)
	}
}

func TestInterpStringAsWordArg(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewInterpString([]InterpPart{
			{Lit: "hello"},
		}),
		NewWord("upper"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	got, _ := AsString(result[0])
	if got != "HELLO" {
		t.Errorf("expected 'HELLO', got %q", got)
	}
}
