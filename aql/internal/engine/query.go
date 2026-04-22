package engine

import (
	"fmt"
	"strings"
)

// JoinClause represents a single JOIN in a query.
type JoinClause struct {
	Type      string // "JOIN", "LEFT JOIN", "CROSS JOIN"
	Table     string // table name in the store
	Alias     string // optional alias
	On        string // ON condition (raw SQL)
	UsingCols string // USING(col1, col2) clause
}

// SetOp represents a set operation combining two queries.
type SetOp struct {
	Op    string       // "UNION", "UNION ALL", "INTERSECT", "EXCEPT"
	Right QueryBuilder // the right-hand query
}

// QueryBuilder accumulates SQL clauses for deferred query execution.
// Instead of running a separate SQL query for each pipeline word
// (where, order, limit), the QueryBuilder collects all clauses and
// executes a single combined query when materialized.
type QueryBuilder struct {
	Source   TableData // the source table data
	Registry *Registry // needed for SQLite access during materialization
	Where    string    // WHERE condition (without keyword)
	OrderBy  string    // ORDER BY clause (without keyword)
	Limit    int       // -1 = no limit
	Offset   int       // -1 = no offset
	Distinct bool      // true for SELECT DISTINCT
	GroupBy  string    // GROUP BY clause (without keyword)
	Having   string    // HAVING clause (without keyword)
	Joins    []JoinClause
	Alias    string // table alias for the FROM source
	SetOps   []SetOp
}

// NewQueryBuilder creates a QueryBuilder from a table data source.
func NewQueryBuilder(r *Registry, td TableData) QueryBuilder {
	return QueryBuilder{
		Source:   td,
		Registry: r,
		Limit:    -1,
		Offset:   -1,
	}
}

// clone returns a shallow copy of the QueryBuilder for safe mutation.
func (qb QueryBuilder) clone() QueryBuilder {
	if qb.Joins != nil {
		cp := make([]JoinClause, len(qb.Joins))
		copy(cp, qb.Joins)
		qb.Joins = cp
	}
	if qb.SetOps != nil {
		cp := make([]SetOp, len(qb.SetOps))
		copy(cp, qb.SetOps)
		qb.SetOps = cp
	}
	return qb
}

// buildSQL constructs the full SQL query string.
func (qb *QueryBuilder) buildSQL(tableName string, colSQL string) string {
	var buf strings.Builder
	buf.WriteString("SELECT ")
	if qb.Distinct {
		buf.WriteString("DISTINCT ")
	}
	buf.WriteString(colSQL)
	buf.WriteString(" FROM ")
	buf.WriteString(quoteIdent(tableName))
	if qb.Alias != "" {
		buf.WriteString(" AS ")
		buf.WriteString(quoteIdent(qb.Alias))
	}
	for _, j := range qb.Joins {
		buf.WriteString(" ")
		buf.WriteString(j.Type)
		buf.WriteString(" ")
		buf.WriteString(quoteIdent(j.Table))
		if j.Alias != "" {
			buf.WriteString(" AS ")
			buf.WriteString(quoteIdent(j.Alias))
		}
		if j.On != "" {
			buf.WriteString(" ON ")
			buf.WriteString(j.On)
		}
		if j.UsingCols != "" {
			buf.WriteString(" USING(")
			buf.WriteString(j.UsingCols)
			buf.WriteString(")")
		}
	}
	if qb.Where != "" {
		buf.WriteString(" WHERE ")
		buf.WriteString(qb.Where)
	}
	if qb.GroupBy != "" {
		buf.WriteString(" GROUP BY ")
		buf.WriteString(qb.GroupBy)
	}
	if qb.Having != "" {
		buf.WriteString(" HAVING ")
		buf.WriteString(qb.Having)
	}

	// Set operations (UNION, INTERSECT, EXCEPT) are appended after the
	// first SELECT but before ORDER BY / LIMIT which apply to the combined result.
	for _, so := range qb.SetOps {
		buf.WriteString(" ")
		buf.WriteString(so.Op)
		buf.WriteString(" ")
		// Build the right-hand SELECT. It uses its own source table.
		rightTable := so.Right.Source.TableName
		if !so.Right.Source.SQLite {
			rightTable = "" // will be resolved during materialize
		}
		rightSQL := so.Right.buildSQL(rightTable, "*")
		buf.WriteString(rightSQL)
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

// ensureJoinSources ensures all joined tables are in SQLite.
// Returns a list of temp table names that should be cleaned up.
func (qb *QueryBuilder) ensureJoinSources() ([]string, error) {
	var tmpNames []string
	for i := range qb.Joins {
		j := &qb.Joins[i]
		if qb.Registry.SQLite.HasTable(j.Table) {
			continue
		}
		// Look up the table in the context store and load it.
		val, ok := contextStoreLookup(qb.Registry, j.Table)
		if !ok {
			return tmpNames, fmt.Errorf("join: unknown table %q", j.Table)
		}
		td, ok := val.Data.(TableData)
		if !ok {
			return tmpNames, fmt.Errorf("join: %q has no table data", j.Table)
		}
		if td.SQLite {
			j.Table = td.TableName
		} else {
			tmpName, err := qb.Registry.SQLite.StoreTempTable(td)
			if err != nil {
				return tmpNames, err
			}
			j.Table = tmpName
			tmpNames = append(tmpNames, tmpName)
		}
	}
	return tmpNames, nil
}

// mergedSchema returns a combined schema from the source and all joined tables.
func (qb *QueryBuilder) mergedSchema() RecordTypeInfo {
	fields := NewOrderedMap()
	// Add source fields.
	for _, k := range qb.Source.Record.Fields.Keys() {
		v, _ := qb.Source.Record.Fields.Get(k)
		fields.Set(k, v)
	}
	// Add joined table fields.
	for _, j := range qb.Joins {
		val, ok := contextStoreLookup(qb.Registry, j.Table)
		if !ok {
			// Try the original name if it was remapped to a temp table.
			continue
		}
		td, ok := val.Data.(TableData)
		if !ok {
			continue
		}
		for _, k := range td.Record.Fields.Keys() {
			if _, exists := fields.Get(k); !exists {
				v, _ := td.Record.Fields.Get(k)
				fields.Set(k, v)
			}
		}
	}
	return RecordTypeInfo{Fields: fields}
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

	joinTmps, err := qb.ensureJoinSources()
	if err != nil {
		return TableData{}, err
	}
	for _, t := range joinTmps {
		defer qb.Registry.SQLite.DropTable(t)
	}

	// Ensure set-op right-hand sources are in SQLite.
	setOpTmps, err := qb.ensureSetOpSources()
	if err != nil {
		return TableData{}, err
	}
	for _, t := range setOpTmps {
		defer qb.Registry.SQLite.DropTable(t)
	}

	schema := qb.mergedSchema()
	query := qb.buildSQL(tableName, "*")
	result, err := qb.Registry.SQLite.Query(query, &schema)
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

	joinTmps, err := qb.ensureJoinSources()
	if err != nil {
		return TableData{}, err
	}
	for _, t := range joinTmps {
		defer qb.Registry.SQLite.DropTable(t)
	}

	setOpTmps, err := qb.ensureSetOpSources()
	if err != nil {
		return TableData{}, err
	}
	for _, t := range setOpTmps {
		defer qb.Registry.SQLite.DropTable(t)
	}

	var colSQL string
	if cols == nil {
		colSQL = "*"
	} else {
		parts := make([]string, len(cols))
		for i, c := range cols {
			if c.Raw != "" {
				// Raw SQL expression (aggregate, cast, etc.)
				if c.Alias != "" {
					parts[i] = c.Raw + " AS " + quoteIdent(c.Alias)
				} else {
					parts[i] = c.Raw
				}
			} else if c.Alias != "" {
				parts[i] = quoteIdent(c.Name) + " AS " + quoteIdent(c.Alias)
			} else {
				parts[i] = quoteIdent(c.Name)
			}
		}
		colSQL = strings.Join(parts, ", ")
	}

	// Build schema hint for the result columns.
	merged := qb.mergedSchema()
	resultSchema := &merged
	if cols != nil {
		resultFields := NewOrderedMap()
		for _, c := range cols {
			outputName := c.Name
			if c.Alias != "" {
				outputName = c.Alias
			}
			if c.Raw != "" && c.Alias != "" {
				outputName = c.Alias
			}
			if len(c.ResultType.Parts) > 0 {
				resultFields.Set(outputName, NewTypeLiteral(c.ResultType))
			} else if fieldVal, ok := merged.Fields.Get(c.Name); ok {
				resultFields.Set(outputName, fieldVal)
			} else {
				resultFields.Set(outputName, NewTypeLiteral(TString))
			}
		}
		resultSchema = &RecordTypeInfo{Fields: resultFields}
	}

	query := qb.buildSQL(tableName, colSQL)
	result, err := qb.Registry.SQLite.Query(query, resultSchema)
	if err != nil {
		return TableData{}, err
	}
	return result, nil
}

// ensureSetOpSources ensures all set-op right-hand sources are in SQLite.
func (qb *QueryBuilder) ensureSetOpSources() ([]string, error) {
	var tmpNames []string
	for i := range qb.SetOps {
		so := &qb.SetOps[i]
		if so.Right.Source.SQLite {
			continue
		}
		tmpName, err := qb.Registry.SQLite.StoreTempTable(so.Right.Source)
		if err != nil {
			return tmpNames, err
		}
		so.Right.Source.SQLite = true
		so.Right.Source.TableName = tmpName
		tmpNames = append(tmpNames, tmpName)
	}
	return tmpNames, nil
}

// RegisterQuery registers the select, from, star, where, order, by, limit,
// offset, distinct, groupby, having, join, on, using, union, intersect,
// except, cast, and aggregate words.
func RegisterQuery(r *Registry) {
	// star: [] -> [atom("*")]
	r.RegisterStackOnly("star", Signature{
		Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewAtom("*")}, nil
		},
		Returns: []Type{TAtom},
	})

	// from: [atom] -> [query-builder]
	fromHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name, _ := args[0].AsAtom()
		val, ok := contextStoreLookup(r, name)
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
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register("from",
		Signature{
			Args:    []Type{TAtom},
			Handler: fromHandler,
			Returns: []Type{TList},
		},
	)

	// as: [table/query(prefix), atom(forward)] -> [query-builder with alias]
	// Usage: from people as p
	asHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		alias, _ := args[1].AsAtom()

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("as: %w", err)
		}
		qb.Alias = alias
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register("as",
		Signature{
			Args:    []Type{TList, TAtom},
			Handler: asHandler,
			Returns: []Type{TList},
		},
	)

	// select: [list, atom] -> [table]  (select * from ...)
	// select: [list, list] -> [table]  (select [a, b] from ...)
	// Infix star handler: "from products select star" → args=[table, star]
	selectStarInfixHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		colSpec := args[1]

		_as0, _ := colSpec.AsAtom()
		if _as0 != "*" {
			_as1, _ := colSpec.AsAtom()
			return nil, fmt.Errorf("select: expected * or column list, got atom %q", _as1)
		}

		return doSelect(r, nil, table)
	}

	// Suffix star handler: "select star from products" → args=[star, table]
	selectStarForwardHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		colSpec := args[0]
		table := args[1]

		_as2, _ := colSpec.AsAtom()
		if _as2 != "*" {
			_as3, _ := colSpec.AsAtom()
			return nil, fmt.Errorf("select: expected * or column list, got atom %q", _as3)
		}

		return doSelect(r, nil, table)
	}

	selectColsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		colList := args[0]
		table := args[1]

		// Resolve any parenthesized sub-expressions (e.g. scalar subqueries).
		colList, err := resolveSelectSubExprs(r, colList)
		if err != nil {
			return nil, fmt.Errorf("select: %w", err)
		}

		cols, err := parseColumnSpec(colList)
		if err != nil {
			return nil, err
		}

		return doSelect(r, cols, table)
	}

	r.Register("select",
		// Suffix: "select star from ..." → [TAtom, TList]
		Signature{
			Args:    []Type{TAtom, TList},
			Handler: selectStarForwardHandler,
			Returns: []Type{TList},
		},
		// Infix: "from ... select star" → [TList, TAtom]
		Signature{
			Args:    []Type{TList, TAtom},
			Handler: selectStarInfixHandler,
			Returns: []Type{TList},
		},
		Signature{
			Args:    []Type{TList, TList},
			Handler: selectColsHandler,
			Returns: []Type{TList},
		},
	)

	// where: [condition(forward), table/query(prefix)] -> [query-builder]
	whereHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		condList := args[1]

		// Resolve any parenthesized sub-expressions (e.g. subqueries in IN).
		condList, err := resolveWhereSubExprs(r, condList)
		if err != nil {
			return nil, fmt.Errorf("where: %w", err)
		}

		clause, err := buildWhereClause(condList)
		if err != nil {
			return nil, fmt.Errorf("where: %w", err)
		}

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("where: %w", err)
		}
		qb.Where = clause
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register("where",
		Signature{
			Args:    []Type{TList, TList},
			Handler: whereHandler,
			Returns: []Type{TList},
		},
	)

	// order: [columns(forward), table/query(prefix)] -> [query-builder]
	orderListHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		colList := args[1]

		clause, err := buildOrderClause(colList)
		if err != nil {
			return nil, fmt.Errorf("order: %w", err)
		}

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("order: %w", err)
		}
		qb.OrderBy = clause
		return []Value{newValue(TList, qb)}, nil
	}

	orderAtomHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		col := args[1]

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("order: %w", err)
		}
		_as4, _ := col.AsAtom()
		qb.OrderBy = quoteIdent(_as4)
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register("order",
		Signature{
			Args:    []Type{TList, TList},
			Handler: orderListHandler,
			Returns: []Type{TList},
		},
		Signature{
			Args:    []Type{TList, TAtom},
			Handler: orderAtomHandler,
			Returns: []Type{TList},
		},
	)

	// by: [atom] -> [list], [list] -> [list]
	r.Register("by",
		Signature{
			Args: []Type{TAtom},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewList(args)}, nil
			},
			Returns: []Type{TList},
		},
		Signature{
			Args: []Type{TList},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return args, nil
			},
			Returns: []Type{TList},
		},
	)

	// limit: [table/query(prefix), integer(forward)] -> [query-builder]
	limitHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		n, _ := args[1].AsInteger()

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("limit: %w", err)
		}
		qb.Limit = int(n)
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register("limit",
		Signature{
			Args:    []Type{TList, TInteger},
			Handler: limitHandler,
			Returns: []Type{TList},
		},
	)

	// offset: [table/query(prefix), integer(forward)] -> [query-builder]
	offsetHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		n, _ := args[1].AsInteger()

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("offset: %w", err)
		}
		qb.Offset = int(n)
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register("offset",
		Signature{
			Args:    []Type{TList, TInteger},
			Handler: offsetHandler,
			Returns: []Type{TList},
		},
	)

	// distinct: [table/query(prefix)] -> [query-builder]
	distinctHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("distinct: %w", err)
		}
		qb.Distinct = true
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register("distinct",
		Signature{
			Args:    []Type{TList},
			Handler: distinctHandler,
			Returns: []Type{TList},
		},
	)

	// group: [columns(forward), table/query(prefix)] -> [query-builder]
	// Usage: from sales group by [region]
	//        from sales group by [region product]
	//        from sales group [region]
	groupListHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		colList := args[1]

		clause, err := buildGroupByClause(colList)
		if err != nil {
			return nil, fmt.Errorf("group: %w", err)
		}

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("group: %w", err)
		}
		qb.GroupBy = clause
		return []Value{newValue(TList, qb)}, nil
	}

	groupAtomHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		col := args[1]

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("group: %w", err)
		}
		_as5, _ := col.AsAtom()
		qb.GroupBy = quoteIdent(_as5)
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register("group",
		Signature{
			Args:    []Type{TList, TList},
			Handler: groupListHandler,
			Returns: []Type{TList},
		},
		Signature{
			Args:    []Type{TList, TAtom},
			Handler: groupAtomHandler,
			Returns: []Type{TList},
		},
	)

	// having: [condition(forward), table/query(prefix)] -> [query-builder]
	// Usage: from sales groupby [region] having [count gt 5]
	havingHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		condList := args[1]

		condList, err := resolveWhereSubExprs(r, condList)
		if err != nil {
			return nil, fmt.Errorf("having: %w", err)
		}

		clause, err := buildWhereClause(condList)
		if err != nil {
			return nil, fmt.Errorf("having: %w", err)
		}

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("having: %w", err)
		}
		qb.Having = clause
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register("having",
		Signature{
			Args:    []Type{TList, TList},
			Handler: havingHandler,
			Returns: []Type{TList},
		},
	)

	// join: [atom(forward), table/query(prefix)] -> [query-builder]
	// Usage: from orders join products on [...]
	RegisterJoinWord(r, "join", "JOIN")
	RegisterJoinWord(r, "innerjoin", "JOIN")
	RegisterJoinWord(r, "leftjoin", "LEFT JOIN")
	RegisterJoinWord(r, "crossjoin", "CROSS JOIN")

	// on: [condition(forward), table/query(prefix)] -> [query-builder]
	// Sets the ON condition for the most recent join.
	// Usage: from orders join products on [orders.product_id eq products.id]
	onHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		condList := args[1]

		clause, err := buildJoinCondition(condList)
		if err != nil {
			return nil, fmt.Errorf("on: %w", err)
		}

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("on: %w", err)
		}
		if len(qb.Joins) == 0 {
			return nil, fmt.Errorf("on: no preceding join")
		}
		qb.Joins[len(qb.Joins)-1].On = clause
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register("on",
		Signature{
			Args:    []Type{TList, TList},
			Handler: onHandler,
			Returns: []Type{TList},
		},
	)

	// using: [columns(forward), table/query(prefix)] -> [query-builder]
	// Usage: from orders join products using [id]
	usingHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		colList := args[1]

		elems := colList.AsList().Slice()
		cols := make([]string, 0, len(elems))
		for _, e := range elems {
			name := valueToColName(e)
			if name == "" {
				return nil, fmt.Errorf("using: expected column name, got %s", e.VType)
			}
			cols = append(cols, quoteIdent(name))
		}

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("using: %w", err)
		}
		if len(qb.Joins) == 0 {
			return nil, fmt.Errorf("using: no preceding join")
		}
		qb.Joins[len(qb.Joins)-1].UsingCols = strings.Join(cols, ", ")
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register("using",
		Signature{
			Args:    []Type{TList, TList},
			Handler: usingHandler,
			Returns: []Type{TList},
		},
	)

	// Set operations: union, unionall, intersect, except
	RegisterSetOpWord(r, "union", "UNION")
	RegisterSetOpWord(r, "unionall", "UNION ALL")
	RegisterSetOpWord(r, "intersect", "INTERSECT")
	RegisterSetOpWord(r, "except", "EXCEPT")

	// Aggregate functions (count, sum, avg, min, max) and CAST are handled
	// directly in parseColumnSpec when they appear as the first element of a
	// sub-list in the column spec, e.g.:
	//   select [[count name cnt]] from people
	//   select [[cast age integer]] from people
}

// RegisterJoinWord registers a join word (join, innerjoin, leftjoin, crossjoin).
func RegisterJoinWord(r *Registry, name string, joinType string) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		table := args[0]
		tableName, _ := args[1].AsAtom()

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		qb.Joins = append(qb.Joins, JoinClause{
			Type:  joinType,
			Table: tableName,
		})
		return []Value{newValue(TList, qb)}, nil
	}

	r.Register(name,
		Signature{
			Args:    []Type{TList, TAtom},
			Handler: handler,
			Returns: []Type{TList},
		},
	)
}

// RegisterSetOpWord registers a set operation word (union, unionall, intersect, except).
func RegisterSetOpWord(r *Registry, name string, op string) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		left := args[0]
		right := args[1]

		leftQB, err := toQueryBuilder(r, left)
		if err != nil {
			return nil, fmt.Errorf("%s: left operand: %w", name, err)
		}
		rightQB, err := toQueryBuilder(r, right)
		if err != nil {
			return nil, fmt.Errorf("%s: right operand: %w", name, err)
		}

		leftQB.SetOps = append(leftQB.SetOps, SetOp{
			Op:    op,
			Right: rightQB,
		})
		return []Value{newValue(TList, leftQB)}, nil
	}

	r.Register(name,
		Signature{
			Args:    []Type{TList, TList},
			Handler: handler,
			Returns: []Type{TList},
		},
	)
}

// aggregateFuncs is the set of recognized aggregate function names.
var aggregateFuncs = map[string]bool{
	"count": true,
	"sum":   true,
	"avg":   true,
	"min":   true,
	"max":   true,
}

// columnSpec describes a column selection, with optional alias and raw SQL.
type columnSpec struct {
	Name       string // column name (empty if Raw is set)
	Alias      string // empty means no alias
	Raw        string // raw SQL expression (for aggregates, cast, etc.)
	ResultType Type   // expected result type (zero value means inherit from source)
}

// parseColumnSpec parses the column list from a select word.
// Supports:
//   - [a, b]                     — plain columns
//   - [[a x] b]                  — column aliasing
//   - [[count name cnt]]         — aggregate with alias: COUNT("name") AS "cnt"
//   - [[count * total]]          — COUNT(*) AS "total"
//   - [[cast age integer]]       — CAST("age" AS INTEGER)
//   - [[cast age integer a]]     — CAST("age" AS INTEGER) AS "a"
func parseColumnSpec(colList Value) ([]columnSpec, error) {
	elems := colList.AsList().Slice()
	cols := make([]columnSpec, 0, len(elems))
	for _, e := range elems {
		switch {
		case e.VType.Equal(TAtom):
			_as6, _ := e.AsAtom()
			cols = append(cols, columnSpec{Name: _as6})
		case e.VType.Matches(TString):
			_as7, _ := e.AsString()
			cols = append(cols, columnSpec{Name: _as7})
		case e.IsWord():
			// A word that appears in the column list without evaluation
			// is treated as a column name OR as an aggregate function name.
			_as8, _ := e.AsWord()
			wname := _as8.Name
			cols = append(cols, columnSpec{Name: wname})
		case e.VType.Equal(TList):
			pair := e.AsList().Slice()
			if len(pair) < 2 {
				return nil, fmt.Errorf("select: column spec list must have at least 2 elements")
			}

			firstName := nameFromValue(pair[0])

			// Check for cast: [cast col type] or [cast col type alias]
			if firstName == "cast" {
				spec, err := parseCastSpec(pair)
				if err != nil {
					return nil, err
				}
				cols = append(cols, spec)
				continue
			}

			// Check for aggregate: [count col alias] or [sum col alias] etc.
			if aggregateFuncs[firstName] {
				spec, err := parseAggregateSpec(firstName, pair[1:])
				if err != nil {
					return nil, err
				}
				cols = append(cols, spec)
				continue
			}

			// Check for scalar subquery: [<TableData/QueryBuilder> alias]
			if isTableOrQuery(pair[0]) {
				scalar, err := resolveScalarValue(pair[0])
				if err != nil {
					return nil, fmt.Errorf("select scalar subquery: %w", err)
				}
				sqlVal, err := valueToSQL(scalar)
				if err != nil {
					return nil, fmt.Errorf("select scalar subquery: %w", err)
				}
				alias := ""
				if len(pair) >= 2 {
					alias = nameFromValue(pair[1])
				}
				cols = append(cols, columnSpec{Raw: sqlVal, Alias: alias})
				continue
			}

			// Standard [name alias] pair
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

// nameFromValue extracts a name from an atom, string, or word value.
// Unlike valueToColName, this also recognizes unevaluated word values.
func nameFromValue(v Value) string {
	if v.VType.Equal(TAtom) {
		_as9, _ := v.AsAtom()
		return _as9
	}
	if v.VType.Matches(TString) {
		_as10, _ := v.AsString()
		return _as10
	}
	if v.IsWord() {
		_as11, _ := v.AsWord()
		return _as11.Name
	}
	return ""
}

// parseAggregateSpec parses the arguments after the aggregate function name.
// remaining is the elements after the function name, e.g., for [count name cnt]
// remaining = [name, cnt].
func parseAggregateSpec(fnName string, remaining []Value) (columnSpec, error) {
	if len(remaining) == 0 || len(remaining) > 2 {
		return columnSpec{}, fmt.Errorf("%s: expected [%s col] or [%s col alias]", fnName, fnName, fnName)
	}

	fn := strings.ToUpper(fnName)
	col := nameFromValue(remaining[0])
	if col == "" {
		return columnSpec{}, fmt.Errorf("%s: expected column name", fnName)
	}

	var raw string
	if col == "*" {
		raw = fn + "(*)"
	} else {
		raw = fn + "(" + quoteIdent(col) + ")"
	}

	alias := strings.ToLower(fn) + "_" + col
	if len(remaining) == 2 {
		alias = nameFromValue(remaining[1])
	}

	return columnSpec{
		Raw:        raw,
		Alias:      alias,
		ResultType: TInteger,
	}, nil
}

// parseCastSpec parses [cast col type] or [cast col type alias] into a columnSpec.
func parseCastSpec(elems []Value) (columnSpec, error) {
	if len(elems) < 3 || len(elems) > 4 {
		return columnSpec{}, fmt.Errorf("cast: expected [cast column type] or [cast column type alias]")
	}
	col := valueToColName(elems[1])
	typeName := valueToColName(elems[2])
	if col == "" || typeName == "" {
		return columnSpec{}, fmt.Errorf("cast: column and type must be atoms or strings")
	}

	sqlType := aqlTypenameToSQLType(typeName)
	raw := "CAST(" + quoteIdent(col) + " AS " + sqlType + ")"

	alias := col
	if len(elems) == 4 {
		alias = valueToColName(elems[3])
	}

	return columnSpec{
		Raw:        raw,
		Alias:      alias,
		ResultType: sqlTypeToAQLType(sqlType),
	}, nil
}

// aqlTypenameToSQLType maps an AQL type name string to a SQL type.
func aqlTypenameToSQLType(name string) string {
	switch strings.ToLower(name) {
	case "integer", "int":
		return "INTEGER"
	case "real", "float", "number", "decimal":
		return "REAL"
	case "text", "string":
		return "TEXT"
	case "boolean", "bool":
		return "INTEGER" // SQLite stores booleans as integers
	default:
		return strings.ToUpper(name)
	}
}

// sqlTypeToAQLType maps a SQL type string back to an AQL Type.
func sqlTypeToAQLType(sqlType string) Type {
	switch sqlType {
	case "INTEGER":
		return TInteger
	case "REAL":
		return TDecimal
	case "TEXT":
		return TString
	default:
		return TString
	}
}

// valueToColName extracts the string content from an atom, string, or word value.
func valueToColName(v Value) string {
	if v.VType.Equal(TAtom) {
		_as12, _ := v.AsAtom()
		return _as12
	}
	if v.VType.Matches(TString) {
		_as13, _ := v.AsString()
		return _as13
	}
	if v.IsWord() {
		_as14, _ := v.AsWord()
		return _as14.Name
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

	return []Value{newValue(TList, result)}, nil
}

// comparisonOps maps AQL comparison word names to SQL operators.
var comparisonOps = map[string]string{
	"eq":     "=",
	"neq":    "!=",
	"lt":     "<",
	"gt":     ">",
	"lte":    "<=",
	"gte":    ">=",
	"like":   "LIKE",
	"glob":   "GLOB",
	"regexp": "REGEXP",
}

// logicalOps maps AQL logical word names to SQL connectors.
var logicalOps = map[string]string{
	"and": "AND",
	"or":  "OR",
}

// buildWhereClause translates a condition list into a SQL WHERE clause.
// Supported forms:
//
//	[column op value]                          — standard comparison
//	[column is null]                           — IS NULL
//	[column is not null]                       — IS NOT NULL
//	[column between value1 value2]             — BETWEEN ... AND ...
//
// resolveSelectSubExprs evaluates parenthesized sub-expressions in a
// SELECT column list, replacing them with their results. This enables
// scalar subqueries in the column list:
//
//	select [name [[(select [[max salary]] from emp) top_sal]]] from emp
//
// The inner list elements are scanned for paren tokens. When found, the
// sub-expression is evaluated and the result replaces the paren tokens.
func resolveSelectSubExprs(r *Registry, colList Value) (Value, error) {
	elems := colList.AsList().Slice()
	if len(elems) == 0 {
		return colList, nil
	}

	result := make([]Value, len(elems))
	for i, e := range elems {
		if e.VType.Equal(TList) {
			if _, isTD := e.Data.(TableData); !isTD {
				if _, isQB := e.Data.(QueryBuilder); !isQB {
					resolved, err := resolveSelectSubExprs(r, e)
					if err != nil {
						return Value{}, err
					}
					result[i] = resolved
					continue
				}
			}
		}
		result[i] = e
	}

	// Check for paren tokens at this level.
	hasParen := false
	for _, e := range result {
		_as15, _ := e.AsWord()
		if e.IsWord() && _as15.Name == "(" {
			hasParen = true
			break
		}
	}
	if !hasParen {
		return NewList(result), nil
	}

	// Find matching paren pairs and evaluate them.
	var out []Value
	idx := 0
	for idx < len(result) {
		_as16, _ := result[idx].AsWord()
		if result[idx].IsWord() && _as16.Name == "(" {
			depth := 1
			j := idx + 1
			for j < len(result) && depth > 0 {
				if result[j].IsWord() {
					wj, _ := result[j].AsWord()
					if wj.Name == "(" {
						depth++
					} else if wj.Name == ")" {
						depth--
					}
				}
				j++
			}
			if depth != 0 {
				return Value{}, fmt.Errorf("unmatched parenthesis in select column list")
			}
			subExpr := result[idx+1 : j-1]
			eng := New(r)
			evalResult, err := eng.Run(subExpr)
			if err != nil {
				return Value{}, fmt.Errorf("select subquery evaluation: %w", err)
			}
			out = append(out, evalResult...)
			idx = j
		} else {
			out = append(out, result[idx])
			idx++
		}
	}

	return NewList(out), nil
}

// resolveWhereSubExprs scans a WHERE condition list for parenthesized
// sub-expressions (sequences of tokens between "(" and ")") and evaluates
// them via the engine. The results replace the paren tokens in the list.
// This enables subquery support in IN clauses:
//
//	[city in (select [city] from cities)]
//
// The "(select [city] from cities)" tokens are evaluated, producing a
// TableData value that buildInList can extract values from.
func resolveWhereSubExprs(r *Registry, condList Value) (Value, error) {
	elems := condList.AsList().Slice()
	if len(elems) == 0 {
		return condList, nil
	}

	// Quick scan: any open-paren words?
	hasParen := false
	for _, e := range elems {
		_as19, _ := e.AsWord()
		if e.IsWord() && _as19.Name == "(" {
			hasParen = true
			break
		}
	}
	if !hasParen {
		// Recurse into nested lists.
		result := make([]Value, len(elems))
		for i, e := range elems {
			if e.VType.Equal(TList) {
				if _, isTD := e.Data.(TableData); !isTD {
					if _, isQB := e.Data.(QueryBuilder); !isQB {
						resolved, err := resolveWhereSubExprs(r, e)
						if err != nil {
							return Value{}, err
						}
						result[i] = resolved
						continue
					}
				}
			}
			result[i] = e
		}
		return NewList(result), nil
	}

	// Find matching paren pairs and evaluate them.
	var result []Value
	i := 0
	for i < len(elems) {
		_as20, _ := elems[i].AsWord()
		if elems[i].IsWord() && _as20.Name == "(" {
			// Find the matching close paren.
			depth := 1
			j := i + 1
			for j < len(elems) && depth > 0 {
				if elems[j].IsWord() {
					wj2, _ := elems[j].AsWord()
					if wj2.Name == "(" {
						depth++
					} else if wj2.Name == ")" {
						depth--
					}
				}
				j++
			}
			if depth != 0 {
				return Value{}, fmt.Errorf("unmatched parenthesis in condition")
			}
			// Evaluate the sub-expression (tokens between parens).
			subExpr := elems[i+1 : j-1]
			eng := New(r)
			evalResult, err := eng.Run(subExpr)
			if err != nil {
				return Value{}, fmt.Errorf("subquery evaluation: %w", err)
			}
			result = append(result, evalResult...)
			i = j
		} else {
			result = append(result, elems[i])
			i++
		}
	}

	return NewList(result), nil
}

// [column not between value1 value2]         — NOT BETWEEN ... AND ...
// [column in [v1 v2 v3]]                     — IN (v1, v2, v3)
// [column in (select [col] from table)]      — IN (subquery result)
// [column not in [v1 v2 v3]]                — NOT IN (v1, v2, v3)
// [... and/or ...]                           — logical connectives
func buildWhereClause(condList Value) (string, error) {
	elems := condList.AsList().Slice()
	if len(elems) == 0 {
		return "1=1", nil
	}

	var parts []string
	i := 0
	for i < len(elems) {
		// --- NOT prefix ---
		// [not col op val ...]  → NOT (col op val)
		// [not [sub-condition]] → NOT (sub-condition)
		firstCol := valueToColName(elems[i])
		if firstCol == "not" {
			i++
			if i >= len(elems) {
				return "", fmt.Errorf("incomplete condition: expected condition after not")
			}
			// If followed by a sub-list, negate the whole group.
			if elems[i].VType.Equal(TList) {
				inner, err := buildWhereClause(elems[i])
				if err != nil {
					return "", err
				}
				parts = append(parts, "NOT ("+inner+")")
				i++
				// Check for logical connector after the NOT group.
				if i < len(elems) {
					connName := valueToColName(elems[i])
					if sqlConn, ok := logicalOps[connName]; ok {
						parts = append(parts, sqlConn)
						i++
					}
				}
				continue
			}
			// Otherwise, NOT applies to the next single condition.
			// Collect tokens for the single condition: col op val
			// (or col not in [...], col is null, etc.)
			singleCond, consumed, err := parseSingleCondition(elems, i)
			if err != nil {
				return "", err
			}
			parts = append(parts, "NOT ("+singleCond+")")
			i = consumed
			// Check for logical connector.
			if i < len(elems) {
				connName := valueToColName(elems[i])
				if sqlConn, ok := logicalOps[connName]; ok {
					parts = append(parts, sqlConn)
					i++
				}
			}
			continue
		}

		// --- Sub-list (parenthesized group) ---
		// [[col op val or col op val] and ...]  → (...) AND ...
		if elems[i].VType.Equal(TList) {
			inner, err := buildWhereClause(elems[i])
			if err != nil {
				return "", err
			}
			parts = append(parts, "("+inner+")")
			i++
			// Check for logical connector after the group.
			if i < len(elems) {
				connName := valueToColName(elems[i])
				if sqlConn, ok := logicalOps[connName]; ok {
					parts = append(parts, sqlConn)
					i++
				}
			}
			continue
		}

		// --- Standard condition ---
		singleCond, consumed, err := parseSingleCondition(elems, i)
		if err != nil {
			return "", err
		}
		parts = append(parts, singleCond)
		i = consumed

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

// parseSingleCondition parses one condition starting at elems[start].
// Returns the SQL string and the new index after the condition.
func parseSingleCondition(elems []Value, start int) (string, int, error) {
	i := start
	col := valueToColName(elems[i])
	if col == "" {
		return "", i, fmt.Errorf("expected column name, got %s", elems[i].VType)
	}
	i++

	if i >= len(elems) {
		return "", i, fmt.Errorf("incomplete condition after column %q", col)
	}

	opName := valueToColName(elems[i])

	switch opName {
	case "is":
		// is null / is not null
		i++
		if i >= len(elems) {
			return "", i, fmt.Errorf("incomplete condition: expected null or not after is")
		}
		next := valueToColName(elems[i])
		if next == "null" {
			i++
			return fmt.Sprintf("%s IS NULL", quoteIdent(col)), i, nil
		} else if next == "not" {
			i++
			if i >= len(elems) {
				return "", i, fmt.Errorf("incomplete condition: expected null after is not")
			}
			nn := valueToColName(elems[i])
			if nn != "null" {
				return "", i, fmt.Errorf("expected null after is not, got %q", nn)
			}
			i++
			return fmt.Sprintf("%s IS NOT NULL", quoteIdent(col)), i, nil
		} else {
			return "", i, fmt.Errorf("expected null or not after is, got %q", next)
		}

	case "between":
		// between value1 value2
		i++
		if i+1 >= len(elems) {
			return "", i, fmt.Errorf("between requires two values")
		}
		lo, err := valueToSQL(elems[i])
		if err != nil {
			return "", i, err
		}
		i++
		hi, err := valueToSQL(elems[i])
		if err != nil {
			return "", i, err
		}
		i++
		return fmt.Sprintf("%s BETWEEN %s AND %s", quoteIdent(col), lo, hi), i, nil

	case "in":
		// in [v1 v2 v3]
		i++
		if i >= len(elems) {
			return "", i, fmt.Errorf("in requires a value list")
		}
		inSQL, err := buildInList(elems[i])
		if err != nil {
			return "", i, err
		}
		i++
		return fmt.Sprintf("%s IN (%s)", quoteIdent(col), inSQL), i, nil

	case "not":
		// not between / not in
		i++
		if i >= len(elems) {
			return "", i, fmt.Errorf("incomplete condition: expected between or in after not")
		}
		next := valueToColName(elems[i])
		switch next {
		case "between":
			i++
			if i+1 >= len(elems) {
				return "", i, fmt.Errorf("not between requires two values")
			}
			lo, err := valueToSQL(elems[i])
			if err != nil {
				return "", i, err
			}
			i++
			hi, err := valueToSQL(elems[i])
			if err != nil {
				return "", i, err
			}
			i++
			return fmt.Sprintf("%s NOT BETWEEN %s AND %s", quoteIdent(col), lo, hi), i, nil
		case "in":
			i++
			if i >= len(elems) {
				return "", i, fmt.Errorf("not in requires a value list")
			}
			inSQL, err := buildInList(elems[i])
			if err != nil {
				return "", i, err
			}
			i++
			return fmt.Sprintf("%s NOT IN (%s)", quoteIdent(col), inSQL), i, nil
		default:
			return "", i, fmt.Errorf("expected between or in after not, got %q", next)
		}

	default:
		// Standard comparison: op value [collate nocase|binary|rtrim]
		sqlOp, ok := comparisonOps[opName]
		if !ok {
			return "", i, fmt.Errorf("unknown comparison operator %q", opName)
		}
		i++
		if i >= len(elems) {
			return "", i, fmt.Errorf("incomplete condition: expected value after %q", opName)
		}
		val := elems[i]
		// Handle scalar subquery result (TableData or QueryBuilder).
		val, err := resolveScalarValue(val)
		if err != nil {
			return "", i, fmt.Errorf("where scalar subquery: %w", err)
		}
		sqlVal, err := valueToSQL(val)
		if err != nil {
			return "", i, err
		}
		i++

		// Optional COLLATE modifier.
		collateSuffix := ""
		if i < len(elems) {
			next := valueToColName(elems[i])
			if strings.ToLower(next) == "collate" {
				i++
				if i >= len(elems) {
					return "", i, fmt.Errorf("collate must be followed by nocase, binary, or rtrim")
				}
				cname := strings.ToLower(valueToColName(elems[i]))
				switch cname {
				case "nocase", "binary", "rtrim":
					collateSuffix = " COLLATE " + strings.ToUpper(cname)
					i++
				default:
					return "", i, fmt.Errorf("collate must be followed by nocase, binary, or rtrim, got %q", cname)
				}
			}
		}

		return fmt.Sprintf("%s %s %s%s", quoteIdent(col), sqlOp, sqlVal, collateSuffix), i, nil
	}
}

// buildInList converts a list value to a comma-separated SQL value list.
// If the value is a table result (from a subquery), extracts the first column values.
func buildInList(v Value) (string, error) {
	if !v.VType.Equal(TList) {
		// Single value
		sql, err := valueToSQL(v)
		if err != nil {
			return "", err
		}
		return sql, nil
	}

	// Check for subquery result: TableData or QueryBuilder.
	if td, ok := v.Data.(TableData); ok {
		return buildInListFromTable(td)
	}
	if qb, ok := v.Data.(QueryBuilder); ok {
		td, err := qb.Materialize()
		if err != nil {
			return "", fmt.Errorf("in subquery: %w", err)
		}
		return buildInListFromTable(td)
	}

	elems := v.AsList().Slice()
	if len(elems) == 0 {
		return "", fmt.Errorf("empty IN list")
	}
	parts := make([]string, len(elems))
	for i, e := range elems {
		sql, err := valueToSQL(e)
		if err != nil {
			return "", err
		}
		parts[i] = sql
	}
	return strings.Join(parts, ", "), nil
}

// buildInListFromTable extracts the first column values from a TableData
// and returns them as a comma-separated SQL value list for use in IN clauses.
func buildInListFromTable(td TableData) (string, error) {
	cols := td.Record.Fields.Keys()
	if len(cols) == 0 {
		return "", fmt.Errorf("in subquery: result has no columns")
	}
	firstCol := cols[0]

	if len(td.Rows) == 0 {
		// Empty subquery result — use a value that matches nothing.
		return "NULL", nil
	}

	parts := make([]string, 0, len(td.Rows))
	for _, row := range td.Rows {
		m := row.AsMap()
		val, ok := m.Get(firstCol)
		if !ok {
			continue
		}
		sql, err := valueToSQL(val)
		if err != nil {
			return "", fmt.Errorf("in subquery value: %w", err)
		}
		parts = append(parts, sql)
	}
	if len(parts) == 0 {
		return "NULL", nil
	}
	return strings.Join(parts, ", "), nil
}

// isTableOrQuery returns true if the Value wraps a TableData or QueryBuilder.
func isTableOrQuery(v Value) bool {
	if _, ok := v.Data.(TableData); ok {
		return true
	}
	if _, ok := v.Data.(QueryBuilder); ok {
		return true
	}
	return false
}

// scalarFromTable extracts a single scalar value from a TableData that has
// exactly one row. Used for scalar subquery results.
func scalarFromTable(td TableData) (Value, error) {
	cols := td.Record.Fields.Keys()
	if len(cols) == 0 {
		return Value{}, fmt.Errorf("scalar subquery returned no columns")
	}
	if len(td.Rows) == 0 {
		return NewTypeLiteral(TNone), nil
	}
	if len(td.Rows) > 1 {
		return Value{}, fmt.Errorf("scalar subquery returned %d rows, expected 1", len(td.Rows))
	}
	val, ok := td.Rows[0].AsMap().Get(cols[0])
	if !ok {
		return NewTypeLiteral(TNone), nil
	}
	return val, nil
}

// resolveScalarValue extracts a scalar value from a Value that may be a
// TableData or QueryBuilder (result of a scalar subquery).
func resolveScalarValue(v Value) (Value, error) {
	if td, ok := v.Data.(TableData); ok {
		return scalarFromTable(td)
	}
	if qb, ok := v.Data.(QueryBuilder); ok {
		td, err := qb.Materialize()
		if err != nil {
			return Value{}, fmt.Errorf("scalar subquery: %w", err)
		}
		return scalarFromTable(td)
	}
	return v, nil
}

// valueToSQL converts a Value to a SQL literal string.
func valueToSQL(v Value) (string, error) {
	switch {
	case v.VType.Matches(TString):
		_as23, _ := v.AsString()
		return "'" + strings.ReplaceAll(_as23, "'", "''") + "'", nil
	case v.VType.Matches(TInteger):
		_as24, _ := v.AsInteger()
		return fmt.Sprintf("%d", _as24), nil
	case v.VType.Matches(TBoolean):
		_as25, _ := v.AsBoolean()
		if _as25 {
			return "'true'", nil
		}
		return "'false'", nil
	case v.VType.Equal(TAtom):
		_as26, _ := v.AsAtom()
		return "'" + strings.ReplaceAll(_as26, "'", "''") + "'", nil
	case v.VType.Equal(TNone):
		return "NULL", nil
	case v.VType.Equal(TWord):
		_as27, _ := v.AsWord()
		return "'" + strings.ReplaceAll(_as27.Name, "'", "''") + "'", nil
	default:
		return "", fmt.Errorf("unsupported value type in condition: %s", v.VType)
	}
}

// buildGroupByClause translates a column list into a SQL GROUP BY clause.
func buildGroupByClause(colList Value) (string, error) {
	elems := colList.AsList().Slice()
	if len(elems) == 0 {
		return "", fmt.Errorf("empty group by column list")
	}
	parts := make([]string, 0, len(elems))
	for _, e := range elems {
		name := valueToColName(e)
		if name == "" {
			return "", fmt.Errorf("groupby: expected column name, got %s", e.VType)
		}
		parts = append(parts, quoteIdent(name))
	}
	return strings.Join(parts, ", "), nil
}

// buildJoinCondition translates a condition list into a SQL ON clause.
// Supports dot-separated qualified names: [a.id eq b.id]
func buildJoinCondition(condList Value) (string, error) {
	elems := condList.AsList().Slice()
	if len(elems) == 0 {
		return "1=1", nil
	}

	var parts []string
	i := 0
	for i < len(elems) {
		lhs := valueToColName(elems[i])
		if lhs == "" {
			return "", fmt.Errorf("expected column name, got %s", elems[i].VType)
		}
		i++

		if i >= len(elems) {
			return "", fmt.Errorf("incomplete join condition after %q", lhs)
		}

		opName := valueToColName(elems[i])
		sqlOp, ok := comparisonOps[opName]
		if !ok {
			return "", fmt.Errorf("unknown comparison operator %q in join condition", opName)
		}
		i++

		if i >= len(elems) {
			return "", fmt.Errorf("incomplete join condition: expected value after %q", opName)
		}

		rhs := valueToColName(elems[i])
		if rhs == "" {
			return "", fmt.Errorf("expected column name on right side of join condition, got %s", elems[i].VType)
		}
		i++

		parts = append(parts, fmt.Sprintf("%s %s %s", quoteJoinCol(lhs), sqlOp, quoteJoinCol(rhs)))

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

// quoteJoinCol quotes a column reference that may be dot-qualified (table.col).
func quoteJoinCol(name string) string {
	if idx := strings.IndexByte(name, '.'); idx >= 0 {
		return quoteIdent(name[:idx]) + "." + quoteIdent(name[idx+1:])
	}
	return quoteIdent(name)
}

// buildOrderClause translates a column list into a SQL ORDER BY clause.
// Supports:
//   - [col1, col2]                   — multiple columns
//   - [col1 asc, col2 desc]          — with direction
//   - [col1 asc nulls first]         — with nulls placement
//   - [1, 2 desc]                    — positional (1-based)
func buildOrderClause(colList Value) (string, error) {
	elems := colList.AsList().Slice()
	if len(elems) == 0 {
		return "", fmt.Errorf("empty order column list")
	}

	isModifier := func(name string) (string, bool) {
		switch name {
		case "asc":
			return "ASC", true
		case "desc":
			return "DESC", true
		case "nulls":
			return "NULLS", true
		case "first":
			return "FIRST", true
		case "last":
			return "LAST", true
		case "collate":
			return "COLLATE", true
		default:
			return "", false
		}
	}

	collations := map[string]string{
		"nocase": "NOCASE",
		"binary": "BINARY",
		"rtrim":  "RTRIM",
	}

	var parts []string
	i := 0
	for i < len(elems) {
		e := elems[i]

		if e.VType.Matches(TInteger) {
			_as28, _ := e.AsInteger()
			parts = append(parts, fmt.Sprintf("%d", _as28))
			i++
			continue
		}

		name := valueToColName(e)
		if name == "" {
			return "", fmt.Errorf("expected column name, position, or modifier, got %s", e.VType)
		}
		lower := strings.ToLower(name)

		if sql, ok := isModifier(lower); ok {
			if len(parts) == 0 {
				return "", fmt.Errorf("%s without preceding column name", lower)
			}
			if lower == "nulls" {
				i++
				if i >= len(elems) {
					return "", fmt.Errorf("nulls must be followed by first or last")
				}
				next := strings.ToLower(valueToColName(elems[i]))
				if next != "first" && next != "last" {
					return "", fmt.Errorf("nulls must be followed by first or last, got %q", next)
				}
				parts[len(parts)-1] += " " + sql + " " + strings.ToUpper(next)
			} else if lower == "collate" {
				i++
				if i >= len(elems) {
					return "", fmt.Errorf("collate must be followed by nocase, binary, or rtrim")
				}
				next := strings.ToLower(valueToColName(elems[i]))
				colSQL, ok := collations[next]
				if !ok {
					return "", fmt.Errorf("collate must be followed by nocase, binary, or rtrim, got %q", next)
				}
				parts[len(parts)-1] += " " + sql + " " + colSQL
			} else {
				parts[len(parts)-1] += " " + sql
			}
		} else {
			parts = append(parts, quoteIdent(name))
		}
		i++
	}

	return strings.Join(parts, ", "), nil
}
