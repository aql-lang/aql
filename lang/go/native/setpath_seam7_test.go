package native

import (
	"testing"
)

// Wave-7 coverage for setpath.go: the path-not-found reject, the
// stack-order data/newVal swap, the reach AsReach / reachPathKeys /
// setReachNative error arms, the dotted-string AsString reject, the
// isAllDigits guards, reachPathKeys' computed-key arms, and
// setReachNative's Object-instance branch (design/TEST-SEAMS.10.md).

func TestSetpathPathNotFound(t *testing.T) {
	r := b2Reg(t)
	// No String / Reach arg → pathIdx stays -1.
	if _, err := setpathHandler([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}, nil, nil, r); err == nil {
		t.Fatal("setpath with no path arg: expected error")
	}
}

func TestSetpathDataNewValSwapArm(t *testing.T) {
	r := b2Reg(t)
	om := NewOrderedMap()
	om.Set("k", NewInteger(1))
	// Two-arg call: the path sits at index 1, leaving only one non-path arg,
	// so others[0] >= others[1] and the else (stack-order) branch fires.
	out, err := setpathHandler([]Value{NewMap(om), NewString("x")}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("setpath 2-arg swap arm = %v / %v", out, err)
	}
}

func TestSetpathReachAsReachError(t *testing.T) {
	r := b2Reg(t)
	// A Reach-typed value whose Data is not a ReachInfo passes IsReach but
	// fails AsReach.
	badReach := NewTypeLiteral(TReach)
	if _, err := setpathHandler([]Value{mapVal("a", NewInteger(1)), badReach, NewInteger(9)}, nil, nil, r); err == nil {
		t.Fatal("setpath with reach type literal: expected AsReach error")
	}
}

func TestSetpathReachPathKeysError(t *testing.T) {
	r := b2Reg(t)
	// A reach whose computed key raises propagates through reachPathKeys.
	reach := NewReach(ReachInfo{Segments: []ReachSeg{
		{Computed: true, KeyExpr: []Value{NewWord("zzz_undef_word_xyz")}},
	}})
	if _, err := setpathHandler([]Value{mapVal("a", NewInteger(1)), reach, NewInteger(9)}, nil, nil, r); err == nil {
		t.Fatal("setpath with bad computed reach key: expected error")
	}
}

func TestSetpathReachSetNativeError(t *testing.T) {
	r := b2Reg(t)
	// A reach with a literal integer key out of the target list's range makes
	// setReachNative fail, exercising the reach-form error arm.
	reach := NewReach(ReachInfo{Segments: []ReachSeg{{KeyLit: NewInteger(5)}}})
	list := NewList([]Value{NewInteger(1), NewInteger(2)})
	if _, err := setpathHandler([]Value{list, reach, NewInteger(9)}, nil, nil, r); err == nil {
		t.Fatal("setpath reach index out of range: expected error")
	}
	// Positive pair: an in-range literal key updates the list.
	reach0 := NewReach(ReachInfo{Segments: []ReachSeg{{KeyLit: NewInteger(0)}}})
	out, err := setpathHandler([]Value{list, reach0, NewInteger(9)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("setpath reach in-range = %v / %v", out, err)
	}
}

func TestSetpathStringPathAsStringError(t *testing.T) {
	r := b2Reg(t)
	// A bare String type literal is String-conforming (so it is picked as the
	// path) but AsString rejects its nil data.
	if _, err := setpathHandler([]Value{mapVal("a", NewInteger(1)), NewTypeLiteral(TString), NewInteger(9)}, nil, nil, r); err == nil {
		t.Fatal("setpath with String type-literal path: expected AsString error")
	}
	// Positive pair: a real dotted path writes.
	out, err := setpathHandler([]Value{mapVal("a", NewInteger(1)), NewString("a"), NewInteger(9)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("setpath string path = %v / %v", out, err)
	}
}

func TestIsAllDigits(t *testing.T) {
	if isAllDigits("") {
		t.Fatal("isAllDigits(\"\") should be false")
	}
	if isAllDigits("1a") {
		t.Fatal("isAllDigits(\"1a\") should be false")
	}
	if !isAllDigits("12") {
		t.Fatal("isAllDigits(\"12\") should be true")
	}
}

func TestReachPathKeysComputed(t *testing.T) {
	r := b2Reg(t)
	// Success: a computed key that evaluates to a value.
	keys, err := reachPathKeys(r, ReachInfo{Segments: []ReachSeg{
		{Computed: true, KeyExpr: []Value{NewInteger(7)}},
	}})
	if err != nil || len(keys) != 1 || mustI(keys[0]) != 7 {
		t.Fatalf("computed key success = %v / %v", keys, err)
	}
	// Error: a computed key whose sub-run raises.
	if _, err := reachPathKeys(r, ReachInfo{Segments: []ReachSeg{
		{Computed: true, KeyExpr: []Value{NewWord("zzz_undef_word_xyz")}},
	}}); err == nil {
		t.Fatal("computed key error: expected error")
	}
	// No value: an empty key expression produces nothing.
	if _, err := reachPathKeys(r, ReachInfo{Segments: []ReachSeg{
		{Computed: true, KeyExpr: []Value{}},
	}}); err == nil {
		t.Fatal("computed key with no value: expected error")
	}
	// Literal key passes through.
	keys, err = reachPathKeys(r, ReachInfo{Segments: []ReachSeg{{KeyLit: NewString("a")}}})
	if err != nil || len(keys) != 1 {
		t.Fatalf("literal key = %v / %v", keys, err)
	}
}

func objInstance(t *testing.T, field string, v Value) Value {
	t.Helper()
	fields := NewOrderedMap()
	fields.Set(field, v)
	return NewClassInstance(TClass, ClassInstanceInfo{Fields: fields})
}

func TestSetReachNativeObjectInstance(t *testing.T) {
	r := b2Reg(t)
	// Multi-key path into an Object instance whose field is a nested map:
	// rebuilds the instance (non-class → NewClassInstance).
	inner := NewOrderedMap()
	inner.Set("b", NewInteger(1))
	obj := objInstance(t, "a", NewMap(inner))
	out, err := setReachNative(obj, []Value{NewString("a"), NewString("b")}, NewInteger(9), r)
	if err != nil || !IsClassInstance(out) {
		t.Fatalf("setReachNative obj multi-key = %v / %v", out, err)
	}

	// Error propagation: a nested set that fails (list index out of range)
	// bubbles up through the Object-instance branch.
	objList := objInstance(t, "a", NewList([]Value{NewInteger(1)}))
	if _, err := setReachNative(objList, []Value{NewString("a"), NewInteger(5)}, NewInteger(9), r); err == nil {
		t.Fatal("setReachNative obj nested error: expected error")
	}
}
