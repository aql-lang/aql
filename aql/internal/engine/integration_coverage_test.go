package engine

import (
	"bytes"
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
)

// === 1. var word ===

func TestIntegVarWithValueAssignment(t *testing.T) {
	r, _ := DefaultRegistry()
	// var [[x 10]] x end => 10
	varBody := NewList([]Value{
		NewList([]Value{
			NewList([]Value{NewWord("x"), NewInteger(10)}),
		}),
		NewWord("x"),
	})
	result := runAQL(t, r, []Value{NewWord("var"), varBody})
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("var [[x 10]] x = %v, want 10", result)
	}
}

func TestIntegVarWithTypeValue(t *testing.T) {
	r, _ := DefaultRegistry()
	// var [[x Integer]] x end  — x is the Integer type literal
	varBody := NewList([]Value{
		NewList([]Value{
			NewList([]Value{NewWord("x"), NewTypeLiteral(TInteger)}),
		}),
		NewWord("x"),
	})
	result := runAQL(t, r, []Value{NewWord("var"), varBody})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestIntegVarMultipleDecls(t *testing.T) {
	r, _ := DefaultRegistry()
	// var [[[x 2] [y 3]] x add y]
	varBody := NewList([]Value{
		NewList([]Value{
			NewList([]Value{NewWord("x"), NewInteger(2)}),
			NewList([]Value{NewWord("y"), NewInteger(3)}),
		}),
		NewWord("x"), NewWord("add"), NewWord("y"),
	})
	result := runAQL(t, r, []Value{NewWord("var"), varBody})
	if len(result) != 1 || result[0].AsNumber() != 5 {
		t.Errorf("var [[[x 2] [y 3]] x add y] = %v, want 5", result)
	}
}

func TestIntegVarStringName(t *testing.T) {
	r, _ := DefaultRegistry()
	// 42 var [["myvar"] myvar]  — string name, takes value from stack
	varBody := NewList([]Value{
		NewList([]Value{NewString("myvar")}),
		NewWord("myvar"),
	})
	result := runAQL(t, r, []Value{NewInteger(42), NewWord("var"), varBody})
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("42 var [[\"myvar\"] myvar] = %v, want 42", result)
	}
}

func TestIntegVarNestedDoBlock(t *testing.T) {
	r, _ := DefaultRegistry()
	// var [[[x 10]] do [x add 5]]
	varBody := NewList([]Value{
		NewList([]Value{
			NewList([]Value{NewWord("x"), NewInteger(10)}),
		}),
		NewWord("do"), NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(5)}),
	})
	result := runAQL(t, r, []Value{NewWord("var"), varBody})
	if len(result) != 1 || result[0].AsNumber() != 15 {
		t.Errorf("var [[[x 10]] do [x add 5]] = %v, want 15", result)
	}
}

func TestIntegVarErrorInvalidDecl(t *testing.T) {
	r, _ := DefaultRegistry()
	// var [[42] x] — number as declaration should fail
	varBody := NewList([]Value{
		NewList([]Value{NewInteger(42)}),
		NewWord("x"),
	})
	err := runAQLError(t, r, []Value{NewWord("var"), varBody})
	if err == nil {
		t.Error("expected error for invalid var declaration")
	}
}

func TestIntegVarEmptyList(t *testing.T) {
	r, _ := DefaultRegistry()
	// var [] — empty list should fail
	varBody := NewList([]Value{})
	err := runAQLError(t, r, []Value{NewWord("var"), varBody})
	if err == nil {
		t.Error("expected error for empty var list")
	}
}

func TestIntegVarDeclListTooShort(t *testing.T) {
	r, _ := DefaultRegistry()
	// var [[[x]] x] — decl list with only name, no value => error
	varBody := NewList([]Value{
		NewList([]Value{
			NewList([]Value{NewWord("x")}),
		}),
		NewWord("x"),
	})
	err := runAQLError(t, r, []Value{NewWord("var"), varBody})
	if err == nil {
		t.Error("expected error for declaration list with only 1 element")
	}
}

// === 2. undef word ===

func TestIntegUndefRemovesDef(t *testing.T) {
	r, _ := DefaultRegistry()
	// def myVal 99 end myVal undef myVal end
	// After undef, myVal should not be found (error or just word)
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("myVal"), NewInteger(99), NewWord("end"),
		NewWord("myVal"),
	})
	if len(result) != 1 || result[0].AsInteger() != 99 {
		t.Fatalf("def myVal 99 end myVal = %v, want 99", result)
	}

	// Now undef it and verify it's gone
	result = runAQL(t, r, []Value{
		NewWord("undef"), NewWord("myVal"),
	})
	// Should return nothing (undef returns nil)
	if len(result) != 0 {
		t.Errorf("undef myVal should return nothing, got %v", result)
	}
}

func TestIntegUndefWithString(t *testing.T) {
	r, _ := DefaultRegistry()
	// def myVal 42 end undef "myVal"
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("myVal"), NewInteger(42), NewWord("end"),
		NewWord("undef"), NewString("myVal"),
	})
	if len(result) != 0 {
		t.Errorf("undef by string should return nothing, got %v", result)
	}
}

func TestIntegUndefFnTargeted(t *testing.T) {
	r, _ := DefaultRegistry()
	// def myFn fn [[x:Number] [Number] [x add 1]] end
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX)}),
		NewList([]Value{NewTypeLiteral(TNumber)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(1)}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("myFn"),
		NewWord("fn"), fnBody,
		NewWord("end"),
	})

	// Verify myFn works: 5 myFn => 6
	result := runAQL(t, r, []Value{NewInteger(5), NewWord("myFn")})
	if len(result) != 1 || result[0].AsNumber() != 6 {
		t.Fatalf("5 myFn = %v, want 6", result)
	}

	// undef myFn (complete removal)
	runAQL(t, r, []Value{NewWord("undef"), NewWord("myFn")})
}

// === 3. fn word ===

func TestIntegFnMultipleParams(t *testing.T) {
	r, _ := DefaultRegistry()
	// def addTwo fn [[x:Number y:Number] [Number] [x add y]] end
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	pairY := NewOrderedMap()
	pairY.Set("y", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX), NewImplicitMap(pairY)}),
		NewList([]Value{NewTypeLiteral(TNumber)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewWord("y")}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("addTwo"),
		NewWord("fn"), fnBody,
		NewWord("end"),
	})

	// 3 5 addTwo => 8
	result := runAQL(t, r, []Value{NewInteger(3), NewInteger(5), NewWord("addTwo")})
	if len(result) != 1 || result[0].AsNumber() != 8 {
		t.Errorf("3 5 addTwo = %v, want 8", result)
	}
}

func TestIntegFnUnnamedParams(t *testing.T) {
	r, _ := DefaultRegistry()
	// fn [[Number] [Number] [add 1]]  — unnamed Number param
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("add"), NewInteger(1)}),
	})
	result := runAQL(t, r, []Value{
		NewWord("def"), NewWord("inc"),
		NewWord("fn"), fnBody,
		NewWord("end"),
	})
	if len(result) != 0 {
		t.Fatalf("def should return nothing, got %v", result)
	}

	result = runAQL(t, r, []Value{NewInteger(10), NewWord("inc")})
	if len(result) != 1 || result[0].AsNumber() != 11 {
		t.Errorf("10 inc = %v, want 11", result)
	}
}

func TestIntegFnUndefSpecPairs(t *testing.T) {
	r, _ := DefaultRegistry()
	// fn with 2 elements (pairs) => FnUndefInfo
	// fn [[Number] [Number]]
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
	})
	result := runAQL(t, r, []Value{NewWord("fn"), fnBody})
	if len(result) != 1 {
		t.Fatalf("fn undef spec should return 1 value, got %d", len(result))
	}
	// Should be a FnUndef type
	if !result[0].VType.Equal(TFnUndef) {
		t.Errorf("expected TFnUndef, got %s", result[0].VType)
	}
}

func TestIntegFnEmptyListError(t *testing.T) {
	r, _ := DefaultRegistry()
	err := runAQLError(t, r, []Value{NewWord("fn"), NewList([]Value{})})
	if err == nil {
		t.Error("expected error for fn with empty list")
	}
}

func TestIntegFnBadLengthError(t *testing.T) {
	r, _ := DefaultRegistry()
	// fn with 5 elements (not divisible by 2 or 3)
	err := runAQLError(t, r, []Value{
		NewWord("fn"), NewList([]Value{
			NewInteger(1), NewInteger(2), NewInteger(3),
			NewInteger(4), NewInteger(5),
		}),
	})
	if err == nil {
		t.Error("expected error for fn with 5 elements")
	}
}

func TestIntegFnSingleValueAbbreviation(t *testing.T) {
	r, _ := DefaultRegistry()
	// fn with non-list input/output/body (abbreviation: treated as single-element lists)
	// fn [Number Number [add 1]]
	fnBody := NewList([]Value{
		NewWord("Number"),
		NewWord("Number"),
		NewList([]Value{NewWord("add"), NewInteger(1)}),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("inc2"),
		NewWord("fn"), fnBody,
		NewWord("end"),
	})

	result := runAQL(t, r, []Value{NewInteger(7), NewWord("inc2")})
	if len(result) != 1 || result[0].AsNumber() != 8 {
		t.Errorf("7 inc2 = %v, want 8", result)
	}
}

// === 4. module word ===

func TestIntegModuleWithExport(t *testing.T) {
	r, _ := DefaultRegistry()
	// module [
	//   def x 42 end
	//   export :myExport {val: x}
	// ] end
	moduleBody := NewList([]Value{
		NewString("def"), NewString("x"), NewInteger(42), NewString("end"),
		NewString("export"), NewAtom("myExport"),
		NewMap(singleMap("val", NewString("x"))),
	})
	result := runAQL(t, r, []Value{NewWord("module"), moduleBody})
	if len(result) != 1 {
		t.Fatalf("module should return 1 value, got %d", len(result))
	}
	if !result[0].VType.Equal(TModule) {
		t.Fatalf("expected TModule, got %s", result[0].VType)
	}

	desc := result[0].AsModule()
	if _, ok := desc.Exports["myExport"]; !ok {
		t.Error("expected 'myExport' in module exports")
	}
}

func TestIntegModuleImportAll(t *testing.T) {
	r, _ := DefaultRegistry()
	// Build module, then import it
	moduleBody := NewList([]Value{
		NewString("def"), NewString("x"), NewInteger(99), NewString("end"),
		NewString("export"), NewAtom("stuff"),
		NewMap(singleMap("val", NewString("x"))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("mymod"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})

	// import mymod
	runAQL(t, r, []Value{NewWord("import"), NewWord("mymod")})

	// Now "stuff" should be defined as a map with val: 99
	result := runAQL(t, r, []Value{NewWord("stuff")})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("stuff should be a map, got %v", result)
	}
	m := result[0].AsMap()
	v, ok := m.Get("val")
	if !ok || v.AsInteger() != 99 {
		t.Errorf("stuff.val = %v, want 99", v)
	}
}

func TestIntegModuleImportRename(t *testing.T) {
	r, _ := DefaultRegistry()
	moduleBody := NewList([]Value{
		NewString("def"), NewString("x"), NewInteger(55), NewString("end"),
		NewString("export"), NewAtom("orig"),
		NewMap(singleMap("val", NewString("x"))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("mymod2"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})

	// import [orig renamed] mymod2
	renameList := NewList([]Value{NewAtom("orig"), NewAtom("renamed")})
	runAQL(t, r, []Value{NewWord("import"), renameList, NewWord("mymod2")})

	result := runAQL(t, r, []Value{NewWord("renamed")})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("renamed should be a map, got %v", result)
	}
}

func TestIntegModuleImportMultiRename(t *testing.T) {
	r, _ := DefaultRegistry()
	moduleBody := NewList([]Value{
		NewString("def"), NewString("a"), NewInteger(1), NewString("end"),
		NewString("def"), NewString("b"), NewInteger(2), NewString("end"),
		NewString("export"), NewAtom("expA"),
		NewMap(singleMap("val", NewString("a"))),
		NewString("export"), NewAtom("expB"),
		NewMap(singleMap("val", NewString("b"))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("mm"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})

	// import [[expA newA] [expB newB]] mm
	renameList := NewList([]Value{
		NewList([]Value{NewAtom("expA"), NewAtom("newA")}),
		NewList([]Value{NewAtom("expB"), NewAtom("newB")}),
	})
	runAQL(t, r, []Value{NewWord("import"), renameList, NewWord("mm")})

	result := runAQL(t, r, []Value{NewWord("newA")})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("newA should be a map, got %v", result)
	}
}

func TestIntegModuleExportWithAtomName(t *testing.T) {
	r, _ := DefaultRegistry()
	// export with atom name (word signature removed; unknown words become atoms)
	moduleBody := NewList([]Value{
		NewString("def"), NewString("x"), NewInteger(7), NewString("end"),
		NewString("export"), NewAtom("wrdexp"),
		NewMap(singleMap("val", NewString("x"))),
	})
	result := runAQL(t, r, []Value{NewWord("module"), moduleBody})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	desc := result[0].AsModule()
	if _, ok := desc.Exports["wrdexp"]; !ok {
		t.Error("expected 'wrdexp' in module exports")
	}
}

func TestIntegValToAtomOrStringWord(t *testing.T) {
	// Test valToAtomOrString with word values (used in import rename)
	r, _ := DefaultRegistry()
	moduleBody := NewList([]Value{
		NewString("def"), NewString("x"), NewInteger(10), NewString("end"),
		NewString("export"), NewAtom("orig"),
		NewMap(singleMap("val", NewString("x"))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("wmod"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})

	// import with word names (instead of atoms/strings)
	renameList := NewList([]Value{NewWord("orig"), NewWord("wordRenamed")})
	runAQL(t, r, []Value{NewWord("import"), renameList, NewWord("wmod")})

	result := runAQL(t, r, []Value{NewWord("wordRenamed")})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("wordRenamed should be a map, got %v", result)
	}
}

func TestIntegImportSingleRenameWord(t *testing.T) {
	r, _ := DefaultRegistry()
	moduleBody := NewList([]Value{
		NewString("def"), NewString("x"), NewInteger(42), NewString("end"),
		NewString("export"), NewAtom("Orig"),
		NewMap(singleMap("val", NewString("x"))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("mymod"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})

	// import NewName mymod — renames single export Orig to NewName
	// (unknown word NewName becomes atom at runtime)
	runAQL(t, r, []Value{NewWord("import"), NewAtom("NewName"), NewWord("mymod")})

	result := runAQL(t, r, []Value{NewWord("NewName")})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("NewName should be a map, got %v", result)
	}
}

func TestIntegImportSingleRenameAtom(t *testing.T) {
	r, _ := DefaultRegistry()
	moduleBody := NewList([]Value{
		NewString("def"), NewString("x"), NewInteger(42), NewString("end"),
		NewString("export"), NewAtom("Orig"),
		NewMap(singleMap("val", NewString("x"))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("mymod"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})

	// import Renamed mymod — renames single export Orig to Renamed (atom)
	runAQL(t, r, []Value{NewWord("import"), NewAtom("Renamed"), NewWord("mymod")})

	result := runAQL(t, r, []Value{NewWord("Renamed")})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("Renamed should be a map, got %v", result)
	}
}

func TestIntegImportSingleRenameMultiExportError(t *testing.T) {
	r, _ := DefaultRegistry()
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	moduleBody := NewList([]Value{
		NewString("export"), NewAtom("X"), NewMap(singleMap("v", NewInteger(1))),
		NewString("export"), NewAtom("Y"), NewMap(singleMap("v", NewInteger(2))),
	})
	runAQL(t, r, []Value{
		NewWord("def"), NewWord("mm"),
		NewWord("module"), moduleBody,
		NewWord("end"),
	})

	_, err := New(r).Run([]Value{NewWord("import"), NewWord("Only"), NewWord("mm")})
	if err == nil {
		t.Fatal("expected error when renaming module with multiple exports")
	}
}

// === 5. convert word ===

func TestIntegConvertIntegerToString(t *testing.T) {
	r, _ := DefaultRegistry()
	// 42 convert String
	result := runAQL(t, r, []Value{
		NewInteger(42), NewWord("convert"), NewTypeLiteral(TString),
	})
	if len(result) != 1 || result[0].AsString() != "42" {
		t.Errorf("42 convert String = %v, want '42'", result)
	}
}

func TestIntegConvertStringToInteger(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewString("123"), NewWord("convert"), NewTypeLiteral(TInteger),
	})
	if len(result) != 1 || result[0].AsInteger() != 123 {
		t.Errorf("'123' convert Integer = %v, want 123", result)
	}
}

func TestIntegConvertStringToDecimal(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewString("3.14"), NewWord("convert"), NewTypeLiteral(TDecimal),
	})
	if len(result) != 1 || result[0].AsDecimal() != 3.14 {
		t.Errorf("'3.14' convert Decimal = %v, want 3.14", result)
	}
}

func TestIntegConvertToBoolean(t *testing.T) {
	r, _ := DefaultRegistry()
	// integer to boolean
	result := runAQL(t, r, []Value{
		NewInteger(1), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("1 convert Boolean = %v, want true", result)
	}

	// 0 to boolean
	result = runAQL(t, r, []Value{
		NewInteger(0), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("0 convert Boolean = %v, want false", result)
	}

	// string "true" to boolean
	result = runAQL(t, r, []Value{
		NewString("true"), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("'true' convert Boolean = %v, want true", result)
	}

	// string "false" to boolean
	result = runAQL(t, r, []Value{
		NewString("false"), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("'false' convert Boolean = %v, want false", result)
	}

	// non-empty string to boolean (truthy)
	result = runAQL(t, r, []Value{
		NewString("hello"), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("'hello' convert Boolean = %v, want true", result)
	}

	// empty string to boolean (falsy)
	result = runAQL(t, r, []Value{
		NewString(""), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	if len(result) != 1 || result[0].AsBoolean() {
		t.Errorf("'' convert Boolean = %v, want false", result)
	}
}

func TestIntegConvertIntToHexString(t *testing.T) {
	r, _ := DefaultRegistry()
	// 255 convert String {base: "hex"}
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	result := runAQL(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewTypeLiteral(TString), NewMap(hexOpts),
	})
	if len(result) != 1 || result[0].AsString() != "ff" {
		t.Errorf("255 convert String {base:hex} = %v, want 'ff'", result)
	}
}

func TestIntegConvertIntToHEXString(t *testing.T) {
	r, _ := DefaultRegistry()
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("HEX"))
	result := runAQL(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewTypeLiteral(TString), NewMap(hexOpts),
	})
	if len(result) != 1 || result[0].AsString() != "FF" {
		t.Errorf("255 convert String {base:HEX} = %v, want 'FF'", result)
	}
}

func TestIntegConvertIntToBinString(t *testing.T) {
	r, _ := DefaultRegistry()
	binOpts := NewOrderedMap()
	binOpts.Set("base", NewString("bin"))
	result := runAQL(t, r, []Value{
		NewInteger(10), NewWord("convert"), NewTypeLiteral(TString), NewMap(binOpts),
	})
	if len(result) != 1 || result[0].AsString() != "1010" {
		t.Errorf("10 convert String {base:bin} = %v, want '1010'", result)
	}
}

func TestIntegConvertIntToOctString(t *testing.T) {
	r, _ := DefaultRegistry()
	octOpts := NewOrderedMap()
	octOpts.Set("base", NewString("oct"))
	result := runAQL(t, r, []Value{
		NewInteger(8), NewWord("convert"), NewTypeLiteral(TString), NewMap(octOpts),
	})
	if len(result) != 1 || result[0].AsString() != "10" {
		t.Errorf("8 convert String {base:oct} = %v, want '10'", result)
	}
}

func TestIntegConvertHexStringToNumber(t *testing.T) {
	r, _ := DefaultRegistry()
	// "ff" convert Number {base: "hex"}
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	result := runAQL(t, r, []Value{
		NewString("ff"), NewWord("convert"), NewTypeLiteral(TNumber), NewMap(hexOpts),
	})
	if len(result) != 1 || result[0].AsInteger() != 255 {
		t.Errorf("'ff' convert Number {base:hex} = %v, want 255", result)
	}
}

func TestIntegConvertBinStringToNumber(t *testing.T) {
	r, _ := DefaultRegistry()
	binOpts := NewOrderedMap()
	binOpts.Set("base", NewString("bin"))
	result := runAQL(t, r, []Value{
		NewString("1010"), NewWord("convert"), NewTypeLiteral(TNumber), NewMap(binOpts),
	})
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("'1010' convert Number {base:bin} = %v, want 10", result)
	}
}

func TestIntegConvertOctStringToNumber(t *testing.T) {
	r, _ := DefaultRegistry()
	octOpts := NewOrderedMap()
	octOpts.Set("base", NewString("oct"))
	result := runAQL(t, r, []Value{
		NewString("10"), NewWord("convert"), NewTypeLiteral(TNumber), NewMap(octOpts),
	})
	if len(result) != 1 || result[0].AsInteger() != 8 {
		t.Errorf("'10' convert Number {base:oct} = %v, want 8", result)
	}
}

func TestIntegConvertWithSettingsMap(t *testing.T) {
	r, _ := DefaultRegistry()
	// 255 convert String {base: "hex"}
	opts := NewOrderedMap()
	opts.Set("base", NewString("hex"))
	result := runAQL(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewTypeLiteral(TString), NewMap(opts),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].AsString() != "ff" {
		t.Errorf("255 convert String {base:hex} = %v, want 'ff'", result)
	}
}

func TestIntegConvertBooleanPassthrough(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewBoolean(true), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("true convert Boolean = %v, want true", result)
	}
}

func TestIntegConvertErrorBadDecimal(t *testing.T) {
	r, _ := DefaultRegistry()
	err := runAQLError(t, r, []Value{
		NewString("notanumber"), NewWord("convert"), NewTypeLiteral(TDecimal),
	})
	if err == nil {
		t.Error("expected error converting non-numeric string to decimal")
	}
}

func TestIntegConvertErrorBadNumber(t *testing.T) {
	r, _ := DefaultRegistry()
	err := runAQLError(t, r, []Value{
		NewString("notanumber"), NewWord("convert"), NewTypeLiteral(TNumber),
	})
	if err == nil {
		t.Error("expected error converting non-numeric string to number")
	}
}

func TestIntegConvertErrorBadVariant(t *testing.T) {
	r, _ := DefaultRegistry()
	badOpts := NewOrderedMap()
	badOpts.Set("base", NewString("badvariant"))
	err := runAQLError(t, r, []Value{
		NewInteger(42), NewWord("convert"), NewTypeLiteral(TString), NewMap(badOpts),
	})
	if err == nil {
		t.Error("expected error for unknown string variant")
	}
}

func TestIntegConvertErrorBadNumberVariant(t *testing.T) {
	r, _ := DefaultRegistry()
	badOpts := NewOrderedMap()
	badOpts.Set("base", NewString("badvariant"))
	err := runAQLError(t, r, []Value{
		NewString("ff"), NewWord("convert"), NewTypeLiteral(TNumber), NewMap(badOpts),
	})
	if err == nil {
		t.Error("expected error for unknown number variant")
	}
}

func TestIntegConvertErrorVariantNotInteger(t *testing.T) {
	r, _ := DefaultRegistry()
	// Variant conversion only supported for integer to string
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	err := runAQLError(t, r, []Value{
		NewString("hello"), NewWord("convert"), NewTypeLiteral(TString), NewMap(hexOpts),
	})
	if err == nil {
		t.Error("expected error for variant with non-integer source")
	}
}

// === 6. do word ===

func TestIntegDoList(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("do"), NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}),
	})
	if len(result) != 1 || result[0].AsNumber() != 3 {
		t.Errorf("do [1 add 2] = %v, want 3", result)
	}
}

func TestIntegDoMap(t *testing.T) {
	r, _ := DefaultRegistry()
	// do {x: [3 add 4]}
	innerList := NewList([]Value{NewInteger(3), NewString("add"), NewInteger(4)})
	m := NewOrderedMap()
	m.Set("x", innerList)
	result := runAQL(t, r, []Value{NewWord("do"), NewMap(m)})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("do map should return a map, got %v", result)
	}
	rm := result[0].AsMap()
	xVal, ok := rm.Get("x")
	if !ok || xVal.AsNumber() != 7 {
		t.Errorf("do {x:[3 add 4]}.x = %v, want 7", xVal)
	}
}

func TestIntegDoNestedMap(t *testing.T) {
	r, _ := DefaultRegistry()
	// do {outer: {inner: [2 add 3]}}
	innerList := NewList([]Value{NewInteger(2), NewString("add"), NewInteger(3)})
	innerMap := NewOrderedMap()
	innerMap.Set("inner", innerList)
	outerMap := NewOrderedMap()
	outerMap.Set("outer", NewMap(innerMap))
	result := runAQL(t, r, []Value{NewWord("do"), NewMap(outerMap)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	outer := result[0].AsMap()
	innerVal, _ := outer.Get("outer")
	innerResult := innerVal.AsMap()
	v, _ := innerResult.Get("inner")
	if v.AsNumber() != 5 {
		t.Errorf("do nested map inner = %v, want 5", v)
	}
}

func TestIntegDoListMultipleResults(t *testing.T) {
	r, _ := DefaultRegistry()
	// do [1 2 3] => returns list of all results
	result := runAQL(t, r, []Value{
		NewWord("do"), NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
	})
	// The sub-engine should return all 3 values
	if len(result) != 3 {
		t.Errorf("do [1 2 3] = %v, want 3 results", result)
	}
}

// === 7. fileio with MemFileOps ===

func TestIntegFileIOWriteAndRead(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	// write "test.txt" "hello world"
	result := runAQL(t, r, []Value{
		NewWord("write"), NewString("test.txt"), NewString("hello world"),
	})
	if len(result) != 1 || result[0].AsString() != "test.txt" {
		t.Errorf("write should return path, got %v", result)
	}

	// read "test.txt"
	result = runAQL(t, r, []Value{
		NewWord("read"), NewString("test.txt"),
	})
	if len(result) != 1 || result[0].AsString() != "hello world" {
		t.Errorf("read test.txt = %v, want 'hello world'", result)
	}
}

func TestIntegFileIOWriteAppend(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	// write "test.txt" "hello"
	runAQL(t, r, []Value{
		NewWord("write"), NewString("test.txt"), NewString("hello"),
	})

	// write "test.txt" " world" {mode:"append"}
	opts := NewOrderedMap()
	opts.Set("mode", NewString("append"))
	runAQL(t, r, []Value{
		NewWord("write"), NewString("test.txt"), NewString(" world"), NewMap(opts),
	})

	// read back
	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("test.txt"),
	})
	if len(result) != 1 || result[0].AsString() != "hello world" {
		t.Errorf("read after append = %v, want 'hello world'", result)
	}
}

func TestIntegFileIOWriteJSON(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	// Write a map as JSON
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("json"))
	runAQL(t, r, []Value{
		NewWord("write"), NewString("data.json"), NewMap(m), NewMap(opts),
	})

	// Read it back
	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("data.json"),
	})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("read data.json should return map, got %v", result)
	}
	rm := result[0].AsMap()
	v, ok := rm.Get("x")
	if !ok || v.AsInteger() != 1 {
		t.Errorf("read data.json x = %v, want 1", v)
	}
}

func TestIntegFileIOWriteStdout(t *testing.T) {
	r, _ := DefaultRegistry()
	var buf bytes.Buffer
	r.Output = &buf

	// write to stdout
	result := runAQL(t, r, []Value{
		NewWord("write"), NewString("<stdout>"), NewString("printed text"),
	})
	if len(result) != 1 || result[0].AsString() != "<stdout>" {
		t.Errorf("write stdout should return path, got %v", result)
	}
	if buf.String() != "printed text" {
		t.Errorf("stdout output = %q, want 'printed text'", buf.String())
	}
}

func TestIntegFileIOWriteStderr(t *testing.T) {
	r, _ := DefaultRegistry()
	var buf bytes.Buffer
	r.ErrOutput = &buf

	result := runAQL(t, r, []Value{
		NewWord("write"), NewString("<stderr>"), NewString("error text"),
	})
	if len(result) != 1 || result[0].AsString() != "<stderr>" {
		t.Errorf("write stderr should return path, got %v", result)
	}
	if buf.String() != "error text" {
		t.Errorf("stderr output = %q, want 'error text'", buf.String())
	}
}

func TestIntegFileIOReadWithFmtOption(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	// Write JSON content to a txt file, read with explicit fmt
	_ = mem.WriteFile("data.txt", []byte(`{"a":1}`), 0644)
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("json"))
	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("data.txt"), NewMap(opts),
	})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("read data.txt with fmt:json should return map, got %v", result)
	}
}

func TestIntegFileIOReadJsonExtension(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	_ = mem.WriteFile("data.json", []byte(`{"b":2}`), 0644)
	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("data.json"),
	})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("read data.json should auto-detect json, got %v", result)
	}
}

func TestIntegFileIOWriteStringWithOpts(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	opts.Set("nl", NewString("lf"))
	runAQL(t, r, []Value{
		NewWord("write"), NewString("output.txt"), NewString("line1\nline2"), NewMap(opts),
	})

	data, _ := mem.ReadFile("output.txt")
	if string(data) != "line1\nline2" {
		t.Errorf("write with opts = %q, want 'line1\\nline2'", data)
	}
}

// === 8. CSV/TSV format ===

func TestIntegCSVReadWrite(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	csvContent := "name,age\nAlice,30\nBob,25\n"
	_ = mem.WriteFile("people.csv", []byte(csvContent), 0644)

	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("people.csv"),
	})
	if len(result) != 1 {
		t.Fatalf("read csv should return 1 value, got %d", len(result))
	}
	// Should be a list (table data)
	if !result[0].VType.Equal(TList) {
		t.Fatalf("csv result should be TList, got %s", result[0].VType)
	}
}

func TestIntegTSVRead(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	tsvContent := "col1\tcol2\nval1\tval2\n"
	_ = mem.WriteFile("data.tsv", []byte(tsvContent), 0644)

	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("data.tsv"),
	})
	if len(result) != 1 {
		t.Fatalf("read tsv should return 1 value, got %d", len(result))
	}
}

func TestIntegCSVReadWithFmtOption(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	csvContent := "x,y\n1,2\n"
	_ = mem.WriteFile("data.txt", []byte(csvContent), 0644)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("csv"))
	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("data.txt"), NewMap(opts),
	})
	if len(result) != 1 || !result[0].VType.Equal(TList) {
		t.Fatalf("read with csv fmt should return table, got %v", result)
	}
}

// === 9. or for disjuncts ===

func TestIntegOrDisjunctValues(t *testing.T) {
	r, _ := DefaultRegistry()
	// 1 or "hello" or true
	result := runAQL(t, r, []Value{
		NewInteger(1), NewWord("or"), NewString("hello"), NewWord("or"), NewBoolean(true),
	})
	if len(result) != 1 || !result[0].IsDisjunct() {
		t.Fatalf("1 or 'hello' or true should be disjunct, got %v", result)
	}
	alts := result[0].AsDisjunct().Alternatives
	if len(alts) != 3 {
		t.Errorf("disjunct should have 3 alternatives, got %d", len(alts))
	}
}

func TestIntegOrDisjunctTwoValues(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewInteger(42), NewWord("or"), NewString("hello"),
	})
	if len(result) != 1 || !result[0].IsDisjunct() {
		t.Fatalf("42 or 'hello' should be disjunct, got %v", result)
	}
	alts := result[0].AsDisjunct().Alternatives
	if len(alts) != 2 {
		t.Errorf("disjunct should have 2 alternatives, got %d", len(alts))
	}
}

func TestIntegOrDisjunctFlattensLeft(t *testing.T) {
	r, _ := DefaultRegistry()
	// Build a disjunct then or with another value
	result := runAQL(t, r, []Value{
		NewInteger(1), NewWord("or"), NewInteger(2), NewWord("or"), NewInteger(3),
	})
	if len(result) != 1 || !result[0].IsDisjunct() {
		t.Fatalf("chained or should produce disjunct, got %v", result)
	}
	alts := result[0].AsDisjunct().Alternatives
	if len(alts) != 3 {
		t.Errorf("should flatten to 3 alternatives, got %d", len(alts))
	}
}

func TestIntegOrDisjunctFlattensRight(t *testing.T) {
	r, _ := DefaultRegistry()
	// Pre-build a disjunct on the right side
	rightDisjunct := NewDisjunct([]Value{NewInteger(2), NewInteger(3)})
	result := runAQL(t, r, []Value{
		NewInteger(1), NewWord("or"), rightDisjunct,
	})
	if len(result) != 1 || !result[0].IsDisjunct() {
		t.Fatalf("or with right disjunct should produce disjunct, got %v", result)
	}
	alts := result[0].AsDisjunct().Alternatives
	if len(alts) != 3 {
		t.Errorf("should flatten right disjunct to 3 alternatives, got %d", len(alts))
	}
}

func TestIntegOrBooleanStillWorks(t *testing.T) {
	r, _ := DefaultRegistry()
	// Boolean or should still work as logical or
	result := runAQL(t, r, []Value{
		NewBoolean(false), NewWord("or"), NewBoolean(true),
	})
	if len(result) != 1 || !result[0].AsBoolean() {
		t.Errorf("false or true = %v, want true", result)
	}
}

// === 10. context word ===

func TestIntegContextSetGet(t *testing.T) {
	r, _ := DefaultRegistry()
	// context set "key" 42 context get "key"
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("mykey"), NewInteger(42),
		NewWord("context"), NewWord("get"), NewString("mykey"),
	})
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("context set/get = %v, want 42", result)
	}
}

func TestIntegContextGetMissing(t *testing.T) {
	r, _ := DefaultRegistry()
	// context get on non-existent key => None
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("get"), NewString("nonexistent"),
	})
	if len(result) != 1 || !result[0].VType.Equal(TNone) {
		t.Errorf("context get missing = %v, want None", result)
	}
}

func TestIntegContextSetWithWord(t *testing.T) {
	r, _ := DefaultRegistry()
	// context set wordKey 99 context get wordKey
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewWord("wkey"), NewInteger(99),
		NewWord("context"), NewWord("get"), NewWord("wkey"),
	})
	if len(result) != 1 || result[0].AsInteger() != 99 {
		t.Errorf("context set/get with word key = %v, want 99", result)
	}
}

func TestIntegContextOverwrite(t *testing.T) {
	r, _ := DefaultRegistry()
	// set, then overwrite, then get
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("k"), NewInteger(1),
		NewWord("context"), NewWord("set"), NewString("k"), NewInteger(2),
		NewWord("context"), NewWord("get"), NewString("k"),
	})
	if len(result) != 1 || result[0].AsInteger() != 2 {
		t.Errorf("context overwrite = %v, want 2", result)
	}
}

func TestIntegContextUnknownSubCommand(t *testing.T) {
	r, _ := DefaultRegistry()
	err := runAQLError(t, r, []Value{
		NewWord("context"), NewWord("badcmd"),
	})
	if err == nil {
		t.Error("expected error for unknown context sub-command")
	}
}

// === Additional edge cases ===

func TestIntegFileIOWriteListAsJSON(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	// Write a list value with fmt:json
	list := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("json"))
	runAQL(t, r, []Value{
		NewWord("write"), NewString("list.json"), list, NewMap(opts),
	})

	data, err := mem.ReadFile("list.json")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !strings.Contains(string(data), "[1,2,3]") {
		t.Errorf("written JSON = %q, want to contain [1,2,3]", data)
	}
}

func TestIntegFileIOReadLines(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	_ = mem.WriteFile("lines.txt", []byte("a\nb\nc"), 0644)
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("lines"))
	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("lines.txt"), NewMap(opts),
	})
	if len(result) != 1 || !result[0].VType.Equal(TList) {
		t.Fatalf("read lines should return list, got %v", result)
	}
	elems := result[0].AsList()
	if len(elems) != 3 {
		t.Errorf("read lines should have 3 elements, got %d", len(elems))
	}
}

func TestIntegDoMapWithNonListValues(t *testing.T) {
	r, _ := DefaultRegistry()
	// do {x: 42, y: "hello"} — non-list values should pass through
	m := NewOrderedMap()
	m.Set("x", NewInteger(42))
	m.Set("y", NewString("hello"))
	result := runAQL(t, r, []Value{NewWord("do"), NewMap(m)})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("do map should return map, got %v", result)
	}
	rm := result[0].AsMap()
	xVal, _ := rm.Get("x")
	if xVal.AsInteger() != 42 {
		t.Errorf("do {x:42}.x = %v, want 42", xVal)
	}
}

func TestIntegVarWithDoBlock(t *testing.T) {
	r, _ := DefaultRegistry()
	// var [[[a 3] [b 4]] do [a add b]]
	varBody := NewList([]Value{
		NewList([]Value{
			NewList([]Value{NewWord("a"), NewInteger(3)}),
			NewList([]Value{NewWord("b"), NewInteger(4)}),
		}),
		NewWord("do"), NewList([]Value{NewWord("a"), NewWord("add"), NewWord("b")}),
	})
	result := runAQL(t, r, []Value{NewWord("var"), varBody})
	if len(result) != 1 || result[0].AsNumber() != 7 {
		t.Errorf("var [[[a 3] [b 4]] do [a add b]] = %v, want 7", result)
	}
}

func TestIntegFileIOReadJsonicFormat(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	_ = mem.WriteFile("data.jsonic", []byte(`{x: 1, y: 2}`), 0644)
	result := runAQL(t, r, []Value{
		NewWord("read"), NewString("data.jsonic"),
	})
	if len(result) != 1 || !result[0].VType.Equal(TMap) {
		t.Fatalf("read jsonic should return map, got %v", result)
	}
}

func TestIntegFileIOWriteAppendNewFile(t *testing.T) {
	r, _ := DefaultRegistry()
	mem := fileops.NewMem()
	r.SetFileOps(mem)

	// append to non-existent file should just write
	opts := NewOrderedMap()
	opts.Set("mode", NewString("append"))
	runAQL(t, r, []Value{
		NewWord("write"), NewString("new.txt"), NewString("fresh"), NewMap(opts),
	})

	data, _ := mem.ReadFile("new.txt")
	if string(data) != "fresh" {
		t.Errorf("append to new file = %q, want 'fresh'", data)
	}
}

