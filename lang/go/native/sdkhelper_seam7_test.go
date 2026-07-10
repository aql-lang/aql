package native

import (
	"strings"
	"testing"

	udk "github.com/voxgig/udk/go"
)

// Seam-7 coverage for sdk_helper.go (design/TEST-SEAMS.10.md).

// b3ReadMap builds a ReadMap from alternating key/value pairs.
func b3ReadMap(t *testing.T, kv ...any) ReadMap {
	t.Helper()
	om := NewOrderedMap()
	for i := 0; i+1 < len(kv); i += 2 {
		om.Set(kv[i].(string), kv[i+1].(Value))
	}
	m, err := AsMap(NewMap(om))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestB3GetSDKFieldErrors(t *testing.T) {
	r := seam5Reg(t)
	// spec present but not a string.
	if _, _, err := getSDK(b3ReadMap(t, "spec", NewInteger(1)), "op", r); err == nil ||
		!strings.Contains(err.Error(), "op: spec:") {
		t.Fatalf("getSDK spec: expected string error, got %v", err)
	}
	// entity present but not a string.
	if _, _, err := getSDK(b3ReadMap(t, "spec", NewString("s"), "entity", NewInteger(1)), "op", r); err == nil ||
		!strings.Contains(err.Error(), "op: entity:") {
		t.Fatalf("getSDK entity: expected string error, got %v", err)
	}
}

func TestB3GetSDKManagerArms(t *testing.T) {
	// No manager configured (default r.Manager == nil).
	r := seam5Reg(t)
	if _, _, err := getSDK(b3ReadMap(t, "spec", NewString("nospec")), "op", r); err == nil ||
		!strings.Contains(err.Error(), "no manager configured") {
		t.Fatalf("getSDK: expected no-manager error, got %v", err)
	}
	// Manager configured -> mgr.Make caches an SDK instance.
	r2 := seam5Reg(t)
	r2.Manager = udk.NewUniversalManager(map[string]any{})
	sdk, ent, err := getSDK(b3ReadMap(t, "spec", NewString("spec.json"), "entity", NewString("thing")), "op", r2)
	if err != nil || sdk == nil || ent != "thing" {
		t.Fatalf("getSDK with manager: unexpected sdk=%v ent=%q err=%v", sdk, ent, err)
	}
	if _, ok := r2.SDKCache["spec"]; !ok {
		t.Fatalf("expected SDK cached under stripped spec name")
	}
}

func TestB3EntityToAPIMapGuards(t *testing.T) {
	// Non-concrete value -> empty map.
	if m := entityToAPIMap(NewTypeLiteral(TMap)); m.Len() != 0 {
		t.Fatalf("entityToAPIMap(non-concrete): expected empty, got %d keys", m.Len())
	}
	// Concrete but not a Resource/Entity instance -> empty map.
	if m := entityToAPIMap(NewMap(NewOrderedMap())); m.Len() != 0 {
		t.Fatalf("entityToAPIMap(non-resource): expected empty, got %d keys", m.Len())
	}
}

func TestB3ConvertResultItemEntity(t *testing.T) {
	// item implementing udk.Entity -> unwrapped via Data().
	v, err := convertResultItem(&fakeUDKEntity6{}, "op")
	if err != nil {
		t.Fatalf("convertResultItem(entity): unexpected error %v", err)
	}
	// fakeUDKEntity6.Data returns nil -> None value.
	if !IsNoneShape(v) {
		t.Fatalf("expected None from nil entity data, got %v", v)
	}
}

func TestB3MergeAPIOptionsExistingField(t *testing.T) {
	// base already carries a "query" map: its entries copy through.
	inner := NewOrderedMap()
	inner.Set("existing", NewInteger(1))
	base := b3ReadMap(t, "kind", NewString("api"), "query", NewMap(inner))
	opts := b3ReadMap(t, "added", NewInteger(2))
	merged := mergeAPIOptions(base, opts, "query")
	qv, ok := merged.Get("query")
	if !ok {
		t.Fatal("merged missing query field")
	}
	qm, _ := AsMap(qv)
	if _, ok := qm.Get("existing"); !ok {
		t.Fatalf("expected existing query key preserved")
	}
	if _, ok := qm.Get("added"); !ok {
		t.Fatalf("expected added opts key merged")
	}
}
