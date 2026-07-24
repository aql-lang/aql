package eng

import "testing"

// NUR031: the opaque Ideal handles are eq/deq-comparable instead of
// equal to nothing. Store gets reference identity (eq) and structural
// entry equality (deq); Error compares by fields (eq ≡ deq); the timer
// handles compare by identity under both. The code/opaque values
// (Function, Module, Word) stay equal to nothing — an argued remainder.

func storeVal(entries map[string]Value) Value {
	return NewStoreValue(TStore, &StoreInstanceInfo{Data: entries})
}

func errVal(code, msg string, data *OrderedMap) Value {
	return NewValueRaw(TError, ErrorInfo{Code: code, Message: msg, Data: data})
}

func TestNUR031StoreEquality(t *testing.T) {
	si := &StoreInstanceInfo{Data: map[string]Value{"x": NewInteger(1)}}
	sA := NewStoreValue(TStore, si)
	sB := NewStoreValue(TStore, si) // same handle
	twin := storeVal(map[string]Value{"x": NewInteger(1)})
	other := storeVal(map[string]Value{"x": NewInteger(2)})
	bigger := storeVal(map[string]Value{"x": NewInteger(1), "y": NewInteger(2)})
	crossLeaf := storeVal(map[string]Value{"x": NewFloat(1.0)})

	// eq is reference identity.
	if !ExactEqual(sA, sB) {
		t.Error("same store handle must be eq to itself")
	}
	if ExactEqual(sA, twin) {
		t.Error("two distinct stores with equal entries must NOT be eq (that is deq)")
	}
	// deq is structural over own entries.
	if !DeepEqual(sA, sB) {
		t.Error("same store must be deq")
	}
	if !DeepEqual(sA, twin) {
		t.Error("distinct stores with equal entries must be deq")
	}
	if !DeepEqual(sA, crossLeaf) {
		t.Error("store deq must apply cross-leaf magnitude to entry values (1 deq 1.0)")
	}
	if DeepEqual(sA, other) {
		t.Error("stores with a differing entry value must not be deq")
	}
	if DeepEqual(sA, bigger) {
		t.Error("stores of differing size must not be deq")
	}
	// storeDeepEqual nil arms.
	if storeDeepEqual(nil, si) || storeDeepEqual(si, nil) {
		t.Error("a nil store info is deq to nothing")
	}
	if !storeDeepEqual(si, si) {
		t.Error("identical pointer must short-circuit true")
	}
	// A store is never eq/deq to a non-store (handled=false path for b).
	if ExactEqual(sA, NewInteger(1)) || DeepEqual(sA, NewInteger(1)) {
		t.Error("store vs non-store must be unequal")
	}
	// key-present-but-value-differs vs key-absent both fail.
	if DeepEqual(sA, storeVal(map[string]Value{"z": NewInteger(1)})) {
		t.Error("differing KEY set must not be deq")
	}
}

// TestNUR031SubtypeRequiresSameType is the regression guard for the
// store-subtype defect the NUR031 review caught: deq of the value-like
// handles (Store, Error) requires the SAME exact type, like flat
// instances, so a `refine Store` subtype is never deq to a plain Store
// even with equal entries. Without this, storeDeepEqual ignored Parent
// while DeqKey's deqFam keyed by type ID, so a plain store and a
// subtype store landed in different scan families and unique/member/
// group MISSED the (wrongly-)equal pair.
func TestNUR031SubtypeRequiresSameType(t *testing.T) {
	entries := func() map[string]Value { return map[string]Value{"x": NewInteger(1)} }
	plain := NewStoreValue(TStore, &StoreInstanceInfo{Data: entries()})
	sub := NewStoreValue(TStoreSystem, &StoreInstanceInfo{Data: entries()}) // a Store subtype
	if DeepEqual(plain, sub) {
		t.Error("a Store subtype must not be deq to a plain Store, even with equal entries")
	}
	if ExactEqual(plain, sub) {
		t.Error("...nor eq (distinct handles of distinct types)")
	}
	// Two subtype stores with equal entries ARE deq (same exact type).
	sub2 := NewStoreValue(TStoreSystem, &StoreInstanceInfo{Data: entries()})
	if !DeepEqual(sub, sub2) {
		t.Error("two subtype stores with equal entries must be deq")
	}
	// Errors of differing type are likewise not equal even with equal fields.
	errPlain := NewValueRaw(TError, ErrorInfo{Code: "c", Message: "m"})
	errOther := NewValueRaw(TStoreSystem, ErrorInfo{Code: "c", Message: "m"})
	if DeepEqual(errPlain, errOther) || ExactEqual(errPlain, errOther) {
		t.Error("errors of differing type must not be equal even with equal fields")
	}
	// The DeqKey contract now holds: a plain store and a subtype store
	// are NOT deq, so a DeqIndex correctly keeps them separate.
	var idx DeqIndex
	idx.Add(plain)
	if idx.FirstMatch(sub) != -1 {
		t.Error("DeqIndex must not match a subtype store to a plain store")
	}
	if idx.FirstMatch(NewStoreValue(TStore, &StoreInstanceInfo{Data: entries()})) != 0 {
		t.Error("DeqIndex must still match a same-type structural twin")
	}
}

func TestNUR031ErrorEquality(t *testing.T) {
	base := errVal("bad_input", "boom", nil)
	same := errVal("bad_input", "boom", nil)
	diffCode := errVal("other", "boom", nil)
	diffMsg := errVal("bad_input", "bang", nil)

	d1 := NewOrderedMap()
	d1.Set("k", NewInteger(1))
	d2 := NewOrderedMap()
	d2.Set("k", NewInteger(1))
	d3 := NewOrderedMap()
	d3.Set("k", NewInteger(2))
	withData := errVal("bad_input", "boom", d1)
	withData2 := errVal("bad_input", "boom", d2)
	withData3 := errVal("bad_input", "boom", d3)

	for _, tc := range []struct {
		name string
		a, b Value
		want bool
	}{
		{"identical fields", base, same, true},
		{"differing code", base, diffCode, false},
		{"differing message", base, diffMsg, false},
		{"equal data maps", withData, withData2, true},
		{"differing data maps", withData, withData3, false},
		{"one nil data, one empty", base, errVal("bad_input", "boom", NewOrderedMap()), true},
		{"data vs no data", withData, base, false},
	} {
		if got := ExactEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("eq %s = %v, want %v", tc.name, got, tc.want)
		}
		if got := DeepEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("deq %s = %v, want %v", tc.name, got, tc.want)
		}
	}
	// Error vs non-error (handled=false for b).
	if ExactEqual(base, NewInteger(1)) || DeepEqual(base, NewInteger(1)) {
		t.Error("error vs non-error must be unequal")
	}
}

func TestNUR031TimerEquality(t *testing.T) {
	// The equality dispatches on the payload type, so the Parent used to
	// wrap it is cosmetic here — a *TimeoutInfo / *IntervalInfo payload
	// exercises the timer arms of both eq and deq.
	to := &TimeoutInfo{ID: "t1", Ms: 100}
	toA := NewValueRaw(TIdeal, to)
	toB := NewValueRaw(TIdeal, to) // same handle
	toOther := NewValueRaw(TIdeal, &TimeoutInfo{ID: "t2", Ms: 100})
	iv := &IntervalInfo{ID: "i1", Ms: 100}
	ivA := NewValueRaw(TIdeal, iv)
	ivB := NewValueRaw(TIdeal, iv)
	ivOther := NewValueRaw(TIdeal, &IntervalInfo{ID: "i2"})

	for _, tc := range []struct {
		name string
		a, b Value
		want bool
	}{
		{"same timeout", toA, toB, true},
		{"distinct timeouts", toA, toOther, false},
		{"same interval", ivA, ivB, true},
		{"distinct intervals", ivA, ivOther, false},
		{"timeout vs interval", toA, ivA, false},
	} {
		if got := ExactEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("eq %s = %v, want %v", tc.name, got, tc.want)
		}
		if got := DeepEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("deq %s = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestNUR031HandleFamilyByKind is the regression guard for the timer
// deqFam defect the PR #311 review caught: a timer's deq is pointer
// identity (Parent-independent), so a timer reparented to a `refine`
// subtype — same handle pointer, different Parent — is DeepEqual to its
// base and MUST share its deq scan family. deqFam therefore buckets
// handles by PAYLOAD KIND, not by ValueType, so DeqIndex finds the
// alias. A Store/Error, whose deq requires the same type, stays sound
// under kind-bucketing too (DeepEqual settles the pair within the one
// family).
func TestNUR031HandleFamilyByKind(t *testing.T) {
	to := &TimeoutInfo{ID: "t", Ms: 5}
	base := NewValueRaw(TIdeal, to)      // "base type" timer
	sub := NewValueRaw(TStoreSystem, to) // reparented alias: same pointer, other Parent
	if !DeepEqual(base, sub) {
		t.Fatal("a timer alias (same handle, different type) must be deq to its base")
	}
	if deqFam(base) != deqFam(sub) {
		t.Fatalf("timer aliases must share a deq family: %q vs %q", deqFam(base), deqFam(sub))
	}
	var idx DeqIndex
	idx.Add(base)
	if idx.FirstMatch(sub) != 0 {
		t.Fatal("DeqIndex must find a reparented timer alias (the P2 defect)")
	}
	// A Store and an Error are different kinds → different families, so
	// the scan stays tight (they can never be deq).
	if deqFam(storeVal(nil)) == deqFam(errVal("c", "m", nil)) {
		t.Fatal("Store and Error must be in different deq families")
	}
	// handleKind classifies each handle and nothing else.
	if handleKind(storeVal(nil)) != "Store" || handleKind(errVal("c", "m", nil)) != "Error" ||
		handleKind(base) != "Timeout" || handleKind(NewValueRaw(TIdeal, &IntervalInfo{ID: "i"})) != "Interval" {
		t.Fatal("handleKind must tag each deq-comparable handle by payload kind")
	}
	if handleKind(NewInteger(1)) != "" {
		t.Fatal("handleKind must return empty for a non-handle")
	}
}

// TestNUR031DeqKeyClassification pins that the newly deq-comparable
// handles bucket as DeqUnkeyed (scanned pairwise) while the code/opaque
// values remain DeqNeverEqual, and that a DeqIndex finds a store's
// structural twin.
func TestNUR031DeqKeyClassification(t *testing.T) {
	for _, v := range []Value{
		storeVal(map[string]Value{"x": NewInteger(1)}),
		errVal("c", "m", nil),
		NewValueRaw(TIdeal, &TimeoutInfo{ID: "t"}),
		NewValueRaw(TIdeal, &IntervalInfo{ID: "i"}),
	} {
		if _, c := DeqKey(v); c != DeqUnkeyed {
			t.Errorf("handle %T must be DeqUnkeyed, got class %d", v.Data, c)
		}
		if deqFam(v) == "" {
			t.Errorf("handle %T must have a deq family", v.Data)
		}
	}
	// A code value stays never-equal.
	if _, c := DeqKey(NewWord("f")); c != DeqNeverEqual {
		t.Error("a Word must stay DeqNeverEqual")
	}
	// DeqIndex finds a structural store twin (deq), not an eq-only match.
	var idx DeqIndex
	idx.Add(storeVal(map[string]Value{"x": NewInteger(1)}))
	if idx.FirstMatch(storeVal(map[string]Value{"x": NewInteger(1)})) != 0 {
		t.Error("DeqIndex must find the structural store twin")
	}
	if idx.FirstMatch(storeVal(map[string]Value{"x": NewInteger(9)})) != -1 {
		t.Error("DeqIndex must not match a differing store")
	}
}
