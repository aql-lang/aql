package native

import (
	"testing"
)

// Wave-7 coverage for native_inspect.go buildTypeInspection: the
// record-shape, object-type (with parent + inherited fields), table-type,
// disjunct, typed-list and typed-map switch arms
// (design/TEST-SEAMS.10.md). Each is driven through inspectTypeHandler,
// the real `inspect V` entry point.

func inspectKind(t *testing.T, r *Registry, tv Value) string {
	t.Helper()
	out, err := inspectTypeHandler([]Value{tv}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("inspect = %v / %v", out, err)
	}
	m, _ := AsMap(out[0])
	if m == nil {
		t.Fatalf("inspect result not a map: %v", out[0])
	}
	if k, ok := m.Get("kind"); ok {
		s, _ := AsAtom(k)
		return s
	}
	return ""
}

func TestInspectRecordShape(t *testing.T) {
	r := b2Reg(t)
	shape, err := b2Run(r, "{a:Integer}")
	if err != nil || len(shape) != 1 || !IsRecordShape(shape[0]) {
		t.Fatalf("could not build record shape: %v / %v", shape, err)
	}
	if k := inspectKind(t, r, shape[0]); k != "record" {
		t.Fatalf("record-shape inspect kind = %q, want record", k)
	}
}

func TestInspectDisjunctTypedListMap(t *testing.T) {
	r := b2Reg(t)
	cases := map[string]string{
		"(Integer tor String)": "disjunct",
		"[:Integer]":           "typed_list",
		"{:Integer}":           "typed_map",
	}
	for src, want := range cases {
		out, err := b2Run(r, src)
		if err != nil || len(out) != 1 {
			t.Fatalf("build %q: %v / %v", src, out, err)
		}
		if k := inspectKind(t, r, out[0]); k != want {
			t.Fatalf("inspect %q kind = %q, want %q", src, k, want)
		}
	}
}

func TestInspectObjectTypeWithParent(t *testing.T) {
	r := b2Reg(t)
	// A parent object type contributing an inherited field, plus a child
	// that references it — exercises the parent-name and AllFields arms.
	pfields := NewOrderedMap()
	pfields.Set("base", NewTypeLiteral(TString))
	parent := ClassTypeInfo{Fields: pfields, ID: GenerateObjectTypeID(), Name: "Object/Base"}

	cfields := NewOrderedMap()
	cfields.Set("x", NewTypeLiteral(TInteger))
	child := ClassTypeInfo{Fields: cfields, ID: GenerateObjectTypeID(), Name: "Object/Child", Parent: &parent}

	tv := NewClassType(TClass, child)
	if k := inspectKind(t, r, tv); k != "object" {
		t.Fatalf("object-type inspect kind = %q, want object", k)
	}
}

func TestInspectTableType(t *testing.T) {
	r := b2Reg(t)
	rfields := NewOrderedMap()
	rfields.Set("a", NewTypeLiteral(TInteger))
	tv := NewTableType(RecordTypeInfo{Fields: rfields})
	if !IsTableType(tv) {
		t.Fatalf("NewTableType did not build a table type: %v", tv)
	}
	if k := inspectKind(t, r, tv); k != "table" {
		t.Fatalf("table-type inspect kind = %q, want table", k)
	}
}
