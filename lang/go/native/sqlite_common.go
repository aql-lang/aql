package native

import "fmt"

// Build-agnostic SQLiteStore methods. These depend only on Query plus
// generic helpers (no database/sql or sql.js specifics), so a single
// definition serves both the native (sqlite.go) and wasm
// (sqlite_stub.go) builds.

// Dialect reports the SQL dialect of the embedded store: SQLite.
func (s *SQLiteStore) Dialect() Dialect { return SQLiteDialect{} }

// ListTables returns the user tables in the database.
func (s *SQLiteStore) ListTables() ([]string, error) {
	td, err := s.Query(SQLiteDialect{}.IntrospectTablesSQL(), nil)
	if err != nil {
		return nil, err
	}
	return firstColumnStrings(td), nil
}

// DescribeTable returns the column schema of a table via PRAGMA.
func (s *SQLiteStore) DescribeTable(name string) (RecordTypeInfo, error) {
	// PRAGMA table_info exposes a name column and a type column.
	td, err := s.Query(fmt.Sprintf("PRAGMA table_info(%s)", quoteIdent(name)), nil)
	if err != nil {
		return RecordTypeInfo{}, err
	}
	fields := NewOrderedMap()
	for _, row := range td.Rows {
		m, err := AsMap(row)
		if err != nil {
			continue
		}
		nv, _ := m.Get("name")
		tv, _ := m.Get("type")
		cn := ValToString(nv)
		if cn == "" {
			continue
		}
		fields.Set(cn, NewTypeLiteral(SQLiteDialect{}.ColumnTypeToAQLType(ValToString(tv))))
	}
	if fields.Len() == 0 {
		return RecordTypeInfo{}, fmt.Errorf("sqlite: no such table %q", name)
	}
	return RecordTypeInfo{Fields: fields}, nil
}
