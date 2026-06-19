package native

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// KeyValBackend is the pluggable storage abstraction behind the
// aql:keyval module. The in-memory store (MemKVStore) is the default;
// external backends (file, Redis, MongoDB, S3) implement the same
// interface in build-tagged files so the language surface is identical
// across stores.
//
// Keys are strings. Values are AQL Values; backends that persist to
// bytes serialize them (see valueToKVBytes / kvBytesToValue), while the
// in-memory store keeps them directly.
type KeyValBackend interface {
	// Get returns the value for key and whether it was present.
	Get(key string) (Value, bool, error)
	// Set stores val under key, overwriting any existing entry.
	Set(key string, val Value) error
	// Delete removes key, reporting whether it existed.
	Delete(key string) (bool, error)
	// Has reports whether key is present.
	Has(key string) (bool, error)
	// Keys returns the stored keys with the given prefix (all keys when
	// prefix is empty), sorted.
	Keys(prefix string) ([]string, error)
	// Kind is the backend identifier ("memory", "file", "redis", …).
	Kind() string
	// Close releases the backend's resources.
	Close() error
}

// MemKVStore is the default in-memory KeyValBackend: a mutex-guarded
// map. Values are held directly (no serialization).
type MemKVStore struct {
	mu   sync.RWMutex
	data map[string]Value
}

var _ KeyValBackend = (*MemKVStore)(nil)

// NewMemKVStore creates an empty in-memory key-value store.
func NewMemKVStore() *MemKVStore {
	return &MemKVStore{data: make(map[string]Value)}
}

func (m *MemKVStore) Get(key string) (Value, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *MemKVStore) Set(key string, val Value) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = val
	return nil
}

func (m *MemKVStore) Delete(key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	delete(m.data, key)
	return ok, nil
}

func (m *MemKVStore) Has(key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[key]
	return ok, nil
}

func (m *MemKVStore) Keys(prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.data))
	for k := range m.data {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m *MemKVStore) Kind() string { return "memory" }

func (m *MemKVStore) Close() error { return nil }

// OpenKVBackend resolves a backend kind to an open KeyValBackend. The
// in-memory store needs no DSN; external kinds are dispatched to
// openExternalKV (build-tagged: real drivers on native, a
// not-supported stub on wasm).
func OpenKVBackend(kind, dsn string) (KeyValBackend, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "memory", "mem", "":
		return NewMemKVStore(), nil
	default:
		return openExternalKV(kind, dsn)
	}
}

// CanonicalKVKind reports the canonical name of a backend kind, or false
// if unknown. Exposed so the module can validate without opening.
func CanonicalKVKind(kind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "memory", "mem", "":
		return "memory", true
	default:
		return canonicalExternalKVKind(kind)
	}
}

// kvUnknownKind is the shared error for an unrecognised backend kind.
func kvUnknownKind(kind string) error {
	return fmt.Errorf("unknown keyval backend %q (want memory, file, redis, mongo, or s3)", kind)
}

// openExternalKV opens a non-memory backend. The external backends
// (file/redis/mongo/s3) land in later phases; until then every external
// kind is unknown.
//
// TODO(keyval): split into build-tagged files (native drivers vs wasm
// stub) when the first external backend is added.
func openExternalKV(kind, _ string) (KeyValBackend, error) {
	return nil, kvUnknownKind(kind)
}

// canonicalExternalKVKind canonicalizes a non-memory backend kind.
func canonicalExternalKVKind(string) (string, bool) {
	return "", false
}
