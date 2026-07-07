package native

import (
	"testing"
)

// Seam-9 coverage (W9_nativeC) for setpath.go: setReachNative over
// resource-instance / nested-list / nested-map containers and the
// reachKeyString helper, driven directly with crafted key paths.

func TestW9SetReachNativeResourceInstance(t *testing.T) {
	r := seam5Reg(t)
	// A resource instance with no TypeRef: a nested set rebuilds it as a
	// plain resource instance (216.9,219.18 / 219.18,221.5 / 222.4,222.26 /
	// 231.3,234.10).
	out, err := setReachNative(entityInst6(),
		[]Value{NewString("kind"), NewString("sub")}, NewInteger(9), r)
	if err != nil {
		t.Fatalf("setReachNative(resource) err = %v", err)
	}
	if !IsResourceInstance(out) {
		t.Fatalf("setReachNative(resource) must preserve the resource type, got %v", out)
	}
}

func TestW9SetReachNativeResourceNestedError(t *testing.T) {
	r := seam5Reg(t)
	// A resource instance whose field is a list: an out-of-range nested
	// index errors and propagates through the resource branch (219.18,221.5).
	fields := NewOrderedMap()
	fields.Set("items", NewList([]Value{NewInteger(1)}))
	res := NewResourceInstance(TResource, ResourceInstanceInfo{Fields: fields})
	if _, err := setReachNative(res,
		[]Value{NewString("items"), NewInteger(5)}, NewInteger(9), r); err == nil {
		t.Fatalf("setReachNative(resource nested OOR) must error")
	}
}

func TestW9SetReachNativeNestedListError(t *testing.T) {
	r := seam5Reg(t)
	// Nested list index out of range propagates up through the list branch
	// (251.17,253.4).
	inner := NewList([]Value{NewInteger(1)})
	outer := NewList([]Value{inner})
	if _, err := setReachNative(outer,
		[]Value{NewInteger(0), NewInteger(5)}, NewInteger(9), r); err == nil {
		t.Fatalf("setReachNative nested list OOR must error")
	}
}

func TestW9SetReachNativeNestedMapError(t *testing.T) {
	r := seam5Reg(t)
	// A map whose value is a list, with an out-of-range nested index, errors
	// through the map branch (274.17,276.4).
	om := NewOrderedMap()
	om.Set("a", NewList([]Value{NewInteger(1)}))
	if _, err := setReachNative(NewMap(om),
		[]Value{NewString("a"), NewInteger(5)}, NewInteger(9), r); err == nil {
		t.Fatalf("setReachNative nested map->list OOR must error")
	}
}

func TestW9ReachKeyString(t *testing.T) {
	// Atom key → its name (289.17,291.11).
	if got := reachKeyString(NewAtom("field")); got != "field" {
		t.Fatalf("reachKeyString(atom) = %q, want field", got)
	}
	// A non-word/atom/string key falls to ValToString (295.10,296.24).
	if got := reachKeyString(NewInteger(7)); got == "" {
		t.Fatalf("reachKeyString(int) = %q, want non-empty", got)
	}
}
