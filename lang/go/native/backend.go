package native

// SQLBackend is the storage/execution abstraction the query layer
// targets. The embedded engine (*SQLiteStore) is one implementation;
// external connections (*SQLDatabase, non-js builds) are another. It
// generalizes the method set SQLiteStore already exposed, adding
// parameterized queries, raw DML/DDL execution, and schema
// introspection for the external-connection and raw-SQL paths.
//
// A compile-time assertion that *SQLiteStore satisfies this interface
// lives in sqlite.go / sqlite_stub.go (one per build tag) so both the
// native and wasm builds keep the implementation complete.
type SQLBackend interface {
	// StoreTable creates (replacing any existing) a table named name
	// from td and inserts its rows. Embedded backends use this to
	// materialize CSV/JSON/remote data; read-only remote backends
	// reject it.
	StoreTable(name string, td TableData) error

	// StoreTempTable stores td under an auto-generated name and returns
	// that name.
	StoreTempTable(td TableData) (string, error)

	// Query runs a SELECT and returns its rows. schema, when non-nil,
	// supplies the AQL result-column types; when nil the backend infers
	// them from the driver's reported column types.
	Query(sql string, schema *RecordTypeInfo) (TableData, error)

	// QueryParams is Query with positional bind parameters, used by the
	// raw-SQL passthrough and the remote pushdown path.
	QueryParams(sql string, params []any, schema *RecordTypeInfo) (TableData, error)

	// Exec runs a non-SELECT statement (INSERT/UPDATE/DELETE/DDL) and
	// returns the number of affected rows when the driver reports it.
	Exec(sql string, params []any) (int64, error)

	// HasTable reports whether a table with the given name is known to
	// the backend.
	HasTable(name string) bool

	// DropTable removes a table if it exists.
	DropTable(name string)

	// ListTables returns the backend's user table names.
	ListTables() ([]string, error)

	// DescribeTable returns the column schema of a table.
	DescribeTable(name string) (RecordTypeInfo, error)

	// Dialect is the SQL dialect this backend speaks.
	Dialect() Dialect

	// Close releases the backend's resources.
	Close() error
}

// Compile-time assertion that the embedded SQLite store satisfies the
// backend interface. Checked under both build tags (native sqlite.go
// and wasm sqlite_stub.go), keeping each implementation complete.
var _ SQLBackend = (*SQLiteStore)(nil)
