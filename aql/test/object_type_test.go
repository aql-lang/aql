package test

import (
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// TestObjectTypeDefine defines a named object type and verifies its structure.
// def Foo object {a:String,b:Boolean} → Object/Foo with fields a and b
func TestObjectTypeDefine(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String,b:Boolean}`,
		`Foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	s := result[0].String()
	if !strings.Contains(s, "Object/Foo") {
		t.Errorf("expected type name to contain 'Object/Foo', got %s", s)
	}
	if !strings.Contains(s, "a:Scalar/String") {
		t.Errorf("expected field a:Scalar/String, got %s", s)
	}
	if !strings.Contains(s, "b:Scalar/Boolean") {
		t.Errorf("expected field b:Scalar/Boolean, got %s", s)
	}
}

// TestObjectTypeAnonymous creates an anonymous object type.
// object {c:99} → anonymous object with type Object/<internal-id>
func TestObjectTypeAnonymous(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`object {c:99}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	s := result[0].String()
	if !strings.Contains(s, "Object/T_") {
		t.Errorf("expected anonymous object type with Object/T_ prefix, got %s", s)
	}
	if !strings.Contains(s, "c:") {
		t.Errorf("expected field c, got %s", s)
	}
}

// TestObjectTypeInheritance defines a child object type that inherits fields.
// def Bar object {d:Integer} Foo → Object/Foo/Bar with fields a,b,d
func TestObjectTypeInheritance(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String,b:Boolean}`,
		`def Bar object {d:Integer} Foo`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	s := result[0].String()
	if !strings.Contains(s, "Object/Foo/Bar") {
		t.Errorf("expected type name to contain 'Object/Foo/Bar', got %s", s)
	}
	// Should have inherited fields a,b from Foo plus own field d
	if !strings.Contains(s, "a:Scalar/String") {
		t.Errorf("expected inherited field a:Scalar/String, got %s", s)
	}
	if !strings.Contains(s, "b:Scalar/Boolean") {
		t.Errorf("expected inherited field b:Scalar/Boolean, got %s", s)
	}
	if !strings.Contains(s, "d:Scalar/Number/Integer") {
		t.Errorf("expected own field d:Scalar/Number/Integer, got %s", s)
	}
}

// TestObjectTypeParentFields verifies that parent fields are accessible
// through AllFields on the child type.
func TestObjectTypeParentFields(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String,b:Boolean}`,
		`def Bar object {d:Integer} Foo`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	ot := result[0].AsObjectType()
	all := ot.AllFields()
	if all.Len() != 3 {
		t.Fatalf("expected 3 total fields (a,b,d), got %d", all.Len())
	}
	keys := all.Keys()
	// Parent fields come first (a,b), then own (d)
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "d" {
		t.Errorf("expected field order [a,b,d], got %v", keys)
	}
}

// TestObjectTypeOwnFieldsOnly verifies that own fields do not include inherited.
func TestObjectTypeOwnFieldsOnly(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String,b:Boolean}`,
		`def Bar object {d:Integer} Foo`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	ot := result[0].AsObjectType()
	if ot.Fields.Len() != 1 {
		t.Fatalf("expected 1 own field (d), got %d", ot.Fields.Len())
	}
	keys := ot.Fields.Keys()
	if keys[0] != "d" {
		t.Errorf("expected own field 'd', got %s", keys[0])
	}
}

// TestObjectTypeDeepInheritance tests three-level inheritance: Foo → Bar → Baz.
func TestObjectTypeDeepInheritance(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String}`,
		`def Bar object {b:Integer} Foo`,
		`def Baz object {c:Boolean} Bar`,
		`Baz`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	s := result[0].String()
	if !strings.Contains(s, "Object/Foo/Bar/Baz") {
		t.Errorf("expected type name 'Object/Foo/Bar/Baz', got %s", s)
	}
	ot := result[0].AsObjectType()
	all := ot.AllFields()
	if all.Len() != 3 {
		t.Fatalf("expected 3 fields (a,b,c), got %d", all.Len())
	}
	keys := all.Keys()
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("expected field order [a,b,c], got %v", keys)
	}
}

// TestObjectTypeUniqueID verifies that each object type gets a unique ID.
func TestObjectTypeUniqueID(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String}`,
		`def Bar object {b:String}`,
		`Foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	fooID := result[0].AsObjectType().ID

	result2, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String}`,
		`def Bar object {b:String}`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	barID := result2[0].AsObjectType().ID

	if fooID == barID {
		t.Errorf("expected different IDs for Foo and Bar, both got %s", fooID)
	}
	if !strings.HasPrefix(fooID, "T_") {
		t.Errorf("expected ID to start with 'T_', got %s", fooID)
	}
	if len(fooID) != 34 { // "T_" + 32 hex chars
		t.Errorf("expected ID length 34, got %d for %s", len(fooID), fooID)
	}
}

// TestObjectTypeParentIsNilForRoot verifies that a root object type has no parent.
func TestObjectTypeParentIsNilForRoot(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String}`,
		`Foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	ot := result[0].AsObjectType()
	if ot.Parent != nil {
		t.Errorf("expected nil parent for root object type, got %+v", ot.Parent)
	}
	if ot.Name != "Object/Foo" {
		t.Errorf("expected name 'Object/Foo', got %s", ot.Name)
	}
}

// TestObjectTypeParentReference verifies the parent reference in a child type.
func TestObjectTypeParentReference(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String}`,
		`def Bar object {b:Integer} Foo`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	ot := result[0].AsObjectType()
	if ot.Parent == nil {
		t.Fatal("expected non-nil parent for child object type")
	}
	if ot.Parent.Name != "Object/Foo" {
		t.Errorf("expected parent name 'Object/Foo', got %s", ot.Parent.Name)
	}
}

// TestObjectTypeFieldOverride verifies that a child can override parent fields.
func TestObjectTypeFieldOverride(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String,b:Boolean}`,
		`def Bar object {a:Integer} Foo`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	ot := result[0].AsObjectType()
	all := ot.AllFields()
	// a should be overridden to Integer, b inherited as Boolean
	if all.Len() != 2 {
		t.Fatalf("expected 2 fields (a,b), got %d", all.Len())
	}
	aVal, _ := all.Get("a")
	if !strings.Contains(aVal.String(), "Integer") {
		t.Errorf("expected overridden field a to be Integer, got %s", aVal.String())
	}
}

// TestObjectTypeVTypeMatches verifies VType hierarchy matching.
func TestObjectTypeVTypeMatches(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String}`,
		`def Bar object {b:Integer} Foo`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	barType := result[0].VType
	// Bar (Object/Foo/Bar) should match Object
	tObj, _ := engine.NewType("Object")
	if !barType.Matches(tObj) {
		t.Error("Object/Foo/Bar should match Object")
	}
	// Bar (Object/Foo/Bar) should match Object/Foo
	tObjFoo, _ := engine.NewType("Object/Foo")
	if !barType.Matches(tObjFoo) {
		t.Error("Object/Foo/Bar should match Object/Foo")
	}
	// Bar (Object/Foo/Bar) should match Object/Foo/Bar
	tObjFooBar, _ := engine.NewType("Object/Foo/Bar")
	if !barType.Matches(tObjFooBar) {
		t.Error("Object/Foo/Bar should match Object/Foo/Bar")
	}
}

// TestBuiltinTypeFixedIDs verifies that builtin types have stable, fixed IDs.
func TestBuiltinTypeFixedIDs(t *testing.T) {
	// Builtin types must have non-empty fixed IDs
	if engine.TAny.ID == "" {
		t.Error("TAny should have a fixed ID")
	}
	if engine.TString.ID == "" {
		t.Error("TString should have a fixed ID")
	}
	if engine.TList.ID == "" {
		t.Error("TList should have a fixed ID")
	}
	if engine.TWord.ID == "" {
		t.Error("TWord should have a fixed ID")
	}
	if engine.TObject.ID == "" {
		t.Error("TObject should have a fixed ID")
	}

	// Fixed IDs must be 34 chars (prefix + 32 hex)
	if len(engine.TAny.ID) != 34 {
		t.Errorf("TAny ID should be 34 chars, got %d: %s", len(engine.TAny.ID), engine.TAny.ID)
	}

	// Correct prefixes
	if !strings.HasPrefix(engine.TAny.ID, "T_") {
		t.Errorf("TAny ID should start with T_, got %s", engine.TAny.ID)
	}
	if !strings.HasPrefix(engine.TString.ID, "S_") {
		t.Errorf("TString ID should start with S_, got %s", engine.TString.ID)
	}
	if !strings.HasPrefix(engine.TList.ID, "N_") {
		t.Errorf("TList ID should start with N_, got %s", engine.TList.ID)
	}
	if !strings.HasPrefix(engine.TWord.ID, "W_") {
		t.Errorf("TWord ID should start with W_, got %s", engine.TWord.ID)
	}
	if !strings.HasPrefix(engine.TObject.ID, "T_") {
		t.Errorf("TObject ID should start with T_, got %s", engine.TObject.ID)
	}

	// Specific known values: TAny=1, TNone=2, TScalar=3, TString=4
	expectedAny := "T_00000000000000000000000000000001"
	if engine.TAny.ID != expectedAny {
		t.Errorf("TAny ID should be %s, got %s", expectedAny, engine.TAny.ID)
	}
	expectedNone := "T_00000000000000000000000000000002"
	if engine.TNone.ID != expectedNone {
		t.Errorf("TNone ID should be %s, got %s", expectedNone, engine.TNone.ID)
	}
	expectedString := "S_00000000000000000000000000000004"
	if engine.TString.ID != expectedString {
		t.Errorf("TString ID should be %s, got %s", expectedString, engine.TString.ID)
	}

	// IDs are stable across multiple accesses (no regeneration)
	id1 := engine.TAny.ID
	id2 := engine.TAny.ID
	if id1 != id2 {
		t.Errorf("TAny ID should be stable, got %s then %s", id1, id2)
	}

	// All builtin IDs are unique
	ids := map[string]string{}
	builtins := map[string]engine.Type{
		"TAny": engine.TAny, "TNone": engine.TNone, "TScalar": engine.TScalar,
		"TString": engine.TString, "TStringProper": engine.TStringProper,
		"TStringEmpty": engine.TStringEmpty, "TNumber": engine.TNumber,
		"TInteger": engine.TInteger, "TDecimal": engine.TDecimal,
		"TBoolean": engine.TBoolean, "TNode": engine.TNode,
		"TList": engine.TList, "TListArgs": engine.TListArgs,
		"TMap": engine.TMap, "TTable": engine.TTable, "TRecord": engine.TRecord,
		"TAtom": engine.TAtom, "TWord": engine.TWord, "TFunction": engine.TFunction,
		"TObject": engine.TObject,
	}
	for name, typ := range builtins {
		if prev, exists := ids[typ.ID]; exists {
			t.Errorf("duplicate ID: %s and %s both have %s", prev, name, typ.ID)
		}
		ids[typ.ID] = name
	}

	// Runtime-created types should NOT have fixed IDs
	rt, _ := engine.NewType("Scalar/String/Custom")
	if rt.ID != "" {
		t.Errorf("runtime type should have empty ID, got %s", rt.ID)
	}
}

// TestValueIDPrefixes verifies that all value categories get the correct ID prefix.
func TestValueIDPrefixes(t *testing.T) {
	// Scalar values get S_ prefix
	str := engine.NewString("hello")
	if !strings.HasPrefix(str.ID, "S_") {
		t.Errorf("string ID should start with S_, got %s", str.ID)
	}
	if len(str.ID) != 34 { // "S_" + 32 hex chars
		t.Errorf("string ID should be 34 chars, got %d: %s", len(str.ID), str.ID)
	}

	num := engine.NewInteger(42)
	if !strings.HasPrefix(num.ID, "S_") {
		t.Errorf("integer ID should start with S_, got %s", num.ID)
	}

	dec := engine.NewDecimal(3.14)
	if !strings.HasPrefix(dec.ID, "S_") {
		t.Errorf("decimal ID should start with S_, got %s", dec.ID)
	}

	boolv := engine.NewBoolean(true)
	if !strings.HasPrefix(boolv.ID, "S_") {
		t.Errorf("boolean ID should start with S_, got %s", boolv.ID)
	}

	// Node values get N_ prefix
	list := engine.NewList([]engine.Value{})
	if !strings.HasPrefix(list.ID, "N_") {
		t.Errorf("list ID should start with N_, got %s", list.ID)
	}

	m := engine.NewMap(engine.NewOrderedMap())
	if !strings.HasPrefix(m.ID, "N_") {
		t.Errorf("map ID should start with N_, got %s", m.ID)
	}

	// Word values get W_ prefix
	word := engine.NewWord("test")
	if !strings.HasPrefix(word.ID, "W_") {
		t.Errorf("word ID should start with W_, got %s", word.ID)
	}

	atom := engine.NewAtom("foo")
	if !strings.HasPrefix(atom.ID, "W_") {
		t.Errorf("atom ID should start with W_, got %s", atom.ID)
	}

	// Type/Object values get T_ prefix
	typeLit := engine.NewTypeLiteral(engine.TString)
	if !strings.HasPrefix(typeLit.ID, "S_") {
		t.Errorf("string type literal ID should start with S_ (type's own category), got %s", typeLit.ID)
	}

	noneLit := engine.NewTypeLiteral(engine.TNone)
	if !strings.HasPrefix(noneLit.ID, "T_") {
		t.Errorf("none type literal ID should start with T_, got %s", noneLit.ID)
	}

	// All IDs should be unique
	ids := map[string]bool{str.ID: true, num.ID: true, dec.ID: true, boolv.ID: true,
		list.ID: true, m.ID: true, word.ID: true, atom.ID: true}
	if len(ids) != 8 {
		t.Errorf("expected 8 unique IDs, got %d (some duplicates)", len(ids))
	}
}

// TestObjectTypeNonObjectParentIgnored verifies that when the second arg
// doesn't match TObject, object uses the 1-arg signature (map only).
func TestObjectTypeNonObjectParentIgnored(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`object {d:Integer} 42`,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The 1-arg signature matches: object gets {d:Integer}, 42 stays on stack.
	if len(result) != 2 {
		t.Fatalf("expected 2 results (object type + 42), got %d", len(result))
	}
	if !result[0].IsObjectType() {
		t.Errorf("expected first result to be object type, got %s", result[0].String())
	}
}
