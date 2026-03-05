package engine

import (
	"fmt"
	"strings"
)

// registerQuery registers the select and from words.
func registerQuery(r *Registry) {
	// from: [atom] -> [table]
	// Looks up a table by name from the registry store.
	fromHandler := func(args []Value) ([]Value, error) {
		name := args[0].AsAtom()
		val, ok := r.Store[name]
		if !ok {
			return nil, fmt.Errorf("from: unknown table %q", name)
		}
		if !val.IsTableType() {
			return nil, fmt.Errorf("from: %q is not a table", name)
		}
		return []Value{val}, nil
	}

	r.Register("from",
		Signature{
			Args:    []Type{TAtom},
			Handler: fromHandler,
		},
	)

	// select: [atom, list] -> [table]  (select * from ...)
	// select: [list, list] -> [table]  (select [a, b] from ...)
	selectStarHandler := func(args []Value) ([]Value, error) {
		colSpec := args[0] // atom "*"
		table := args[1]   // table value

		if colSpec.AsAtom() != "*" {
			return nil, fmt.Errorf("select: expected * or column list, got atom %q", colSpec.AsAtom())
		}

		return doSelect(r, nil, table)
	}

	selectColsHandler := func(args []Value) ([]Value, error) {
		colList := args[0] // list of columns/aliases
		table := args[1]   // table value

		cols, err := parseColumnSpec(colList)
		if err != nil {
			return nil, err
		}

		return doSelect(r, cols, table)
	}

	r.Register("select",
		Signature{
			Args:    []Type{TList, TList},
			Handler: selectColsHandler,
		},
		Signature{
			Args:    []Type{TAtom, TList},
			Handler: selectStarHandler,
		},
	)
}

// columnSpec describes a column selection, with optional alias.
type columnSpec struct {
	Name  string
	Alias string // empty means no alias
}

// parseColumnSpec parses the column list from a select word.
// Supports: [a, b] and [[a x] b] for aliasing.
func parseColumnSpec(colList Value) ([]columnSpec, error) {
	elems := colList.AsList()
	cols := make([]columnSpec, 0, len(elems))
	for _, e := range elems {
		switch {
		case e.VType.Equal(TAtom):
			cols = append(cols, columnSpec{Name: e.AsAtom()})
		case e.VType.Matches(TString):
			cols = append(cols, columnSpec{Name: e.AsString()})
		case e.IsWord():
			cols = append(cols, columnSpec{Name: e.AsWord().Name})
		case e.VType.Equal(TList):
			// [name alias] pair
			pair := e.AsList()
			if len(pair) != 2 {
				return nil, fmt.Errorf("select: column alias must be [name alias], got %d elements", len(pair))
			}
			name := valueToColName(pair[0])
			alias := valueToColName(pair[1])
			if name == "" || alias == "" {
				return nil, fmt.Errorf("select: column alias elements must be atoms, strings, or words")
			}
			cols = append(cols, columnSpec{Name: name, Alias: alias})
		default:
			return nil, fmt.Errorf("select: unsupported column spec type: %s", e.VType)
		}
	}
	return cols, nil
}

// valueToColName extracts the string content from an atom, string, or word value.
func valueToColName(v Value) string {
	if v.VType.Equal(TAtom) {
		return v.AsAtom()
	}
	if v.VType.Matches(TString) {
		return v.AsString()
	}
	if v.IsWord() {
		return v.AsWord().Name
	}
	return ""
}

// doSelect performs a SELECT query on a table value.
// If cols is nil, selects all columns (*).
func doSelect(r *Registry, cols []columnSpec, table Value) ([]Value, error) {
	td, ok := table.Data.(TableData)
	if !ok {
		return nil, fmt.Errorf("select: argument is not a table")
	}

	// Ensure the table is in SQLite.
	tableName := td.TableName
	if !td.SQLite {
		// Create a temporary SQLite table for non-SQLite-backed tables.
		if r.SQLite == nil {
			return nil, fmt.Errorf("select: SQLite store not initialized")
		}
		tmpName, err := r.SQLite.StoreTempTable(td)
		if err != nil {
			return nil, fmt.Errorf("select: %w", err)
		}
		tableName = tmpName
		defer r.SQLite.DropTable(tmpName)
	}

	// Build the SQL query.
	query, err := buildSelectSQL(cols, tableName)
	if err != nil {
		return nil, err
	}

	result, err := r.SQLite.Query(query)
	if err != nil {
		return nil, fmt.Errorf("select: %w", err)
	}

	return []Value{Value{VType: TList, Data: result}}, nil
}

// buildSelectSQL constructs a SQL SELECT statement.
func buildSelectSQL(cols []columnSpec, tableName string) (string, error) {
	var colSQL string
	if cols == nil {
		colSQL = "*"
	} else {
		parts := make([]string, len(cols))
		for i, c := range cols {
			if c.Alias != "" {
				parts[i] = quoteIdent(c.Name) + " AS " + quoteIdent(c.Alias)
			} else {
				parts[i] = quoteIdent(c.Name)
			}
		}
		colSQL = strings.Join(parts, ", ")
	}
	return fmt.Sprintf("SELECT %s FROM %s", colSQL, quoteIdent(tableName)), nil
}
