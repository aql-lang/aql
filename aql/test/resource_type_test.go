package test

import (
	"testing"
)

// TestResourceTypeDefine defines the resource record type and verifies
// it is recognized as a record type.
func TestResourceTypeDefine(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:string kind:string meta:map]`,
		`resource`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	s := result[0].String()
	if s != "record{name:string,kind:string,meta:map}" {
		t.Errorf("unexpected type string: %s", s)
	}
}

// TestResourceTypeMakePositional creates a resource using positional fields.
func TestResourceTypeMakePositional(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:string kind:string meta:map]`,
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
		`type resource record [name:string kind:string meta:map]`,
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
		`type resource record [name:string kind:string meta:map]`,
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
		`type resource record [name:string kind:string meta:map]`,
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

// TestResourceTypeAlias verifies the resource type can be aliased via def.
func TestResourceTypeAlias(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`type resource record [name:string kind:string meta:map]`,
		`def res [resource]`,
		`res`,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := result[0].String()
	if s != "record{name:string,kind:string,meta:map}" {
		t.Errorf("unexpected alias type string: %s", s)
	}
}
