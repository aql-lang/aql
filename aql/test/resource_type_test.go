package test

import (
	"testing"
)

// TestResourceTypeDefine defines the resource record type and verifies
// it is recognized as a record type.
func TestResourceTypeDefine(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:String kind:String meta:Map]`,
		`resource`,
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

// TestResourceTypeMakePositional creates a resource using positional fields.
func TestResourceTypeMakePositional(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:String kind:String meta:Map]`,
		`make resource ["users" "entity" {table:"usr"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	name, _ := m.Get("name")
	kind, _ := m.Get("kind")
	meta, _ := m.Get("meta")
	if name.AsString() != "users" {
		t.Errorf("expected name='users', got %s", name)
	}
	if kind.AsString() != "entity" {
		t.Errorf("expected kind='entity', got %s", kind)
	}
	mm := meta.AsMap()
	tbl, _ := mm.Get("table")
	if tbl.AsString() != "usr" {
		t.Errorf("expected meta.table='usr', got %s", tbl)
	}
}

// TestResourceTypeMakeNamed creates a resource using named fields.
func TestResourceTypeMakeNamed(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:String kind:String meta:Map]`,
		`make resource [name:"users" kind:"entity" meta:{table:"usr"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	name, _ := m.Get("name")
	kind, _ := m.Get("kind")
	if name.AsString() != "users" {
		t.Errorf("expected name='users', got %s", name)
	}
	if kind.AsString() != "entity" {
		t.Errorf("expected kind='entity', got %s", kind)
	}
}

// TestResourceTypeMakeNamedReorder creates a resource with fields in
// a different order from the type definition.
func TestResourceTypeMakeNamedReorder(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:String kind:String meta:Map]`,
		`make resource [meta:{x:1} name:"foo" kind:"bar"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	name, _ := m.Get("name")
	kind, _ := m.Get("kind")
	meta, _ := m.Get("meta")
	if name.AsString() != "foo" {
		t.Errorf("expected name='foo', got %s", name)
	}
	if kind.AsString() != "bar" {
		t.Errorf("expected kind='bar', got %s", kind)
	}
	mm := meta.AsMap()
	x, _ := mm.Get("x")
	if x.AsInteger() != 1 {
		t.Errorf("expected meta.x=1, got %v", x)
	}
}

// TestResourceTypeTable creates a table of resource records and lists them.
func TestResourceTypeTable(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:String kind:String meta:Map]`,
		`type resources table resource`,
		`make resources [["users" "entity" {table:"usr"}] ["roles" "entity" {table:"role"}]]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	r0 := rows[0].AsMap()
	name0, _ := r0.Get("name")
	if name0.AsString() != "users" {
		t.Errorf("expected row 0 name='users', got %s", name0)
	}
	r1 := rows[1].AsMap()
	name1, _ := r1.Get("name")
	if name1.AsString() != "roles" {
		t.Errorf("expected row 1 name='roles', got %s", name1)
	}
}

// ==========================================================================
// resource/entity type
// ==========================================================================

// TestEntityTypeDefine defines the entity record type (a resource/entity)
// and verifies it is recognized as a record type with the correct fields.
func TestEntityTypeDefine(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`entity`,
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
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`make entity ["users" "entity" {table:"usr"} {pk:"id"} {base:"user"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	name, _ := m.Get("name")
	kind, _ := m.Get("kind")
	meta, _ := m.Get("meta")
	ent, _ := m.Get("entity")
	model, _ := m.Get("model")
	if name.AsString() != "users" {
		t.Errorf("expected name='users', got %s", name)
	}
	if kind.AsString() != "entity" {
		t.Errorf("expected kind='entity', got %s", kind)
	}
	mm := meta.AsMap()
	tbl, _ := mm.Get("table")
	if tbl.AsString() != "usr" {
		t.Errorf("expected meta.table='usr', got %s", tbl)
	}
	em := ent.AsMap()
	pk, _ := em.Get("pk")
	if pk.AsString() != "id" {
		t.Errorf("expected entity.pk='id', got %s", pk)
	}
	mdl := model.AsMap()
	base, _ := mdl.Get("base")
	if base.AsString() != "user" {
		t.Errorf("expected model.base='user', got %s", base)
	}
}

// TestEntityTypeMakeNamed creates an entity using named fields.
func TestEntityTypeMakeNamed(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`make entity [name:"orders" kind:"entity" meta:{} entity:{pk:"id"} model:{}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	name, _ := m.Get("name")
	kind, _ := m.Get("kind")
	ent, _ := m.Get("entity")
	if name.AsString() != "orders" {
		t.Errorf("expected name='orders', got %s", name)
	}
	if kind.AsString() != "entity" {
		t.Errorf("expected kind='entity', got %s", kind)
	}
	em := ent.AsMap()
	pk, _ := em.Get("pk")
	if pk.AsString() != "id" {
		t.Errorf("expected entity.pk='id', got %s", pk)
	}
}

// TestEntityTypeKindConstraint verifies the kind field is constrained to "entity".
func TestEntityTypeKindConstraint(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`make entity ["users" "other" {} {} {}]`,
	})
	if err == nil {
		t.Fatal("expected error when kind is not 'entity', got nil")
	}
}

// TestEntityTypeTable creates a table of entity records.
func TestEntityTypeTable(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`type entities table entity`,
		`make entities [["users" "entity" {} {fields:{}} {}] ["roles" "entity" {} {fields:{}} {}]]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	r0 := rows[0].AsMap()
	name0, _ := r0.Get("name")
	kind0, _ := r0.Get("kind")
	if name0.AsString() != "users" {
		t.Errorf("expected row 0 name='users', got %s", name0)
	}
	if kind0.AsString() != "entity" {
		t.Errorf("expected row 0 kind='entity', got %s", kind0)
	}
	r1 := rows[1].AsMap()
	name1, _ := r1.Get("name")
	if name1.AsString() != "roles" {
		t.Errorf("expected row 1 name='roles', got %s", name1)
	}
}

// TestEntityTypeWithResourceType defines both resource and entity types
// and verifies they coexist and work independently.
func TestEntityTypeWithResourceType(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:String kind:String meta:Map]`,
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`make resource ["config" "setting" {}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rm := result[0].AsMap()
	rk, _ := rm.Get("kind")
	if rk.AsString() != "setting" {
		t.Errorf("resource kind should be 'setting', got %s", rk)
	}

	result2, err := runNativeSteps(t, nil, []string{
		`type resource record [name:String kind:String meta:Map]`,
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`make entity ["users" "entity" {} {fields:{}} {}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	em := result2[0].AsMap()
	ek, _ := em.Get("kind")
	if ek.AsString() != "entity" {
		t.Errorf("entity kind should be 'entity', got %s", ek)
	}
}

// ==========================================================================
// CRUD operations on entity record type (return empty tables)
// ==========================================================================

func TestEntityTypeList(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`list entity`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	rows := result[0].AsList()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

func TestEntityTypeListFilter(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`entity list {name:"users"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

func TestEntityTypeCreate(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`entity create {id:"1" name:"users"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

func TestEntityTypeLoad(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`entity load {id:"1"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// load on record type returns empty map
	m := result[0].AsMap()
	if len(m.Keys()) != 0 {
		t.Errorf("expected empty map, got %d keys", len(m.Keys()))
	}
}

func TestEntityTypeUpdate(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`entity update {id:"1" name:"users-v2"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

func TestEntityTypeRemove(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type entity record [name:String kind:"entity" meta:Map entity:Map model:Map]`,
		`entity remove {id:"1"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

// Test CRUD on the base resource type too.
func TestResourceTypeListEmpty(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:String kind:String meta:Map]`,
		`list resource`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

func TestResourceTypeCreateEmpty(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:String kind:String meta:Map]`,
		`resource create {id:"1" name:"foo"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}
}

// ==========================================================================
// Aliases
// ==========================================================================

// TestResourceTypeAlias verifies the resource type can be aliased via def.
func TestResourceTypeAlias(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:String kind:String meta:Map]`,
		`def res [resource]`,
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
