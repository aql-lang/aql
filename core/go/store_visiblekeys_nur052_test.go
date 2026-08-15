package core

import (
	"reflect"
	"testing"
)

// NUR052: a Store's enumeration walks the prototype chain with the SAME
// precedence Get applies, so `size` / `convert Map` describe the keyset
// `get` / `has` answer for. Enumeration used to read only the newest
// copy-on-write layer, so two sets left two keys reachable by lookup and
// one visible to enumeration.
//
// The end-to-end spelling is pinned in lang/spec/storage.tsv; these cover
// the walk's arms from core's own suite, including the two a boru program
// cannot reach directly.

func nur052Store(data map[string]Value, deleted map[string]bool, proto *StoreInstanceInfo) *StoreInstanceInfo {
	return &StoreInstanceInfo{Data: data, Deleted: deleted, Prototype: proto}
}

func TestNUR052VisibleKeysWalksTheChain(t *testing.T) {
	parent := nur052Store(map[string]Value{"a": NewInteger(1)}, nil, nil)
	child := nur052Store(map[string]Value{"b": NewInteger(2)}, nil, parent)
	if got := child.VisibleKeys(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("inherited keys must enumerate, got %v", got)
	}
}

// A child's key SHADOWS the parent's: it appears once, and Get answers
// with the child's value. Enumeration and lookup must agree on which.
func TestNUR052VisibleKeysMasks(t *testing.T) {
	parent := nur052Store(map[string]Value{"k": NewInteger(1)}, nil, nil)
	child := nur052Store(map[string]Value{"k": NewInteger(2)}, nil, parent)
	if got := child.VisibleKeys(); !reflect.DeepEqual(got, []string{"k"}) {
		t.Fatalf("a shadowed key must appear ONCE, got %v", got)
	}
	v, ok := child.Get("k")
	if n, _ := AsInteger(v); !ok || n != 2 {
		t.Fatalf("lookup must answer with the child's value, got %v ok=%v", v, ok)
	}
}

// A tombstone hides the key from every DEEPER layer — the constraint
// NUR022's del had to satisfy, now enforced on the enumeration side too.
func TestNUR052VisibleKeysHonoursTombstones(t *testing.T) {
	parent := nur052Store(map[string]Value{"a": NewInteger(1), "b": NewInteger(2)}, nil, nil)
	child := nur052Store(nil, map[string]bool{"b": true}, parent)
	if got := child.VisibleKeys(); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("a tombstoned key must not enumerate, got %v", got)
	}
	if _, ok := child.Get("b"); ok {
		t.Fatal("…and must not be reachable by lookup either")
	}
}

// The ORDER of the two loops is load-bearing: this layer's own Data is
// consulted BEFORE its tombstones mask deeper layers, so a set-after-del
// revives the key. Reversing them would make the revival invisible while
// Get still answered — reintroducing the split from the other side.
func TestNUR052SetAfterDeleteRevives(t *testing.T) {
	parent := nur052Store(map[string]Value{"a": NewInteger(1)}, nil, nil)
	revived := nur052Store(map[string]Value{"a": NewInteger(3)}, map[string]bool{"a": true}, parent)
	if got := revived.VisibleKeys(); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("a revived key must enumerate, got %v", got)
	}
	v, ok := revived.Get("a")
	if n, _ := AsInteger(v); !ok || n != 3 {
		t.Fatalf("…at the revived value, got %v ok=%v", v, ok)
	}
}

func TestNUR052VisibleKeysEmpty(t *testing.T) {
	if got := nur052Store(nil, nil, nil).VisibleKeys(); len(got) != 0 {
		t.Fatalf("an empty store enumerates nothing, got %v", got)
	}
}
