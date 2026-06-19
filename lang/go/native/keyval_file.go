//go:build !js

package native

import (
	"fmt"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// FileKVStore is a persistent, embedded KeyValBackend backed by a single
// bbolt database file and one bucket. Values are stored as JSON bytes
// (valueToKVBytes / kvBytesToValue), so the file round-trips AQL values.
type FileKVStore struct {
	db   *bolt.DB
	path string
}

var _ KeyValBackend = (*FileKVStore)(nil)

// kvBucket is the single bucket every key lives in.
var kvBucket = []byte("aql_kv")

// NewFileKVStore opens (creating if needed) a bbolt database at path.
func NewFileKVStore(path string) (*FileKVStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("file backend requires a path: KV.open \"file\" \"/path/to/store.db\"")
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("file open %q: %w", path, err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, e := tx.CreateBucketIfNotExists(kvBucket)
		return e
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("file init %q: %w", path, err)
	}
	return &FileKVStore{db: db, path: path}, nil
}

func (s *FileKVStore) Get(key string) (Value, bool, error) {
	var raw []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(kvBucket).Get([]byte(key))
		if v != nil {
			raw = append([]byte(nil), v...) // copy: v is only valid in-tx
		}
		return nil
	}); err != nil {
		return Value{}, false, err
	}
	if raw == nil {
		return Value{}, false, nil
	}
	val, err := kvBytesToValue(raw)
	if err != nil {
		return Value{}, false, err
	}
	return val, true, nil
}

func (s *FileKVStore) Set(key string, val Value) error {
	b, err := valueToKVBytes(val)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(kvBucket).Put([]byte(key), b)
	})
}

func (s *FileKVStore) Delete(key string) (bool, error) {
	existed := false
	err := s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(kvBucket)
		if bkt.Get([]byte(key)) != nil {
			existed = true
		}
		return bkt.Delete([]byte(key))
	})
	return existed, err
}

func (s *FileKVStore) Has(key string) (bool, error) {
	found := false
	err := s.db.View(func(tx *bolt.Tx) error {
		found = tx.Bucket(kvBucket).Get([]byte(key)) != nil
		return nil
	})
	return found, err
}

func (s *FileKVStore) Keys(prefix string) ([]string, error) {
	var out []string
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(kvBucket).Cursor()
		if prefix == "" {
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				out = append(out, string(k))
			}
			return nil
		}
		p := []byte(prefix)
		for k, _ := c.Seek(p); k != nil && strings.HasPrefix(string(k), prefix); k, _ = c.Next() {
			out = append(out, string(k))
		}
		return nil
	})
	return out, err // bbolt iterates byte-sorted, so out is already sorted
}

func (s *FileKVStore) Kind() string { return "file" }

func (s *FileKVStore) Close() error { return s.db.Close() }
