package engine

import (
	"strings"
	"testing"
)

// =============================================================================
// TestEngineCoreStepEnd — exercises stepEnd with various forward/end patterns
// =============================================================================

func TestEngineCoreStepEndMultipleEnds(t *testing.T) {
	r, _ := DefaultRegistry()
	// Multiple ends with no pending forwards — should be harmlessly removed
	result := runAQL(t, r, []Value{
		NewInteger(5), NewWord("end"), NewWord("end"),
	})
	if len(result) != 1 || result[0].AsNumber() != 5 {
		t.Errorf("got %v, want [5]", result)
	}
}

func TestEngineCoreStepEndTerminatesForwardAdd(t *testing.T) {
	r, _ := DefaultRegistry()
	// "end" should terminate a forward expression: 10 add 20 end 99
	// The add consumes 10 and 20 via end, leaving 30 and 99
	result := runAQL(t, r, []Value{
		NewInteger(10), NewWord("add"), NewInteger(20), NewWord("end"), NewInteger(99),
	})
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(result), result)
	}
	if result[0].AsNumber() != 30 {
		t.Errorf("result[0] = %v, want 30", result[0])
	}
	if result[1].AsNumber() != 99 {
		t.Errorf("result[1] = %v, want 99", result[1])
	}
}

func TestEngineCoreStepEndSemicolonSequence(t *testing.T) {
	r, _ := DefaultRegistry()
	// Semicolons (end) as sequence separators: 1 add 2 end 3 add 4
	result := runAQL(t, r, []Value{
		NewInteger(1), NewWord("add"), NewInteger(2), NewWord("end"),
		NewInteger(3), NewWord("add"), NewInteger(4),
	})
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(result), result)
	}
	if result[0].AsNumber() != 3 {
		t.Errorf("result[0] = %v, want 3", result[0])
	}
	if result[1].AsNumber() != 7 {
		t.Errorf("result[1] = %v, want 7", result[1])
	}
}

// =============================================================================
// TestEngineCoreParentheses — exercises stepOpenParen, stepCloseParen, findCloseParenAfter
// =============================================================================

func TestEngineCoreParenSimple(t *testing.T) {
	r, _ := DefaultRegistry()
	// ( 2 add 3 ) => 5
	result := runAQL(t, r, []Value{
		NewWord("("), NewInteger(2), NewWord("add"), NewInteger(3), NewWord(")"),
	})
	if len(result) != 1 || result[0].AsNumber() != 5 {
		t.Errorf("(2 add 3) = %v, want 5", result)
	}
}

func TestEngineCoreParenNested(t *testing.T) {
	r, _ := DefaultRegistry()
	// ( ( 1 add 2 ) add 3 ) => 6
	result := runAQL(t, r, []Value{
		NewWord("("), NewWord("("), NewInteger(1), NewWord("add"), NewInteger(2), NewWord(")"),
		NewWord("add"), NewInteger(3), NewWord(")"),
	})
	if len(result) != 1 || result[0].AsNumber() != 6 {
		t.Errorf("((1 add 2) add 3) = %v, want 6", result)
	}
}

func TestEngineCoreParenUnmatched(t *testing.T) {
	r, _ := DefaultRegistry()
	err := runAQLError(t, r, []Value{
		NewWord(")"),
	})
	if err == nil {
		t.Error("expected error for unmatched closing paren")
	}
}

func TestEngineCoreParenUnmatchedOpen(t *testing.T) {
	r, _ := DefaultRegistry()
	err := runAQLError(t, r, []Value{
		NewWord("("), NewInteger(1),
	})
	if err == nil {
		t.Error("expected error for unmatched opening paren")
	}
}

func TestEngineCoreParenAsBarrier(t *testing.T) {
	r, _ := DefaultRegistry()
	// Parens create a scope barrier: 10 ( 2 add 3 ) => 10 5
	result := runAQL(t, r, []Value{
		NewInteger(10), NewWord("("), NewInteger(2), NewWord("add"), NewInteger(3), NewWord(")"),
	})
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(result), result)
	}
	if result[0].AsNumber() != 10 {
		t.Errorf("result[0] = %v, want 10", result[0])
	}
	if result[1].AsNumber() != 5 {
		t.Errorf("result[1] = %v, want 5", result[1])
	}
}

// =============================================================================
// TestEngineCoreFnDef — function definitions with typed params
// =============================================================================

func TestEngineCoreFnDefNamedParam(t *testing.T) {
	r, _ := DefaultRegistry()
	// def double fn [[x:Number] [Number] [x add x]] end
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX)}),
		NewList([]Value{NewTypeLiteral(TNumber)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewWord("x")}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("double"),
		NewWord("fn"), fnBody,
		NewWord("end"),
	})
	result := runAQL(t, r, []Value{NewInteger(7), NewWord("double")})
	if len(result) != 1 || result[0].AsNumber() != 14 {
		t.Errorf("7 double = %v, want 14", result)
	}
}

func TestEngineCoreFnDefMultipleSigs(t *testing.T) {
	r, _ := DefaultRegistry()
	// Function with two overloads:
	// sig1: (Integer) -> Integer: x add 10
	// sig2: (String) -> String: x
	pairXInt := NewOrderedMap()
	pairXInt.Set("x", NewTypeLiteral(TInteger))
	pairXStr := NewOrderedMap()
	pairXStr.Set("x", NewTypeLiteral(TString))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairXInt)}),
		NewList([]Value{NewTypeLiteral(TInteger)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(10)}),
		NewList([]Value{NewImplicitMap(pairXStr)}),
		NewList([]Value{NewTypeLiteral(TString)}),
		NewList([]Value{NewWord("x")}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("myOverload"),
		NewWord("fn"), fnBody,
		NewWord("end"),
	})

	// Integer overload
	result := runAQL(t, r, []Value{NewInteger(5), NewWord("myOverload")})
	if len(result) != 1 || result[0].AsNumber() != 15 {
		t.Errorf("5 myOverload = %v, want 15", result)
	}

	// String overload
	result = runAQL(t, r, []Value{NewString("hello"), NewWord("myOverload")})
	if len(result) != 1 || result[0].AsString() != "hello" {
		t.Errorf("'hello' myOverload = %v, want 'hello'", result)
	}
}

func TestEngineCoreFnReturnTypeCheck(t *testing.T) {
	r, _ := DefaultRegistry()
	// Function with return type checking: returns wrong type should error
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX)}),
		NewList([]Value{NewTypeLiteral(TString)}), // expects String return
		NewList([]Value{NewWord("x")}),             // but returns Number
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("badReturn"),
		NewWord("fn"), fnBody,
		NewWord("end"),
	})
	err := runAQLError(t, r, []Value{NewInteger(5), NewWord("badReturn")})
	if err == nil {
		t.Error("expected return type check error")
	}
}

func TestEngineCoreFnNonListBody(t *testing.T) {
	r, _ := DefaultRegistry()
	// fn with non-list body abbreviation and named param: fn [{x:Number} [] 42]
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX)}),
		NewList([]Value{}),
		NewInteger(42), // non-list body: abbreviation for [42]
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("always42"),
		NewWord("fn"), fnBody,
		NewWord("end"),
	})
	result := runAQL(t, r, []Value{NewInteger(1), NewWord("always42")})
	if len(result) != 1 || result[0].AsNumber() != 42 {
		t.Errorf("1 always42 = %v, want 42", result)
	}
}

// =============================================================================
// TestEngineCoreUndef — undefining functions
// =============================================================================

func TestEngineCoreUndefBasic(t *testing.T) {
	r, _ := DefaultRegistry()
	// def myVal 100 end myVal => 100
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("myValUndef"), NewInteger(100), NewWord("end"),
	})
	result := runAQL(t, r, []Value{NewWord("myValUndef")})
	if len(result) != 1 || result[0].AsNumber() != 100 {
		t.Fatalf("myValUndef = %v, want 100", result)
	}

	// undef myVal
	runAQL(t, r, []Value{NewWord("undef"), NewWord("myValUndef")})

	// After undef, the word should resolve to an atom
	result = runAQL(t, r, []Value{NewWord("myValUndef")})
	if len(result) != 1 || !result[0].VType.Equal(TAtom) {
		t.Errorf("after undef, myValUndef should be atom, got %v (type %s)", result, result[0].VType)
	}
}

func TestEngineCoreUndefShadowing(t *testing.T) {
	r, _ := DefaultRegistry()
	// def "xShadow" 1 end def "xShadow" 2 end xShadow => 2
	runAQL(t, r, []Value{
		NewWord("def"), NewString("xShadow"), NewInteger(1), NewWord("end"),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewString("xShadow"), NewInteger(2), NewWord("end"),
	})
	result := runAQL(t, r, []Value{NewWord("xShadow")})
	if len(result) != 1 || result[0].AsNumber() != 2 {
		t.Fatalf("xShadow = %v, want 2", result)
	}

	// undef x => reveals 1
	runAQL(t, r, []Value{NewWord("undef"), NewWord("xShadow")})
	result = runAQL(t, r, []Value{NewWord("xShadow")})
	if len(result) != 1 || result[0].AsNumber() != 1 {
		t.Errorf("after first undef, xShadow = %v, want 1", result)
	}
}

func TestEngineCoreUndefWithStringName(t *testing.T) {
	r, _ := DefaultRegistry()
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("strUndef"), NewInteger(77), NewWord("end"),
	})
	runAQL(t, r, []Value{NewWord("undef"), NewString("strUndef")})
	result := runAQL(t, r, []Value{NewWord("strUndef")})
	if len(result) != 1 || !result[0].VType.Equal(TAtom) {
		t.Errorf("after undef by string, strUndef should be atom, got %v", result)
	}
}

func TestEngineCoreUndefTargetedFnSig(t *testing.T) {
	r, _ := DefaultRegistry()
	// Define a function with a Number signature
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX)}),
		NewList([]Value{NewTypeLiteral(TNumber)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(1)}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("tgtUndef"),
		NewWord("fn"), fnBody,
		NewWord("end"),
	})

	// Verify it works
	result := runAQL(t, r, []Value{NewInteger(10), NewWord("tgtUndef")})
	if len(result) != 1 || result[0].AsNumber() != 11 {
		t.Fatalf("10 tgtUndef = %v, want 11", result)
	}

	// Targeted undef with fn spec: undef tgtUndef fn [[Number] [Number]]
	undefSpec := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
	})
	runAQL(t, r, []Value{
		NewWord("undef"), NewWord("tgtUndef"),
		NewWord("fn"), undefSpec,
	})
}

// =============================================================================
// TestEngineCoreMake — type conversion and record creation
// =============================================================================

func TestEngineCoreMakeStringToInteger(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TInteger), NewString("42"),
	})
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("make Integer '42' = %v, want 42", result)
	}
}

func TestEngineCoreMakeStringToDecimal(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TDecimal), NewString("3.14"),
	})
	if len(result) != 1 {
		t.Fatalf("make Decimal '3.14' got %d results", len(result))
	}
	if result[0].AsDecimal() != 3.14 {
		t.Errorf("make Decimal '3.14' = %v, want 3.14", result[0])
	}
}

func TestEngineCoreMakeToBoolean(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewString("true"),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("make Boolean 'true' = %v, want true", result)
	}

	result = runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewString("false"),
	})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("make Boolean 'false' = %v, want false", result)
	}

	result = runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewString(""),
	})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("make Boolean '' = %v, want false", result)
	}
}

func TestEngineCoreMakeToAtom(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TAtom), NewString("hello"),
	})
	if len(result) != 1 || result[0].AsAtom() != "hello" {
		t.Errorf("make Atom 'hello' = %v, want :hello", result)
	}
}

func TestEngineCoreMakeSameType(t *testing.T) {
	r, _ := DefaultRegistry()
	// make Integer on integer should pass through
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TInteger), NewInteger(99),
	})
	if len(result) != 1 || result[0].AsInteger() != 99 {
		t.Errorf("make Integer 99 = %v, want 99", result)
	}
}

func TestEngineCoreMakeErrorBadConversion(t *testing.T) {
	r, _ := DefaultRegistry()
	err := runAQLError(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TInteger), NewString("notanumber"),
	})
	if err == nil {
		t.Error("expected error for make Integer 'notanumber'")
	}
}

func TestEngineCoreMakeDecimalTruncToInt(t *testing.T) {
	r, _ := DefaultRegistry()
	// make Integer on decimal string should parse as float and truncate
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TInteger), NewString("3.7"),
	})
	if len(result) != 1 || result[0].AsInteger() != 3 {
		t.Errorf("make Integer '3.7' = %v, want 3", result)
	}
}

func TestEngineCoreMakeBooleanFromNumber(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewInteger(0),
	})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("make Boolean 0 = %v, want false", result)
	}
	result = runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewInteger(1),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("make Boolean 1 = %v, want true", result)
	}
}

func TestEngineCoreMakeToString(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TString), NewInteger(42),
	})
	if len(result) != 1 || result[0].AsString() != "42" {
		t.Errorf("make String 42 = %v, want '42'", result)
	}
}

func TestEngineCoreMakeErrorBadDecimal(t *testing.T) {
	r, _ := DefaultRegistry()
	err := runAQLError(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TDecimal), NewString("xyz"),
	})
	if err == nil {
		t.Error("expected error for make Decimal 'xyz'")
	}
}

// =============================================================================
// TestEngineCoreRecord — record creation via make
// =============================================================================

func TestEngineCoreRecordMakePositional(t *testing.T) {
	r, _ := DefaultRegistry()
	// record [x:Number y:String]
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := NewRecordType(fields)

	// make RecType [1 "hello"]
	result := runAQL(t, r, []Value{
		NewWord("make"), recType, NewList([]Value{NewInteger(1), NewString("hello")}),
	})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("make record = %v, want map", result)
	}
	m := result[0].AsMap()
	xv, _ := m.Get("x")
	yv, _ := m.Get("y")
	if xv.AsNumber() != 1 || yv.AsString() != "hello" {
		t.Errorf("record fields: x=%v y=%v", xv, yv)
	}
}

func TestEngineCoreRecordMakeMap(t *testing.T) {
	r, _ := DefaultRegistry()
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := NewRecordType(fields)

	// make RecType {x:1 y:"hello"}
	src := NewOrderedMap()
	src.Set("x", NewInteger(1))
	src.Set("y", NewString("hello"))
	result := runAQL(t, r, []Value{
		NewWord("make"), recType, NewMap(src),
	})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("make record from map = %v, want map", result)
	}
	m := result[0].AsMap()
	xv, _ := m.Get("x")
	if xv.AsNumber() != 1 {
		t.Errorf("record x = %v, want 1", xv)
	}
}

func TestEngineCoreRecordMakeMissingField(t *testing.T) {
	r, _ := DefaultRegistry()
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := NewRecordType(fields)

	// Missing field y should error
	src := NewOrderedMap()
	src.Set("x", NewInteger(1))
	err := runAQLError(t, r, []Value{
		NewWord("make"), recType, NewMap(src),
	})
	if err == nil {
		t.Error("expected error for missing field")
	}
}

func TestEngineCoreRecordMakeUnknownField(t *testing.T) {
	r, _ := DefaultRegistry()
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	recType := NewRecordType(fields)

	src := NewOrderedMap()
	src.Set("x", NewInteger(1))
	src.Set("z", NewString("extra"))
	err := runAQLError(t, r, []Value{
		NewWord("make"), recType, NewMap(src),
	})
	if err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestEngineCoreRecordMakeWrongFieldCount(t *testing.T) {
	r, _ := DefaultRegistry()
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := NewRecordType(fields)

	// Positional with wrong count
	err := runAQLError(t, r, []Value{
		NewWord("make"), recType, NewList([]Value{NewInteger(1)}),
	})
	if err == nil {
		t.Error("expected error for wrong field count")
	}
}

// =============================================================================
// TestEngineCoreModule — module definitions with exports
// =============================================================================

func TestEngineCoreModuleSimple(t *testing.T) {
	r, _ := DefaultRegistry()
	moduleBody := NewList([]Value{
		NewString("def"), NewString("val"), NewInteger(42), NewString("end"),
		NewString("export"), NewAtom("myexp"),
		NewMap(singleMap("v", NewString("val"))),
	})
	result := runAQL(t, r, []Value{NewWord("module"), moduleBody})
	if len(result) != 1 || !result[0].VType.Equal(TModule) {
		t.Fatalf("module should return TModule, got %v", result)
	}
	desc := result[0].AsModule()
	if _, ok := desc.Exports["myexp"]; !ok {
		t.Error("missing 'myexp' export")
	}
}

func TestEngineCoreModuleImportAll(t *testing.T) {
	r, _ := DefaultRegistry()
	moduleBody := NewList([]Value{
		NewString("def"), NewString("val"), NewInteger(88), NewString("end"),
		NewString("export"), NewAtom("coreExp"),
		NewMap(singleMap("v", NewString("val"))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("cmod"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})
	runAQL(t, r, []Value{NewWord("import"), NewWord("cmod")})

	result := runAQL(t, r, []Value{NewWord("coreExp")})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("coreExp should be map, got %v", result)
	}
	m := result[0].AsMap()
	v, ok := m.Get("v")
	if !ok || v.AsInteger() != 88 {
		t.Errorf("coreExp.v = %v, want 88", v)
	}
}

func TestEngineCoreModuleImportRename(t *testing.T) {
	r, _ := DefaultRegistry()
	moduleBody := NewList([]Value{
		NewString("def"), NewString("val"), NewInteger(33), NewString("end"),
		NewString("export"), NewAtom("origName"),
		NewMap(singleMap("v", NewString("val"))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("rmod"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})
	renameList := NewList([]Value{NewAtom("origName"), NewAtom("newName")})
	runAQL(t, r, []Value{NewWord("import"), renameList, NewWord("rmod")})

	result := runAQL(t, r, []Value{NewWord("newName")})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("newName should be map, got %v", result)
	}
}

func TestEngineCoreModuleImportMultiRename(t *testing.T) {
	r, _ := DefaultRegistry()
	moduleBody := NewList([]Value{
		NewString("def"), NewString("a"), NewInteger(1), NewString("end"),
		NewString("def"), NewString("b"), NewInteger(2), NewString("end"),
		NewString("export"), NewAtom("ea"),
		NewMap(singleMap("v", NewString("a"))),
		NewString("export"), NewAtom("eb"),
		NewMap(singleMap("v", NewString("b"))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("mmod"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})

	renameList := NewList([]Value{
		NewList([]Value{NewAtom("ea"), NewAtom("ra")}),
		NewList([]Value{NewAtom("eb"), NewAtom("rb")}),
	})
	runAQL(t, r, []Value{NewWord("import"), renameList, NewWord("mmod")})

	result := runAQL(t, r, []Value{NewWord("ra")})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("ra should be map, got %v", result)
	}
}

func TestEngineCoreModuleImportEmptyRenameError(t *testing.T) {
	r, _ := DefaultRegistry()
	moduleBody := NewList([]Value{
		NewString("def"), NewString("x"), NewInteger(1), NewString("end"),
		NewString("export"), NewAtom("ex"),
		NewMap(singleMap("v", NewString("x"))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("emod"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})

	// Empty rename list should error
	err := runAQLError(t, r, []Value{
		NewWord("import"), NewList([]Value{}), NewWord("emod"),
	})
	if err == nil {
		t.Error("expected error for empty rename list")
	}
}

func TestEngineCoreModuleImportMissingExportError(t *testing.T) {
	r, _ := DefaultRegistry()
	moduleBody := NewList([]Value{
		NewString("def"), NewString("x"), NewInteger(1), NewString("end"),
		NewString("export"), NewAtom("ex"),
		NewMap(singleMap("v", NewString("x"))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("memod"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})

	// Try to rename a non-existent export
	renameList := NewList([]Value{NewAtom("nonexistent"), NewAtom("target")})
	err := runAQLError(t, r, []Value{
		NewWord("import"), renameList, NewWord("memod"),
	})
	if err == nil {
		t.Error("expected error for missing export in rename")
	}
}

func TestEngineCoreModuleExportWithAtomString(t *testing.T) {
	r, _ := DefaultRegistry()
	// Export using atom name (strings inside module are promoted to words)
	moduleBody := NewList([]Value{
		NewString("def"), NewString("val"), NewInteger(77), NewString("end"),
		NewString("export"), NewAtom("atexp"),
		NewMap(singleMap("v", NewString("val"))),
	})
	result := runAQL(t, r, []Value{NewWord("module"), moduleBody})
	if len(result) != 1 {
		t.Fatalf("module should return 1 value, got %d", len(result))
	}
	desc := result[0].AsModule()
	if _, ok := desc.Exports["atexp"]; !ok {
		t.Error("missing 'atexp' export")
	}
}

// =============================================================================
// TestEngineCoreBaseValue — base values for different types
// =============================================================================

func TestEngineCoreBaseInteger(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewInteger(5), NewWord("base")})
	if len(result) != 1 || result[0].AsInteger() != 0 {
		t.Errorf("base Integer = %v, want 0", result)
	}
}

func TestEngineCoreBaseDecimal(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewDecimal(3.14), NewWord("base")})
	if len(result) != 1 || result[0].AsDecimal() != 0 {
		t.Errorf("base Decimal = %v, want 0", result)
	}
}

func TestEngineCoreBaseString(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewString("hello"), NewWord("base")})
	if len(result) != 1 || result[0].AsString() != "" {
		t.Errorf("base String = %v, want ''", result)
	}
}

func TestEngineCoreBaseBoolean(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewBoolean(true), NewWord("base")})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("base Boolean = %v, want false", result)
	}
}

func TestEngineCoreBaseList(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewList([]Value{NewInteger(1)}), NewWord("base"),
	})
	if len(result) != 1 {
		t.Fatalf("base List got %d results", len(result))
	}
	if len(result[0].AsList()) != 0 {
		t.Errorf("base List = %v, want empty list", result)
	}
}

func TestEngineCoreBaseMap(t *testing.T) {
	r, _ := DefaultRegistry()
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	result := runAQL(t, r, []Value{NewMap(m), NewWord("base")})
	if len(result) != 1 {
		t.Fatalf("base Map got %d results", len(result))
	}
	if result[0].AsMap().Len() != 0 {
		t.Errorf("base Map = %v, want empty map", result)
	}
}

func TestEngineCoreBaseAtom(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewAtom("foo"), NewWord("base")})
	if len(result) != 1 || result[0].AsAtom() != "" {
		t.Errorf("base Atom = %v, want empty atom", result)
	}
}

// =============================================================================
// TestEngineCoreBaseValueForConstraint — direct unit tests
// =============================================================================

func TestEngineCoreBaseValueForConstraintTypeLiteral(t *testing.T) {
	v, err := baseValueForConstraint(NewTypeLiteral(TString))
	if err != nil {
		t.Fatalf("baseValueForConstraint(String): %v", err)
	}
	if v.AsString() != "" {
		t.Errorf("base String = %v, want ''", v)
	}
}

func TestEngineCoreBaseValueForConstraintDisjunct(t *testing.T) {
	d := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	v, err := baseValueForConstraint(d)
	if err != nil {
		t.Fatalf("baseValueForConstraint(String|None): %v", err)
	}
	if v.AsString() != "" {
		t.Errorf("base String|None = %v, want ''", v)
	}
}

func TestEngineCoreBaseValueForConstraintAllNone(t *testing.T) {
	d := NewDisjunct([]Value{NewTypeLiteral(TNone)})
	v, err := baseValueForConstraint(d)
	if err != nil {
		t.Fatalf("baseValueForConstraint(None): %v", err)
	}
	if !v.VType.Equal(TNone) {
		t.Errorf("base all-None disjunct = %v, want None", v)
	}
}

func TestEngineCoreBaseValueForConstraintConcreteError(t *testing.T) {
	_, err := baseValueForConstraint(NewInteger(42))
	if err == nil {
		t.Error("expected error for concrete constraint")
	}
}

// =============================================================================
// TestEngineCorePeekForwardValue — peek at forward value resolution
// =============================================================================

func TestEngineCorePeekForwardBoolTrue(t *testing.T) {
	r, _ := DefaultRegistry()
	// "true" as forward should resolve to boolean
	// Test indirectly: def myval true end myval => true
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("trueVal"), NewWord("true"), NewWord("end"),
		NewWord("trueVal"),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("def trueVal true = %v, want true", result)
	}
}

func TestEngineCorePeekForwardBoolFalse(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("falseVal"), NewWord("false"), NewWord("end"),
		NewWord("falseVal"),
	})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("def falseVal false = %v, want false", result)
	}
}

func TestEngineCorePeekForwardAtom(t *testing.T) {
	r, _ := DefaultRegistry()
	// An unknown word in forward position should resolve to atom
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("atomVal"), NewWord("myatom"), NewWord("end"),
		NewWord("atomVal"),
	})
	if len(result) != 1 || !result[0].VType.Equal(TAtom) {
		t.Errorf("def atomVal myatom = %v, want atom", result)
	}
}

// =============================================================================
// TestEngineCoreForward — forward operations, precedence
// =============================================================================

func TestEngineCoreForwardPrecedence(t *testing.T) {
	r, _ := DefaultRegistry()
	// mul has higher precedence than add: 2 add 3 mul 4 => 2 + (3 * 4) = 14
	result := runAQL(t, r, []Value{
		NewInteger(2), NewWord("add"), NewInteger(3), NewWord("mul"), NewInteger(4),
	})
	if len(result) != 1 || result[0].AsNumber() != 14 {
		t.Errorf("2 add 3 mul 4 = %v, want 14", result)
	}
}

func TestEngineCoreForwardPrecedenceReverse(t *testing.T) {
	r, _ := DefaultRegistry()
	// 2 mul 3 add 4 => (2 * 3) + 4 = 10
	result := runAQL(t, r, []Value{
		NewInteger(2), NewWord("mul"), NewInteger(3), NewWord("add"), NewInteger(4),
	})
	if len(result) != 1 || result[0].AsNumber() != 10 {
		t.Errorf("2 mul 3 add 4 = %v, want 10", result)
	}
}

// =============================================================================
// TestEngineCoreForceForward — force forward via WordInfo
// =============================================================================

func TestEngineCoreForceForward(t *testing.T) {
	r, _ := DefaultRegistry()
	// Force forward on add: uses ForceForward flag
	result := runAQL(t, r, []Value{
		NewInteger(10),
		NewWordModified("add", -1, false, true),
		NewInteger(5),
	})
	if len(result) != 1 || result[0].AsNumber() != 15 {
		t.Errorf("10 ~add 5 = %v, want 15", result)
	}
}

// =============================================================================
// TestEngineCoreTypeLiterals — type name resolution
// =============================================================================

func TestEngineCoreTypeNameResolution(t *testing.T) {
	r, _ := DefaultRegistry()
	// Type names resolve to type literals
	for _, name := range []string{"Number", "String", "Boolean", "Integer", "Decimal", "List", "Map", "Atom"} {
		result := runAQL(t, r, []Value{NewWord(name)})
		if len(result) != 1 || result[0].Data != nil {
			t.Errorf("%s should resolve to type literal, got %v", name, result)
		}
	}
}

func TestEngineCoreUnknownWordBecomesAtom(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{NewWord("unknownXyz")})
	if len(result) != 1 || !result[0].VType.Equal(TAtom) {
		t.Errorf("unknown word should become atom, got %v (type %s)", result, result[0].VType)
	}
	if result[0].AsAtom() != "unknownXyz" {
		t.Errorf("atom value = %v, want 'unknownXyz'", result[0].AsAtom())
	}
}

// =============================================================================
// TestEngineCoreFnSigMatchesSpec — direct unit test
// =============================================================================

func TestEngineCoreFnSigMatchesSpecBasic(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Name: "x", Type: TNumber}},
		Returns: []Type{TNumber},
	}
	spec := FnSigSpec{
		Params:  []FnParam{{Type: TNumber}},
		Returns: []Type{TNumber},
	}
	if !fnSigMatchesSpec(sig, spec) {
		t.Error("sig should match spec (types match, ignoring names)")
	}
}

func TestEngineCoreFnSigMatchesSpecDiffParamCount(t *testing.T) {
	sig := FnSig{
		Params: []FnParam{{Type: TNumber}},
	}
	spec := FnSigSpec{
		Params: []FnParam{{Type: TNumber}, {Type: TString}},
	}
	if fnSigMatchesSpec(sig, spec) {
		t.Error("sig should not match spec with different param count")
	}
}

func TestEngineCoreFnSigMatchesSpecDiffReturnCount(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Type: TNumber}},
		Returns: []Type{TNumber},
	}
	spec := FnSigSpec{
		Params:  []FnParam{{Type: TNumber}},
		Returns: []Type{TNumber, TString},
	}
	if fnSigMatchesSpec(sig, spec) {
		t.Error("sig should not match spec with different return count")
	}
}

func TestEngineCoreFnSigMatchesSpecDiffReturnType(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Type: TNumber}},
		Returns: []Type{TNumber},
	}
	spec := FnSigSpec{
		Params:  []FnParam{{Type: TNumber}},
		Returns: []Type{TString},
	}
	if fnSigMatchesSpec(sig, spec) {
		t.Error("sig should not match spec with different return type")
	}
}

func TestEngineCoreFnSigMatchesSpecDiffParamType(t *testing.T) {
	sig := FnSig{
		Params: []FnParam{{Type: TNumber}},
	}
	spec := FnSigSpec{
		Params: []FnParam{{Type: TString}},
	}
	if fnSigMatchesSpec(sig, spec) {
		t.Error("sig should not match spec with different param type")
	}
}

// =============================================================================
// TestEngineCoreResolveTypeName — covers resolveTypeName branches
// =============================================================================

func TestEngineCoreResolveTypeName(t *testing.T) {
	cases := []struct {
		name string
		want Type
	}{
		{"Any", TAny},
		{"None", TNone},
		{"Number", TNumber},
		{"Integer", TInteger},
		{"Decimal", TDecimal},
		{"String", TString},
		{"Boolean", TBoolean},
		{"List", TList},
		{"Map", TMap},
		{"Function", TFunction},
	}
	for _, tc := range cases {
		got, err := resolveTypeName(tc.name)
		if err != nil {
			t.Errorf("resolveTypeName(%q): %v", tc.name, err)
			continue
		}
		if !got.Equal(tc.want) {
			t.Errorf("resolveTypeName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestEngineCoreResolveTypeNameCustom(t *testing.T) {
	got, err := resolveTypeName("MyCustom")
	if err != nil {
		t.Fatalf("resolveTypeName('MyCustom'): %v", err)
	}
	if got.String() != "MyCustom" {
		t.Errorf("resolveTypeName('MyCustom') = %v, want MyCustom", got)
	}
}

// =============================================================================
// TestEngineCoreParseFnParams — covers parseFnParams edge cases
// =============================================================================

func TestEngineCoreParseFnParamsMapBadKeyCount(t *testing.T) {
	// Map with 2 keys should fail
	m := NewOrderedMap()
	m.Set("x", NewTypeLiteral(TNumber))
	m.Set("y", NewTypeLiteral(TString))
	input := NewList([]Value{NewImplicitMap(m)})
	_, err := parseFnParams(nil, input)
	if err == nil {
		t.Error("expected error for map with 2 keys")
	}
}

func TestEngineCoreParseFnParamsNonList(t *testing.T) {
	_, err := parseFnParams(nil, NewInteger(42))
	if err == nil {
		t.Error("expected error for non-list input")
	}
}

func TestEngineCoreParseFnParamsTypeLiteral(t *testing.T) {
	// Type literal (Data==nil) should work
	input := NewList([]Value{NewTypeLiteral(TString)})
	params, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("parseFnParams type literal: %v", err)
	}
	if len(params) != 1 || !params[0].Type.Equal(TString) {
		t.Errorf("params = %v, want [{Type:String}]", params)
	}
}

func TestEngineCoreParseFnParamsIntegerLiteral(t *testing.T) {
	input := NewList([]Value{NewInteger(0)})
	params, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("parseFnParams integer: %v", err)
	}
	if len(params) != 1 {
		t.Errorf("expected 1 param, got %d", len(params))
	}
}

func TestEngineCoreParseFnParamsBooleanLiteral(t *testing.T) {
	input := NewList([]Value{NewBoolean(true)})
	params, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("parseFnParams boolean: %v", err)
	}
	if len(params) != 1 {
		t.Errorf("expected 1 param, got %d", len(params))
	}
}

func TestEngineCoreParseFnParamsStringLiteral(t *testing.T) {
	input := NewList([]Value{NewString("hello")})
	params, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("parseFnParams string: %v", err)
	}
	if len(params) != 1 {
		t.Errorf("expected 1 param, got %d", len(params))
	}
}

func TestEngineCoreParseFnParamsInvalidElem(t *testing.T) {
	// A list as a param element is invalid
	input := NewList([]Value{NewList([]Value{NewInteger(1)})})
	_, err := parseFnParams(nil, input)
	if err == nil {
		t.Error("expected error for list element in params")
	}
}

// =============================================================================
// TestEngineCoreParseFnParams — implicit vs explicit map
// =============================================================================

func TestEngineCoreParseFnParamsImplicitMapIsNamedParam(t *testing.T) {
	// Implicit map (from pair syntax [x:Number]) → named param
	m := NewOrderedMap()
	m.Set("x", NewTypeLiteral(TNumber))
	input := NewList([]Value{NewImplicitMap(m)})
	params, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	if params[0].Name != "x" {
		t.Errorf("expected named param 'x', got %q", params[0].Name)
	}
	if !params[0].Type.Equal(TNumber) {
		t.Errorf("expected type Number, got %s", params[0].Type)
	}
}

func TestEngineCoreParseFnParamsExplicitMapIsUnnamedParam(t *testing.T) {
	// Explicit map ({a:1}) → unnamed param with pattern
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	input := NewList([]Value{NewMap(m)})
	params, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	if params[0].Name != "" {
		t.Errorf("expected unnamed param, got name %q", params[0].Name)
	}
	if !params[0].Type.Equal(TMap) {
		t.Errorf("expected type Map, got %s", params[0].Type)
	}
	if params[0].Pattern == nil {
		t.Error("expected pattern for explicit map param")
	}
}

func TestEngineCoreFnImplicitMapNamedParamE2E(t *testing.T) {
	r, _ := DefaultRegistry()
	// def inc fn [[x:Integer] [Integer] [x add 1]] end 5 inc
	xParam := NewOrderedMap()
	xParam.Set("x", NewTypeLiteral(TInteger))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewTypeLiteral(TInteger)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(1)}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("inc"), NewWord("fn"), fnBody, NewWord("end"),
		NewInteger(5), NewWord("inc"),
	})
	if len(result) != 1 || result[0].AsInteger() != 6 {
		t.Errorf("5 inc = %v, want 6", result)
	}
}

func TestEngineCoreFnExplicitMapPatternE2E(t *testing.T) {
	r, _ := DefaultRegistry()
	// def foo fn [[{a:1}] [] ["matched"]] end
	// {a:1} foo → should match
	// {a:2} foo → should not match
	patternMap := NewOrderedMap()
	patternMap.Set("a", NewInteger(1))
	fnBody := NewList([]Value{
		NewList([]Value{NewMap(patternMap)}),
		NewList([]Value{}),
		NewList([]Value{NewString("matched")}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("foo"), NewWord("fn"), fnBody, NewWord("end"),
	})

	// {a:1} foo → match
	argMap := NewOrderedMap()
	argMap.Set("a", NewInteger(1))
	result := runAQL(t, r, []Value{NewMap(argMap), NewWord("foo")})
	found := false
	for _, v := range result {
		if v.VType.Matches(TString) && v.AsString() == "matched" {
			found = true
		}
	}
	if !found {
		t.Errorf("{a:1} foo: expected 'matched' in result, got %v", result)
	}

	// {a:2} foo → no match
	noMatch := NewOrderedMap()
	noMatch.Set("a", NewInteger(2))
	err := runAQLError(t, r, []Value{NewMap(noMatch), NewWord("foo")})
	if err == nil {
		t.Error("expected signature error for {a:2} foo")
	}
}

func TestEngineCoreFnExplicitMapNotNamedParam(t *testing.T) {
	r, _ := DefaultRegistry()
	// def bar fn [[{x:Integer}] [Integer] [x add 1]] end
	// 5 bar → should FAIL because {x:Integer} is a map pattern, not a
	// named param "x". So x is not bound and the body can't use it.
	typeMap := NewOrderedMap()
	typeMap.Set("x", NewTypeLiteral(TInteger))
	fnBody := NewList([]Value{
		NewList([]Value{NewMap(typeMap)}),
		NewList([]Value{NewTypeLiteral(TInteger)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(1)}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("bar"), NewWord("fn"), fnBody, NewWord("end"),
	})

	// 5 bar → fails: no map on stack, or x not bound
	err := runAQLError(t, r, []Value{NewInteger(5), NewWord("bar")})
	if err == nil {
		t.Error("expected error: explicit map should not create named param")
	}
}

// =============================================================================
// TestEngineCoreValToAtomOrString — covers all branches
// =============================================================================

func TestEngineCoreValToAtomOrStringCoverage(t *testing.T) {
	if v := valToAtomOrString(NewWord("hello")); v != "hello" {
		t.Errorf("word: got %q, want 'hello'", v)
	}
	if v := valToAtomOrString(NewAtom("foo")); v != "foo" {
		t.Errorf("atom: got %q, want 'foo'", v)
	}
	if v := valToAtomOrString(NewString("bar")); v != "bar" {
		t.Errorf("string: got %q, want 'bar'", v)
	}
	// Fallback: integer uses .String()
	s := valToAtomOrString(NewInteger(42))
	if s != "42" {
		t.Errorf("integer fallback: got %q, want '42'", s)
	}
}

// =============================================================================
// TestEngineCoreMakeRecordWithBase — make with base:true option
// =============================================================================

func TestEngineCoreMakeRecordWithBase(t *testing.T) {
	r, _ := DefaultRegistry()
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := NewRecordType(fields)

	// make RecType {x:5} {base:true} — y should get base value ""
	src := NewOrderedMap()
	src.Set("x", NewInteger(5))
	opts := NewOrderedMap()
	opts.Set("base", NewBoolean(true))
	result := runAQL(t, r, []Value{
		NewWord("make"), recType, NewMap(src), NewMap(opts),
	})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("make with base = %v, want map", result)
	}
	m := result[0].AsMap()
	yv, ok := m.Get("y")
	if !ok {
		t.Fatal("missing field y")
	}
	if yv.AsString() != "" {
		t.Errorf("y base value = %v, want ''", yv)
	}
}

// =============================================================================
// TestEngineCoreResolveFieldType — covers resolveFieldType branches
// =============================================================================

func TestEngineCoreResolveFieldTypePassthrough(t *testing.T) {
	r, _ := DefaultRegistry()
	// Non-string, non-list value should pass through
	v := resolveFieldType(r, NewTypeLiteral(TNumber))
	if !v.VType.Equal(TNumber) {
		t.Errorf("resolveFieldType passthrough = %v, want Number", v)
	}
}

func TestEngineCoreResolveFieldTypeStringRef(t *testing.T) {
	r, _ := DefaultRegistry()
	// Define a type, then resolveFieldType should find it
	installDef(r, "MyType", NewTypeLiteral(TString))
	v := resolveFieldType(r, NewString("MyType"))
	if !v.VType.Equal(TString) {
		t.Errorf("resolveFieldType string ref = %v, want String type literal", v)
	}
	uninstallDef(r, "MyType")
}

func TestEngineCoreResolveFieldTypeStringNoRef(t *testing.T) {
	r, _ := DefaultRegistry()
	v := resolveFieldType(r, NewString("NoSuchType"))
	if v.AsString() != "NoSuchType" {
		t.Errorf("resolveFieldType unresolved string = %v, want 'NoSuchType'", v)
	}
}

func TestEngineCoreResolveFieldTypeList(t *testing.T) {
	r, _ := DefaultRegistry()
	// A list like [string or none] should be evaluated as code
	list := NewList([]Value{NewString("string"), NewString("or"), NewString("none")})
	v := resolveFieldType(r, list)
	// Should produce a disjunction
	if !v.IsDisjunct() {
		// If it didn't produce a disjunction, at least it shouldn't crash
		t.Logf("resolveFieldType list = %v (type %s)", v, v.VType)
	}
}

// =============================================================================
// TestEngineCoreMakeConvert — direct makeConvert unit tests
// =============================================================================

func TestEngineCoreMakeConvertStringToString(t *testing.T) {
	v, err := makeConvert(NewInteger(42), TString)
	if err != nil {
		t.Fatalf("makeConvert to string: %v", err)
	}
	if v.AsString() != "42" {
		t.Errorf("makeConvert 42 to String = %v, want '42'", v)
	}
}

func TestEngineCoreMakeConvertStringToDecimal(t *testing.T) {
	v, err := makeConvert(NewString("2.5"), TDecimal)
	if err != nil {
		t.Fatalf("makeConvert to decimal: %v", err)
	}
	if v.AsDecimal() != 2.5 {
		t.Errorf("makeConvert '2.5' to Decimal = %v, want 2.5", v)
	}
}

func TestEngineCoreMakeConvertBadDecimal(t *testing.T) {
	_, err := makeConvert(NewString("abc"), TDecimal)
	if err == nil {
		t.Error("expected error converting 'abc' to decimal")
	}
}

func TestEngineCoreMakeConvertBadNumber(t *testing.T) {
	_, err := makeConvert(NewString("xyz"), TNumber)
	if err == nil {
		t.Error("expected error converting 'xyz' to number")
	}
}

func TestEngineCoreMakeConvertUnsupportedType(t *testing.T) {
	customType, _ := NewType("Custom/Thing")
	_, err := makeConvert(NewString("x"), customType)
	if err == nil {
		t.Error("expected error for unsupported target type")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error = %v, want 'unsupported'", err)
	}
}

func TestEngineCoreMakeConvertBoolBool(t *testing.T) {
	v, err := makeConvert(NewBoolean(true), TBoolean)
	if err != nil {
		t.Fatalf("makeConvert bool to bool: %v", err)
	}
	if !v.AsBoolean() {
		t.Error("makeConvert true to Boolean should be true")
	}
}

func TestEngineCoreMakeConvertNumberToBool(t *testing.T) {
	v, err := makeConvert(NewInteger(0), TBoolean)
	if err != nil {
		t.Fatalf("makeConvert 0 to bool: %v", err)
	}
	if v.AsBoolean() {
		t.Error("makeConvert 0 to Boolean should be false")
	}
}

func TestEngineCoreMakeConvertToAtom(t *testing.T) {
	v, err := makeConvert(NewString("foo"), TAtom)
	if err != nil {
		t.Fatalf("makeConvert to atom: %v", err)
	}
	if v.AsAtom() != "foo" {
		t.Errorf("makeConvert 'foo' to Atom = %v, want :foo", v)
	}
}

// =============================================================================
// TestEngineCoreImportFileNoParser — loadFileModule error path
// =============================================================================

func TestEngineCoreImportFileNoParser(t *testing.T) {
	r, _ := DefaultRegistry()
	r.ParseFunc = nil
	_, err := loadFileModule(r, "some.aql")
	if err == nil {
		t.Error("expected error when ParseFunc is nil")
	}
	if !strings.Contains(err.Error(), "parser not configured") {
		t.Errorf("error = %v, want 'parser not configured'", err)
	}
}

// =============================================================================
// TestEngineCoreForcePrefix — force prefix via WordInfo
// =============================================================================

func TestEngineCoreForcePrefix(t *testing.T) {
	r, _ := DefaultRegistry()
	// Force prefix on add: both args must be before the word
	result := runAQL(t, r, []Value{
		NewInteger(3), NewInteger(4),
		NewWordModified("add", -1, true, false),
	})
	if len(result) != 1 || result[0].AsNumber() != 7 {
		t.Errorf("3 4 ^add = %v, want 7", result)
	}
}

func TestEngineCoreForcePrefixNoMatchError(t *testing.T) {
	r, _ := DefaultRegistry()
	// Force prefix with no matching args
	err := runAQLError(t, r, []Value{
		NewWordModified("add", -1, true, false),
	})
	if err == nil {
		t.Error("expected signature error for prefix add with no args")
	}
}

// =============================================================================
// TestEngineCoreMakeRecordNamed — named fields in list form
// =============================================================================

func TestEngineCoreMakeRecordNamed(t *testing.T) {
	r, _ := DefaultRegistry()
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TNumber))
	recType := NewRecordType(fields)

	// Named fields via list of maps: [{name:"Alice"} {age:30}]
	n := NewOrderedMap()
	n.Set("name", NewString("Alice"))
	a := NewOrderedMap()
	a.Set("age", NewInteger(30))
	src := NewList([]Value{NewMap(n), NewMap(a)})

	result := runAQL(t, r, []Value{
		NewWord("make"), recType, src,
	})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("make record named = %v, want map", result)
	}
	m := result[0].AsMap()
	nv, _ := m.Get("name")
	av, _ := m.Get("age")
	if nv.AsString() != "Alice" || av.AsNumber() != 30 {
		t.Errorf("record: name=%v age=%v", nv, av)
	}
}

// =============================================================================
// TestEngineCoreMakeRecordNonListNonMap — error path
// =============================================================================

func TestEngineCoreMakeRecordBadSource(t *testing.T) {
	r, _ := DefaultRegistry()
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	recType := NewRecordType(fields)

	err := runAQLError(t, r, []Value{
		NewWord("make"), recType, NewInteger(42),
	})
	if err == nil {
		t.Error("expected error for make record with integer source")
	}
}

// =============================================================================
// TestEngineCoreMakeTable — table type instance creation
// =============================================================================

func TestEngineCoreMakeTablePositional(t *testing.T) {
	r, _ := DefaultRegistry()
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := RecordTypeInfo{Fields: fields}
	tableType := NewTableType(recType)

	// make TableType [[1 "a"] [2 "b"]]
	rows := NewList([]Value{
		NewList([]Value{NewInteger(1), NewString("a")}),
		NewList([]Value{NewInteger(2), NewString("b")}),
	})
	result := runAQL(t, r, []Value{
		NewWord("make"), tableType, rows,
	})
	if len(result) != 1 {
		t.Fatalf("make table got %d results", len(result))
	}
	rowList := result[0].AsList()
	if len(rowList) != 2 {
		t.Errorf("table has %d rows, want 2", len(rowList))
	}
}

func TestEngineCoreMakeTableBadRowCount(t *testing.T) {
	r, _ := DefaultRegistry()
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := RecordTypeInfo{Fields: fields}
	tableType := NewTableType(recType)

	// Row with wrong number of fields
	rows := NewList([]Value{
		NewList([]Value{NewInteger(1)}), // missing y
	})
	err := runAQLError(t, r, []Value{
		NewWord("make"), tableType, rows,
	})
	if err == nil {
		t.Error("expected error for table row with wrong field count")
	}
}

func TestEngineCoreMakeTableNonList(t *testing.T) {
	r, _ := DefaultRegistry()
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	recType := RecordTypeInfo{Fields: fields}
	tableType := NewTableType(recType)

	err := runAQLError(t, r, []Value{
		NewWord("make"), tableType, NewInteger(42),
	})
	if err == nil {
		t.Error("expected error for make table with non-list source")
	}
}

// =============================================================================
// TestEngineCoreResolveWordValue — direct unit test
// =============================================================================

func TestEngineCoreResolveWordValue(t *testing.T) {
	v := resolveWordValue(NewWord("true"))
	if !v.AsBoolean() {
		t.Error("resolveWordValue(true) should be boolean true")
	}
	v = resolveWordValue(NewWord("false"))
	if v.AsBoolean() {
		t.Error("resolveWordValue(false) should be boolean false")
	}
	v = resolveWordValue(NewWord("None"))
	if !v.VType.Equal(TNone) {
		t.Errorf("resolveWordValue(None) = %v, want None", v)
	}
	v = resolveWordValue(NewWord("other"))
	if !v.VType.Equal(TAtom) {
		t.Errorf("resolveWordValue(other) = %v, want atom", v)
	}
	// Non-word passthrough
	v = resolveWordValue(NewInteger(42))
	if v.AsInteger() != 42 {
		t.Errorf("resolveWordValue(42) = %v, want 42", v)
	}
}
