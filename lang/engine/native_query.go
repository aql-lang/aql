package engine

import (
	"fmt"
	"strings"
)

// queryNatives covers the SQL-style query DSL words: star, from, as,
// select, where, order, by, limit, offset, distinct, group, having,
// on, using, the four join words (join, innerjoin, leftjoin,
// crossjoin), and the four set-operation words (union, unionall,
// intersect, except).
//
// All supporting parser/exec helpers (toQueryBuilder,
// buildWhereClause, parseColumnSpec, etc.) stay in query.go.
var queryNatives = []NativeFunc{
	{
		Name: "star",
		// star is stack-only — emits NewAtom("*") with no args.
		Signatures: []NativeSig{{
			Handler: starHandler,
			Returns: []*Type{TAtom},
		}},
	},
	{
		Name:        "from",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAtom},
			Handler: fromHandler,
			Returns: []*Type{TList},
		}},
	},
	{
		Name:        "as",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList, TAtom},
			Handler: asHandler,
			Returns: []*Type{TList},
		}},
	},
	{
		Name:        "select",
		ForwardArgs: true,
		Signatures: []NativeSig{
			// Suffix: "select star from ..." → [TAtom, TList]
			{
				Args:    []*Type{TAtom, TList},
				Handler: selectStarForwardHandler,
				Returns: []*Type{TList},
			},
			// Infix: "from ... select star" → [TList, TAtom]
			{
				Args:    []*Type{TList, TAtom},
				Handler: selectStarInfixHandler,
				Returns: []*Type{TList},
			},
			{
				Args:    []*Type{TList, TList},
				Handler: selectColsHandler,
				Returns: []*Type{TList},
			},
		},
	},
	{
		Name:        "where",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList, TList},
			Handler: queryWhereHandler,
			Returns: []*Type{TList},
		}},
	},
	{
		Name:        "order",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args:    []*Type{TList, TList},
				Handler: orderListHandler,
				Returns: []*Type{TList},
			},
			{
				Args:    []*Type{TList, TAtom},
				Handler: orderAtomHandler,
				Returns: []*Type{TList},
			},
		},
	},
	{
		Name:        "by",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args:    []*Type{TAtom},
				Handler: byAtomHandler,
				Returns: []*Type{TList},
			},
			{
				Args:    []*Type{TList},
				Handler: byListHandler,
				Returns: []*Type{TList},
			},
		},
	},
	{
		Name:        "limit",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList, TInteger},
			Handler: limitHandler,
			Returns: []*Type{TList},
		}},
	},
	{
		Name:        "offset",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList, TInteger},
			Handler: offsetHandler,
			Returns: []*Type{TList},
		}},
	},
	{
		Name:        "distinct",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList},
			Handler: distinctHandler,
			Returns: []*Type{TList},
		}},
	},
	{
		Name:        "group",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{
				Args:    []*Type{TList, TList},
				Handler: groupListHandler,
				Returns: []*Type{TList},
			},
			{
				Args:    []*Type{TList, TAtom},
				Handler: groupAtomHandler,
				Returns: []*Type{TList},
			},
		},
	},
	{
		Name:        "having",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList, TList},
			Handler: havingHandler,
			Returns: []*Type{TList},
		}},
	},
	{
		Name:        "on",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList, TList},
			Handler: onHandler,
			Returns: []*Type{TList},
		}},
	},
	{
		Name:        "using",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList, TList},
			Handler: usingHandler,
			Returns: []*Type{TList},
		}},
	},

	// Join words — share joinWordNative builder.
	joinWordNative("join", "JOIN"),
	joinWordNative("innerjoin", "JOIN"),
	joinWordNative("leftjoin", "LEFT JOIN"),
	joinWordNative("crossjoin", "CROSS JOIN"),

	// Set operations — share setOpWordNative builder.
	setOpWordNative("union", "UNION"),
	setOpWordNative("unionall", "UNION ALL"),
	setOpWordNative("intersect", "INTERSECT"),
	setOpWordNative("except", "EXCEPT"),
}

// ---- handlers ----

func starHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewAtom("*")}, nil
}

func fromHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name, _ := args[0].AsConcreteAtom()
	val, ok := ContextStoreLookup(r, name)
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
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

// as: [table/query(prefix), atom(forward)] -> [query-builder with alias]
// Usage: from people as p
func asHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	table := args[0]
	alias, _ := args[1].AsConcreteAtom()

	qb, err := toQueryBuilder(r, table)
	if err != nil {
		return nil, fmt.Errorf("as: %w", err)
	}
	qb.Alias = alias
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

// Infix star handler: "from products select star" → args=[table, star]
func selectStarInfixHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	table := args[0]
	colSpec := args[1]

	_as0, _ := AsAtom(colSpec)
	if _as0 != "*" {
		_as1, _ := AsAtom(colSpec)
		return nil, fmt.Errorf("select: expected * or column list, got atom %q", _as1)
	}

	return doSelect(r, nil, table)
}

// Suffix star handler: "select star from products" → args=[star, table]
func selectStarForwardHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	colSpec := args[0]
	table := args[1]

	_as2, _ := AsAtom(colSpec)
	if _as2 != "*" {
		_as3, _ := AsAtom(colSpec)
		return nil, fmt.Errorf("select: expected * or column list, got atom %q", _as3)
	}

	return doSelect(r, nil, table)
}

func selectColsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
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

// where: [condition(forward), table/query(prefix)] -> [query-builder]
func queryWhereHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
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
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

func orderListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
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
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

func orderAtomHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	table := args[0]
	col := args[1]

	qb, err := toQueryBuilder(r, table)
	if err != nil {
		return nil, fmt.Errorf("order: %w", err)
	}
	_as4, _ := AsAtom(col)
	qb.OrderBy = quoteIdent(_as4)
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

func byAtomHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewList(args)}, nil
}

func byListHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return args, nil
}

// limit: [table/query(prefix), integer(forward)] -> [query-builder]
func limitHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	table := args[0]
	n, _ := args[1].AsConcreteInteger()

	qb, err := toQueryBuilder(r, table)
	if err != nil {
		return nil, fmt.Errorf("limit: %w", err)
	}
	qb.Limit = int(n)
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

// offset: [table/query(prefix), integer(forward)] -> [query-builder]
func offsetHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	table := args[0]
	n, _ := args[1].AsConcreteInteger()

	qb, err := toQueryBuilder(r, table)
	if err != nil {
		return nil, fmt.Errorf("offset: %w", err)
	}
	qb.Offset = int(n)
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

// distinct: [table/query(prefix)] -> [query-builder]
func distinctHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	table := args[0]

	qb, err := toQueryBuilder(r, table)
	if err != nil {
		return nil, fmt.Errorf("distinct: %w", err)
	}
	qb.Distinct = true
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

// group: [columns(forward), table/query(prefix)] -> [query-builder]
// Usage: from sales group by [region]
//
//	from sales group by [region product]
//	from sales group [region]
func groupListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
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
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

func groupAtomHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	table := args[0]
	col := args[1]

	qb, err := toQueryBuilder(r, table)
	if err != nil {
		return nil, fmt.Errorf("group: %w", err)
	}
	_as5, _ := AsAtom(col)
	qb.GroupBy = quoteIdent(_as5)
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

// having: [condition(forward), table/query(prefix)] -> [query-builder]
// Usage: from sales groupby [region] having [count gt 5]
func havingHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
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
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

// on: [condition(forward), table/query(prefix)] -> [query-builder]
// Sets the ON condition for the most recent join.
func onHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
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
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

// using: [columns(forward), table/query(prefix)] -> [query-builder]
// Usage: from orders join products using [id]
func usingHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
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
	return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
}

// joinWordNative builds a NativeFunc for join/innerjoin/leftjoin/crossjoin.
// The handler captures the AQL word name (for error messages) and the
// SQL join type ("JOIN", "LEFT JOIN", "CROSS JOIN").
func joinWordNative(name, joinType string) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		table := args[0]
		tableName, _ := args[1].AsConcreteAtom()

		qb, err := toQueryBuilder(r, table)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		qb.Joins = append(qb.Joins, JoinClause{
			Type:  joinType,
			Table: tableName,
		})
		return []Value{NewValueRaw(TList, ExtensionPayload{Body: qb})}, nil
	}

	return NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList, TAtom},
			Handler: handler,
			Returns: []*Type{TList},
		}},
	}
}

// setOpWordNative builds a NativeFunc for union/unionall/intersect/except.
// The handler captures the AQL word name (for error messages) and the
// SQL operator ("UNION", "UNION ALL", "INTERSECT", "EXCEPT").
func setOpWordNative(name, op string) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
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
		return []Value{NewValueRaw(TList, ExtensionPayload{Body: leftQB})}, nil
	}

	return NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TList, TList},
			Handler: handler,
			Returns: []*Type{TList},
		}},
	}
}
