package eng

import "testing"

// W9 store_shape.go coverage: the abstract check-mode store-shape payload
// helpers. All pure — driven by direct calls with crafted shapes and values.

func TestW9StoreShapeRecordAndLookupKey(t *testing.T) {
	// nil receiver / empty key are no-ops.
	var nilShape *StoreShapeInfo
	nilShape.RecordKey("k", NewInteger(1)) // must not panic
	if _, ok := nilShape.LookupKey("k"); ok {
		t.Error("nil shape must not resolve a key")
	}

	s := &StoreShapeInfo{}
	s.RecordKey("", NewInteger(1))
	if s.KeyTypes != nil {
		t.Error("an empty key must not allocate KeyTypes")
	}

	// First write, then a rewrite that joins.
	s.RecordKey("a", NewCarrier(TInteger))
	if _, ok := s.LookupKey("a"); !ok {
		t.Error("recorded key must resolve")
	}
	s.RecordKey("a", NewCarrier(TString))
	joined, ok := s.LookupKey("a")
	if !ok || !joined.Carrier {
		t.Errorf("rewrite should join into a carrier, got %v", joined)
	}
}

func TestW9StoreShapeRecordVal(t *testing.T) {
	// nil receiver and poisoned shape are no-ops.
	var nilShape *StoreShapeInfo
	nilShape.RecordVal(NewInteger(1)) // must not panic
	poisoned := &StoreShapeInfo{ValsPoisoned: true}
	poisoned.RecordVal(NewInteger(1))

	// A dispatch-bearing value poisons the unkeyed join.
	s := &StoreShapeInfo{}
	s.RecordVal(NewFunction(FnDefInfo{}))
	if !s.ValsPoisoned {
		t.Error("a function write must poison the unkeyed join")
	}
	if _, ok := s.LookupVals(); ok {
		t.Error("a poisoned join must decline LookupVals")
	}

	// A nil-Parent value also poisons.
	s2 := &StoreShapeInfo{}
	s2.RecordVal(Value{})
	if !s2.ValsPoisoned {
		t.Error("a nil-Parent write must poison the unkeyed join")
	}

	// First value sets Vals; a second joins.
	s3 := &StoreShapeInfo{}
	if _, ok := s3.LookupVals(); ok {
		t.Error("an empty shape has no unkeyed value")
	}
	s3.RecordVal(NewCarrier(TInteger))
	got, ok := s3.LookupVals()
	if !ok || got.Parent == nil {
		t.Errorf("first unkeyed write should set Vals, got %v (%v)", got, ok)
	}
	s3.RecordVal(NewCarrier(TString))
	if _, ok := s3.LookupVals(); !ok {
		t.Error("a second unkeyed write should still resolve (joined)")
	}
}

func TestW9StoreShapeCloneNil(t *testing.T) {
	var nilShape *StoreShapeInfo
	if nilShape.CloneShape() != nil {
		t.Error("cloning a nil shape must yield nil")
	}
}

func TestW9ContextShapeNilGuards(t *testing.T) {
	var nilCheck *CheckState
	if v := nilCheck.ContextShape(nil, 0); !v.Carrier {
		t.Error("nil check should return a bare store carrier")
	}
	c := &CheckState{}
	if v := c.ContextShape(nil, 0); !v.Carrier {
		t.Error("nil store should return a bare store carrier")
	}
}

func TestW9MintFlexShapeCarrierDeclines(t *testing.T) {
	// Depth over the cap declines.
	if _, ok := MintFlexShapeCarrier(NewMap(NewOrderedMap()), flexShapeMaxDepth+1); ok {
		t.Error("depth over the cap must decline")
	}
	// A TMap-tagged value whose payload is NOT a map payload passes the
	// concrete/conforms guards but declines at AsMap (err arm).
	badMap := Value{Parent: TMap, Data: ListPayload{}}
	if _, ok := MintFlexShapeCarrier(badMap, 0); ok {
		t.Error("a non-map payload must decline at AsMap")
	}
	// A non-map input declines.
	if _, ok := MintFlexShapeCarrier(NewInteger(1), 0); ok {
		t.Error("a non-map input must decline")
	}
}

func TestW9AdoptShapeValueXml(t *testing.T) {
	// A concrete Xml value adopts as a bare FlexXml carrier.
	got := AdoptShapeValue(NewXmlElement("a", nil, nil), 0)
	if !got.Carrier || !got.Parent.ConformsTo(TFlexXml) {
		t.Errorf("an Xml field should adopt as a FlexXml carrier, got %v", got)
	}
}

func TestW9ShapeFieldRead(t *testing.T) {
	// A disjunct read keeps its payload as a dynamic carrier value.
	dj := ShapeFieldRead(NewDisjunct([]Value{NewString("x")}))
	if !dj.Dynamic {
		t.Error("a disjunct read should be dynamic")
	}
	// A function-typed field keeps the dynamic(Any) hatch.
	fn := ShapeFieldRead(NewFunction(FnDefInfo{}))
	if !fn.Dynamic {
		t.Error("a function field should read dynamic(Any)")
	}
	// A nil-Parent value (no ValueType) also keeps dynamic(Any).
	none := ShapeFieldRead(Value{})
	if !none.Dynamic {
		t.Error("a typeless value should read dynamic(Any)")
	}
}
