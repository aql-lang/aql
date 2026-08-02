package eng

import "testing"

// A Store is a copy-on-write prototype chain: CowSet layers the new
// binding over the old store rather than editing it, because the old
// store may be shared with an enclosing scope. Removal cannot work the
// same way by subtraction — there is nothing in the new layer to leave
// out — so the layer records a TOMBSTONE and Get stops there.
//
// These tests pin the three properties the design turns on: a tombstone
// hides an inherited key from the deleting layer down, it never reaches
// the layer that owns the key, and a later set in the same layer wins
// over it.

// tsStore builds a bare root store with the given own entries.
func tsStore(entries map[string]Value) *StoreInstanceInfo {
	si := &StoreInstanceInfo{TypeName: "Object/Store", Data: map[string]Value{}}
	for k, v := range entries {
		si.Data[k] = v
	}
	return si
}

func tsGet(t *testing.T, si *StoreInstanceInfo, key string) (Value, bool) {
	t.Helper()
	return si.Get(key)
}

func TestStoreTombstoneHidesInheritedKey(t *testing.T) {
	base := tsStore(map[string]Value{"k": NewInteger(1)})
	if _, ok := tsGet(t, base, "k"); !ok {
		t.Fatal("base store must hold k")
	}

	// A layer over base that tombstones k.
	layer := &StoreInstanceInfo{
		TypeName:  base.TypeName,
		Deleted:   map[string]bool{"k": true},
		Prototype: base,
	}
	if _, ok := tsGet(t, layer, "k"); ok {
		t.Error("a tombstoned key must read as absent through the deleting layer")
	}
	// The owning layer is untouched — that is the whole point of not
	// editing it.
	if _, ok := tsGet(t, base, "k"); !ok {
		t.Error("the prototype must keep its own binding; the tombstone is layer-local")
	}
	// A key that was never tombstoned still resolves through the chain.
	base.Data["other"] = NewInteger(2)
	if _, ok := tsGet(t, layer, "other"); !ok {
		t.Error("an untombstoned key must still resolve through the prototype")
	}
}

func TestStoreOwnDataBeatsTombstone(t *testing.T) {
	base := tsStore(map[string]Value{"k": NewInteger(1)})
	layer := &StoreInstanceInfo{
		TypeName:  base.TypeName,
		Data:      map[string]Value{"k": NewInteger(9)},
		Deleted:   map[string]bool{"k": true},
		Prototype: base,
	}
	v, ok := tsGet(t, layer, "k")
	if !ok {
		t.Fatal("a re-bound key must read, tombstone or not")
	}
	if n, _ := AsInteger(v); n != 9 {
		t.Errorf("own Data must win over Deleted; got %v", v)
	}
}

func TestStoreTombstoneOnRootWithNoPrototype(t *testing.T) {
	// A tombstone with nothing below it: the key was never there, and the
	// lookup must terminate rather than fall through to a nil prototype.
	root := &StoreInstanceInfo{TypeName: "Object/Store", Deleted: map[string]bool{"k": true}}
	if _, ok := tsGet(t, root, "k"); ok {
		t.Error("a tombstone on a root store must still read absent")
	}
	if _, ok := tsGet(t, root, "never"); ok {
		t.Error("an unknown key on a root store must read absent")
	}
}

func TestCowDelLayersAndPropagates(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	base := tsStore(map[string]Value{"k": NewInteger(1)})
	r.Contexts.PushExisting(base)

	CowDel(base, "k", r)
	top := r.Contexts.Top()
	if top == base {
		t.Fatal("CowDel must layer, not edit the store in place")
	}
	if _, ok := tsGet(t, top, "k"); ok {
		t.Error("after CowDel the key must read absent from the new top")
	}
	if _, ok := tsGet(t, base, "k"); !ok {
		t.Error("CowDel must not edit the layer that owns the key")
	}

	// Deleting a key that was never bound is a no-op layer, not an error:
	// del is idempotent and total, like has.
	CowDel(r.Contexts.Top(), "never-there", r)
	if _, ok := tsGet(t, r.Contexts.Top(), "never-there"); ok {
		t.Error("deleting an unbound key must leave it absent")
	}
	// …and it must not resurrect the earlier deletion.
	if _, ok := tsGet(t, r.Contexts.Top(), "k"); ok {
		t.Error("a second CowDel must not undo the first tombstone")
	}
}

func TestCowDelPropagatesUpTheParentChain(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// child is nested under parent at key "kid" — CowDel must rebuild the
	// parent layer too, exactly as CowSet does, or the parent keeps
	// pointing at the pre-delete child.
	parent := tsStore(nil)
	child := tsStore(map[string]Value{"k": NewInteger(1)})
	child.Parent = parent
	child.ParentKey = "kid"
	parent.Data["kid"] = NewStoreValue(nil, child)
	r.Contexts.PushExisting(parent)

	CowDel(child, "k", r)

	newParent := r.Contexts.Top()
	if newParent == parent {
		t.Fatal("the parent layer must be rebuilt so it references the new child")
	}
	kid, ok := tsGet(t, newParent, "kid")
	if !ok {
		t.Fatal("the rebuilt parent must still reference the child")
	}
	nested, serr := AsStore(kid)
	if serr != nil {
		t.Fatalf("AsStore on the rebuilt child: %v", serr)
	}
	if _, ok := tsGet(t, nested, "k"); ok {
		t.Error("the child reached through the rebuilt parent must see the tombstone")
	}
}

func TestCloneCarriesTombstones(t *testing.T) {
	base := tsStore(map[string]Value{"k": NewInteger(1)})
	layer := &StoreInstanceInfo{
		TypeName:  base.TypeName,
		Deleted:   map[string]bool{"k": true},
		Prototype: base,
	}
	cp := CloneValue(NewStoreValue(nil, layer))
	cs, serr := AsStore(cp)
	if serr != nil {
		t.Fatalf("AsStore: %v", serr)
	}
	// Without the Deleted copy the clone's prototype chain resurrects k.
	if _, ok := tsGet(t, cs, "k"); ok {
		t.Error("a clone must keep its tombstones; k came back through the cloned prototype")
	}
	// The clone is detached: mutating it must not disturb the original.
	cs.Deleted["k"] = false
	if _, ok := tsGet(t, layer, "k"); ok {
		t.Error("the original tombstone must be independent of the clone's")
	}
	// A store with no tombstones clones to a nil map, not an empty one —
	// same shape as before, so nothing downstream sees a new allocation.
	plain := CloneValue(NewStoreValue(nil, base))
	ps, serr := AsStore(plain)
	if serr != nil {
		t.Fatalf("AsStore(base clone): %v", serr)
	}
	if ps.Deleted != nil {
		t.Errorf("a tombstone-free store must clone with a nil Deleted map, got %v", ps.Deleted)
	}
}
