package native

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
	instanceVal := NewObjectInstance(TObject, instance)

	// Verify initial value
	result := runAQL(t, r, []Value{
		instanceVal, NewWord("get"), NewWord("name"),
	})
	_as0, _ := AsString(result[0])
	if len(result) != 1 || _as0 != "Alice" {
		t.Fatalf("initial: got %v, want Alice", result)
	}

	// Mutate: set name "Bob" on the instance
	runAQL(t, r, []Value{
		instanceVal, NewWord("set"), NewWord("name"), NewString("Bob"),
	})

	// Verify mutation persisted (same instance)
	result = runAQL(t, r, []Value{
		instanceVal, NewWord("get"), NewWord("name"),
	})
	_as1, _ := AsString(result[0])
	if len(result) != 1 || _as1 != "Bob" {
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
	instanceVal := NewObjectInstance(TObject, instance)

	// Mutate via string key
	result := runAQL(t, r, []Value{
		instanceVal, NewWord("set"), NewString("x"), NewInteger(99),
	})
	_ = result

	// Verify
	result = runAQL(t, r, []Value{
		instanceVal, NewWord("get"), NewString("x"),
	})
	_as2, _ := AsInteger(result[0])
	if len(result) != 1 || _as2 != 99 {
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
	instanceVal := NewObjectInstance(TObject, instance)

	// Add a new field "b"
	runAQL(t, r, []Value{
		instanceVal, NewWord("set"), NewWord("b"), NewInteger(2),
	})

	// Read it back
	result := runAQL(t, r, []Value{
		instanceVal, NewWord("get"), NewWord("b"),
	})
	_as3, _ := AsInteger(result[0])
	if len(result) != 1 || _as3 != 2 {
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
	ref1 := NewObjectInstance(TObject, instance)
	ref2 := NewObjectInstance(TObject, instance) // same underlying Fields pointer

	// Mutate via ref1
	runAQL(t, r, []Value{
		ref1, NewWord("set"), NewWord("v"), NewInteger(42),
	})

	// Read via ref2 — should see the mutation
	result := runAQL(t, r, []Value{
		ref2, NewWord("get"), NewWord("v"),
	})
	_as4, _ := AsInteger(result[0])
	if len(result) != 1 || _as4 != 42 {
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
	_as5, _ := AsInteger(result[0])
	if len(result) != 1 || _as5 != 1 {
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

	rm, _ := AsMap(mapVal)
	if rm == nil {
		t.Fatal("AsMap returned nil")
	}

	// ReadMap supports Get, Keys, SortedKeys, Len
	v, ok := rm.Get("x")
	_as6, _ := AsInteger(v)
	if !ok || _as6 != 1 {
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
	om, err := AsMutableMap(rawMap)
	if err != nil {
		t.Fatalf("AsMutableMap returned err for raw map: %v", err)
	}
	// *OrderedMap supports Set (for internal construction paths)
	om.Set("y", NewInteger(2))
	v2, ok2 := om.Get("y")
	_as7, _ := AsInteger(v2)
	if !ok2 || _as7 != 2 {
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
	inst := NewObjectInstance(TObject, ObjectInstanceInfo{
		TypeRef: &objType,
		Fields:  objFields,
	})
	// AsMap returns an error for Object (Data is ObjectInstanceInfo, not *OrderedMap)
	if _m, err := AsMap(inst); err == nil && _m != nil {
		t.Fatal("AsMap should return nil/error for Object instance")
	}
	// But Fields are mutable via the ObjectInstanceInfo
	oi := inst.Data.(ObjectInstanceInfo)
	oi.Fields.Set("v", NewInteger(42))
	v3, _ := oi.Fields.Get("v")
	_as8, _ := AsInteger(v3)
	if _as8 != 42 {
		t.Fatalf("object field mutation: got %v, want 42", v3)
	}
}

// --- ReadList interface enforces List immutability at compile time ---

func TestAsListReturnsReadList(t *testing.T) {
	listVal := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})

	rl, _ := AsList(listVal)
	if rl.IsNil() {
		t.Fatal("AsList returned nil ReadList")
	}

	// ReadList supports Get, Len, Slice, IsNil
	if rl.Len() != 3 {
		t.Fatalf("Len: got %d, want 3", rl.Len())
	}
	_as9, _ := AsInteger(rl.Get(0))
	if _as9 != 1 {
		t.Fatalf("Get(0): got %v, want 1", rl.Get(0))
	}
	_as10, _ := AsInteger(rl.Get(2))
	if _as10 != 3 {
		t.Fatalf("Get(2): got %v, want 3", rl.Get(2))
	}

	// Slice returns a copy — mutating the copy doesn't affect the original
	sliceCopy := rl.Slice()
	sliceCopy[0] = NewInteger(99)
	_as11, _ := AsInteger(rl.Get(0))
	if _as11 != 1 {
		t.Fatal("mutating Slice() copy should not affect ReadList")
	}
}

// --- Array mutability ---

func TestArrayGetByIndex(t *testing.T) {
	r, _ := DefaultRegistry()
	arr := NewArray([]Value{NewInteger(10), NewInteger(20), NewInteger(30)})

	result := runAQL(t, r, []Value{
		arr, NewWord("get"), NewInteger(1),
	})
	_as12, _ := AsInteger(result[0])
	if len(result) != 1 || _as12 != 20 {
		t.Fatalf("got %v, want 20", result)
	}
}

func TestArrayGetOutOfBoundsReturnsNone(t *testing.T) {
	r, _ := DefaultRegistry()
	arr := NewArray([]Value{NewInteger(10)})

	result := runAQL(t, r, []Value{
		arr, NewWord("get"), NewInteger(5),
	})
	if len(result) != 1 || !result[0].VType.Equal(TNone) {
		t.Fatalf("got %v, want None", result)
	}
}

func TestArraySetByIndex(t *testing.T) {
	r, _ := DefaultRegistry()
	arr := NewArray([]Value{NewInteger(10), NewInteger(20)})

	// set 0 99 arr
	runAQL(t, r, []Value{
		arr, NewWord("set"), NewInteger(0), NewInteger(99),
	})

	// Verify mutation
	result := runAQL(t, r, []Value{
		arr, NewWord("get"), NewInteger(0),
	})
	_as13, _ := AsInteger(result[0])
	if len(result) != 1 || _as13 != 99 {
		t.Fatalf("after set: got %v, want 99", result)
	}
}

func TestArraySetOutOfBoundsErrors(t *testing.T) {
	r, _ := DefaultRegistry()
	arr := NewArray([]Value{NewInteger(10)})

	err := runAQLError(t, r, []Value{
		arr, NewWord("set"), NewInteger(5), NewInteger(99),
	})
	if err == nil {
		t.Fatal("expected error for out-of-bounds set")
	}
}

func TestArrayMutationSharedReference(t *testing.T) {
	// Two values wrapping the same ArrayInstanceInfo see mutations
	r, _ := DefaultRegistry()
	ai := &ArrayInstanceInfo{Elems: []Value{NewInteger(0)}}
	ref1 := NewValueRaw(TArray, ai)
	ref2 := NewValueRaw(TArray, ai)

	runAQL(t, r, []Value{
		ref1, NewWord("set"), NewInteger(0), NewInteger(42),
	})

	result := runAQL(t, r, []Value{
		ref2, NewWord("get"), NewInteger(0),
	})
	_as14, _ := AsInteger(result[0])
	if len(result) != 1 || _as14 != 42 {
		t.Fatalf("ref2 got %v, want 42 (shared mutation)", result)
	}
}

func TestArrayIsDistinctFromList(t *testing.T) {
	// Array is Ideal/Array; List is Node/List — different branches.
	arr := NewArray([]Value{NewInteger(1)})
	list := NewList([]Value{NewInteger(1)})

	if arr.VType.Matches(TList) {
		t.Error("Array should not match TList")
	}
	if list.VType.Matches(TArray) {
		t.Error("List should not match TArray")
	}
	if !arr.VType.Matches(TIdeal) {
		t.Error("Array should match TIdeal")
	}
	if !arr.VType.Matches(TArray) {
		t.Error("Array should match TArray")
	}
}

func TestArrayStringRepresentation(t *testing.T) {
	arr := NewArray([]Value{NewInteger(1), NewString("hello")})
	s := arr.String()
	if s != "Array[1,'hello']" {
		t.Errorf("got %q, want Array[1,'hello']", s)
	}
}

// --- Store copy-on-write ---

func TestStoreCOWBasic(t *testing.T) {
	// set on a Store creates a new COW layer; get resolves through prototype.
	r, _ := DefaultRegistry()
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewWord("k"), NewInteger(7),
		NewEnd(),
		NewWord("context"), NewWord("get"), NewWord("k"),
	})
	_as15, _ := AsInteger(result[0])
	if len(result) != 1 || _as15 != 7 {
		t.Fatalf("got %v, want 7", result)
	}
}

func TestStoreCOWDoesNotMutateOriginal(t *testing.T) {
	// After COW set, the original Store is unchanged.
	store := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     map[string]Value{"x": NewInteger(1)},
	}
	r, _ := DefaultRegistry()
	r.InitRootContext()
	// Put store in context
	ctx := r.Contexts.Top()
	ctx.Set("s", NewStoreValue(TStore, store))

	e := New(r)
	// Get the nested store, set a key on it
	_, err := e.Run([]Value{
		NewWord("context"), NewWord("get"), NewWord("s"),
		NewWord("set"), NewWord("y"), NewInteger(99),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The original store should NOT have key "y"
	if _, ok := store.Data["y"]; ok {
		t.Fatal("COW violated: original store was mutated")
	}
	// Original "x" is still 1
	_as16, _ := AsInteger(store.Data["x"])
	if _as16 != 1 {
		t.Fatal("original store's x was changed")
	}
}

func TestStoreCOWParentPropagation(t *testing.T) {
	// Nested stores: context → parent → child
	// Set on child should COW child AND propagate to parent in context.
	r, _ := DefaultRegistry()
	r.InitRootContext()
	ctx := r.Contexts.Top()

	child := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     make(map[string]Value),
	}
	child.Set("val", NewInteger(10))
	parent := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     make(map[string]Value),
	}
	parent.Set("child", NewStoreValue(TStore, child))
	ctx.Set("parent", NewStoreValue(TStore, parent))

	e := New(r)
	// context get parent → get child → set val 42
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("get"), NewWord("parent"),
		NewWord("get"), NewWord("child"),
		NewWord("set"), NewWord("val"), NewInteger(42),
		NewEnd(),
		// Now read it back through the context
		NewWord("context"), NewWord("get"), NewWord("parent"),
		NewWord("get"), NewWord("child"),
		NewWord("get"), NewWord("val"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as17, _ := AsInteger(result[0])
	if len(result) != 1 || _as17 != 42 {
		t.Fatalf("got %v, want 42 (COW propagated through parent)", result)
	}

	// Original child store should be unchanged
	_as18, _ := AsInteger(child.Data["val"])
	if _as18 != 10 {
		t.Fatal("COW violated: original child was mutated")
	}
}

func TestStoreCOWPrototypeResolution(t *testing.T) {
	// After COW, unchanged keys resolve through the prototype chain.
	r, _ := DefaultRegistry()
	r.InitRootContext()
	ctx := r.Contexts.Top()

	store := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     map[string]Value{"a": NewInteger(1), "b": NewInteger(2)},
	}
	ctx.Set("s", NewStoreValue(TStore, store))

	e := New(r)
	// Set "a" to 99, then read both "a" and "b"
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("get"), NewWord("s"),
		NewWord("set"), NewWord("a"), NewInteger(99),
		NewEnd(),
		// Read "a" (from COW layer)
		NewOpenParen(),
		NewWord("context"), NewWord("get"), NewWord("s"),
		NewWord("get"), NewWord("a"),
		NewCloseParen(),
		// Read "b" (from prototype, unchanged)
		NewOpenParen(),
		NewWord("context"), NewWord("get"), NewWord("s"),
		NewWord("get"), NewWord("b"),
		NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2", len(result))
	}
	_as19, _ := AsInteger(result[0])
	if _as19 != 99 {
		t.Errorf("a = %v, want 99 (from COW layer)", result[0])
	}
	_as20, _ := AsInteger(result[1])
	if _as20 != 2 {
		t.Errorf("b = %v, want 2 (from prototype)", result[1])
	}
}
