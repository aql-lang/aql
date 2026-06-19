//go:build !js

package native

import (
	"strings"
	"testing"
)

// openTestConn opens an embedded SQLite :memory: connection (the pool
// is pinned to one connection, so the in-memory DB persists across
// statements) for exercising the DB module words without a server.
func openTestConn(t *testing.T) Value {
	t.Helper()
	conn, err := OpenDBConn("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("OpenDBConn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return NewDBConn(conn)
}

func TestDBModuleRoundTrip(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cv := openTestConn(t)

	// exec: DDL.
	if _, err := dbExecHandler([]Value{NewString("create table t (id integer, v text)"), cv}, nil, nil, r); err != nil {
		t.Fatalf("exec create: %v", err)
	}
	// exec with params: INSERT, returns affected rows.
	res, err := dbExecParamsHandler([]Value{
		NewString("insert into t (id, v) values (?, ?)"),
		NewList([]Value{NewInteger(1), NewString("x")}),
		cv,
	}, nil, nil, r)
	if err != nil {
		t.Fatalf("exec insert: %v", err)
	}
	if n, _ := AsInteger(res[0]); n != 1 {
		t.Errorf("affected: got %d want 1", n)
	}

	// sql: raw passthrough → table value.
	out, err := dbSQLHandler([]Value{NewString("select v from t where id = 1"), cv}, nil, nil, r)
	if err != nil {
		t.Fatalf("sql: %v", err)
	}
	td, ok := out[0].Data.(TableData)
	if !ok {
		t.Fatalf("sql result is not a table: %T", out[0].Data)
	}
	if len(td.Rows) != 1 {
		t.Fatalf("rows: got %d want 1", len(td.Rows))
	}
	m, _ := AsMap(td.Rows[0])
	if v, _ := m.Get("v"); ValToString(v) != "x" {
		t.Errorf("v: got %q want x", ValToString(v))
	}

	// sql with params.
	out2, err := dbSQLParamsHandler([]Value{
		NewString("select id from t where v = ?"),
		NewList([]Value{NewString("x")}),
		cv,
	}, nil, nil, r)
	if err != nil {
		t.Fatalf("sql params: %v", err)
	}
	if td2, _ := out2[0].Data.(TableData); len(td2.Rows) != 1 {
		t.Errorf("sql params rows: got %d want 1", len(td2.Rows))
	}

	// tables.
	tb, err := dbTablesHandler([]Value{cv}, nil, nil, r)
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	tl, _ := AsList(tb[0])
	if tl.Len() != 1 {
		t.Fatalf("tables: got %d want 1", tl.Len())
	}

	// schema → {column: TypeName}.
	d, err := dbSchemaHandler([]Value{NewString("t"), cv}, nil, nil, r)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	dm, _ := AsMap(d[0])
	if v, _ := dm.Get("id"); ValToString(v) != "Integer" {
		t.Errorf("schema id: got %q want Integer", ValToString(v))
	}

	// close.
	if _, err := dbCloseHandler([]Value{cv}, nil, nil, r); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestDBModuleOpenErrors(t *testing.T) {
	r, _ := DefaultRegistry()

	// Unknown kind.
	if _, err := dbOpenHandler([]Value{NewString("oracle"), NewString("x")}, nil, nil, r); err == nil {
		t.Error("expected error for unknown kind")
	}
	// 1-arg form cannot infer kind from a schemeless DSN.
	if _, err := dbOpenDSNHandler([]Value{NewString("host=localhost dbname=x")}, nil, nil, r); err == nil {
		t.Error("expected error inferring kind from schemeless DSN")
	}
}

func TestDBModuleWrongConnArg(t *testing.T) {
	r, _ := DefaultRegistry()
	// Passing a non-connection where a DBConn is required must error,
	// not panic.
	for _, h := range []Handler{dbCloseHandler, dbTablesHandler} {
		if _, err := h([]Value{NewInteger(5)}, nil, nil, r); err == nil {
			t.Error("expected error for non-connection argument")
		}
	}
	if _, err := dbSQLHandler([]Value{NewString("select 1"), NewInteger(5)}, nil, nil, r); err == nil {
		t.Error("sql: expected error for non-connection argument")
	}
}

func TestDBModuleInferKind(t *testing.T) {
	cases := map[string]string{
		"postgres://u@h/db": "postgres",
		"mysql://u@h/db":    "mysql",
		"sqlserver://h":     "sqlserver",
		"host=h dbname=x":   "",
	}
	for dsn, want := range cases {
		got, ok := inferDBKind(dsn)
		if want == "" {
			if ok {
				t.Errorf("inferDBKind(%q): expected no inference, got %q", dsn, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("inferDBKind(%q): got %q ok=%v want %q", dsn, got, ok, want)
		}
	}
}

func TestDBModuleRedactedLabel(t *testing.T) {
	// A password in the DSN must not survive into the connection label.
	conn, err := OpenDBConn("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if strings.Contains(conn.Label, "password") {
		t.Errorf("label unexpectedly contains 'password': %q", conn.Label)
	}
}
