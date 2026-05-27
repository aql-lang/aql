package test

import (
	"github.com/aql-lang/aql/lang/go/native"
	"testing"
)

// TestResourceTypeDefine defines the Resrc record type and verifies
// it is recognized as a record type.
func TestResourceTypeDefine(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Resrc refine Record [name:String kind:String meta:Map]`,
		`Resrc`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	s := result[0].String()
	if s != "record{name:String,kind:String,meta:Map}" {
		t.Errorf("unexpected type string: %s", s)
	}
}

// TestResourceTypeMakePositional creates a Resrc using positional fields.
func TestResourceTypeMakePositional(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Resrc refine Record [name:String kind:String meta:Map]`,
		`make Resrc ["users" "entity" {table:"usr"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := native.AsMap(result[0])
	name, _ := m.Get("name")
	kind, _ := m.Get("kind")
	meta, _ := m.Get("meta")
	nameS, _ := native.AsString(name)
	kindS, _ := native.AsString(kind)
	if nameS != "users" {
		t.Errorf("expected name='users', got %s", name)
	}
	if kindS != "entity" {
		t.Errorf("expected kind='entity', got %s", kind)
	}
	mm, _ := native.AsMap(meta)
	tbl, _ := mm.Get("table")
	tblS, _ := native.AsString(tbl)
	if tblS != "usr" {
		t.Errorf("expected meta.table='usr', got %s", tbl)
	}
}

// TestResourceTypeMakeNamed creates a Resrc using named fields.
func TestResourceTypeMakeNamed(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Resrc refine Record [name:String kind:String meta:Map]`,
		`make Resrc [name:"users" kind:"entity" meta:{table:"usr"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := native.AsMap(result[0])
	name, _ := m.Get("name")
	kind, _ := m.Get("kind")
	nameS2, _ := native.AsString(name)
	kindS2, _ := native.AsString(kind)
	if nameS2 != "users" {
		t.Errorf("expected name='users', got %s", name)
	}
	if kindS2 != "entity" {
		t.Errorf("expected kind='entity', got %s", kind)
	}
}

// TestResourceTypeMakeNamedReorder creates a Resrc with fields in
// a different order from the type definition.
func TestResourceTypeMakeNamedReorder(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Resrc refine Record [name:String kind:String meta:Map]`,
		`make Resrc [meta:{x:1} name:"foo" kind:"bar"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := native.AsMap(result[0])
	name, _ := m.Get("name")
	kind, _ := m.Get("kind")
	meta, _ := m.Get("meta")
	nameS3, _ := native.AsString(name)
	kindS3, _ := native.AsString(kind)
	if nameS3 != "foo" {
		t.Errorf("expected name='foo', got %s", name)
	}
	if kindS3 != "bar" {
		t.Errorf("expected kind='bar', got %s", kind)
	}
	mm, _ := native.AsMap(meta)
	x, _ := mm.Get("x")
	xi, _ := native.AsInteger(x)
	if xi != 1 {
		t.Errorf("expected meta.x=1, got %v", x)
	}
}

// TestResourceTypeTable creates a table of Resrc records and lists them.
func TestResourceTypeTable(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Resrc refine Record [name:String kind:String meta:Map]`,
		`def Resrcs refine Table Resrc`,
		`make Resrcs [["users" "entity" {table:"usr"}] ["roles" "entity" {table:"role"}]]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	r0, _ := native.AsMap(rows[0])
	name0, _ := r0.Get("name")
	name0S, _ := native.AsString(name0)
	if name0S != "users" {
		t.Errorf("expected row 0 name='users', got %s", name0)
	}
	r1, _ := native.AsMap(rows[1])
	name1, _ := r1.Get("name")
	name1S, _ := native.AsString(name1)
	if name1S != "roles" {
		t.Errorf("expected row 1 name='roles', got %s", name1)
	}
}

// ==========================================================================
// Resrc/entity type
// ==========================================================================

// TestEntityTypeDefine defines the entity record type (a Resrc/entity)
// and verifies it is recognized as a record type with the correct fields.
func TestEntityTypeDefine(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`Ent`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	s := result[0].String()
	if s != "record{name:String,kind:'entity',meta:Map,entity:Map,model:Map}" {
		t.Errorf("unexpected type string: %s", s)
	}
}

// TestEntityTypeMakePositional creates an entity using positional fields.
func TestEntityTypeMakePositional(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`make Ent ["users" "entity" {table:"usr"} {pk:"id"} {base:"user"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := native.AsMap(result[0])
	name, _ := m.Get("name")
	kind, _ := m.Get("kind")
	meta, _ := m.Get("meta")
	ent, _ := m.Get("entity")
	model, _ := m.Get("model")
	nameS4, _ := native.AsString(name)
	kindS4, _ := native.AsString(kind)
	if nameS4 != "users" {
		t.Errorf("expected name='users', got %s", name)
	}
	if kindS4 != "entity" {
		t.Errorf("expected kind='entity', got %s", kind)
	}
	mm, _ := native.AsMap(meta)
	tbl, _ := mm.Get("table")
	tblS4, _ := native.AsString(tbl)
	if tblS4 != "usr" {
		t.Errorf("expected meta.table='usr', got %s", tbl)
	}
	em, _ := native.AsMap(ent)
	pk, _ := em.Get("pk")
	pkS4, _ := native.AsString(pk)
	if pkS4 != "id" {
		t.Errorf("expected entity.pk='id', got %s", pk)
	}
	mdl, _ := native.AsMap(model)
	base, _ := mdl.Get("base")
	baseS4, _ := native.AsString(base)
	if baseS4 != "user" {
		t.Errorf("expected model.base='user', got %s", base)
	}
}

// TestEntityTypeMakeNamed creates an entity using named fields.
func TestEntityTypeMakeNamed(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`make Ent [name:"orders" kind:"entity" meta:{} entity:{pk:"id"} model:{}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := native.AsMap(result[0])
	name, _ := m.Get("name")
	kind, _ := m.Get("kind")
	ent, _ := m.Get("entity")
	nameS5, _ := native.AsString(name)
	kindS5, _ := native.AsString(kind)
	if nameS5 != "orders" {
		t.Errorf("expected name='orders', got %s", name)
	}
	if kindS5 != "entity" {
		t.Errorf("expected kind='entity', got %s", kind)
	}
	em, _ := native.AsMap(ent)
	pk, _ := em.Get("pk")
	pkS5, _ := native.AsString(pk)
	if pkS5 != "id" {
		t.Errorf("expected entity.pk='id', got %s", pk)
	}
}

// TestEntityTypeKindConstraint verifies the kind field is constrained to "entity".
func TestEntityTypeKindConstraint(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`make Ent ["users" "other" {} {} {}]`,
	})
	if err == nil {
		t.Fatal("expected error when kind is not 'entity', got nil")
	}
}

// TestEntityTypeTable creates a table of entity records.
func TestEntityTypeTable(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`def Ents refine Table Ent`,
		`make Ents [["users" "entity" {} {fields:{}} {}] ["roles" "entity" {} {fields:{}} {}]]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	r0, _ := native.AsMap(rows[0])
	name0, _ := r0.Get("name")
	kind0, _ := r0.Get("kind")
	name0S2, _ := native.AsString(name0)
	kind0S, _ := native.AsString(kind0)
	if name0S2 != "users" {
		t.Errorf("expected row 0 name='users', got %s", name0)
	}
	if kind0S != "entity" {
		t.Errorf("expected row 0 kind='entity', got %s", kind0)
	}
	r1, _ := native.AsMap(rows[1])
	name1, _ := r1.Get("name")
	name1S2, _ := native.AsString(name1)
	if name1S2 != "roles" {
		t.Errorf("expected row 1 name='roles', got %s", name1)
	}
}

// TestEntityTypeWithResourceType defines both Resrc and entity types
// and verifies they coexist and work independently.
func TestEntityTypeWithResourceType(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Resrc refine Record [name:String kind:String meta:Map]`,
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`make Resrc ["config" "setting" {}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rm, _ := native.AsMap(result[0])
	rk, _ := rm.Get("kind")
	rkS, _ := native.AsString(rk)
	if rkS != "setting" {
		t.Errorf("Resrc kind should be 'setting', got %s", rk)
	}

	result2, err := runNativeSteps(t, nil, []string{
		`def Resrc refine Record [name:String kind:String meta:Map]`,
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`make Ent ["users" "entity" {} {fields:{}} {}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	em, _ := native.AsMap(result2[0])
	ek, _ := em.Get("kind")
	ekS, _ := native.AsString(ek)
	if ekS != "entity" {
		t.Errorf("entity kind should be 'entity', got %s", ek)
	}
}

// ==========================================================================
// CRUD operations on entity record type (return empty tables)
// ==========================================================================

func TestEntityTypeList(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`list Ent`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

func TestEntityTypeListFilter(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`Ent list {name:"users"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

func TestEntityTypeCreate(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`Ent create {id:"1" name:"users"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

func TestEntityTypeLoad(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`Ent load {id:"1"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// load on record type returns empty map
	m, _ := native.AsMap(result[0])
	if len(m.Keys()) != 0 {
		t.Errorf("expected empty map, got %d keys", len(m.Keys()))
	}
}

func TestEntityTypeUpdate(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`Ent update {id:"1" name:"users-v2"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

func TestEntityTypeRemove(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Ent refine Record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`Ent remove {id:"1"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

// Test CRUD on the base Resrc type too.
func TestResourceTypeListEmpty(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Resrc refine Record [name:String kind:String meta:Map]`,
		`list Resrc`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

func TestResourceTypeCreateEmpty(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Resrc refine Record [name:String kind:String meta:Map]`,
		`Resrc create {id:"1" name:"foo"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := native.AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

// ==========================================================================
// Aliases
// ==========================================================================

// TestResourceTypeAlias verifies the Resrc type can be aliased via def.
func TestResourceTypeAlias(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Resrc refine Record [name:String kind:String meta:Map]`,
		`def res [Resrc]`,
		`res`,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := result[0].String()
	if s != "record{name:String,kind:String,meta:Map}" {
		t.Errorf("unexpected alias type string: %s", s)
	}
}
