//go:build js

package engine

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"syscall/js"
)

// aqlTypeToSQLType maps an AQL field type to a SQLite column type.
func aqlTypeToSQLType(t Type) string {
	switch {
	case t.Matches(TInteger):
		return "INTEGER"
	case t.Matches(TDecimal):
		return "REAL"
	case t.Matches(TNumber):
		return "REAL"
	case t.Matches(TBoolean):
		return "INTEGER"
	default:
		return "TEXT"
	}
}

// SQLiteStore bridges to sql.js running in the browser.
type SQLiteStore struct {
	mu     sync.Mutex
	db     js.Value
	tables map[string]RecordTypeInfo
	seq    int
}

// NewSQLiteStore creates a sql.js in-memory database.
// Requires sql.js to be loaded and window.__sqlJS set before Go starts.
func NewSQLiteStore() (*SQLiteStore, error) {
	sqlJS := js.Global().Get("__sqlJS")
	if sqlJS.IsUndefined() || sqlJS.IsNull() {
		// sql.js not loaded — return nil (features requiring SQLite will be unavailable)
		return nil, nil
	}

	db := sqlJS.Get("Database").New()

	// Register a regexp function matching the native build.
	regexpFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 2 || args[0].IsNull() || args[1].IsNull() {
			return nil
		}
		pattern := args[0].String()
		value := args[1].String()
		re := js.Global().Get("RegExp").New(pattern)
		if re.Call("test", value).Bool() {
			return 1
		}
		return 0
	})
	db.Call("create_function", "regexp", regexpFn)

	return &SQLiteStore{
		db:     db,
		tables: make(map[string]RecordTypeInfo),
	}, nil
}

// Close shuts down the database.
func (s *SQLiteStore) Close() error {
	if !s.db.IsUndefined() && !s.db.IsNull() {
		s.db.Call("close")
	}
	return nil
}

// StoreTable creates a table and inserts all rows.
func (s *SQLiteStore) StoreTable(name string, td TableData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	columns := td.Record.Fields.Keys()
	if len(columns) == 0 {
		return nil
	}

	// Drop existing table.
	s.db.Call("run", fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(name)))

	// Create table with typed columns.
	colDefs := make([]string, len(columns))
	colTypes := make([]Type, len(columns))
	for i, col := range columns {
		fieldVal, _ := td.Record.Fields.Get(col)
		colTypes[i] = fieldVal.VType
		colDefs[i] = quoteIdent(col) + " " + aqlTypeToSQLType(fieldVal.VType)
	}
	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", quoteIdent(name), strings.Join(colDefs, ", "))
	s.db.Call("run", createSQL)

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

		for _, row := range td.Rows {
			m, ok := row.Data.(*OrderedMap)
			if !ok {
				continue
			}
			params := js.Global().Get("Array").New(len(columns))
			for i, col := range columns {
				v, exists := m.Get(col)
				if !exists {
					params.SetIndex(i, js.Null())
				} else {
					params.SetIndex(i, aqlValueToJSParam(v, colTypes[i]))
				}
			}
			s.db.Call("run", insertSQL, params)
		}
	}

	s.tables[name] = td.Record
	return nil
}

// StoreTempTable stores a table under an auto-generated name.
func (s *SQLiteStore) StoreTempTable(td TableData) (string, error) {
	s.mu.Lock()
	s.seq++
	name := fmt.Sprintf("_tmp_%d", s.seq)
	s.mu.Unlock()
	return name, s.StoreTable(name, td)
}

// Query executes a SELECT and returns results as TableData.
func (s *SQLiteStore) Query(querySQL string, schema *RecordTypeInfo) (TableData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := s.db.Call("exec", querySQL)
	if results.Length() == 0 {
		// Empty result set.
		fields := NewOrderedMap()
		if schema != nil {
			for _, k := range schema.Fields.Keys() {
				v, _ := schema.Fields.Get(k)
				fields.Set(k, v)
			}
		}
		return TableData{Record: RecordTypeInfo{Fields: fields}}, nil
	}

	result := results.Index(0)
	jsCols := result.Get("columns")
	jsVals := result.Get("values")
	numCols := jsCols.Length()

	cols := make([]string, numCols)
	colTypes := make([]Type, numCols)
	for i := 0; i < numCols; i++ {
		cols[i] = jsCols.Index(i).String()
		colTypes[i] = TString // default
		if schema != nil {
			if fieldVal, ok := schema.Fields.Get(cols[i]); ok {
				colTypes[i] = fieldVal.VType
			}
		}
	}

	// Build record schema.
	fields := NewOrderedMap()
	for i, col := range cols {
		fields.Set(col, NewTypeLiteral(colTypes[i]))
	}
	record := RecordTypeInfo{Fields: fields}

	// Convert rows.
	numRows := jsVals.Length()
	resultRows := make([]Value, 0, numRows)
	for r := 0; r < numRows; r++ {
		jsRow := jsVals.Index(r)
		om := NewOrderedMap()
		for i, col := range cols {
			jsVal := jsRow.Index(i)
			om.Set(col, jsValueToAQL(jsVal, colTypes[i]))
		}
		resultRows = append(resultRows, NewMap(om))
	}

	return TableData{Record: record, Rows: resultRows}, nil
}

// HasTable reports whether a table exists.
func (s *SQLiteStore) HasTable(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.tables[name]
	return ok
}

// DropTable removes a table.
func (s *SQLiteStore) DropTable(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Call("run", fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(name)))
	delete(s.tables, name)
}

// aqlValueToJSParam converts an AQL Value to a JS value for sql.js binding.
func aqlValueToJSParam(v Value, colType Type) any {
	if v.VType.Equal(TNone) {
		return js.Null()
	}
	switch {
	case colType.Matches(TInteger):
		if v.VType.Matches(TInteger) {
			_as0, _ := v.AsInteger()
			return _as0
		}
		if v.VType.Matches(TString) {
			_as1, _ := v.AsString()
			if n, err := strconv.ParseInt(_as1, 10, 64); err == nil {
				return n
			}
		}
		if v.VType.Matches(TBoolean) {
			_as2, _ := v.AsBoolean()
			if _as2 {
				return 1
			}
			return 0
		}
		return ValToString(v)
	case colType.Matches(TNumber):
		if v.VType.Matches(TDecimal) {
			_as3, _ := v.AsDecimal()
			return _as3
		}
		if v.VType.Matches(TInteger) {
			_as4, _ := v.AsInteger()
			return float64(_as4)
		}
		if v.VType.Matches(TString) {
			_as5, _ := v.AsString()
			if f, err := strconv.ParseFloat(_as5, 64); err == nil {
				return f
			}
		}
		return ValToString(v)
	case colType.Matches(TBoolean):
		if v.VType.Matches(TBoolean) {
			_as6, _ := v.AsBoolean()
			if _as6 {
				return 1
			}
			return 0
		}
		if v.VType.Matches(TString) {
			_as7, _ := v.AsString()
			if _as7 == "true" {
				return 1
			}
			return 0
		}
		return ValToString(v)
	default:
		if v.VType.Matches(TString) {
			_as8, _ := v.AsString()
			return _as8
		}
		return ValToString(v)
	}
}

// jsValueToAQL converts a sql.js result value to an AQL Value.
func jsValueToAQL(v js.Value, colType Type) Value {
	if v.IsNull() || v.IsUndefined() {
		return NewValueRaw(TNone, nil)
	}
	switch {
	case colType.Matches(TInteger):
		return NewInteger(int64(v.Float()))
	case colType.Matches(TNumber):
		return NewDecimal(v.Float())
	case colType.Matches(TBoolean):
		return NewBoolean(v.Float() != 0)
	default:
		return NewString(v.String())
	}
}
