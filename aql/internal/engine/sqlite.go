package engine

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// SQLiteStore manages an in-memory SQLite database for table storage.
type SQLiteStore struct {
	mu     sync.Mutex
	db     *sql.DB
	tables map[string]RecordTypeInfo // schema per table name
	seq    int                       // auto-incrementing counter for temp tables
}

// NewSQLiteStore opens a shared in-memory SQLite database.
func NewSQLiteStore() (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("sqlite: %w", err)
	}
	return &SQLiteStore{
		db:     db,
		tables: make(map[string]RecordTypeInfo),
	}, nil
}

// Close shuts down the SQLite database.
func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// StoreTable creates a SQLite table and inserts all rows from a TableData.
// Returns the table name used in SQLite.
func (s *SQLiteStore) StoreTable(name string, td TableData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	columns := td.Record.Fields.Keys()
	if len(columns) == 0 {
		return nil
	}

	// Drop existing table if it exists.
	_, _ = s.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(name)))

	// Create table with all TEXT columns.
	colDefs := make([]string, len(columns))
	for i, col := range columns {
		colDefs[i] = quoteIdent(col) + " TEXT"
	}
	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", quoteIdent(name), strings.Join(colDefs, ", "))
	if _, err := s.db.Exec(createSQL); err != nil {
		return fmt.Errorf("sqlite create: %w", err)
	}

	// Insert rows.
	if len(td.Rows) > 0 {
		placeholders := make([]string, len(columns))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			quoteIdent(name),
			joinQuoted(columns),
			strings.Join(placeholders, ", "))

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("sqlite begin: %w", err)
		}
		stmt, err := tx.Prepare(insertSQL)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("sqlite prepare: %w", err)
		}
		defer stmt.Close()

		for _, row := range td.Rows {
			m, ok := row.Data.(*OrderedMap)
			if !ok {
				continue
			}
			vals := make([]interface{}, len(columns))
			for i, col := range columns {
				v, exists := m.Get(col)
				if !exists {
					vals[i] = ""
				} else if v.VType.Matches(TString) {
					vals[i] = v.AsString()
				} else if v.VType.Matches(TInteger) {
					vals[i] = fmt.Sprintf("%d", v.AsInteger())
				} else if v.VType.Matches(TBoolean) {
					if v.AsBoolean() {
						vals[i] = "true"
					} else {
						vals[i] = "false"
					}
				} else {
					vals[i] = v.String()
				}
			}
			if _, err := stmt.Exec(vals...); err != nil {
				tx.Rollback()
				return fmt.Errorf("sqlite insert: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("sqlite commit: %w", err)
		}
	}

	s.tables[name] = td.Record
	return nil
}

// StoreTempTable stores a table under an auto-generated name and returns
// the name. Used for non-SQLite tables that need querying.
func (s *SQLiteStore) StoreTempTable(td TableData) (string, error) {
	s.mu.Lock()
	s.seq++
	name := fmt.Sprintf("_tmp_%d", s.seq)
	s.mu.Unlock()
	return name, s.StoreTable(name, td)
}

// Query executes a SELECT query and returns results as TableData.
func (s *SQLiteStore) Query(querySQL string) (TableData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(querySQL)
	if err != nil {
		return TableData{}, fmt.Errorf("sqlite query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return TableData{}, fmt.Errorf("sqlite columns: %w", err)
	}

	// Build record schema.
	fields := NewOrderedMap()
	for _, col := range cols {
		fields.Set(col, NewTypeLiteral(TString))
	}
	record := RecordTypeInfo{Fields: fields}

	// Read rows.
	var resultRows []Value
	scanDest := make([]interface{}, len(cols))
	scanPtrs := make([]sql.NullString, len(cols))
	for i := range scanDest {
		scanDest[i] = &scanPtrs[i]
	}

	for rows.Next() {
		if err := rows.Scan(scanDest...); err != nil {
			return TableData{}, fmt.Errorf("sqlite scan: %w", err)
		}
		om := NewOrderedMap()
		for i, col := range cols {
			if scanPtrs[i].Valid {
				om.Set(col, NewString(scanPtrs[i].String))
			} else {
				om.Set(col, NewString(""))
			}
		}
		resultRows = append(resultRows, NewMap(om))
	}
	if err := rows.Err(); err != nil {
		return TableData{}, fmt.Errorf("sqlite rows: %w", err)
	}

	return TableData{Record: record, Rows: resultRows}, nil
}

// HasTable reports whether a table with the given name exists in the store.
func (s *SQLiteStore) HasTable(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.tables[name]
	return ok
}

// DropTable removes a table from the SQLite store.
func (s *SQLiteStore) DropTable(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(name)))
	delete(s.tables, name)
}

// quoteIdent quotes a SQL identifier with double quotes.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// joinQuoted joins identifiers with commas, each quoted.
func joinQuoted(names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = quoteIdent(n)
	}
	return strings.Join(parts, ", ")
}
