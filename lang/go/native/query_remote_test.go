//go:build !js

package native

import "testing"

// remoteConn opens an embedded SQLite connection seeded with a users
// table, used as a stand-in "remote" backend for pushdown tests.
func remoteConn(t *testing.T) *DBConn {
	t.Helper()
	conn, err := OpenDBConn("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("OpenDBConn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.Backend.Exec("create table users (id integer, name text)", nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := conn.Backend.Exec("insert into users (id, name) values (1,'ann'),(2,'bob'),(3,'cal')", nil); err != nil {
		t.Fatalf("insert: %v", err)
	}
	return conn
}

// remoteQB builds a builder bound to a remote source, mirroring what the
// select/from/where/order handlers assemble.
func remoteQB(r *Registry, conn *DBConn) QueryBuilder {
	qb := QueryBuilder{
		Registry:    r,
		Limit:       -1,
		Offset:      -1,
		HasSource:   true,
		RemoteConn:  conn,
		RemoteTable: "users",
		Dialect:     conn.Backend.Dialect(),
	}
	return qb
}

func TestRemotePushdownProjectionFilterOrder(t *testing.T) {
	r, _ := DefaultRegistry()
	conn := remoteConn(t)

	qb := remoteQB(r, conn)
	qb.RawCols = NewList([]Value{NewWord("name")})
	var err error
	qb.Where, err = buildWhereClause(qb.Dialect, NewList([]Value{NewWord("id"), NewWord("gt"), NewInteger(1)}))
	if err != nil {
		t.Fatal(err)
	}
	qb.OrderBy, err = buildOrderClause(qb.Dialect, NewList([]Value{NewWord("id"), NewWord("desc")}))
	if err != nil {
		t.Fatal(err)
	}

	td, err := qb.Materialize()
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if len(td.Rows) != 2 {
		t.Fatalf("rows: got %d want 2", len(td.Rows))
	}
	// id>1 ordered desc → cal (3), bob (2).
	first, _ := AsMap(td.Rows[0])
	if v, _ := first.Get("name"); ValToString(v) != "cal" {
		t.Errorf("first name: got %q want cal", ValToString(v))
	}
	// projection kept only `name`.
	if _, ok := first.Get("id"); ok {
		t.Error("projection should have dropped the id column")
	}
}

func TestRemotePushdownSelectStar(t *testing.T) {
	r, _ := DefaultRegistry()
	conn := remoteConn(t)
	qb := remoteQB(r, conn)
	qb.RawCols = NewList([]Value{}) // SELECT *
	qb.Limit = 1

	td, err := qb.Materialize()
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if len(td.Rows) != 1 {
		t.Fatalf("rows: got %d want 1 (limit)", len(td.Rows))
	}
	// SELECT * keeps both columns, types inferred from the driver.
	m, _ := AsMap(td.Rows[0])
	if _, ok := m.Get("id"); !ok {
		t.Error("select * should include id")
	}
	if _, ok := m.Get("name"); !ok {
		t.Error("select * should include name")
	}
}

func TestRemotePushdownAggregate(t *testing.T) {
	r, _ := DefaultRegistry()
	conn := remoteConn(t)
	qb := remoteQB(r, conn)
	// select [[count id n]] — aggregate Raw must be rebuilt in the
	// remote dialect (here SQLite).
	qb.RawCols = NewList([]Value{NewList([]Value{NewWord("count"), NewWord("id"), NewWord("n")})})

	td, err := qb.Materialize()
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if len(td.Rows) != 1 {
		t.Fatalf("rows: got %d want 1", len(td.Rows))
	}
	m, _ := AsMap(td.Rows[0])
	// count of 3 rows (rendered numeric).
	if v, _ := m.Get("n"); ValToString(v) != "3" && ValToString(v) != "3.0" {
		t.Errorf("count: got %q want 3", ValToString(v))
	}
}

func TestRemotePushdownRejectsJoin(t *testing.T) {
	r, _ := DefaultRegistry()
	conn := remoteConn(t)
	qb := remoteQB(r, conn)
	qb.RawCols = NewList([]Value{})
	qb.Joins = []JoinClause{{Type: "JOIN", Table: "orders"}}

	if _, err := qb.Materialize(); err == nil {
		t.Error("expected error: joins across a remote connection unsupported")
	}
}

func TestRemotePushdownRejectsSetOp(t *testing.T) {
	r, _ := DefaultRegistry()
	conn := remoteConn(t)
	qb := remoteQB(r, conn)
	qb.RawCols = NewList([]Value{})
	qb.SetOps = []SetOp{{Op: "UNION", Right: QueryBuilder{}}}

	if _, err := qb.Materialize(); err == nil {
		t.Error("expected error: set operations across a remote connection unsupported")
	}
}

func TestRemoteSourceCarrier(t *testing.T) {
	r, _ := DefaultRegistry()
	conn := remoteConn(t)
	out, err := dbTableHandler([]Value{NewString("users"), NewDBConn(conn)}, nil, nil, r)
	if err != nil {
		t.Fatalf("dbTableHandler: %v", err)
	}
	rs, ok := AsRemoteSource(out[0])
	if !ok {
		t.Fatal("expected a RemoteSource value")
	}
	if rs.Table != "users" || rs.Conn != conn {
		t.Errorf("RemoteSource: got table=%q conn=%v", rs.Table, rs.Conn)
	}
	// Wrong-arg negative: a non-connection receiver errors.
	if _, err := dbTableHandler([]Value{NewString("users"), NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("expected error for non-connection argument")
	}
}

func TestRemoteSourceRecord(t *testing.T) {
	r, _ := DefaultRegistry()
	conn := remoteConn(t)
	qb := remoteQB(r, conn)
	qb.RawCols = NewList([]Value{NewWord("name")})
	// SourceRecord must not run the query, only describe its shape.
	rec := qb.SourceRecord()
	keys := rec.Fields.Keys()
	if len(keys) != 1 || keys[0] != "name" {
		t.Errorf("SourceRecord: got %v want [name]", keys)
	}
}
