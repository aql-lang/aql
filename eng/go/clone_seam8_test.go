package eng

import "testing"

// TestS8CloneObject covers cloneObject (clone.go), reached when CloneValue
// deep-clones a class instance: the field map is duplicated value-by-value
// while the immutable TypeRef descriptor is shared.
func TestS8CloneObject(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewInteger(1))
	tref := &ClassTypeInfo{Name: "Class/C", Fields: NewOrderedMap()}
	inst := NewClassInstance(TClass, ClassInstanceInfo{TypeRef: tref, Fields: fields})

	cl := CloneValue(inst)
	ci, ok := cl.Data.(ClassInstanceInfo)
	if !ok {
		t.Fatalf("clone Data = %T, want ClassInstanceInfo", cl.Data)
	}
	if ci.TypeRef != tref {
		t.Error("cloneObject should share the immutable TypeRef descriptor")
	}
	if ci.Fields == fields {
		t.Error("cloneObject should duplicate the field map, not share it")
	}
	// Mutating the clone's field map must not touch the original.
	ci.Fields.Set("x", NewInteger(99))
	if got, _ := fields.Get("x"); func() int64 { n, _ := AsInteger(got); return n }() != 1 {
		t.Error("deep clone leaked: mutating the clone changed the original field map")
	}

	// nil-Fields arm: cloneObject leaves Fields nil rather than allocating.
	empty := CloneValue(NewClassInstance(TClass, ClassInstanceInfo{TypeRef: tref}))
	if ec, _ := empty.Data.(ClassInstanceInfo); ec.Fields != nil {
		t.Error("cloneObject with nil Fields should keep Fields nil")
	}
}
