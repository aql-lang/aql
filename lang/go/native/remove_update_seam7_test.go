package native

import (
	"errors"
	"strings"
	"testing"

	udk "github.com/voxgig/udk/go"
)

// Seam-7 coverage for remove.go and update.go (design/TEST-SEAMS.10.md).
// A scripted sdkClient (seam shape 3) is planted in r.SDKCache so the
// Entity / opts-merging / API entry points and their error arms are
// drivable without a network or spec file.

type fakeRUEntity struct {
	updRes, remRes any
	updErr, remErr error
}

func (e *fakeRUEntity) GetName() string  { return "fake" }
func (e *fakeRUEntity) Make() udk.Entity { return e }
func (e *fakeRUEntity) Data(...any) any  { return nil }
func (e *fakeRUEntity) Match(...any) any { return nil }
func (e *fakeRUEntity) Load(map[string]any, map[string]any) (any, error) {
	return nil, errors.New("unused")
}
func (e *fakeRUEntity) List(map[string]any, map[string]any) (any, error) {
	return nil, errors.New("unused")
}
func (e *fakeRUEntity) Create(map[string]any, map[string]any) (any, error) {
	return nil, errors.New("unused")
}
func (e *fakeRUEntity) Update(map[string]any, map[string]any) (any, error) {
	return e.updRes, e.updErr
}
func (e *fakeRUEntity) Remove(map[string]any, map[string]any) (any, error) {
	return e.remRes, e.remErr
}

type fakeRUSDK struct{ ent *fakeRUEntity }

func (s *fakeRUSDK) Entity(string, map[string]any) udk.UniversalEntity { return s.ent }
func (s *fakeRUSDK) Prepare(map[string]any) (map[string]any, error)    { return nil, nil }
func (s *fakeRUSDK) Direct(map[string]any) (map[string]any, error)     { return nil, nil }

func plantRUSDK(t *testing.T, r *Registry) *fakeRUSDK {
	t.Helper()
	sdk := &fakeRUSDK{ent: &fakeRUEntity{}}
	r.SDKCache["fakespec"] = sdk
	return sdk
}

func TestSeam7RemoveAPIHandlers(t *testing.T) {
	r := seam5Reg(t)
	sdk := plantRUSDK(t, r)
	sdk.ent.remRes = map[string]any{"id": "p1"}

	// Entity entry point.
	if _, err := removeEntityHandler([]Value{entityInst6()}, nil, nil, r); err != nil {
		t.Fatalf("removeEntityHandler: %v", err)
	}

	// Entity + opts entry point.
	opts := NewOrderedMap()
	opts.Set("id", NewString("p1"))
	if _, err := removeEntityOptsHandler([]Value{NewMap(opts), entityInst6()}, nil, nil, r); err != nil {
		t.Fatalf("removeEntityOptsHandler: %v", err)
	}

	// API + opts entry point.
	if _, err := removeAPIOptsHandler([]Value{NewMap(opts), apiMap6()}, nil, nil, r); err != nil {
		t.Fatalf("removeAPIOptsHandler: %v", err)
	}

	// getSDK failure: missing spec.
	if _, err := removeAPIHandler([]Value{noSpecMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "spec") {
		t.Fatalf("expected missing-spec error, got %v", err)
	}

	// SDK Remove failure.
	sdk.ent.remErr = errors.New("remove boom")
	if _, err := removeAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "remove boom") {
		t.Fatalf("expected remove error, got %v", err)
	}

	// Result-conversion failure.
	sdk.ent.remErr = nil
	sdk.ent.remRes = struct{}{}
	if _, err := removeAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "converting result") {
		t.Fatalf("expected conversion error, got %v", err)
	}
}

func TestSeam7RemoveRecordAndTableHandlers(t *testing.T) {
	r := seam5Reg(t)

	// Record handler returns an empty table.
	out, err := removeRecordHandler(nil, nil, nil, r)
	if err != nil {
		t.Fatalf("removeRecordHandler: %v", err)
	}
	if lst, _ := AsList(out[0]); lst.Len() != 0 {
		t.Fatalf("expected empty table, got %d", lst.Len())
	}

	// Table handler: filter missing "id".
	empty := NewList(nil)
	fm := NewOrderedMap()
	if _, err := removeHandler([]Value{NewMap(fm), empty}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "id") {
		t.Fatalf("expected missing-id error, got %v", err)
	}

	// Table handler: id present but not a string.
	fm2 := NewOrderedMap()
	fm2.Set("id", NewMap(NewOrderedMap()))
	if _, err := removeHandler([]Value{NewMap(fm2), empty}, nil, nil, r); err == nil {
		t.Fatal("expected non-string id error")
	}

	// Table handler: no matching record.
	fm3 := NewOrderedMap()
	fm3.Set("id", NewString("zzz"))
	rec := NewOrderedMap()
	rec.Set("id", NewString("a"))
	rows := NewList([]Value{NewMap(rec)})
	if _, err := removeHandler([]Value{NewMap(fm3), rows}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "no record") {
		t.Fatalf("expected no-record error, got %v", err)
	}

	// Positive: remove the only record, plus a non-map row that is kept.
	fm4 := NewOrderedMap()
	fm4.Set("id", NewString("a"))
	rec2 := NewOrderedMap()
	rec2.Set("id", NewString("a"))
	// A non-map row exercises the ConformsTo(TMap) skip; it is retained.
	mixed := NewList([]Value{NewInteger(99), NewMap(rec2)})
	out, err = removeHandler([]Value{NewMap(fm4), mixed}, nil, nil, r)
	if err != nil {
		t.Fatalf("removeHandler positive: %v", err)
	}
	if lst, _ := AsList(out[0]); lst.Len() != 1 {
		t.Fatalf("expected 1 remaining row, got %d", lst.Len())
	}

	// Positive: remove the single row so the result-nil arm sets an
	// empty slice.
	fm5 := NewOrderedMap()
	fm5.Set("id", NewString("only"))
	solo := NewOrderedMap()
	solo.Set("id", NewString("only"))
	out, err = removeHandler([]Value{NewMap(fm5), NewList([]Value{NewMap(solo)})}, nil, nil, r)
	if err != nil {
		t.Fatalf("removeHandler solo: %v", err)
	}
	if lst, _ := AsList(out[0]); lst.Len() != 0 {
		t.Fatalf("expected empty table after removing only row, got %d", lst.Len())
	}
}

func TestSeam7UpdateAPIHandlers(t *testing.T) {
	r := seam5Reg(t)
	sdk := plantRUSDK(t, r)
	sdk.ent.updRes = map[string]any{"id": "p1"}

	if _, err := updateEntityHandler([]Value{entityInst6()}, nil, nil, r); err != nil {
		t.Fatalf("updateEntityHandler: %v", err)
	}

	data := NewOrderedMap()
	data.Set("name", NewString("Mars"))
	if _, err := updateEntityOptsHandler([]Value{NewMap(data), entityInst6()}, nil, nil, r); err != nil {
		t.Fatalf("updateEntityOptsHandler: %v", err)
	}
	if _, err := updateAPIOptsHandler([]Value{NewMap(data), apiMap6()}, nil, nil, r); err != nil {
		t.Fatalf("updateAPIOptsHandler: %v", err)
	}

	// getSDK failure: missing spec.
	if _, err := updateAPIHandler([]Value{noSpecMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "spec") {
		t.Fatalf("expected missing-spec error, got %v", err)
	}

	// SDK Update failure.
	sdk.ent.updErr = errors.New("update boom")
	if _, err := updateAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "update boom") {
		t.Fatalf("expected update error, got %v", err)
	}

	// Result-conversion failure.
	sdk.ent.updErr = nil
	sdk.ent.updRes = struct{}{}
	if _, err := updateAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "converting result") {
		t.Fatalf("expected conversion error, got %v", err)
	}
}

func TestSeam7UpdateRecordAndTableHandlers(t *testing.T) {
	r := seam5Reg(t)

	out, err := updateRecordHandler(nil, nil, nil, r)
	if err != nil {
		t.Fatalf("updateRecordHandler: %v", err)
	}
	if lst, _ := AsList(out[0]); lst.Len() != 0 {
		t.Fatalf("expected empty table, got %d", lst.Len())
	}

	empty := NewList(nil)

	// Patch missing "id".
	if _, err := updateHandler([]Value{NewMap(NewOrderedMap()), empty}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "id") {
		t.Fatalf("expected missing-id error, got %v", err)
	}

	// id present but not a string.
	pm := NewOrderedMap()
	pm.Set("id", NewMap(NewOrderedMap()))
	if _, err := updateHandler([]Value{NewMap(pm), empty}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "id") {
		t.Fatalf("expected non-string id error, got %v", err)
	}

	// No matching record.
	pm2 := NewOrderedMap()
	pm2.Set("id", NewString("zzz"))
	rec := NewOrderedMap()
	rec.Set("id", NewString("a"))
	if _, err := updateHandler([]Value{NewMap(pm2), NewList([]Value{NewMap(rec)})}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "no record") {
		t.Fatalf("expected no-record error, got %v", err)
	}

	// Positive: patch merges into the matching row; a non-map row and a
	// map row without an "id" field are both retained untouched.
	patch := NewOrderedMap()
	patch.Set("id", NewString("a"))
	patch.Set("name", NewString("new"))
	match := NewOrderedMap()
	match.Set("id", NewString("a"))
	match.Set("name", NewString("old"))
	noID := NewOrderedMap()
	noID.Set("k", NewString("v"))
	rows := NewList([]Value{NewInteger(7), NewMap(noID), NewMap(match)})
	out, err = updateHandler([]Value{NewMap(patch), rows}, nil, nil, r)
	if err != nil {
		t.Fatalf("updateHandler positive: %v", err)
	}
	lst, _ := AsList(out[0])
	if lst.Len() != 3 {
		t.Fatalf("expected 3 rows, got %d", lst.Len())
	}
	merged, _ := AsMap(lst.Slice()[2])
	nameVal, _ := merged.Get("name")
	if s, _ := AsString(nameVal); s != "new" {
		t.Fatalf("expected merged name=new, got %q", s)
	}
}
