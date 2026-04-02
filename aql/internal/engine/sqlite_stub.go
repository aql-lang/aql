//go:build js

package engine

import "fmt"

// SQLiteStore is a no-op stub for WASM builds where SQLite is unavailable.
type SQLiteStore struct{}

// NewSQLiteStore returns nil in WASM builds (SQLite is not available).
func NewSQLiteStore() (*SQLiteStore, error) {
	return nil, nil
}

func (s *SQLiteStore) Close() error                                         { return nil }
func (s *SQLiteStore) StoreTable(name string, td TableData) error           { return fmt.Errorf("sqlite not available in wasm") }
func (s *SQLiteStore) StoreTempTable(td TableData) (string, error)          { return "", fmt.Errorf("sqlite not available in wasm") }
func (s *SQLiteStore) Query(q string, schema *RecordTypeInfo) (TableData, error) { return TableData{}, fmt.Errorf("sqlite not available in wasm") }
func (s *SQLiteStore) HasTable(name string) bool                            { return false }
func (s *SQLiteStore) DropTable(name string)                                {}
