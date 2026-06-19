package native

import "testing"

// peopleTable builds a small typed table for backend tests.
func peopleTable() (RecordTypeInfo, TableData) {
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	mk := func(name string, age int64) Value {
		m := NewOrderedMap()
		m.Set("name", NewString(name))
		m.Set("age", NewInteger(age))
		return NewMap(m)
	}
	td := TableData{Record: rec, Rows: []Value{mk("alice", 30), mk("bob", 20), mk("carol", 40)}}
	return rec, td
}

func newPeopleStore(t *testing.T) (*SQLiteStore, RecordTypeInfo) {
	t.Helper()
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	rec, td := peopleTable()
	if err := store.StoreTable("people", td); err != nil {
		t.Fatal(err)
	}
	return store, rec
}

func TestSQLiteStoreSatisfiesBackend(t *testing.T) {
	store, _ := newPeopleStore(t)
	defer store.Close()
	var b SQLBackend = store // must satisfy the interface at runtime too
	if b.Dialect().Name() != "sqlite" {
		t.Errorf("Dialect: got %q want sqlite", b.Dialect().Name())
	}
}

func TestSQLiteStoreQueryParams(t *testing.T) {
	store, rec := newPeopleStore(t)
	defer store.Close()

	got, err := store.QueryParams("SELECT * FROM `people` WHERE age > ? ORDER BY age", []any{int64(25)}, &rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("expected 2 rows (alice, carol), got %d", len(got.Rows))
	}
	m, _ := AsMap(got.Rows[0])
	if v, _ := m.Get("name"); ValToString(v) != "alice" {
		t.Errorf("first row name: got %q want alice", ValToString(v))
	}
}

func TestSQLiteStoreExec(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.Exec("CREATE TABLE kv (id INTEGER, v TEXT)", nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	n, err := store.Exec("INSERT INTO kv (id, v) VALUES (?, ?)", []any{int64(1), "x"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if n != 1 {
		t.Errorf("affected rows: got %d want 1", n)
	}

	got, err := store.QueryParams("SELECT v FROM kv WHERE id = ?", []any{int64(1)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got.Rows))
	}
	m, _ := AsMap(got.Rows[0])
	if v, _ := m.Get("v"); ValToString(v) != "x" {
		t.Errorf("v: got %q want x", ValToString(v))
	}
}

func TestSQLiteStoreExecError(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Syntactically invalid SQL must surface an error, not panic.
	if _, err := store.Exec("NOT VALID SQL", nil); err == nil {
		t.Error("expected error for invalid SQL")
	}
}

func TestSQLiteStoreListTables(t *testing.T) {
	store, _ := newPeopleStore(t)
	defer store.Close()
	_, td := peopleTable()
	if err := store.StoreTable("places", td); err != nil {
		t.Fatal(err)
	}
	tables, err := store.ListTables()
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by name: people, places.
	if len(tables) != 2 || tables[0] != "people" || tables[1] != "places" {
		t.Errorf("ListTables: got %v want [people places]", tables)
	}
}

func TestSQLiteStoreDescribeTable(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create with explicit column types so DescribeTable is checked
	// against known declared types (independent of how StoreTable maps
	// AQL type literals).
	if _, err := store.Exec("CREATE TABLE typed (id INTEGER, score REAL, label TEXT)", nil); err != nil {
		t.Fatal(err)
	}

	schema, err := store.DescribeTable("typed")
	if err != nil {
		t.Fatal(err)
	}
	keys := schema.Fields.Keys()
	if len(keys) != 3 || keys[0] != "id" || keys[1] != "score" || keys[2] != "label" {
		t.Fatalf("DescribeTable columns: got %v want [id score label]", keys)
	}
	// Field values are type literals; the denoted type is recovered via
	// TypeNameOf (what the renderers use), not via .Parent (supertype).
	want := map[string]string{"id": "Integer", "score": "Float", "label": "String"}
	for col, wt := range want {
		fv, _ := schema.Fields.Get(col)
		if got := TypeNameOf(fv); got != wt {
			t.Errorf("%s type: got %s want %s", col, got, wt)
		}
	}

	// Negative: unknown table errors rather than returning empty.
	if _, err := store.DescribeTable("nope"); err == nil {
		t.Error("expected error describing unknown table")
	}
}
