package eng

import "testing"

// TestDottedParamTypeArms drives dottedParamType's decline arms directly
// (the in-package seam): every malformed dotted param spec must fall back
// to the invalid-parameter error, never resolve to a type.
func TestDottedParamTypeArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	seg := ReachSeg{KeyLit: NewAtom("Matrix")}
	reach := func(recv Value, segs ...ReachSeg) Value {
		return NewValueRaw(TReach, ReachInfo{Receiver: []Value{recv}, Segments: segs})
	}

	// nil registry.
	if _, ok := dottedParamType(nil, reach(NewWord("Pkg"), seg)); ok {
		t.Fatal("nil registry must decline")
	}
	// Not a reach at all / no segments / multi-receiver.
	if _, ok := dottedParamType(r, NewInteger(1)); ok {
		t.Fatal("non-reach must decline")
	}
	if _, ok := dottedParamType(r, reach(NewWord("Pkg"))); ok {
		t.Fatal("segmentless reach must decline")
	}
	if _, ok := dottedParamType(r, NewValueRaw(TReach, ReachInfo{
		Receiver: []Value{NewWord("A"), NewWord("B")}, Segments: []ReachSeg{seg},
	})); ok {
		t.Fatal("multi-token receiver must decline")
	}
	// Computed segment.
	if _, ok := dottedParamType(r, reach(NewWord("Pkg"), ReachSeg{Computed: true, KeyExpr: []Value{NewWord("k")}})); ok {
		t.Fatal("computed segment must decline")
	}
	// A Word-typed receiver with a non-Word payload (AsWord error).
	if _, ok := dottedParamType(r, reach(NewValueRaw(TWord, StrPayload{S: "x"}), seg)); ok {
		t.Fatal("malformed word receiver must decline")
	}
	// Map receivers: multi-key, non-word value, malformed word value,
	// invalid param name.
	m2 := NewOrderedMap()
	m2.Set("a", NewWord("Pkg"))
	m2.Set("b", NewWord("Pkg"))
	if _, ok := dottedParamType(r, reach(NewMap(m2), seg)); ok {
		t.Fatal("multi-key map receiver must decline")
	}
	m3 := NewOrderedMap()
	m3.Set("mat", NewInteger(1))
	if _, ok := dottedParamType(r, reach(NewMap(m3), seg)); ok {
		t.Fatal("non-word map value must decline")
	}
	m4 := NewOrderedMap()
	m4.Set("mat", NewValueRaw(TWord, StrPayload{S: "x"}))
	if _, ok := dottedParamType(r, reach(NewMap(m4), seg)); ok {
		t.Fatal("malformed word map value must decline")
	}
	m5 := NewOrderedMap()
	m5.Set("bad name", NewWord("Pkg"))
	if _, ok := dottedParamType(r, reach(NewMap(m5), seg)); ok {
		t.Fatal("invalid param name must decline")
	}
	// Receiver of a kind the spec grammar never produces.
	if _, ok := dottedParamType(r, reach(NewInteger(7), seg)); ok {
		t.Fatal("integer receiver must decline")
	}
	// Unbound base word.
	if _, ok := dottedParamType(r, reach(NewWord("no-such-pkg"), seg)); ok {
		t.Fatal("unbound base must decline")
	}
}
