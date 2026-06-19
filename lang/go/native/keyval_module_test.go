package native

import "testing"

func kvOpenMem(t *testing.T, r *Registry) Value {
	t.Helper()
	out, err := kvOpenKindHandler([]Value{NewString("memory")}, nil, nil, r)
	if err != nil {
		t.Fatalf("open memory: %v", err)
	}
	return out[0]
}

func TestKVModuleRoundTrip(t *testing.T) {
	r, _ := DefaultRegistry()
	kv := kvOpenMem(t, r)

	// set returns the store (for chaining).
	out, err := kvSetHandler([]Value{NewString("user:1"), NewInteger(42), kv}, nil, nil, r)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, ok := AsKVStore(out[0]); !ok {
		t.Error("set should return the store")
	}

	// get returns the stored value.
	got, err := kvGetHandler([]Value{NewString("user:1"), kv}, nil, nil, r)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if n, _ := AsInteger(got[0]); n != 42 {
		t.Errorf("get: got %v want 42", ValToString(got[0]))
	}

	// get miss → None.
	miss, err := kvGetHandler([]Value{NewString("nope"), kv}, nil, nil, r)
	if err != nil {
		t.Fatalf("get miss: %v", err)
	}
	if !IsNone(miss[0]) {
		t.Errorf("get miss: got %v want None", miss[0].Parent)
	}

	// has.
	h, _ := kvHasHandler([]Value{NewString("user:1"), kv}, nil, nil, r)
	if b, _ := AsBoolean(h[0]); !b {
		t.Error("has user:1 should be true")
	}
	h2, _ := kvHasHandler([]Value{NewString("nope"), kv}, nil, nil, r)
	if b, _ := AsBoolean(h2[0]); b {
		t.Error("has nope should be false")
	}

	// keys with prefix.
	if _, err := kvSetHandler([]Value{NewString("user:2"), NewInteger(7), kv}, nil, nil, r); err != nil {
		t.Fatal(err)
	}
	if _, err := kvSetHandler([]Value{NewString("other"), NewInteger(0), kv}, nil, nil, r); err != nil {
		t.Fatal(err)
	}
	ks, _ := kvKeysHandler([]Value{NewString("user:"), kv}, nil, nil, r)
	kl, _ := AsList(ks[0])
	if kl.Len() != 2 {
		t.Errorf("keys user: got %d want 2", kl.Len())
	}
	all, _ := kvKeysAllHandler([]Value{kv}, nil, nil, r)
	al, _ := AsList(all[0])
	if al.Len() != 3 {
		t.Errorf("keys all: got %d want 3", al.Len())
	}

	// del returns whether the key existed.
	d, _ := kvDelHandler([]Value{NewString("user:1"), kv}, nil, nil, r)
	if b, _ := AsBoolean(d[0]); !b {
		t.Error("del existing should be true")
	}
	d2, _ := kvDelHandler([]Value{NewString("user:1"), kv}, nil, nil, r)
	if b, _ := AsBoolean(d2[0]); b {
		t.Error("del missing should be false")
	}

	// close.
	if _, err := kvCloseHandler([]Value{kv}, nil, nil, r); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestKVModuleStoresStructuredValues(t *testing.T) {
	r, _ := DefaultRegistry()
	kv := kvOpenMem(t, r)
	m := NewOrderedMap()
	m.Set("name", NewString("ann"))
	if _, err := kvSetHandler([]Value{NewString("u"), NewMap(m), kv}, nil, nil, r); err != nil {
		t.Fatal(err)
	}
	got, _ := kvGetHandler([]Value{NewString("u"), kv}, nil, nil, r)
	gm, merr := AsMap(got[0])
	if merr != nil {
		t.Fatalf("expected a map back, got %v", got[0].Parent)
	}
	if v, _ := gm.Get("name"); ValToString(v) != "ann" {
		t.Errorf("name: got %q want ann", ValToString(v))
	}
}

func TestKVModuleOpenUnknownKind(t *testing.T) {
	r, _ := DefaultRegistry()
	if _, err := kvOpenKindHandler([]Value{NewString("cassandra")}, nil, nil, r); err == nil {
		t.Error("expected error for unknown backend kind")
	}
}

func TestKVModuleWrongStoreArg(t *testing.T) {
	r, _ := DefaultRegistry()
	for _, h := range []Handler{kvCloseHandler, kvKeysAllHandler} {
		if _, err := h([]Value{NewInteger(1)}, nil, nil, r); err == nil {
			t.Error("expected error for non-store argument")
		}
	}
	if _, err := kvGetHandler([]Value{NewString("k"), NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("get: expected error for non-store argument")
	}
}

func TestKVStoreSatisfiesBackend(t *testing.T) {
	var _ KeyValBackend = (*MemKVStore)(nil)
	m := NewMemKVStore()
	if m.Kind() != "memory" {
		t.Errorf("Kind: got %q want memory", m.Kind())
	}
	if err := m.Set("a", NewInteger(1)); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := m.Get("a"); !ok || func() int64 { n, _ := AsInteger(v); return n }() != 1 {
		t.Error("mem get/set round trip failed")
	}
}
