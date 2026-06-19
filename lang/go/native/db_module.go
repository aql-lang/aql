package native

import (
	"fmt"
	"strings"

	"github.com/aql-lang/aql/lang/go/policy"
)

// DBModuleNatives holds the external-database words surfaced through the
// loadable `aql:db` module (namespace `DB`). They are registered ONLY
// into that module's sub-registry by modules.BuildDBModule.
//
//	open      open a connection         DB.open "postgres" "postgres://…"
//	                                     DB.open "postgres://user@host/db"
//	close     close a connection        pg DB.close
//	sql       raw SQL passthrough        pg DB.sql "select * from users"
//	                                     pg DB.sql "… where id = ?" [42]
//	exec      raw DML/DDL (rows changed) pg DB.exec "delete from t where x<0"
//	tables    list table names          pg DB.tables
//	schema    column schema of a table   pg DB.schema "users"
//
// (The introspection word is `schema`, not `describe`, because
// `describe` is a core word and would shadow the dotted access.)
//
// All inner sigs use BarrierPos -1 (required for module-wrapper
// swap-form dispatch). Each op is gated through the `db` policy scope
// (which binds the network global); credentials never appear in values
// or errors (DBConn stores only a redacted label).
var DBModuleNatives = []NativeFunc{
	{
		Name: "open",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TString}, Handler: dbOpenHandler, Returns: []*Type{TDBConn}, BarrierPos: -1},
			{Args: []*Type{TString}, Handler: dbOpenDSNHandler, Returns: []*Type{TDBConn}, BarrierPos: -1},
		},
	},
	{
		Name: "close",
		Signatures: []NativeSig{
			{Args: []*Type{TDBConn}, Handler: dbCloseHandler, Returns: []*Type{}, BarrierPos: -1},
		},
	},
	{
		Name: "sql",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TList, TDBConn}, Handler: dbSQLParamsHandler, Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TString, TDBConn}, Handler: dbSQLHandler, Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "exec",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TList, TDBConn}, Handler: dbExecParamsHandler, Returns: []*Type{TInteger}, BarrierPos: -1},
			{Args: []*Type{TString, TDBConn}, Handler: dbExecHandler, Returns: []*Type{TInteger}, BarrierPos: -1},
		},
	},
	{
		Name: "tables",
		Signatures: []NativeSig{
			{Args: []*Type{TDBConn}, Handler: dbTablesHandler, Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "schema",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TDBConn}, Handler: dbSchemaHandler, Returns: []*Type{TMap}, BarrierPos: -1},
		},
	},
}

// checkDBPolicy gates a db-scope operation. A nil registry or absent
// policy is the historical allow-everything default; db.install=false
// produces capability_not_installed.
func checkDBPolicy(r *Registry, op string, args policy.Args) error {
	if r == nil {
		return nil
	}
	pol := HostPolicy(r)
	if pol == nil {
		return nil
	}
	if !pol.Installed("db") {
		return &policy.Denied{
			Code:    policy.CodeCapabilityNotInstalled,
			Scope:   "db",
			Op:      op,
			Profile: pol.Name(),
			Blame:   "db.install=false",
			Args:    args,
		}
	}
	return pol.Check("db", op, args)
}

// inferDBKind extracts the backend kind from a URL-form DSN scheme
// ("postgres://…" → "postgres"). Key-value DSNs have no scheme.
func inferDBKind(dsn string) (string, bool) {
	if i := strings.Index(dsn, "://"); i > 0 {
		return dsn[:i], true
	}
	return "", false
}

// dbOpen performs the policy check and connection open shared by both
// open forms.
func dbOpen(r *Registry, kind, dsn string) ([]Value, error) {
	host, port := hostPortFromURL(dsn)
	if err := checkDBPolicy(r, "open", policy.Args{"kind": kind, "host": host, "port": port}); err != nil {
		return nil, err
	}
	conn, err := OpenDBConn(kind, dsn)
	if err != nil {
		return nil, r.AqlError("db_error", "open: "+err.Error(), "open")
	}
	return []Value{NewDBConn(conn)}, nil
}

func dbOpenHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	kind, err := args[0].AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}
	dsn, err := args[1].AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}
	return dbOpen(r, kind, dsn)
}

func dbOpenDSNHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	dsn, err := args[0].AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}
	kind, ok := inferDBKind(dsn)
	if !ok {
		return nil, r.AqlError("db_error",
			"open: cannot infer database kind from DSN; use: DB.open <kind> <dsn>", "open")
	}
	return dbOpen(r, kind, dsn)
}

// dbConnArg extracts the *DBConn payload from a value or returns a
// clear error.
func dbConnArg(r *Registry, v Value, word string) (*DBConn, error) {
	c, ok := AsDBConn(v)
	if !ok {
		return nil, r.AqlError("db_error", word+": expected a DB connection, got "+v.Parent.String(), word)
	}
	return c, nil
}

func dbCloseHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	c, err := dbConnArg(r, args[0], "close")
	if err != nil {
		return nil, err
	}
	if err := c.Close(); err != nil {
		return nil, r.AqlError("db_error", "close: "+err.Error(), "close")
	}
	return []Value{}, nil
}

func dbSQLHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return dbRunQuery(r, args[0], nil, args[1])
}

func dbSQLParamsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	params, err := aqlListToParams(args[1])
	if err != nil {
		return nil, r.AqlError("db_error", "sql: "+err.Error(), "sql")
	}
	return dbRunQuery(r, args[0], params, args[2])
}

func dbRunQuery(r *Registry, sqlV Value, params []any, connV Value) ([]Value, error) {
	sqlStr, err := sqlV.AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("db sql: %w", err)
	}
	c, err := dbConnArg(r, connV, "sql")
	if err != nil {
		return nil, err
	}
	if err := checkDBPolicy(r, "query", policy.Args{"kind": c.Kind}); err != nil {
		return nil, err
	}
	td, err := c.Backend.QueryParams(sqlStr, params, nil)
	if err != nil {
		return nil, r.AqlError("db_error", "sql: "+err.Error(), "sql")
	}
	return []Value{NewValueRaw(TList, td)}, nil
}

func dbExecHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return dbRunExec(r, args[0], nil, args[1])
}

func dbExecParamsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	params, err := aqlListToParams(args[1])
	if err != nil {
		return nil, r.AqlError("db_error", "exec: "+err.Error(), "exec")
	}
	return dbRunExec(r, args[0], params, args[2])
}

func dbRunExec(r *Registry, sqlV Value, params []any, connV Value) ([]Value, error) {
	sqlStr, err := sqlV.AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("db exec: %w", err)
	}
	c, err := dbConnArg(r, connV, "exec")
	if err != nil {
		return nil, err
	}
	if err := checkDBPolicy(r, "exec", policy.Args{"kind": c.Kind}); err != nil {
		return nil, err
	}
	n, err := c.Backend.Exec(sqlStr, params)
	if err != nil {
		return nil, r.AqlError("db_error", "exec: "+err.Error(), "exec")
	}
	return []Value{NewInteger(n)}, nil
}

func dbTablesHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	c, err := dbConnArg(r, args[0], "tables")
	if err != nil {
		return nil, err
	}
	if err := checkDBPolicy(r, "query", policy.Args{"kind": c.Kind}); err != nil {
		return nil, err
	}
	names, err := c.Backend.ListTables()
	if err != nil {
		return nil, r.AqlError("db_error", "tables: "+err.Error(), "tables")
	}
	out := make([]Value, len(names))
	for i, n := range names {
		out[i] = NewString(n)
	}
	return []Value{NewList(out)}, nil
}

func dbSchemaHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name, err := args[0].AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("db schema: %w", err)
	}
	c, err := dbConnArg(r, args[1], "schema")
	if err != nil {
		return nil, err
	}
	if err := checkDBPolicy(r, "query", policy.Args{"kind": c.Kind}); err != nil {
		return nil, err
	}
	rec, err := c.Backend.DescribeTable(name)
	if err != nil {
		return nil, r.AqlError("db_error", "schema: "+err.Error(), "schema")
	}
	// Present the schema as a {column: TypeName} map.
	out := NewOrderedMap()
	for _, k := range rec.Fields.Keys() {
		fv, _ := rec.Fields.Get(k)
		out.Set(k, NewString(TypeNameOf(fv)))
	}
	return []Value{NewMap(out)}, nil
}

// aqlListToParams converts a list value of scalars into positional bind
// parameters. A non-concrete value (None / type literal) yields none.
func aqlListToParams(v Value) ([]any, error) {
	if !IsConcrete(v) {
		return nil, nil
	}
	lst, err := AsList(v)
	if err != nil {
		return nil, fmt.Errorf("parameters must be a list")
	}
	elems := lst.Slice()
	params := make([]any, len(elems))
	for i, e := range elems {
		params[i] = aqlValueToParam(e)
	}
	return params, nil
}

// aqlValueToParam converts a scalar AQL value to a Go value for a SQL
// bind parameter.
func aqlValueToParam(v Value) any {
	switch {
	case v.Parent.Equal(TNone):
		return nil
	case v.Parent.ConformsTo(TInteger):
		n, _ := AsInteger(v)
		return n
	case v.Parent.ConformsTo(TFloat):
		f, _ := AsFloat(v)
		return f
	case v.Parent.ConformsTo(TBoolean):
		b, _ := AsBoolean(v)
		return b
	case v.Parent.ConformsTo(TString):
		s, _ := AsString(v)
		return s
	default:
		return ValToString(v)
	}
}
