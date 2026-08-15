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

// A tombstone masks only when its value is TRUE. `Deleted` is an exported
// map a host can write, and Get tests the boolean rather than the key's
// presence — so `Deleted[k] = false` leaves the inherited key live. The
// walk must test it the same way, or enumeration would hide a key that
// lookup still answers: this record's divergence in miniature, and the
// state is already exercised by the store-tombstone tests.
func TestNUR052FalseTombstoneDoesNotMask(t *testing.T) {
	parent := nur052Store(map[string]Value{"a": NewInteger(1)}, nil, nil)
	child := nur052Store(nil, map[string]bool{"a": false}, parent)
	if _, ok := child.Get("a"); !ok {
		t.Fatal("a false tombstone must leave the key reachable by lookup")
	}
	if got := child.VisibleKeys(); len(got) != 1 || got[0] != "a" {
		t.Fatalf("…and enumeration must agree, got %v", got)
	}
}

// VisibleEntries carries the WINNING value with each key, captured as the
// walk first sees it. Resolving each key with a separate Get from the head
// would re-walk the chain per key — quadratic in the layer count.
func TestNUR052VisibleEntriesCarryTheWinningValue(t *testing.T) {
	parent := nur052Store(map[string]Value{"k": NewInteger(1), "p": NewInteger(9)}, nil, nil)
	child := nur052Store(map[string]Value{"k": NewInteger(2)}, nil, parent)
	got := child.VisibleEntries()
	if len(got) != 2 || got[0].Key != "k" || got[1].Key != "p" {
		t.Fatalf("entries must be sorted by key, got %v", got)
	}
	if n, _ := AsInteger(got[0].Value); n != 2 {
		t.Fatalf("the shadowing child's value must win, got %v", got[0].Value)
	}
	if n, _ := AsInteger(got[1].Value); n != 9 {
		t.Fatalf("the inherited value must ride along, got %v", got[1].Value)
	}
}

// storeDeepEqual compares the VISIBLE entries, so two stores that convert
// to the same map — and answer identically to every lookup — are deq even
// when their newest layers hold different keys. Which layer a key was
// written on is not part of a store's value (NUR031 defined a Store's deq
// as the same projection as `convert Map`, and that projection now walks).
func TestNUR052DeqUsesTheVisibleProjection(t *testing.T) {
	base := nur052Store(map[string]Value{"a": NewInteger(1), "b": NewInteger(2)}, nil, nil)
	// s1: everything on one layer. s2: `a` rewritten on a child layer.
	s1 := base
	s2 := nur052Store(map[string]Value{"a": NewInteger(1)}, nil,
		nur052Store(map[string]Value{"a": NewInteger(7), "b": NewInteger(2)}, nil, nil))
	if !storeDeepEqual(s1, s2) {
		t.Fatal("same visible entries must be deq, whatever the layer shape")
	}
	// A genuine value difference still separates them.
	s3 := nur052Store(map[string]Value{"a": NewInteger(1), "b": NewInteger(99)}, nil, nil)
	if storeDeepEqual(s1, s3) {
		t.Fatal("a differing visible value must NOT be deq")
	}
}
