package native

import (
	"errors"
	"strings"
	"testing"

	udk "voxgiguniversalsdk"
)

// Wave-6 coverage for the SDK-backed API words (create.go, load.go,
// list.go, direct.go). A fake sdkClient (seams.go seam shape 3) is
// planted in r.SDKCache so every handler arm — entity-instance entry
// points, opts-merging entry points, SDK errors, and result-conversion
// errors — is drivable without a network or a spec file.

// fakeUDKEntity6 is a scripted udk.UniversalEntity.
type fakeUDKEntity6 struct {
	loadRes, listRes, createRes any
	loadErr, listErr, createErr error
}

func (e *fakeUDKEntity6) GetName() string  { return "fake" }
func (e *fakeUDKEntity6) Make() udk.Entity { return e }
func (e *fakeUDKEntity6) Data(...any) any  { return nil }
func (e *fakeUDKEntity6) Match(...any) any { return nil }
func (e *fakeUDKEntity6) Load(map[string]any, map[string]any) (any, error) {
	return e.loadRes, e.loadErr
}
func (e *fakeUDKEntity6) List(map[string]any, map[string]any) (any, error) {
	return e.listRes, e.listErr
}
func (e *fakeUDKEntity6) Create(map[string]any, map[string]any) (any, error) {
	return e.createRes, e.createErr
}
func (e *fakeUDKEntity6) Update(map[string]any, map[string]any) (any, error) {
	return nil, errors.New("unused")
}
func (e *fakeUDKEntity6) Remove(map[string]any, map[string]any) (any, error) {
	return nil, errors.New("unused")
}

// fakeSDK6 is a scripted sdkClient.
type fakeSDK6 struct {
	ent       *fakeUDKEntity6
	directRes map[string]any
	directErr error
}

func (s *fakeSDK6) Entity(string, map[string]any) udk.UniversalEntity { return s.ent }
func (s *fakeSDK6) Prepare(map[string]any) (map[string]any, error)    { return nil, nil }
func (s *fakeSDK6) Direct(map[string]any) (map[string]any, error) {
	return s.directRes, s.directErr
}

// plantSDK6 installs a fake sdkClient under spec "fakespec" and returns it.
func plantSDK6(t *testing.T, r *Registry) *fakeSDK6 {
	t.Helper()
	sdk := &fakeSDK6{ent: &fakeUDKEntity6{}}
	r.SDKCache["fakespec"] = sdk
	return sdk
}

// apiMap6 builds the {kind:"api", spec:"fakespec", entity:"planet"} map value.
func apiMap6() Value {
	om := NewOrderedMap()
	om.Set("kind", NewString("api"))
	om.Set("spec", NewString("fakespec"))
	om.Set("entity", NewString("planet"))
	return NewMap(om)
}

// entityInst6 builds a Resource instance carrying kind/spec/entity fields.
func entityInst6() Value {
	fields := NewOrderedMap()
	fields.Set("kind", NewString("api"))
	fields.Set("spec", NewString("fakespec"))
	fields.Set("entity", NewString("planet"))
	return NewResourceInstance(TResource, ResourceInstanceInfo{Fields: fields})
}

// noSpecMap6 builds an API map with the required spec field missing.
func noSpecMap6() Value {
	om := NewOrderedMap()
	om.Set("kind", NewString("api"))
	return NewMap(om)
}

func TestCreateEntityAndOptsHandlers(t *testing.T) {
	r := seam5Reg(t)
	sdk := plantSDK6(t, r)
	sdk.ent.createRes = map[string]any{"id": "p1"}

	out, err := createEntityHandler([]Value{entityInst6()}, nil, nil, r)
	if err != nil {
		t.Fatalf("createEntityHandler: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one result, got %d", len(out))
	}

	data := NewOrderedMap()
	data.Set("name", NewString("Mars"))
	out, err = createAPIOptsHandler([]Value{NewMap(data), apiMap6()}, nil, nil, r)
	if err != nil {
		t.Fatalf("createAPIOptsHandler: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one result, got %d", len(out))
	}
}

func TestCreateAPIHandlerErrorArms(t *testing.T) {
	r := seam5Reg(t)
	sdk := plantSDK6(t, r)

	// getSDK failure: missing spec.
	if _, err := createAPIHandler([]Value{noSpecMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "spec") {
		t.Fatalf("expected missing-spec error, got %v", err)
	}

	// SDK Create failure.
	sdk.ent.createErr = errors.New("backend down")
	if _, err := createAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "backend down") {
		t.Fatalf("expected create error, got %v", err)
	}

	// Result-conversion failure: an unconvertible Go value.
	sdk.ent.createErr = nil
	sdk.ent.createRes = struct{}{}
	if _, err := createAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "converting result") {
		t.Fatalf("expected conversion error, got %v", err)
	}
}

func TestCreateTableHandlerEdgeArms(t *testing.T) {
	r := seam5Reg(t)

	// Non-string id → AsString error.
	rec := NewOrderedMap()
	rec.Set("id", NewInteger(5))
	if _, err := createHandler([]Value{NewMap(rec), NewList(nil)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "id") {
		t.Fatalf("expected id-type error, got %v", err)
	}

	// A non-map row in the table is skipped during the duplicate check.
	rec2 := NewOrderedMap()
	rec2.Set("id", NewString("a"))
	rows := NewList([]Value{NewInteger(42)})
	out, err := createHandler([]Value{NewMap(rec2), rows}, nil, nil, r)
	if err != nil {
		t.Fatalf("createHandler with non-map row: %v", err)
	}
	lst, _ := AsList(out[0])
	if lst.Len() != 2 {
		t.Fatalf("expected 2 rows after append, got %d", lst.Len())
	}
}

func TestLoadEntityAndOptsHandlers(t *testing.T) {
	r := seam5Reg(t)
	sdk := plantSDK6(t, r)
	sdk.ent.loadRes = map[string]any{"id": "p1"}

	out, err := loadEntityHandler([]Value{entityInst6()}, nil, nil, r)
	if err != nil {
		t.Fatalf("loadEntityHandler: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one result, got %d", len(out))
	}

	opts := NewOrderedMap()
	opts.Set("id", NewString("p1"))
	out, err = loadAPIOptsHandler([]Value{NewMap(opts), apiMap6()}, nil, nil, r)
	if err != nil {
		t.Fatalf("loadAPIOptsHandler: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one result, got %d", len(out))
	}
}

func TestLoadAPIHandlerErrorArms(t *testing.T) {
	r := seam5Reg(t)
	sdk := plantSDK6(t, r)

	// getSDK failure: missing spec.
	if _, err := loadAPIHandler([]Value{noSpecMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "spec") {
		t.Fatalf("expected missing-spec error, got %v", err)
	}

	// Result-conversion failure.
	sdk.ent.loadRes = struct{}{}
	if _, err := loadAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "converting result") {
		t.Fatalf("expected conversion error, got %v", err)
	}
}

func TestListAPIHandlerArms(t *testing.T) {
	r := seam5Reg(t)
	sdk := plantSDK6(t, r)

	// Opts-merging entry point, happy path.
	sdk.ent.listRes = []any{map[string]any{"id": "p1"}}
	opts := NewOrderedMap()
	opts.Set("active", NewBoolean(true))
	out, err := listAPIOptsHandler([]Value{NewMap(opts), apiMap6()}, nil, nil, r)
	if err != nil {
		t.Fatalf("listAPIOptsHandler: %v", err)
	}
	lst, _ := AsList(out[0])
	if lst.Len() != 1 {
		t.Fatalf("expected 1 row, got %d", lst.Len())
	}

	// getSDK failure: missing spec.
	if _, err := listAPIHandler([]Value{noSpecMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "spec") {
		t.Fatalf("expected missing-spec error, got %v", err)
	}

	// SDK List failure.
	sdk.ent.listErr = errors.New("list boom")
	if _, err := listAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "list boom") {
		t.Fatalf("expected list error, got %v", err)
	}

	// Result-conversion failure inside the row list.
	sdk.ent.listErr = nil
	sdk.ent.listRes = []any{struct{}{}}
	if _, err := listAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "converting result") {
		t.Fatalf("expected conversion error, got %v", err)
	}
}

func TestValuesEqualStructuralFallback(t *testing.T) {
	a := NewList([]Value{NewInteger(1)})
	b := NewList([]Value{NewInteger(1)})
	c := NewList([]Value{NewInteger(2)})
	if !valuesEqual(a, b) {
		t.Fatal("equal lists must compare equal via the String fallback")
	}
	if valuesEqual(a, c) {
		t.Fatal("different lists must not compare equal")
	}
}

func TestDirectAPIHandlerArms(t *testing.T) {
	r := seam5Reg(t)
	sdk := plantSDK6(t, r)

	// SDK Direct failure.
	sdk.directErr = errors.New("direct boom")
	if _, err := directAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "direct boom") {
		t.Fatalf("expected direct error, got %v", err)
	}

	// Result-conversion failure.
	sdk.directErr = nil
	sdk.directRes = map[string]any{"bad": struct{}{}}
	if _, err := directAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "converting result") {
		t.Fatalf("expected conversion error, got %v", err)
	}

	// Positive pair: a convertible result round-trips.
	sdk.directRes = map[string]any{"ok": true}
	out, err := directAPIHandler([]Value{apiMap6()}, nil, nil, r)
	if err != nil {
		t.Fatalf("directAPIHandler: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one result, got %d", len(out))
	}
}
