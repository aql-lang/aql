package aqleng

// TableData holds a concrete table value: its record schema plus the
// row data.
//
// The engine references TableData structurally because list-shaped
// values can carry table semantics (printing, AsList, AsTableType all
// special-case it). Concrete query producers (such as a SQL-backed
// query builder) live in the host package and surface their result via
// the Materializer interface below.
type TableData struct {
	Record    RecordTypeInfo
	Rows      []Value
	SQLite    bool   // true if data is backed by an in-memory SQLite table
	TableName string // name of the table in the SQLite store
}

// Materializer is implemented by lazy table producers (e.g. query
// builders). The engine holds them as Value.Data and triggers
// Materialize() when it needs concrete rows for printing, iteration, or
// schema introspection.
type Materializer interface {
	Materialize() (TableData, error)
	// SourceRecord returns the schema of the underlying table without
	// triggering materialization. Used by Value.AsTableType so callers
	// that only need the record can avoid the materialization cost.
	SourceRecord() RecordTypeInfo
}
