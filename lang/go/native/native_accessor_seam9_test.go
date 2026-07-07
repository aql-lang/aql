package native

import (
	"strings"
	"testing"
)

// Seam-9 coverage (W9_nativeC) for native_accessor.go: has/getr handler
// arms over map / resource-instance / non-instance containers, driven
// directly with crafted containers.

func TestW9HasNodeNonMapFalse(t *testing.T) {
	r := seam5Reg(t)
	// A list container with a non-integer key falls through to the map
	// lookup, where AsMap returns nil → false (144.2,144.40).
	out, err := hasNodeHandler([]Value{NewString("k"), NewList([]Value{NewInteger(1)})}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("hasNodeHandler: %v / %v", out, err)
	}
	if b, _ := AsBoolean(out[0]); b {
		t.Fatalf("hasNode over a list with a string key must be false")
	}
}

func TestW9HasObjectResourceAndNonInstance(t *testing.T) {
	r := seam5Reg(t)

	// Resource instance: the AsResourceInstance branch (157.58,160.3).
	out, err := hasObjectHandler([]Value{NewString("kind"), entityInst6()}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("hasObject resource: %v / %v", out, err)
	}
	if b, _ := AsBoolean(out[0]); !b {
		t.Fatalf("hasObject must find 'kind' on the resource instance")
	}

	// A concrete non-instance value → AsClassInstance errors → false (162.16,164.3).
	out, _ = hasObjectHandler([]Value{NewString("k"), NewInteger(1)}, nil, nil, r)
	if b, _ := AsBoolean(out[0]); b {
		t.Fatalf("hasObject over an Integer must be false")
	}
}

func TestW9GetrObjectMutableMapArms(t *testing.T) {
	r := seam5Reg(t)
	m := NewOrderedMap()
	m.Set("k", NewInteger(7))
	container := NewMap(m)

	// present key → value (232.51 → 237.3,237.27).
	out, err := getrObjectHandler([]Value{NewString("k"), container}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("getrObject present: %v / %v", out, err)
	}
	// missing key → not_found (234.13,236.4).
	if _, err := getrObjectHandler([]Value{NewString("missing"), container}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not found in object") {
		t.Fatalf("getrObject missing map key: %v", err)
	}
}

func TestW9GetrObjectResourceArms(t *testing.T) {
	r := seam5Reg(t)

	// present field on a resource instance (239.58 → 244.3,244.27).
	out, err := getrObjectHandler([]Value{NewString("kind"), entityInst6()}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("getrObject resource present: %v / %v", out, err)
	}
	// missing field → not_found (241.10,243.4).
	if _, err := getrObjectHandler([]Value{NewString("nope"), entityInst6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not found in resource") {
		t.Fatalf("getrObject missing resource field: %v", err)
	}
}
