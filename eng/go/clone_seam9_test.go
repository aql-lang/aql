package eng

import (
	"errors"
	"testing"
)

// errTestW9 is a plain Go error (no BoruError Code / Data).
var errTestW9 = errors.New("plain boom")

// W9 clone.go coverage: the mutable-payload clone arms not reached by the
// existing seam8 object test — TableData, ErrorInfo (with/without Data),
// the ExtensionPayload no-DeepCloner share arm, and the nil / already-seen
// / Meta arms of cloneSlice and cloneOrderedMap.

func TestW9CloneTableData(t *testing.T) {
	rows := []Value{NewInteger(1), NewInteger(2)}
	tv := NewValueRaw(TTable, TableData{
		Record:    RecordTypeInfo{},
		Rows:      rows,
		TableName: "t",
	})
	cl := CloneValue(tv)
	td, ok := cl.Data.(TableData)
	if !ok {
		t.Fatalf("clone Data = %T, want TableData", cl.Data)
	}
	if td.TableName != "t" {
		t.Errorf("clone lost TableName: %q", td.TableName)
	}
	if len(td.Rows) != 2 {
		t.Fatalf("clone rows = %d, want 2", len(td.Rows))
	}
	// Rows must be duplicated, not aliased.
	td.Rows[0] = NewInteger(99)
	if n, _ := AsInteger(rows[0]); n != 1 {
		t.Error("clone leaked: mutating the clone's rows changed the original")
	}
}

func TestW9CloneErrorWithData(t *testing.T) {
	data := NewOrderedMap()
	data.Set("k", NewInteger(7))
	ae := &BoruError{Code: "user_error", Detail: "boom", Data: data}
	ev := NewError(ae)
	cl := CloneValue(ev)
	ci, ok := cl.Data.(ErrorInfo)
	if !ok {
		t.Fatalf("clone Data = %T, want ErrorInfo", cl.Data)
	}
	if ci.Code != "user_error" || ci.Message != "boom" {
		t.Errorf("clone lost immutable fields: %+v", ci)
	}
	if ci.Data == data {
		t.Error("cloneError must duplicate the Data map, not share it")
	}
	// Mutating the clone's Data must not touch the original.
	ci.Data.Set("k", NewInteger(0))
	if got, _ := data.Get("k"); func() int64 { n, _ := AsInteger(got); return n }() != 7 {
		t.Error("clone leaked: mutating the clone's Data changed the original")
	}
}

func TestW9CloneErrorNilData(t *testing.T) {
	// A plain Go error has nil Data: the clone keeps Data nil.
	ev := NewError(errTestW9)
	cl := CloneValue(ev)
	ci, ok := cl.Data.(ErrorInfo)
	if !ok {
		t.Fatalf("clone Data = %T, want ErrorInfo", cl.Data)
	}
	if ci.Data != nil {
		t.Error("cloneError with nil Data should keep Data nil")
	}
}

// extNoCloneW9 is a host body that does NOT implement DeepCloner.
type extNoCloneW9 struct{ n int }

func TestW9CloneExtensionNoDeepCloner(t *testing.T) {
	body := &extNoCloneW9{n: 5}
	ev := NewExtension(TAny, body)
	cl := CloneValue(ev)
	ep, ok := cl.Data.(ExtensionPayload)
	if !ok {
		t.Fatalf("clone Data = %T, want ExtensionPayload", cl.Data)
	}
	if ep.Body != any(body) {
		t.Error("an ExtensionPayload with no DeepCloner must be shared, not copied")
	}
}

func TestW9CloneNilSliceAndMap(t *testing.T) {
	// ListPayload with a nil slice: cloneSlice(nil) returns nil.
	lv := NewList(nil)
	cl := CloneValue(lv)
	lp, ok := cl.Data.(ListPayload)
	if !ok {
		t.Fatalf("clone Data = %T, want ListPayload", cl.Data)
	}
	if lp.Elems != nil {
		t.Error("cloneSlice(nil) should stay nil")
	}
	// MapPayload with a nil map: cloneOrderedMap(nil) returns nil.
	mv := NewMap(nil)
	clm := CloneValue(mv)
	mp, ok := clm.Data.(MapPayload)
	if !ok {
		t.Fatalf("clone Data = %T, want MapPayload", clm.Data)
	}
	if mp.M != nil {
		t.Error("cloneOrderedMap(nil) should stay nil")
	}
}

func TestW9CloneSharedMapDedup(t *testing.T) {
	// A single *OrderedMap referenced by two list elements: cloneOrderedMap
	// clones it once (the second call hits the seen map) so the clone keeps
	// the sharing.
	shared := NewOrderedMap()
	shared.Set("x", NewInteger(1))
	m1 := NewMap(shared)
	m2 := NewMap(shared)
	lv := NewList([]Value{m1, m2})
	cl := CloneValue(lv)
	lp := cl.Data.(ListPayload)
	c1 := lp.Elems[0].Data.(MapPayload).M
	c2 := lp.Elems[1].Data.(MapPayload).M
	if c1 != c2 {
		t.Error("shared map should clone once and stay shared in the clone")
	}
	if c1 == shared {
		t.Error("clone should duplicate the shared map, not alias the original")
	}
}

func TestW9CloneMapMeta(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	m.Meta = map[string]any{"src": "test"}
	cl := CloneValue(NewMap(m))
	cm := cl.Data.(MapPayload).M
	if cm.Meta == nil || cm.Meta["src"] != "test" {
		t.Errorf("clone lost Meta: %+v", cm.Meta)
	}
	// The Meta map must be duplicated, not aliased.
	cm.Meta["src"] = "changed"
	if m.Meta["src"] != "test" {
		t.Error("clone leaked: mutating the clone's Meta changed the original")
	}
}
