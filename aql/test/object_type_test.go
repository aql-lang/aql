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
	if len(fooID) != 14 { // "T_" + 12 hex chars
		t.Errorf("expected ID length 14, got %d for %s", len(fooID), fooID)
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

// TestObjectTypeFieldOverride verifies that a child can narrow parent fields.
func TestObjectTypeFieldOverride(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:Number,b:Boolean}`,
		`def Bar object {a:Integer} Foo`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	ot := result[0].AsObjectType()
	all := ot.AllFields()
	// a should be narrowed to Integer, b inherited as Boolean
	if all.Len() != 2 {
		t.Fatalf("expected 2 fields (a,b), got %d", all.Len())
	}
	aVal, _ := all.Get("a")
	if !strings.Contains(aVal.String(), "Integer") {
		t.Errorf("expected narrowed field a to be Integer, got %s", aVal.String())
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

	// Fixed IDs must be 14 chars (prefix + 12 hex)
	if len(engine.TAny.ID) != 14 {
		t.Errorf("TAny ID should be 14 chars, got %d: %s", len(engine.TAny.ID), engine.TAny.ID)
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
	expectedAny := "T_000000000001"
	if engine.TAny.ID != expectedAny {
		t.Errorf("TAny ID should be %s, got %s", expectedAny, engine.TAny.ID)
	}
	expectedNone := "T_000000000002"
	if engine.TNone.ID != expectedNone {
		t.Errorf("TNone ID should be %s, got %s", expectedNone, engine.TNone.ID)
	}
	expectedString := "S_000000000004"
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
	if len(str.ID) != 14 { // "S_" + 12 hex chars
		t.Errorf("string ID should be 14 chars, got %d: %s", len(str.ID), str.ID)
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

// --- make object tests ---

// objFields is a test helper that extracts fields from an object instance result.
func objFields(t *testing.T, result []engine.Value) *engine.OrderedMap {
	t.Helper()
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	v := result[0]
	if !v.IsObjectInstance() {
		t.Fatalf("expected object instance, got %s", v.String())
	}
	oi := v.AsObjectInstance()
	return oi.AllFields()
}

// TestMakeObjectBasic creates an object instance with type-literal fields.
func TestMakeObjectBasic(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:String}`,
		`make Foo {x:"hello"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	inst := result[0]
	if !inst.IsObjectInstance() {
		t.Fatalf("expected object instance, got %s", inst.String())
	}
	oi := inst.AsObjectInstance()
	if oi.TypeRef.Name != "Object/Foo" {
		t.Errorf("expected type ref Object/Foo, got %s", oi.TypeRef.Name)
	}
	v, ok := oi.Fields.Get("x")
	if !ok {
		t.Fatal("missing field x")
	}
	if v.AsString() != "hello" {
		t.Errorf("expected x='hello', got %s", v.AsString())
	}
}

// TestMakeObjectTypeConversion converts field values to match type constraints.
func TestMakeObjectTypeConversion(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:String}`,
		`make Foo {x:42}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, _ := om.Get("x")
	if v.AsString() != "42" {
		t.Errorf("expected x='42' (converted), got %s", v.AsString())
	}
}

// TestMakeObjectDefaultValues uses concrete defaults when fields are omitted.
func TestMakeObjectDefaultValues(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:1}`,
		`make Foo {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, ok := om.Get("x")
	if !ok {
		t.Fatal("missing field x")
	}
	if v.AsInteger() != 1 {
		t.Errorf("expected x=1 (default), got %d", v.AsInteger())
	}
}

// TestMakeObjectOverrideDefault overrides a concrete default with a new value.
func TestMakeObjectOverrideDefault(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:1}`,
		`make Foo {x:2}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, _ := om.Get("x")
	if v.AsInteger() != 2 {
		t.Errorf("expected x=2, got %d", v.AsInteger())
	}
}

// TestMakeObjectMultipleFields handles multiple fields with mixed types.
func TestMakeObjectMultipleFields(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:String,y:Integer}`,
		`make Foo {x:"hi",y:7}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	x, _ := om.Get("x")
	y, _ := om.Get("y")
	if x.AsString() != "hi" {
		t.Errorf("expected x='hi', got %s", x.AsString())
	}
	if y.AsInteger() != 7 {
		t.Errorf("expected y=7, got %d", y.AsInteger())
	}
}

// TestMakeObjectMixedDefaultsAndTypes mixes type-literal and concrete-default fields.
func TestMakeObjectMixedDefaultsAndTypes(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:String,y:10}`,
		`make Foo {x:"hi"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	x, _ := om.Get("x")
	y, _ := om.Get("y")
	if x.AsString() != "hi" {
		t.Errorf("expected x='hi', got %s", x.AsString())
	}
	if y.AsInteger() != 10 {
		t.Errorf("expected y=10 (default), got %d", y.AsInteger())
	}
}

// TestMakeObjectUnknownFieldError rejects unknown fields.
func TestMakeObjectUnknownFieldError(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:String}`,
		`make Foo {x:"hi",z:1}`,
	})
	if err == nil {
		t.Fatal("expected error for unknown field z")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("expected 'unknown field' error, got: %s", err)
	}
}

// TestMakeObjectMissingRequiredFieldError rejects missing type-literal fields.
func TestMakeObjectMissingRequiredFieldError(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:String}`,
		`make Foo {}`,
	})
	if err == nil {
		t.Fatal("expected error for missing required field x")
	}
	if !strings.Contains(err.Error(), "missing field") {
		t.Errorf("expected 'missing field' error, got: %s", err)
	}
}

// TestMakeObjectNonMapSourceError rejects non-map source values.
func TestMakeObjectNonMapSourceError(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:String}`,
		`make Foo [1 2 3]`,
	})
	if err == nil {
		t.Fatal("expected error for non-map source")
	}
	if !strings.Contains(err.Error(), "must be a map") {
		t.Errorf("expected 'must be a map' error, got: %s", err)
	}
}

// TestMakeObjectEmptyMapAllDefaults creates instance with all-default fields.
func TestMakeObjectEmptyMapAllDefaults(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:1,y:"default"}`,
		`make Foo {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	x, _ := om.Get("x")
	y, _ := om.Get("y")
	if x.AsInteger() != 1 {
		t.Errorf("expected x=1, got %d", x.AsInteger())
	}
	if y.AsString() != "default" {
		t.Errorf("expected y='default', got %s", y.AsString())
	}
}

// TestMakeObjectInheritedFields creates instance of child type with parent fields.
func TestMakeObjectInheritedFields(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String,b:Integer}`,
		`def Bar object {c:Boolean} Foo`,
		`make Bar {a:"hi",b:3,c:true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	a, _ := om.Get("a")
	b, _ := om.Get("b")
	c, _ := om.Get("c")
	if a.AsString() != "hi" {
		t.Errorf("expected a='hi', got %s", a.AsString())
	}
	if b.AsInteger() != 3 {
		t.Errorf("expected b=3, got %d", b.AsInteger())
	}
	if !c.Data.(bool) {
		t.Error("expected c=true")
	}
}

// TestMakeObjectInheritedDefaults uses parent defaults in child type.
func TestMakeObjectInheritedDefaults(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:1,b:2}`,
		`def Bar object {c:3} Foo`,
		`make Bar {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	a, _ := om.Get("a")
	b, _ := om.Get("b")
	c, _ := om.Get("c")
	if a.AsInteger() != 1 {
		t.Errorf("expected a=1, got %d", a.AsInteger())
	}
	if b.AsInteger() != 2 {
		t.Errorf("expected b=2, got %d", b.AsInteger())
	}
	if c.AsInteger() != 3 {
		t.Errorf("expected c=3, got %d", c.AsInteger())
	}
}

// TestMakeObjectInheritedUnknownFieldError rejects fields not in parent or child.
func TestMakeObjectInheritedUnknownFieldError(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:String}`,
		`def Bar object {b:Integer} Foo`,
		`make Bar {a:"hi",b:1,z:99}`,
	})
	if err == nil {
		t.Fatal("expected error for unknown field z")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("expected 'unknown field' error, got: %s", err)
	}
}

// TestMakeObjectOverrideInheritedDefault overrides a parent's default in child instance.
func TestMakeObjectOverrideInheritedDefault(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:1}`,
		`def Bar object {b:String} Foo`,
		`make Bar {a:99,b:"x"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	a, _ := om.Get("a")
	if a.AsInteger() != 99 {
		t.Errorf("expected a=99, got %d", a.AsInteger())
	}
}

// TestMakeObjectStringDefault uses string default value.
func TestMakeObjectStringDefault(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:"hello"}`,
		`make Foo {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, _ := om.Get("x")
	if v.AsString() != "hello" {
		t.Errorf("expected x='hello', got %s", v.AsString())
	}
}

// TestMakeObjectStringDefaultOverride overrides string default with different string.
func TestMakeObjectStringDefaultOverride(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:"hello"}`,
		`make Foo {x:"world"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, _ := om.Get("x")
	if v.AsString() != "world" {
		t.Errorf("expected x='world', got %s", v.AsString())
	}
}

// TestMakeObjectBooleanDefault uses boolean default value.
func TestMakeObjectBooleanDefault(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:true}`,
		`make Foo {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, _ := om.Get("x")
	if !v.Data.(bool) {
		t.Error("expected x=true (default)")
	}
}

// TestMakeObjectBooleanDefaultOverride overrides boolean default.
func TestMakeObjectBooleanDefaultOverride(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:true}`,
		`make Foo {x:false}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, _ := om.Get("x")
	if v.Data.(bool) {
		t.Error("expected x=false (overridden)")
	}
}

// TestMakeObjectMultipleInstances creates multiple independent instances.
func TestMakeObjectMultipleInstances(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:1}`,
		`make Foo {x:10}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om1 := objFields(t, result)

	result2, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:1}`,
		`make Foo {x:20}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om2 := objFields(t, result2)

	v1, _ := om1.Get("x")
	v2, _ := om2.Get("x")
	if v1.AsInteger() == v2.AsInteger() {
		t.Error("expected independent instances with different values")
	}
}

// TestMakeObjectOnlyUnknownFieldsError rejects when only unknown fields given.
func TestMakeObjectOnlyUnknownFieldsError(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:String}`,
		`make Foo {z:"hi"}`,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("expected 'unknown field' error, got: %s", err)
	}
}

// TestMakeObjectFieldOrderPreserved verifies field order matches type definition.
func TestMakeObjectFieldOrderPreserved(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:1,b:2,c:3}`,
		`make Foo {c:30,a:10,b:20}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	keys := om.Keys()
	// Fields should be in definition order, not input order.
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("expected field order [a,b,c], got %v", keys)
	}
}

// TestMakeObjectDeepInheritance tests 3-level inheritance chain.
func TestMakeObjectDeepInheritance(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def A object {x:1}`,
		`def B object {y:2} A`,
		`def C object {z:3} B`,
		`make C {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	x, _ := om.Get("x")
	y, _ := om.Get("y")
	z, _ := om.Get("z")
	if x.AsInteger() != 1 || y.AsInteger() != 2 || z.AsInteger() != 3 {
		t.Errorf("expected x=1,y=2,z=3, got x=%d,y=%d,z=%d", x.AsInteger(), y.AsInteger(), z.AsInteger())
	}
}

// TestMakeObjectChildOverridesParentConcreteRejected tests that a child cannot
// replace one concrete value with a different concrete value (99 vs 1).
func TestMakeObjectChildOverridesParentConcreteRejected(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:1}`,
		`def Bar object {x:99} Foo`,
	})
	if err == nil {
		t.Fatal("expected error: child concrete 99 cannot replace parent concrete 1")
	}
	if !strings.Contains(err.Error(), "cannot expand") {
		t.Errorf("expected 'cannot expand' error, got: %s", err)
	}
}

// TestMakeObjectInstanceTypeMatchesObjectType verifies instance type path matches its type.
func TestMakeObjectInstanceTypeMatchesObjectType(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:1}`,
		`make Foo {x:5}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	inst := result[0]
	if !inst.VType.Matches(engine.TObject) {
		t.Errorf("expected instance type to match TObject, got %s", inst.VType)
	}
	oi := inst.AsObjectInstance()
	if oi.TypeRef.Name != "Object/Foo" {
		t.Errorf("expected TypeRef.Name='Object/Foo', got %s", oi.TypeRef.Name)
	}
}

// TestMakeObjectInstanceChildTypeRef verifies child instance references child type.
func TestMakeObjectInstanceChildTypeRef(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {a:1}`,
		`def Bar object {b:2} Foo`,
		`make Bar {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi := result[0].AsObjectInstance()
	if oi.TypeRef.Name != "Object/Foo/Bar" {
		t.Errorf("expected TypeRef.Name='Object/Foo/Bar', got %s", oi.TypeRef.Name)
	}
	if oi.TypeRef.Parent == nil {
		t.Fatal("expected child TypeRef to have a parent")
	}
	if oi.TypeRef.Parent.Name != "Object/Foo" {
		t.Errorf("expected parent name='Object/Foo', got %s", oi.TypeRef.Parent.Name)
	}
}

// TestMakeObjectInstanceStringFormat verifies the String() representation.
func TestMakeObjectInstanceStringFormat(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:1}`,
		`make Foo {x:5}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := result[0].String()
	if !strings.Contains(s, "Object/Foo") {
		t.Errorf("expected String() to contain 'Object/Foo', got %s", s)
	}
	if !strings.Contains(s, "x:5") {
		t.Errorf("expected String() to contain 'x:5', got %s", s)
	}
}

// --- prototype tests ---

// TestMakeObjectPrototypeBasic creates a child instance with an explicit prototype.
func TestMakeObjectPrototypeBasic(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:1}`,
		`def Bar object {y:String} Foo`,
		`make Bar {y:"A"} foo1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi := result[0].AsObjectInstance()
	allF := oi.AllFields()
	y, _ := allF.Get("y")
	x, _ := allF.Get("x")
	if y.AsString() != "A" {
		t.Errorf("expected y='A', got %s", y.AsString())
	}
	if x.AsInteger() != 1 {
		t.Errorf("expected x=1 (from prototype), got %d", x.AsInteger())
	}
}

// TestMakeObjectPrototypeChainRef verifies the prototype pointer is set correctly.
func TestMakeObjectPrototypeChainRef(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:42}`,
		`def Bar object {y:String} Foo`,
		`make Bar {y:"hi"} foo1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi := result[0].AsObjectInstance()
	if oi.Prototype == nil {
		t.Fatal("expected prototype to be set")
	}
	if oi.Prototype.TypeRef.Name != "Object/Foo" {
		t.Errorf("expected prototype type Object/Foo, got %s", oi.Prototype.TypeRef.Name)
	}
	px, _ := oi.Prototype.Fields.Get("x")
	if px.AsInteger() != 42 {
		t.Errorf("expected prototype x=42, got %d", px.AsInteger())
	}
}

// TestMakeObjectAutoPrototypeBaseValues verifies that a child without explicit
// prototype auto-creates a parent instance with base values.
func TestMakeObjectAutoPrototypeBaseValues(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def Bar object {y:String} Foo`,
		`make Bar {y:"test"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi := result[0].AsObjectInstance()
	if oi.Prototype == nil {
		t.Fatal("expected auto-created prototype")
	}
	allF := oi.AllFields()
	x, _ := allF.Get("x")
	if x.AsInteger() != 0 {
		t.Errorf("expected auto-prototype x=0 (base), got %d", x.AsInteger())
	}
}

// TestMakeObjectAutoPrototypeWithDefaults verifies auto-prototype uses
// concrete defaults from the parent type definition.
func TestMakeObjectAutoPrototypeWithDefaults(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:10}`,
		`def Bar object {y:String} Foo`,
		`make Bar {y:"test"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	allF := result[0].AsObjectInstance().AllFields()
	x, _ := allF.Get("x")
	if x.AsInteger() != 10 {
		t.Errorf("expected auto-prototype x=10 (default), got %d", x.AsInteger())
	}
}

// TestMakeObjectPrototypeOverrideInherited overrides an inherited field via make source.
func TestMakeObjectPrototypeOverrideInherited(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:1}`,
		`def Bar object {y:String} Foo`,
		`make Bar {y:"A",x:99} foo1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	allF := result[0].AsObjectInstance().AllFields()
	x, _ := allF.Get("x")
	if x.AsInteger() != 99 {
		t.Errorf("expected x=99 (overridden), got %d", x.AsInteger())
	}
}

// TestMakeObjectPrototypeGetField tests GetField on the prototype chain.
func TestMakeObjectPrototypeGetField(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:7}`,
		`def Bar object {y:String} Foo`,
		`make Bar {y:"hi"} foo1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi := result[0].AsObjectInstance()
	x, ok := oi.GetField("x")
	if !ok {
		t.Fatal("expected GetField to find x via prototype")
	}
	if x.AsInteger() != 7 {
		t.Errorf("expected x=7, got %d", x.AsInteger())
	}
	y, ok := oi.GetField("y")
	if !ok {
		t.Fatal("expected GetField to find y directly")
	}
	if y.AsString() != "hi" {
		t.Errorf("expected y='hi', got %s", y.AsString())
	}
}

// --- field narrowing tests ---

// TestObjectTypeFieldNarrowingAllowed verifies a child can narrow a parent field.
func TestObjectTypeFieldNarrowingAllowed(t *testing.T) {
	// Integer is narrower than Number — should be allowed.
	_, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:Number}`,
		`def Bar object {x:Integer} Foo`,
	})
	if err != nil {
		t.Fatalf("narrowing Number→Integer should be allowed: %s", err)
	}
}

// TestObjectTypeFieldNarrowingConcreteAllowed verifies concrete narrows type literal.
func TestObjectTypeFieldNarrowingConcreteAllowed(t *testing.T) {
	// Concrete 42 narrows Integer — should be allowed.
	_, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def Bar object {x:42} Foo`,
	})
	if err != nil {
		t.Fatalf("narrowing Integer→42 should be allowed: %s", err)
	}
}

// TestObjectTypeFieldExpandingRejected verifies a child cannot expand a parent field type.
func TestObjectTypeFieldExpandingRejected(t *testing.T) {
	// String does not unify with Integer — should be rejected.
	_, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def Bar object {x:String} Foo`,
	})
	if err == nil {
		t.Fatal("expected error for expanding Integer→String")
	}
	if !strings.Contains(err.Error(), "cannot expand") {
		t.Errorf("expected 'cannot expand' error, got: %s", err)
	}
}

// TestObjectTypeFieldExpandingConcreteRejected rejects incompatible concrete override.
func TestObjectTypeFieldExpandingConcreteRejected(t *testing.T) {
	// "hello" (string) does not unify with Integer.
	_, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def Bar object {x:"hello"} Foo`,
	})
	if err == nil {
		t.Fatal("expected error for incompatible concrete override")
	}
	if !strings.Contains(err.Error(), "cannot expand") {
		t.Errorf("expected 'cannot expand' error, got: %s", err)
	}
}

// --- deep inheritance tests (7 levels) ---

// TestObjectTypeDeep7Levels tests 7-level type hierarchy definition.
func TestObjectTypeDeep7Levels(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def L1 object {a:Integer}`,
		`def L2 object {b:String} L1`,
		`def L3 object {c:Boolean} L2`,
		`def L4 object {d:Integer} L3`,
		`def L5 object {e:String} L4`,
		`def L6 object {f:Boolean} L5`,
		`def L7 object {g:Integer} L6`,
	})
	if err != nil {
		t.Fatalf("7-level type hierarchy should succeed: %s", err)
	}
}

// TestMakeObjectDeep7LevelsAllDefaults tests 7-level instance with all defaults.
func TestMakeObjectDeep7LevelsAllDefaults(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def L1 object {a:1}`,
		`def L2 object {b:"two"} L1`,
		`def L3 object {c:true} L2`,
		`def L4 object {d:4} L3`,
		`def L5 object {e:"five"} L4`,
		`def L6 object {f:false} L5`,
		`def L7 object {g:7} L6`,
		`make L7 {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	allF := result[0].AsObjectInstance().AllFields()
	checks := map[string]interface{}{
		"a": int64(1), "b": "two", "c": true, "d": int64(4),
		"e": "five", "f": false, "g": int64(7),
	}
	for k, expected := range checks {
		v, ok := allF.Get(k)
		if !ok {
			t.Errorf("missing field %s", k)
			continue
		}
		switch exp := expected.(type) {
		case int64:
			if v.AsInteger() != exp {
				t.Errorf("field %s: expected %d, got %d", k, exp, v.AsInteger())
			}
		case string:
			if v.AsString() != exp {
				t.Errorf("field %s: expected %q, got %q", k, exp, v.AsString())
			}
		case bool:
			if v.Data.(bool) != exp {
				t.Errorf("field %s: expected %v, got %v", k, exp, v.Data)
			}
		}
	}
}

// TestMakeObjectDeep7LevelsPrototypeChain tests 7-level prototype chain.
func TestMakeObjectDeep7LevelsPrototypeChain(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def L1 object {a:Integer}`,
		`def l1 make L1 {a:10}`,
		`def L2 object {b:String} L1`,
		`def l2 make L2 {b:"twenty"} l1`,
		`def L3 object {c:Boolean} L2`,
		`def l3 make L3 {c:true} l2`,
		`def L4 object {d:Integer} L3`,
		`def l4 make L4 {d:40} l3`,
		`def L5 object {e:String} L4`,
		`def l5 make L5 {e:"fifty"} l4`,
		`def L6 object {f:Boolean} L5`,
		`def l6 make L6 {f:false} l5`,
		`def L7 object {g:Integer} L6`,
		`make L7 {g:70} l6`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi := result[0].AsObjectInstance()
	allF := oi.AllFields()
	checks := map[string]interface{}{
		"a": int64(10), "b": "twenty", "c": true, "d": int64(40),
		"e": "fifty", "f": false, "g": int64(70),
	}
	for k, expected := range checks {
		v, ok := allF.Get(k)
		if !ok {
			t.Errorf("missing field %s", k)
			continue
		}
		switch exp := expected.(type) {
		case int64:
			if v.AsInteger() != exp {
				t.Errorf("field %s: expected %d, got %d", k, exp, v.AsInteger())
			}
		case string:
			if v.AsString() != exp {
				t.Errorf("field %s: expected %q, got %q", k, exp, v.AsString())
			}
		case bool:
			if v.Data.(bool) != exp {
				t.Errorf("field %s: expected %v, got %v", k, exp, v.Data)
			}
		}
	}
}

// TestMakeObjectDeep7LevelsPrototypeDepth verifies prototype chain has correct depth.
func TestMakeObjectDeep7LevelsPrototypeDepth(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def L1 object {a:Integer}`,
		`def l1 make L1 {a:1}`,
		`def L2 object {b:String} L1`,
		`def l2 make L2 {b:"x"} l1`,
		`def L3 object {c:Boolean} L2`,
		`def l3 make L3 {c:true} l2`,
		`def L4 object {d:Integer} L3`,
		`def l4 make L4 {d:4} l3`,
		`def L5 object {e:String} L4`,
		`def l5 make L5 {e:"y"} l4`,
		`def L6 object {f:Boolean} L5`,
		`def l6 make L6 {f:false} l5`,
		`def L7 object {g:Integer} L6`,
		`make L7 {g:7} l6`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi := result[0].AsObjectInstance()
	depth := 0
	for p := oi.Prototype; p != nil; p = p.Prototype {
		depth++
	}
	if depth != 6 {
		t.Errorf("expected prototype chain depth=6, got %d", depth)
	}
}

// TestMakeObjectDeep7GrandparentFieldAccess verifies field access from grandparent+.
func TestMakeObjectDeep7GrandparentFieldAccess(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def L1 object {a:Integer}`,
		`def l1 make L1 {a:100}`,
		`def L2 object {b:String} L1`,
		`def l2 make L2 {b:"hi"} l1`,
		`def L3 object {c:Boolean} L2`,
		`def l3 make L3 {c:true} l2`,
		`def L4 object {d:Integer} L3`,
		`make L4 {d:999} l3`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi := result[0].AsObjectInstance()

	// GetField should find a from great-grandparent (L1).
	a, ok := oi.GetField("a")
	if !ok {
		t.Fatal("expected GetField to find 'a' from L1 via prototype chain")
	}
	if a.AsInteger() != 100 {
		t.Errorf("expected a=100, got %d", a.AsInteger())
	}

	// GetField should find b from grandparent (L2).
	b, ok := oi.GetField("b")
	if !ok {
		t.Fatal("expected GetField to find 'b' from L2 via prototype chain")
	}
	if b.AsString() != "hi" {
		t.Errorf("expected b='hi', got %s", b.AsString())
	}
}

// TestMakeObjectDeep7OverrideGrandparentField overrides grandparent field at make time.
func TestMakeObjectDeep7OverrideGrandparentField(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def L1 object {a:Integer}`,
		`def l1 make L1 {a:1}`,
		`def L2 object {b:String} L1`,
		`def l2 make L2 {b:"x"} l1`,
		`def L3 object {c:Boolean} L2`,
		// Override grandparent field a at L3 make time.
		`make L3 {c:true,a:999} l2`,
	})
	if err != nil {
		t.Fatal(err)
	}
	allF := result[0].AsObjectInstance().AllFields()
	a, _ := allF.Get("a")
	if a.AsInteger() != 999 {
		t.Errorf("expected a=999 (overridden grandparent), got %d", a.AsInteger())
	}
}

// TestMakeObjectDeep7NarrowingChain tests narrowing through multiple levels.
func TestMakeObjectDeep7NarrowingChain(t *testing.T) {
	// L1: x:Number, L2: x:Integer (narrows Number), L3: x:42 (narrows Integer)
	result, err := runNativeSteps(t, nil, []string{
		`def L1 object {x:Number}`,
		`def L2 object {x:Integer} L1`,
		`def L3 object {x:42} L2`,
		`make L3 {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	allF := result[0].AsObjectInstance().AllFields()
	x, _ := allF.Get("x")
	if x.AsInteger() != 42 {
		t.Errorf("expected x=42 (narrowed default), got %d", x.AsInteger())
	}
}

// TestMakeObjectDeep7AutoPrototypeStringFormat tests String output with deep auto-prototype.
func TestMakeObjectDeep7AutoPrototypeStringFormat(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def L1 object {a:1}`,
		`def L2 object {b:2} L1`,
		`def L3 object {c:3} L2`,
		`make L3 {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := result[0].String()
	if !strings.Contains(s, "a:1") {
		t.Errorf("expected String to contain 'a:1', got %s", s)
	}
	if !strings.Contains(s, "b:2") {
		t.Errorf("expected String to contain 'b:2', got %s", s)
	}
	if !strings.Contains(s, "c:3") {
		t.Errorf("expected String to contain 'c:3', got %s", s)
	}
}

// TestMakeObjectPrototypeDotAccess verifies the full prototype example:
// define Foo, create foo1, define Bar extending Foo, create barA with foo1
// as prototype, then access fields via dot notation.
func TestMakeObjectPrototypeDotAccess(t *testing.T) {
	// foo1.x => 1
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:1}`,
		`foo1 x dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result[0].AsInteger() != 1 {
		t.Errorf("expected foo1.x=1, got %d", result[0].AsInteger())
	}

	// barA.y => 'A'
	result, err = runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:1}`,
		`def Bar object {y:String} Foo`,
		`def barA make Bar {y:"A"} foo1`,
		`barA y dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result[0].AsString() != "A" {
		t.Errorf("expected barA.y='A', got %s", result[0].AsString())
	}

	// barA.x => 1 (from prototype foo1)
	result, err = runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:1}`,
		`def Bar object {y:String} Foo`,
		`def barA make Bar {y:"A"} foo1`,
		`barA x dot`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result[0].AsInteger() != 1 {
		t.Errorf("expected barA.x=1 (from prototype foo1), got %d", result[0].AsInteger())
	}
}

// TestMakeObjectPrototypeDotAccessEndToEnd runs the full prototype example
// as a single program: define Foo, create foo1, define Bar extending Foo,
// create barA with foo1 as prototype, then print each dot-access result.
func TestMakeObjectPrototypeDotAccessEndToEnd(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:1}`,
		`foo1.x`,
		`def Bar object {y:String} Foo`,
		`def barA make Bar {y:"A"} foo1`,
		`barA.y`,
		`barA.x`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// barA.x is the last step, so result comes from that.
	if result[0].AsInteger() != 1 {
		t.Errorf("expected barA.x=1 (inherited from prototype foo1), got %d", result[0].AsInteger())
	}

	// Also verify each step individually in a single shared engine.
	var results []string
	result, err = runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:1}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// foo1.x => 1
	result, err = runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:1}`,
		`foo1.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	results = append(results, result[0].String())

	// barA.y => A
	result, err = runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:1}`,
		`def Bar object {y:String} Foo`,
		`def barA make Bar {y:"A"} foo1`,
		`barA.y`,
	})
	if err != nil {
		t.Fatal(err)
	}
	results = append(results, result[0].String())

	// barA.x => 1
	result, err = runNativeSteps(t, nil, []string{
		`def Foo object {x:Integer}`,
		`def foo1 make Foo {x:1}`,
		`def Bar object {y:String} Foo`,
		`def barA make Bar {y:"A"} foo1`,
		`barA.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	results = append(results, result[0].String())

	// Verify: 1, 'A', 1 (strings include quotes in String() output)
	want := []string{"1", "'A'", "1"}
	for i, w := range want {
		if results[i] != w {
			t.Errorf("step %d: got %q, want %q", i, results[i], w)
		}
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
