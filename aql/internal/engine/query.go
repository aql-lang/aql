package engine

import (
	"fmt"
	"strings"
)

// QueryBuilder accumulates SQL clauses for deferred query execution.
// Instead of running a separate SQL query for each pipeline word
// (where, order, limit), the QueryBuilder collects all clauses and
// executes a single combined query when materialized.
type QueryBuilder struct {
	Source   TableData  // the source table data
	Registry *Registry // needed for SQLite access during materialization
	Where    string    // WHERE condition (without keyword)
	OrderBy  string    // ORDER BY clause (without keyword)
	Limit    int       // -1 = no limit
	Offset   int       // -1 = no offset
}

// NewQueryBuilder creates a QueryBuilder from a table data source.
func NewQueryBuilder(r *Registry, td TableData) QueryBuilder {
	return QueryBuilder{
		Source:   td,
		Registry: r,
		Limit:   -1,
		Offset:  -1,
	}
}

// clone returns a shallow copy of the QueryBuilder for safe mutation.
func (qb QueryBuilder) clone() QueryBuilder {
	return qb
}

// buildSQL constructs the full SQL query string.
func (qb *QueryBuilder) buildSQL(tableName string, colSQL string) string {
	var buf strings.Builder
	buf.WriteString("SELECT ")
	buf.WriteString(colSQL)
	buf.WriteString(" FROM ")
	buf.WriteString(quoteIdent(tableName))
	if qb.Where != "" {
		buf.WriteString(" WHERE ")
		buf.WriteString(qb.Where)
	}
	if qb.OrderBy != "" {
		buf.WriteString(" ORDER BY ")
		buf.WriteString(qb.OrderBy)
	}
	if qb.Limit >= 0 {
		fmt.Fprintf(&buf, " LIMIT %d", qb.Limit)
	}
	if qb.Offset >= 0 {
		fmt.Fprintf(&buf, " OFFSET %d", qb.Offset)
	}
	return buf.String()
}

// ensureSource loads the source table into SQLite if needed.
// Returns the table name to use and whether a temp table was created.
func (qb *QueryBuilder) ensureSource() (string, bool, error) {
	if qb.Source.SQLite {
		return qb.Source.TableName, false, nil
	}
	r := qb.Registry
	if r.SQLite == nil {
		return "", false, fmt.Errorf("SQLite store not initialized")
	}
	tmpName, err := r.SQLite.StoreTempTable(qb.Source)
	if err != nil {
		return "", false, err
	}
	return tmpName, true, nil
}

// Materialize executes the accumulated query and returns the result as TableData.
func (qb *QueryBuilder) Materialize() (TableData, error) {
	tableName, ownsTmp, err := qb.ensureSource()
	if err != nil {
		return TableData{}, err
	}
	if ownsTmp {
		defer qb.Registry.SQLite.DropTable(tableName)
	}

	query := qb.buildSQL(tableName, "*")
	result, err := qb.Registry.SQLite.Query(query)
	if err != nil {
		return TableData{}, err
	}
	return result, nil
}

// MaterializeWithColumns executes with specific column selection.
func (qb *QueryBuilder) MaterializeWithColumns(cols []columnSpec) (TableData, error) {
	tableName, ownsTmp, err := qb.ensureSource()
	if err != nil {
		return TableData{}, err
	}
	if ownsTmp {
		defer qb.Registry.SQLite.DropTable(tableName)
	}

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

	query := qb.buildSQL(tableName, colSQL)
	result, err := qb.Registry.SQLite.Query(query)
	if err != nil {
		return TableData{}, err
	}
	return result, nil
}

// registerQuery registers the select, from, star, where, order, by, and limit words.
func registerQuery(r *Registry) {
	// star: [] -> [atom("*")]
	// Alias for the * wildcard, usable in definitions where * cannot be typed.
	r.RegisterPrefixOnly("star", Signature{
		Handler: func(_ []Value) ([]Value, error) {
			return []Value{NewAtom("*")}, nil
		},
	})

	// from: [atom] -> [query-builder]
	// Looks up a table by name from the registry store and wraps it
	// in a QueryBuilder for deferred clause accumulation.
	fromHandler := func(args []Value) ([]Value, error) {
		name := args[0].AsAtom()
		val, ok := r.Store[name]
		if !ok {
			return nil, fmt.Errorf("from: unknown table %q", name)
		}
		if !val.IsTableType() {
			return nil, fmt.Errorf("from: %q is not a table", name)
		}

		td, ok := val.Data.(TableData)
		if !ok {
			return nil, fmt.Errorf("from: %q has no table data", name)
		}

		qb := NewQueryBuilder(r, td)
		return []Value{Value{VType: TList, Data: qb}}, nil
	}

	r.Register("from",
		Signature{
			Args:    []Type{TAtom},
			Handler: fromHandler,
		},
	)

	// select: [atom, list] -> [table]  (select * from ...)
	// select: [list, list] -> [table]  (select [a, b] from ...)
	// Materializes QueryBuilder or TableData into final result.
	selectStarHandler := func(args []Value) ([]Value, error) {
		colSpec := args[0] // atom "*"
		table := args[1]   // table/query-builder value

		if colSpec.AsAtom() != "*" {
			return nil, fmt.Errorf("select: expected * or column list, got atom %q", colSpec.AsAtom())
		}

		return doSelect(r, nil, table)
	}

	selectColsHandler := func(args []Value) ([]Value, error) {
		colList := args[0] // list of columns/aliases
		table := args[1]   // table/query-builder value

		cols, err := parseColumnSpec(colList)
		if err != nil {
			return nil, err
		}

		return doSelect(r, cols, table)
	}

	r.Register("select",
		Signature{
			Args:    []Type{TAtom, TList},
			Handler: selectStarHandler,
		},
		Signature{
			Args:    []Type{TList, TList},
			Handler: selectColsHandler,
		},
	)

	// where: [condition(suffix), table/query(prefix)] -> [query-builder]
	// Filters table rows using a SQL WHERE clause derived from the condition list.
	// Condition list elements: column-name op value [and|or column-name op value ...]
	// Supported ops: eq, lt, gt, lte, gte, like
	// Usage: from people where [age gt "25"]
	whereHandler := func(args []Value) ([]Value, error) {
		table := args[0]    // prefix: table/query from stack
		condList := args[1] // suffix: condition list

		clause, err := buildWhereClause(condList)
		if err != nil {
			return nil, fmt.Errorf("where: %w", err)
		}

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("where: %w", err)
		}
		qb.Where = clause
		return []Value{Value{VType: TList, Data: qb}}, nil
	}

	r.Register("where",
		Signature{
			Args:       []Type{TList, TList},
			Precedence: 1,
			Handler:    whereHandler,
		},
	)

	// order: [columns(suffix), table/query(prefix)] -> [query-builder]
	// Sorts table rows using ORDER BY. Accepts a column atom or a list
	// of columns with optional asc/desc direction.
	// Usage: from people order name
	//        from people order [name desc]
	//        from people order by name
	//        from people order by [name desc]
	orderListHandler := func(args []Value) ([]Value, error) {
		table := args[0]   // prefix: table/query from stack
		colList := args[1] // suffix: column list

		clause, err := buildOrderClause(colList)
		if err != nil {
			return nil, fmt.Errorf("order: %w", err)
		}

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("order: %w", err)
		}
		qb.OrderBy = clause
		return []Value{Value{VType: TList, Data: qb}}, nil
	}

	orderAtomHandler := func(args []Value) ([]Value, error) {
		col := args[0]   // column name (TAtom)
		table := args[1] // table (TList)

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("order: %w", err)
		}
		qb.OrderBy = quoteIdent(col.AsAtom())
		return []Value{Value{VType: TList, Data: qb}}, nil
	}

	r.Register("order",
		Signature{
			Args:       []Type{TList, TList},
			Precedence: 1,
			Handler:    orderListHandler,
		},
		Signature{
			Args:       []Type{TAtom, TList},
			Precedence: 1,
			Handler:    orderAtomHandler,
		},
	)

	// by: [atom] -> [list], [list] -> [list]
	// Syntactic sugar so "order by name" reads naturally.
	// Wraps atoms into a list so "order" always receives TList from "by".
	r.Register("by",
		Signature{
			Args: []Type{TAtom},
			Handler: func(args []Value) ([]Value, error) {
				return []Value{NewList(args)}, nil
			},
		},
		Signature{
			Args: []Type{TList},
			Handler: func(args []Value) ([]Value, error) {
				return args, nil
			},
		},
	)

	// limit: [integer(suffix), table/query(prefix)] -> [query-builder]
	// Restricts the number of rows returned.
	// Usage: from people limit 2
	limitHandler := func(args []Value) ([]Value, error) {
		n := args[0].AsInteger() // suffix: count (TInteger)
		table := args[1]         // prefix: table/query from stack (TList)

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("limit: %w", err)
		}
		qb.Limit = int(n)
		return []Value{Value{VType: TList, Data: qb}}, nil
	}

	r.Register("limit",
		Signature{
			Args:       []Type{TInteger, TList},
			Precedence: 1,
			Handler:    limitHandler,
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

// toQueryBuilder converts a Value (QueryBuilder or TableData) into a QueryBuilder.
func toQueryBuilder(r *Registry, v Value) (QueryBuilder, error) {
	if qb, ok := v.Data.(QueryBuilder); ok {
		return qb.clone(), nil
	}
	if td, ok := v.Data.(TableData); ok {
		return NewQueryBuilder(r, td), nil
	}
	return QueryBuilder{}, fmt.Errorf("argument is not a table or query")
}

// doSelect performs a SELECT query, materializing a QueryBuilder or TableData.
// If cols is nil, selects all columns (*).
func doSelect(r *Registry, cols []columnSpec, table Value) ([]Value, error) {
	qb, err := toQueryBuilder(r, table)
	if err != nil {
		return nil, fmt.Errorf("select: %w", err)
	}

	var result TableData
	if cols == nil {
		result, err = qb.Materialize()
	} else {
		result, err = qb.MaterializeWithColumns(cols)
	}
	if err != nil {
		return nil, fmt.Errorf("select: %w", err)
	}

	return []Value{Value{VType: TList, Data: result}}, nil
}

// comparisonOps maps AQL comparison word names to SQL operators.
var comparisonOps = map[string]string{
	"eq":   "=",
	"neq":  "!=",
	"lt":   "<",
	"gt":   ">",
	"lte":  "<=",
	"gte":  ">=",
	"like": "LIKE",
}

// logicalOps maps AQL logical word names to SQL connectors.
var logicalOps = map[string]string{
	"and": "AND",
	"or":  "OR",
}

// buildWhereClause translates a condition list into a SQL WHERE clause.
// Format: [column op value] or [column op value and/or column op value ...]
func buildWhereClause(condList Value) (string, error) {
	elems := condList.AsList()
	if len(elems) == 0 {
		return "1=1", nil
	}

	var parts []string
	i := 0
	for i < len(elems) {
		// Expect: column op value
		if i+2 >= len(elems) {
			return "", fmt.Errorf("incomplete condition: expected column op value")
		}

		col := valueToColName(elems[i])
		if col == "" {
			return "", fmt.Errorf("expected column name, got %s", elems[i].VType)
		}

		opName := valueToColName(elems[i+1])
		sqlOp, ok := comparisonOps[opName]
		if !ok {
			return "", fmt.Errorf("unknown comparison operator %q (use eq, neq, lt, gt, lte, gte, like)", opName)
		}

		val := elems[i+2]
		sqlVal, err := valueToSQL(val)
		if err != nil {
			return "", err
		}

		parts = append(parts, fmt.Sprintf("%s %s %s", quoteIdent(col), sqlOp, sqlVal))
		i += 3

		// Check for logical connector.
		if i < len(elems) {
			connName := valueToColName(elems[i])
			sqlConn, ok := logicalOps[connName]
			if ok {
				parts = append(parts, sqlConn)
				i++
			}
		}
	}

	return strings.Join(parts, " "), nil
}

// valueToSQL converts a Value to a SQL literal string.
func valueToSQL(v Value) (string, error) {
	switch {
	case v.VType.Matches(TString):
		// Escape single quotes for SQL.
		return "'" + strings.ReplaceAll(v.AsString(), "'", "''") + "'", nil
	case v.VType.Matches(TInteger):
		return fmt.Sprintf("%d", v.AsInteger()), nil
	case v.VType.Matches(TBoolean):
		if v.AsBoolean() {
			return "'true'", nil
		}
		return "'false'", nil
	case v.VType.Equal(TAtom):
		// Atoms used as values are treated as strings.
		return "'" + strings.ReplaceAll(v.AsAtom(), "'", "''") + "'", nil
	case v.VType.Equal(TNone):
		return "NULL", nil
	default:
		return "", fmt.Errorf("unsupported value type in condition: %s", v.VType)
	}
}

// buildOrderClause translates a column list into a SQL ORDER BY clause.
// Supports: [col1, col2] or [col1 asc, col2 desc] or [col1, desc, col2].
// Direction atoms "asc" and "desc" are applied to the preceding column.
func buildOrderClause(colList Value) (string, error) {
	elems := colList.AsList()
	if len(elems) == 0 {
		return "", fmt.Errorf("empty order column list")
	}

	var parts []string
	for _, e := range elems {
		name := valueToColName(e)
		if name == "" {
			return "", fmt.Errorf("expected column name or asc/desc, got %s", e.VType)
		}
		lower := strings.ToLower(name)
		if lower == "asc" || lower == "desc" {
			// Attach direction to previous column.
			if len(parts) == 0 {
				return "", fmt.Errorf("asc/desc without preceding column name")
			}
			parts[len(parts)-1] += " " + strings.ToUpper(lower)
		} else {
			parts = append(parts, quoteIdent(name))
		}
	}

	return strings.Join(parts, ", "), nil
}
