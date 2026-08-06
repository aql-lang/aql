package core

// Stage-5 coverage tests for store_shape_state.go: RecordKey / LookupKey /
// RecordVal / LookupVals / CloneShape, including every nil-receiver guard.
// RecordVal has no production caller today; it is exercised here as part of
// the shape-state model contract (join-only, poison-on-dispatch-bearing).
//
// JoinCarriersHook is a package global: each test that depends on join
// behaviour pins a deterministic first-write-wins stub and restores the
// previous hook with defer.

import "testing"

func TestStage5ShapeRecordKey(t *testing.T) {
	old := JoinCarriersHook
	joins := 0
	JoinCarriersHook = func(existing, v Value) Value {
		joins++
		return existing // first write wins — distinguishable from the default
	}
	defer func() { JoinCarriersHook = old }()

	var nilShape *StoreShapeInfo
	nilShape.RecordKey("k", NewInteger(1)) // nil receiver: no-op, no panic

	s := &StoreShapeInfo{}
	s.RecordKey("", NewInteger(1)) // empty key: no-op
	if s.KeyTypes != nil {
		t.Fatal("empty-key RecordKey must not allocate KeyTypes")
	}

	s.RecordKey("k", NewInteger(1))
	if s.KeyTypes == nil {
		t.Fatal("first RecordKey must allocate KeyTypes")
	}
	got, ok := s.LookupKey("k")
	if !ok {
		t.Fatal("recorded key must be readable")
	}
	if n, err := AsInteger(got); err != nil || n != 1 {
		t.Errorf("recorded carrier = %v (%v), want 1", got, err)
	}

	// A repeat write joins — never replaces.
	s.RecordKey("k", NewInteger(2))
	if joins != 1 {
		t.Errorf("join invocations = %d, want 1", joins)
	}
	got, _ = s.LookupKey("k")
	if n, err := AsInteger(got); err != nil || n != 1 {
		t.Errorf("joined carrier = %v (%v), want the stubbed first-write 1", got, err)
	}
}

func TestStage5ShapeLookupKey(t *testing.T) {
	var nilShape *StoreShapeInfo
	if _, ok := nilShape.LookupKey("k"); ok {
		t.Error("nil-shape LookupKey must decline")
	}
	s := &StoreShapeInfo{}
	if _, ok := s.LookupKey("absent"); ok {
		t.Error("an unwritten key must read as absent")
	}
}

func TestStage5ShapeRecordVal(t *testing.T) {
	old := JoinCarriersHook
	joins := 0
	JoinCarriersHook = func(existing, v Value) Value {
		joins++
		return existing
	}
	defer func() { JoinCarriersHook = old }()

	var nilShape *StoreShapeInfo
	nilShape.RecordVal(NewInteger(1)) // nil receiver: no-op, no panic
	if _, ok := nilShape.LookupVals(); ok {
		t.Error("nil-shape LookupVals must decline")
	}

	s := &StoreShapeInfo{}
	if _, ok := s.LookupVals(); ok {
		t.Error("nothing recorded: LookupVals must decline")
	}

	// First unkeyed write is stored directly.
	s.RecordVal(NewInteger(1))
	got, ok := s.LookupVals()
	if !ok {
		t.Fatal("recorded val must be readable")
	}
	if n, err := AsInteger(got); err != nil || n != 1 {
		t.Errorf("vals = %v (%v), want 1", got, err)
	}

	// A second write joins.
	s.RecordVal(NewInteger(2))
	if joins != 1 {
		t.Errorf("join invocations = %d, want 1", joins)
	}

	// A dispatch-bearing value poisons the join; readers decline from
	// then on, and later writes are ignored.
	s.RecordVal(NewFunction(FnDefInfo{}))
	if !s.ValsPoisoned {
		t.Fatal("a Function write must poison the unkeyed join")
	}
	if _, ok := s.LookupVals(); ok {
		t.Error("poisoned LookupVals must decline")
	}
	s.RecordVal(NewInteger(3)) // ignored: already poisoned
	if joins != 1 {
		t.Error("a write after poisoning must not join")
	}

	// A zero Value (Parent == nil) poisons a fresh shape too.
	s2 := &StoreShapeInfo{}
	s2.RecordVal(Value{})
	if !s2.ValsPoisoned {
		t.Error("a parentless value must poison the join")
	}
}

func TestStage5ShapeCloneShape(t *testing.T) {
	var nilShape *StoreShapeInfo
	if nilShape.CloneShape() != nil {
		t.Error("CloneShape of nil must be nil")
	}

	// A shape with no KeyTypes clones with a nil map.
	bare := &StoreShapeInfo{Scope: 1}
	if cp := bare.CloneShape(); cp == nil || cp.KeyTypes != nil || cp.Scope != 1 {
		t.Errorf("bare clone = %+v, want Scope 1 and nil KeyTypes", cp)
	}

	s := &StoreShapeInfo{Scope: 3, DeclaredVal: TInteger, Vals: NewInteger(9), ValsPoisoned: false}
	s.KeyTypes = map[string]Value{
		"a": NewInteger(1),
		"b": NewString("x"),
	}
	cp := s.CloneShape()
	if cp == nil {
		t.Fatal("CloneShape returned nil for a populated shape")
	}
	if cp.Scope != 3 || cp.DeclaredVal != TInteger || cp.ValsPoisoned {
		t.Errorf("clone lost scalar state: %+v", cp)
	}
	if n, err := AsInteger(cp.Vals); err != nil || n != 9 {
		t.Errorf("clone Vals = %v (%v), want 9", cp.Vals, err)
	}
	if len(cp.KeyTypes) != 2 {
		t.Fatalf("clone KeyTypes size = %d, want 2", len(cp.KeyTypes))
	}
	// The map is COPIED: a later write through the clone must not appear
	// in the original (entry-level sharing only).
	cp.KeyTypes["c"] = NewInteger(3)
	if _, ok := s.KeyTypes["c"]; ok {
		t.Error("clone map must be independent of the original")
	}
}
