package eng

import "testing"

// CloneValue must produce a deep, independent copy: mutating the clone
// (or the original) must never be observable through the other.

func TestCloneMapIndependence(t *testing.T) {
	inner := NewOrderedMap()
	inner.Set("x", NewInteger(1))
	outer := NewOrderedMap()
	outer.Set("inner", NewMap(inner))
	orig := NewMap(outer)

	clone := CloneValue(orig)

	// The clone's nested map must be a distinct *OrderedMap.
	cloneInnerV, _ := clone.Data.(MapPayload).M.Get("inner")
	cloneInner := cloneInnerV.Data.(MapPayload).M
	if cloneInner == inner {
		t.Fatal("clone shares the original's nested *OrderedMap")
	}

	// Mutating the original's nested map must not touch the clone.
	inner.Set("x", NewInteger(999))
	if v, _ := cloneInner.Get("x"); func() int64 { n, _ := AsInteger(v); return n }() != 1 {
		t.Error("original mutation leaked into the clone's nested map")
	}
}

func TestCloneArrayIndependence(t *testing.T) {
	orig := NewArray([]Value{NewInteger(1), NewInteger(2)})
	clone := CloneValue(orig)

	ca, _ := AsArray(clone)
	ca.Elems[0] = NewInteger(42)

	oa, _ := AsArray(orig)
	if n, _ := AsInteger(oa.Elems[0]); n != 1 {
		t.Errorf("clone array mutation leaked: original[0] = %d, want 1", n)
	}
	// And the clone is a distinct backing slice.
	if &ca.Elems[0] == &oa.Elems[0] {
		t.Error("clone shares the original array's backing slice")
	}
}

func TestCloneStoreIndependence(t *testing.T) {
	si := &StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]Value{}}
	si.Set("k", NewInteger(1))
	orig := NewStoreValue(TStore, si)

	clone := CloneValue(orig)
	cs, _ := AsStore(clone)
	cs.Set("k", NewInteger(99))

	if v, _ := si.Get("k"); func() int64 { n, _ := AsInteger(v); return n }() != 1 {
		t.Error("clone store mutation leaked into the original")
	}
}

// A self-referential store must clone without infinite recursion, and
// the clone's cycle must point at the clone (not the original).
func TestCloneStoreCycle(t *testing.T) {
	si := &StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]Value{}}
	si.Set("self", NewStoreValue(TStore, si)) // cycle

	// If the cycle map were absent this would recurse forever and the
	// test would time out — the failure signal we want.
	clone := CloneValue(NewStoreValue(TStore, si))

	cs, _ := AsStore(clone)
	selfV, ok := cs.Data["self"]
	if !ok {
		t.Fatal("cycle key lost in clone")
	}
	selfStore, _ := AsStore(selfV)
	if selfStore != cs {
		t.Error("cloned cycle points at the original (or a second copy), not the clone")
	}
}

func TestCloneListSharedSubstructurePreserved(t *testing.T) {
	// A FlexList referenced twice in a list must clone to ONE shared
	// clone (cycle map), not two independent copies.
	shared := NewFlexList([]Value{NewInteger(1)})
	orig := NewList([]Value{shared, shared})

	clone := CloneValue(orig)
	cl, _ := AsList(clone)
	a := cl.Get(0)
	b := cl.Get(1)
	fa, _ := a.Data.(*FlexListData)
	fb, _ := b.Data.(*FlexListData)
	if fa == nil || fb == nil {
		t.Fatal("flexlist payloads missing after clone")
	}
	if fa != fb {
		t.Error("shared substructure was duplicated; the clone broke aliasing")
	}
}

func TestClonePreservesRefineType(t *testing.T) {
	pos := MintTestType("Scalar/Number/Integer/Pos")
	v := NewInteger(5)
	v.Parent = pos // a refine-subtyped integer
	clone := CloneValue(v)
	if !clone.Parent.Equal(pos) {
		t.Errorf("clone Parent = %v, want the Pos refine type", clone.Parent)
	}
}

func TestCloneSharesImmutableScalars(t *testing.T) {
	// Scalars carry value payloads; a clone is equal and has the same
	// payload (sharing is fine — nothing mutates a scalar in place).
	for _, v := range []Value{NewInteger(7), NewString("hi"), NewBoolean(true), NewFloat(1.5)} {
		c := CloneValue(v)
		if !ExactEqual(c, v) {
			t.Errorf("clone of scalar %v not equal to original", v)
		}
	}
}

func TestCloneTypeLiteralNoPanic(t *testing.T) {
	// A bare type literal (Data == nil) must clone without panicking.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("clone of type literal panicked: %v", r)
		}
	}()
	orig := NewTypeLiteral(TMap)
	c := CloneValue(orig)
	// A type literal IS its lattice node and has no payload; the clone is
	// the same node (shared, immutable).
	if c.ID != orig.ID || !ValueType(c).Equal(ValueType(orig)) {
		t.Error("type-literal clone is not the same lattice node")
	}
}

// fakeBody exercises the DeepCloner capability on an ExtensionPayload.
type fakeBody struct{ data []int }

func (f fakeBody) DeepClone() any {
	cp := make([]int, len(f.data))
	copy(cp, f.data)
	return fakeBody{data: cp}
}

func TestCloneExtensionDeepCloner(t *testing.T) {
	ft := MintTestType("Ideal/Fake")
	orig := NewExtension(ft, fakeBody{data: []int{1, 2, 3}})
	clone := CloneValue(orig)

	ob := orig.Data.(ExtensionPayload).Body.(fakeBody)
	cb := clone.Data.(ExtensionPayload).Body.(fakeBody)
	cb.data[0] = 99
	if ob.data[0] != 1 {
		t.Error("DeepCloner clone shares the original body slice")
	}
}
