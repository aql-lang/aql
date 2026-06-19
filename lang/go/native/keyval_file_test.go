//go:build !js

package native

import (
	"path/filepath"
	"testing"
)

func TestFileKVStoreRoundTripAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")

	s, err := NewFileKVStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// structured value round-trips through the JSON codec.
	m := NewOrderedMap()
	m.Set("name", NewString("ann"))
	m.Set("age", NewInteger(30))
	if err := s.Set("user:1", NewMap(m)); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := s.Set("user:2", NewInteger(7)); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, ok, err := s.Get("user:1")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	gm, merr := AsMap(got)
	if merr != nil {
		t.Fatalf("expected a map, got %v", got.Parent)
	}
	if v, _ := gm.Get("name"); ValToString(v) != "ann" {
		t.Errorf("name: got %q want ann", ValToString(v))
	}

	// keys prefix + sorted.
	keys, err := s.Keys("user:")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "user:1" || keys[1] != "user:2" {
		t.Errorf("keys: got %v want [user:1 user:2]", keys)
	}

	// has / delete.
	if h, _ := s.Has("user:2"); !h {
		t.Error("has user:2 should be true")
	}
	if existed, _ := s.Delete("user:2"); !existed {
		t.Error("delete user:2 should report it existed")
	}
	if h, _ := s.Has("user:2"); h {
		t.Error("user:2 should be gone after delete")
	}

	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Persistence: reopen the same file and the data is still there.
	s2, err := NewFileKVStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got2, ok, err := s2.Get("user:1")
	if err != nil || !ok {
		t.Fatalf("reopened get: ok=%v err=%v", ok, err)
	}
	gm2, _ := AsMap(got2)
	if v, _ := gm2.Get("age"); ValToString(v) != "30" {
		t.Errorf("persisted age: got %q want 30", ValToString(v))
	}
}

func TestFileKVStoreEmptyPathErrors(t *testing.T) {
	if _, err := NewFileKVStore("  "); err == nil {
		t.Error("expected error opening file backend with empty path")
	}
}

func TestKVCodecRoundTrip(t *testing.T) {
	cases := []Value{
		NewInteger(42),
		NewString("hi"),
		NewBoolean(true),
		NewFloat(3.5),
	}
	for _, in := range cases {
		b, err := valueToKVBytes(in)
		if err != nil {
			t.Fatalf("encode %v: %v", in, err)
		}
		out, err := kvBytesToValue(b)
		if err != nil {
			t.Fatalf("decode %v: %v", in, err)
		}
		if ValToString(out) != ValToString(in) {
			t.Errorf("round trip: got %q want %q", ValToString(out), ValToString(in))
		}
	}
	// empty bytes → none.
	if v, _ := kvBytesToValue(nil); !IsNone(v) {
		t.Error("empty bytes should decode to none")
	}
}

func TestFileKVViaModule(t *testing.T) {
	r, _ := DefaultRegistry()
	path := filepath.Join(t.TempDir(), "m.db")
	out, err := kvOpenHandler([]Value{NewString("file"), NewString(path)}, nil, nil, r)
	if err != nil {
		t.Fatalf("open file via module: %v", err)
	}
	kv := out[0]
	s, ok := AsKVStore(kv)
	if !ok || s.Kind != "file" {
		t.Fatalf("expected a file KVStore, got kind=%q ok=%v", s.Kind, ok)
	}
	if _, err := kvSetHandler([]Value{NewString("k"), NewString("v"), kv}, nil, nil, r); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, _ := kvGetHandler([]Value{NewString("k"), kv}, nil, nil, r)
	if ValToString(got[0]) != "v" {
		t.Errorf("get: got %q want v", ValToString(got[0]))
	}
	if _, err := kvCloseHandler([]Value{kv}, nil, nil, r); err != nil {
		t.Fatalf("close: %v", err)
	}
}
