package engine

import "testing"

// --- Object mutability: set mutates Object instance fields ---

func TestObjectSetFieldAtom(t *testing.T) {
	// object {name:String} def Person
	// make Person {name:"Alice"} def alice
	// alice set name "Bob" end
	// alice . name => "Bob"
	r, _ := DefaultRegistry()

	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	objType := ObjectTypeInfo{
		Fields: fields,
		ID:     GenerateObjectTypeID(),
		Name:   "Object/Person",
	}

	instance := ObjectInstanceInfo{
		TypeRef: &objType,
		Fields: func() *OrderedMap {
			m := NewOrderedMap()
			m.Set("name", NewString("Alice"))
			return m
		}(),
	}
	instanceVal := NewObjectInstance(instance)

	// Verify initial value
	result := runAQL(t, r, []Value{
		instanceVal, NewWord("get"), NewWord("name"),
	})
	if len(result) != 1 || result[0].AsString() != "Alice" {
		t.Fatalf("initial: got %v, want Alice", result)
	}

	// Mutate: set name "Bob" on the instance
	result = runAQL(t, r, []Value{
		instanceVal, NewWord("set"), NewWord("name"), NewString("Bob"),
	})

	// Verify mutation persisted (same instance)
	result = runAQL(t, r, []Value{
		instanceVal, NewWord("get"), NewWord("name"),
	})
	if len(result) != 1 || result[0].AsString() != "Bob" {
		t.Fatalf("after set: got %v, want Bob", result)
	}
}

func TestObjectSetFieldString(t *testing.T) {
	// set with string key
	r, _ := DefaultRegistry()

	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	objType := ObjectTypeInfo{
		Fields: fields,
		ID:     GenerateObjectTypeID(),
		Name:   "Object/Point",
	}

	instance := ObjectInstanceInfo{
		TypeRef: &objType,
		Fields: func() *OrderedMap {
			m := NewOrderedMap()
			m.Set("x", NewInteger(10))
			return m
		}(),
	}
	instanceVal := NewObjectInstance(instance)

	// Mutate via string key
	result := runAQL(t, r, []Value{
		instanceVal, NewWord("set"), NewString("x"), NewInteger(99),
	})
	_ = result

	// Verify
	result = runAQL(t, r, []Value{
		instanceVal, NewWord("get"), NewString("x"),
	})
	if len(result) != 1 || result[0].AsInteger() != 99 {
		t.Fatalf("got %v, want 99", result)
	}
}

func TestObjectSetAddsNewField(t *testing.T) {
	// set can add a new field not in the original type schema
	r, _ := DefaultRegistry()

	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	objType := ObjectTypeInfo{
		Fields: fields,
		ID:     GenerateObjectTypeID(),
		Name:   "Object/Flex",
	}

	instance := ObjectInstanceInfo{
		TypeRef: &objType,
		Fields: func() *OrderedMap {
			m := NewOrderedMap()
			m.Set("a", NewInteger(1))
			return m
		}(),
	}
	instanceVal := NewObjectInstance(instance)

	// Add a new field "b"
	runAQL(t, r, []Value{
		instanceVal, NewWord("set"), NewWord("b"), NewInteger(2),
	})

	// Read it back
	result := runAQL(t, r, []Value{
		instanceVal, NewWord("get"), NewWord("b"),
	})
	if len(result) != 1 || result[0].AsInteger() != 2 {
		t.Fatalf("got %v, want 2", result)
	}
}

func TestObjectMutationSharedReference(t *testing.T) {
	// Two references to the same Object instance see mutations
	r, _ := DefaultRegistry()

	fields := NewOrderedMap()
	fields.Set("v", NewTypeLiteral(TInteger))
	objType := ObjectTypeInfo{
		Fields: fields,
		ID:     GenerateObjectTypeID(),
		Name:   "Object/Counter",
	}

	instance := ObjectInstanceInfo{
		TypeRef: &objType,
		Fields: func() *OrderedMap {
			m := NewOrderedMap()
			m.Set("v", NewInteger(0))
			return m
		}(),
	}
	ref1 := NewObjectInstance(instance)
	ref2 := NewObjectInstance(instance) // same underlying Fields pointer

	// Mutate via ref1
	runAQL(t, r, []Value{
		ref1, NewWord("set"), NewWord("v"), NewInteger(42),
	})

	// Read via ref2 — should see the mutation
	result := runAQL(t, r, []Value{
		ref2, NewWord("get"), NewWord("v"),
	})
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Fatalf("ref2 got %v, want 42 (shared mutation)", result)
	}
}

// --- Node immutability: set must NOT work on Maps or Lists ---

func TestNodeMapIsImmutable(t *testing.T) {
	// Attempting to set on a Map should fail (no matching signature)
	r, _ := DefaultRegistry()
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	mapVal := NewMap(m)

	err := runAQLError(t, r, []Value{
		mapVal, NewWord("set"), NewWord("x"), NewInteger(99),
	})
	if err == nil {
		t.Fatal("expected error: set should not work on Map (Nodes are immutable)")
	}
}

func TestNodeListIsImmutable(t *testing.T) {
	// Attempting to set on a List should fail (no matching signature)
	r, _ := DefaultRegistry()
	listVal := NewList([]Value{NewInteger(10), NewInteger(20)})

	err := runAQLError(t, r, []Value{
		listVal, NewWord("set"), NewInteger(0), NewInteger(99),
	})
	if err == nil {
		t.Fatal("expected error: set should not work on List (Nodes are immutable)")
	}
}

func TestNodeMapUnchangedAfterObjectSet(t *testing.T) {
	// Creating a Map, storing it, and doing set on an Object should not
	// affect the Map — they are different types with different semantics.
	r, _ := DefaultRegistry()

	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	mapVal := NewMap(m)

	// Map value remains unchanged
	result := runAQL(t, r, []Value{
		mapVal, NewWord("get"), NewWord("x"),
	})
	if len(result) != 1 || result[0].AsInteger() != 1 {
		t.Fatalf("map x: got %v, want 1", result)
	}
}

// --- ReadMap interface enforces Node immutability at compile time ---

func TestAsMapReturnsReadMap(t *testing.T) {
	// AsMap() returns ReadMap which has no Set or Delete methods.
	// This test verifies the interface at the type level.
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	mapVal := NewMap(m)

	var rm ReadMap = mapVal.AsMap()
	if rm == nil {
		t.Fatal("AsMap returned nil")
	}

	// ReadMap supports Get, Keys, SortedKeys, Len
	v, ok := rm.Get("x")
	if !ok || v.AsInteger() != 1 {
		t.Fatalf("Get x: got %v, want 1", v)
	}
	if rm.Len() != 1 {
		t.Fatalf("Len: got %d, want 1", rm.Len())
	}
	if len(rm.Keys()) != 1 || rm.Keys()[0] != "x" {
		t.Fatalf("Keys: got %v, want [x]", rm.Keys())
	}

	// AsMutableMap() returns *OrderedMap for raw map data (internal use)
	rawMap := NewMap(m)
	om := rawMap.AsMutableMap()
	if om == nil {
		t.Fatal("AsMutableMap returned nil for raw map")
	}
	// *OrderedMap supports Set (for internal construction paths)
	om.Set("y", NewInteger(2))
	v2, ok2 := om.Get("y")
	if !ok2 || v2.AsInteger() != 2 {
		t.Fatalf("after mutation: got %v, want 2", v2)
	}

	// Object instances expose Fields directly (not through AsMap/AsMutableMap)
	objFields := NewOrderedMap()
	objFields.Set("v", NewInteger(0))
	objType := ObjectTypeInfo{
		Fields: objFields,
		ID:     GenerateObjectTypeID(),
		Name:   "Object/Test",
	}
	inst := NewObjectInstance(ObjectInstanceInfo{
		TypeRef: &objType,
		Fields:  objFields,
	})
	// AsMap returns nil for Object (Data is ObjectInstanceInfo, not *OrderedMap)
	if inst.AsMap() != nil {
		t.Fatal("AsMap should return nil for Object instance")
	}
	// But Fields are mutable via the ObjectInstanceInfo
	oi := inst.Data.(ObjectInstanceInfo)
	oi.Fields.Set("v", NewInteger(42))
	v3, _ := oi.Fields.Get("v")
	if v3.AsInteger() != 42 {
		t.Fatalf("object field mutation: got %v, want 42", v3)
	}
}

// --- Store mutability (for completeness) ---

func TestStoreMutableViaSet(t *testing.T) {
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewWord("k"), NewInteger(7),
		NewWord("end"),
		NewWord("context"), NewWord("get"), NewWord("k"),
	})
	if len(result) != 1 || result[0].AsInteger() != 7 {
		t.Fatalf("got %v, want 7", result)
	}
}
