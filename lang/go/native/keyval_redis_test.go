//go:build !js

package native

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
)

// startMiniredis runs an in-process Redis fake and returns its redis://
// URL. Fully hermetic — no external server.
func startMiniredis(t *testing.T) string {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return "redis://" + mr.Addr()
}

func TestRedisKVStoreRoundTrip(t *testing.T) {
	s, err := NewRedisKVStore(startMiniredis(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if s.Kind() != "redis" {
		t.Errorf("Kind: got %q want redis", s.Kind())
	}

	// structured value round-trips through the JSON codec.
	m := NewOrderedMap()
	m.Set("name", NewString("ann"))
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
	gm, _ := AsMap(got)
	if v, _ := gm.Get("name"); ValToString(v) != "ann" {
		t.Errorf("name: got %q want ann", ValToString(v))
	}

	// miss → not present.
	if _, ok, _ := s.Get("nope"); ok {
		t.Error("missing key should not be present")
	}

	// keys with prefix, sorted.
	if err := s.Set("other", NewInteger(0)); err != nil {
		t.Fatal(err)
	}
	keys, err := s.Keys("user:")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "user:1" || keys[1] != "user:2" {
		t.Errorf("keys user:: got %v want [user:1 user:2]", keys)
	}

	// has / delete.
	if h, _ := s.Has("user:1"); !h {
		t.Error("has user:1 should be true")
	}
	if existed, _ := s.Delete("user:1"); !existed {
		t.Error("delete should report user:1 existed")
	}
	if h, _ := s.Has("user:1"); h {
		t.Error("user:1 should be gone after delete")
	}
}

func TestRedisKVStoreBadURL(t *testing.T) {
	if _, err := NewRedisKVStore("not-a-redis-url"); err == nil {
		t.Error("expected error for malformed redis URL")
	}
}

func TestRedisKVViaModule(t *testing.T) {
	r, _ := DefaultRegistry()
	out, err := kvOpenHandler([]Value{NewString("redis"), NewString(startMiniredis(t))}, nil, nil, r)
	if err != nil {
		t.Fatalf("open redis via module: %v", err)
	}
	kv := out[0]
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
