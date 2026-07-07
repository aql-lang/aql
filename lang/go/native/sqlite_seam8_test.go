//go:build !js

package native

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"testing"
)

// --- a minimal, fault-injecting database/sql driver -----------------------
//
// The SQLite store wraps every driver call in a defensive `if err != nil`.
// Those wrappers never fire for a healthy in-memory database, so the only
// way to exercise them is to point the store at a driver whose operations
// fail on demand. This fake succeeds through table creation and then fails
// at exactly one configured stage.

type w8FakeCfg struct {
	failBegin         bool
	failPrepareInsert bool
	failExecInsert    bool
	failCommit        bool
	failRowsNext      bool
}

type w8FakeConnector struct{ cfg *w8FakeCfg }

func (c w8FakeConnector) Connect(context.Context) (driver.Conn, error) {
	return &w8FakeConn{cfg: c.cfg}, nil
}
func (c w8FakeConnector) Driver() driver.Driver { return w8FakeDriver{} }

type w8FakeDriver struct{}

func (w8FakeDriver) Open(string) (driver.Conn, error) { return &w8FakeConn{cfg: &w8FakeCfg{}}, nil }

type w8FakeConn struct{ cfg *w8FakeCfg }

func (c *w8FakeConn) Prepare(query string) (driver.Stmt, error) {
	if c.cfg.failPrepareInsert && strings.Contains(query, "INSERT") {
		return nil, fmt.Errorf("w8: forced prepare failure")
	}
	return &w8FakeStmt{cfg: c.cfg, query: query}, nil
}
func (c *w8FakeConn) Close() error { return nil }
func (c *w8FakeConn) Begin() (driver.Tx, error) {
	if c.cfg.failBegin {
		return nil, fmt.Errorf("w8: forced begin failure")
	}
	return &w8FakeTx{cfg: c.cfg}, nil
}

type w8FakeStmt struct {
	cfg   *w8FakeCfg
	query string
}

func (s *w8FakeStmt) Close() error  { return nil }
func (s *w8FakeStmt) NumInput() int { return -1 }
func (s *w8FakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	if s.cfg.failExecInsert && strings.Contains(s.query, "INSERT") {
		return nil, fmt.Errorf("w8: forced exec failure")
	}
	return w8FakeResult{}, nil
}
func (s *w8FakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &w8FakeRows{cfg: s.cfg}, nil
}

type w8FakeTx struct{ cfg *w8FakeCfg }

func (t *w8FakeTx) Commit() error {
	if t.cfg.failCommit {
		return fmt.Errorf("w8: forced commit failure")
	}
	return nil
}
func (t *w8FakeTx) Rollback() error { return nil }

type w8FakeResult struct{}

func (w8FakeResult) LastInsertId() (int64, error) { return 0, nil }
func (w8FakeResult) RowsAffected() (int64, error) { return 0, nil }

type w8FakeRows struct{ cfg *w8FakeCfg }

func (r *w8FakeRows) Columns() []string { return []string{"c"} }
func (r *w8FakeRows) Close() error      { return nil }
func (r *w8FakeRows) Next(dest []driver.Value) error {
	if r.cfg.failRowsNext {
		return fmt.Errorf("w8: forced rows iteration failure")
	}
	return io.EOF
}

// w8FakeStore builds a store backed by the fault-injecting driver.
func w8FakeStore(cfg *w8FakeCfg) *SQLiteStore {
	db := sql.OpenDB(w8FakeConnector{cfg: cfg})
	return &SQLiteStore{db: db, tables: make(map[string]RecordTypeInfo)}
}

// w8OneRowTable returns a single-column, single-row TableData so StoreTable
// reaches its INSERT path.
func w8OneRowTable() TableData {
	fields := NewOrderedMap()
	fields.Set("c", NewTypeLiteral(TInteger))
	row := NewOrderedMap()
	row.Set("c", NewInteger(1))
	return TableData{Record: RecordTypeInfo{Fields: fields}, Rows: []Value{NewMap(row)}}
}

func TestW8SqliteStoreTableBeginError(t *testing.T) {
	store := w8FakeStore(&w8FakeCfg{failBegin: true})
	defer store.Close()
	if err := store.StoreTable("t", w8OneRowTable()); err == nil {
		t.Fatal("StoreTable must surface a Begin error")
	}
}

func TestW8SqliteStoreTablePrepareError(t *testing.T) {
	store := w8FakeStore(&w8FakeCfg{failPrepareInsert: true})
	defer store.Close()
	if err := store.StoreTable("t", w8OneRowTable()); err == nil {
		t.Fatal("StoreTable must surface an insert-prepare error")
	}
}

func TestW8SqliteStoreTableExecError(t *testing.T) {
	store := w8FakeStore(&w8FakeCfg{failExecInsert: true})
	defer store.Close()
	if err := store.StoreTable("t", w8OneRowTable()); err == nil {
		t.Fatal("StoreTable must surface an insert-exec error")
	}
}

func TestW8SqliteStoreTableCommitError(t *testing.T) {
	store := w8FakeStore(&w8FakeCfg{failCommit: true})
	defer store.Close()
	if err := store.StoreTable("t", w8OneRowTable()); err == nil {
		t.Fatal("StoreTable must surface a Commit error")
	}
}

func TestW8SqliteQueryRowsError(t *testing.T) {
	store := w8FakeStore(&w8FakeCfg{failRowsNext: true})
	defer store.Close()
	if _, err := store.Query(`SELECT c FROM t`, nil); err == nil {
		t.Fatal("Query must surface a rows-iteration error")
	}
}

// w8SqliteStoreWithTable builds a store with a single 2-column table so
// the regexp() SQL function can be exercised over real rows.
func w8SqliteStoreWithTable(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	fields.Set("name", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("id", NewInteger(1))
	row.Set("name", NewString("alice"))

	if err := store.StoreTable("t", TableData{Record: rec, Rows: []Value{NewMap(row)}}); err != nil {
		t.Fatal(err)
	}
	return store
}

// --- regexp() SQL function branches (init) ---

func TestW8SqliteRegexpNullArgs(t *testing.T) {
	store := w8SqliteStoreWithTable(t)
	defer store.Close()
	// NULL first arg -> the function returns NULL (nil, nil). The query
	// itself succeeds.
	if _, err := store.Query(`SELECT regexp(NULL, name) AS r FROM "t"`, nil); err != nil {
		t.Fatalf("regexp(NULL, ...) query: %v", err)
	}
}

func TestW8SqliteRegexpPatternNotString(t *testing.T) {
	store := w8SqliteStoreWithTable(t)
	defer store.Close()
	// First arg is an integer, not a string.
	if _, err := store.Query(`SELECT regexp(1, name) AS r FROM "t"`, nil); err == nil {
		t.Fatal("regexp with non-string pattern must error")
	}
}

func TestW8SqliteRegexpValueNotString(t *testing.T) {
	store := w8SqliteStoreWithTable(t)
	defer store.Close()
	// Second arg is an integer, not a string.
	if _, err := store.Query(`SELECT regexp('a', 1) AS r FROM "t"`, nil); err == nil {
		t.Fatal("regexp with non-string value must error")
	}
}

func TestW8SqliteRegexpBadPattern(t *testing.T) {
	store := w8SqliteStoreWithTable(t)
	defer store.Close()
	// '[' is an invalid regular expression.
	if _, err := store.Query(`SELECT regexp('[', name) AS r FROM "t"`, nil); err == nil {
		t.Fatal("regexp with invalid pattern must error")
	}
}

// --- NewSQLiteStore open failure ---

func TestW8NewSQLiteStoreOpenError(t *testing.T) {
	orig := w8SQLOpen
	w8SQLOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
		return nil, fmt.Errorf("w8: forced open failure")
	}
	t.Cleanup(func() { w8SQLOpen = orig })
	if _, err := NewSQLiteStore(); err == nil {
		t.Fatal("NewSQLiteStore must surface the sql.Open error")
	}
}

// --- StoreTable row-handling branches ---

func TestW8SqliteStoreTableRowNotMap(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	// A non-map row is skipped (AsMutableMap errors -> continue).
	td := TableData{Record: rec, Rows: []Value{NewInteger(99)}}
	if err := store.StoreTable("t", td); err != nil {
		t.Fatalf("StoreTable with a non-map row should skip it, got %v", err)
	}
}

func TestW8SqliteStoreTableMissingColumn(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	fields.Set("name", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	// Row map is missing the "name" column -> that cell binds to nil.
	row := NewOrderedMap()
	row.Set("id", NewInteger(1))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	if err := store.StoreTable("t", td); err != nil {
		t.Fatalf("StoreTable with a row missing a column: %v", err)
	}
}
