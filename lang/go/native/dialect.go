package native

import (
	"fmt"
	"strings"
)

// Dialect abstracts the RDBMS-specific parts of SQL generation and
// type mapping so the query builder (query.go) can target multiple
// backends. SQLite is the embedded default (SQLiteDialect); external
// backends (Postgres, MySQL, SQL Server) supply their own dialects.
//
// Phase 0 surface: everything the existing SQLite query path needs.
// The connection/introspection methods (placeholders for parameterized
// queries, information_schema introspection, driver result coercion)
// are added in later phases as the external connection layer lands.
type Dialect interface {
	// Name is the short dialect identifier ("sqlite", "postgres",
	// "mysql", "sqlserver"). Used in diagnostics.
	Name() string

	// QuoteIdent quotes a SQL identifier (table/column name) for this
	// dialect — backticks for SQLite/MySQL, double quotes for
	// Postgres, square brackets for SQL Server.
	QuoteIdent(name string) string

	// Placeholder renders the n-th (1-based) bind placeholder: "?" for
	// SQLite/MySQL, "$1" for Postgres, "@p1" for SQL Server.
	Placeholder(n int) string

	// LimitOffset renders the trailing LIMIT/OFFSET clause (including a
	// leading space) for the given limit and offset. A negative value
	// means "unset". Dialects diverge sharply here — SQLite needs a
	// bare "LIMIT -1" to carry an offset, SQL Server uses
	// OFFSET..FETCH and requires an ORDER BY (handled in the SQL Server
	// dialect via hasOrderBy).
	LimitOffset(limit, offset int, hasOrderBy bool) string

	// AQLTypeToColumnType maps an AQL field type to a CREATE TABLE
	// column type for this dialect.
	AQLTypeToColumnType(t *Type) string

	// AQLTypenameToColumnType maps an AQL type *name* (as written in a
	// `cast` clause: "integer", "text", …) to a column type for CAST.
	AQLTypenameToColumnType(name string) string

	// ColumnTypeToAQLType maps a SQL column type string (as reported by
	// a CAST target or driver) back to an AQL *Type.
	ColumnTypeToAQLType(sqlType string) *Type

	// BooleanLiteral renders a boolean value as a SQL literal for this
	// dialect (SQLite stores booleans as the strings 'true'/'false';
	// Postgres/SQL Server use TRUE/FALSE or 1/0).
	BooleanLiteral(b bool) string

	// ComparisonOperator returns the SQL operator for an AQL comparison
	// word (eq/neq/lt/gt/lte/gte/like/glob/regexp) and false when this
	// dialect cannot express it (e.g. GLOB outside SQLite, REGEXP on
	// SQL Server, or Postgres's `~` for regexp).
	ComparisonOperator(name string) (string, bool)

	// Caps reports the optional-feature support for this dialect.
	Caps() DialectCaps
}

// DialectCaps reports which optional SQL features a dialect supports,
// so the query builder can reject (or rewrite) clauses a target cannot
// express rather than emitting invalid SQL.
type DialectCaps struct {
	// Collations maps a lowercased AQL collation name (nocase / binary
	// / rtrim) to the dialect's COLLATE name, or is absent when the
	// dialect does not support that collation.
	Collations map[string]string
}

// defaultDialect is the embedded SQLite dialect used wherever a
// QueryBuilder has not been bound to an external connection. Keeping it
// as a single shared value means every existing local-query path
// behaves exactly as before this abstraction was introduced.
func defaultDialect() Dialect { return SQLiteDialect{} }

// sqliteCollations is the COLLATE set SQLite understands. Shared by the
// where-clause and order-clause builders via Caps().
var sqliteCollations = map[string]string{
	"nocase": "NOCASE",
	"binary": "BINARY",
	"rtrim":  "RTRIM",
}

// SQLiteDialect is the embedded-engine dialect. It wraps the existing
// SQLite-specific helpers verbatim so output is byte-identical to the
// pre-Dialect query path.
type SQLiteDialect struct{}

func (SQLiteDialect) Name() string { return "sqlite" }

func (SQLiteDialect) QuoteIdent(name string) string { return quoteIdent(name) }

// Placeholder for SQLite is the positional "?" regardless of index.
func (SQLiteDialect) Placeholder(int) string { return "?" }

// LimitOffset replicates the historical SQLite rendering: an offset
// without a limit must ride on a bare "LIMIT -1".
func (SQLiteDialect) LimitOffset(limit, offset int, _ bool) string {
	var b strings.Builder
	if limit >= 0 {
		fmt.Fprintf(&b, " LIMIT %d", limit)
	} else if offset >= 0 {
		b.WriteString(" LIMIT -1")
	}
	if offset >= 0 {
		fmt.Fprintf(&b, " OFFSET %d", offset)
	}
	return b.String()
}

func (SQLiteDialect) AQLTypeToColumnType(t *Type) string { return aqlTypeToSQLType(t) }

func (SQLiteDialect) AQLTypenameToColumnType(name string) string {
	return aqlTypenameToSQLType(name)
}

func (SQLiteDialect) ColumnTypeToAQLType(sqlType string) *Type {
	return sqlTypeToAQLType(sqlType)
}

// BooleanLiteral matches the legacy valueToSQL behaviour: SQLite stores
// booleans as the quoted strings 'true' / 'false'.
func (SQLiteDialect) BooleanLiteral(b bool) string {
	if b {
		return "'true'"
	}
	return "'false'"
}

// ComparisonOperator for SQLite supports the full operator set,
// including the SQLite-only GLOB and REGEXP.
func (SQLiteDialect) ComparisonOperator(name string) (string, bool) {
	op, ok := comparisonOps[name]
	return op, ok
}

func (SQLiteDialect) Caps() DialectCaps {
	return DialectCaps{Collations: sqliteCollations}
}

// columnTypeFor maps an AQL field type to a dialect column type using
// the same conformance order as the legacy SQLite mapper (Integer
// before the wider Number). Shared by every dialect's
// AQLTypeToColumnType.
func columnTypeFor(t *Type, integer, float, number, boolean, text string) string {
	switch {
	case t.ConformsTo(TInteger):
		return integer
	case t.ConformsTo(TFloat):
		return float
	case t.ConformsTo(TNumber):
		return number
	case t.ConformsTo(TBoolean):
		return boolean
	default:
		return text
	}
}

// columnTypeForName maps an AQL type *name* (as written in a `cast`
// clause) to a dialect column/CAST type. An unrecognised name passes
// through upper-cased, matching the legacy behaviour. Shared by every
// dialect's AQLTypenameToColumnType.
func columnTypeForName(name, integer, float, text, boolean string) string {
	switch strings.ToLower(name) {
	case "integer", "int":
		return integer
	case "real", "float", "number", "decimal":
		return float
	case "text", "string":
		return text
	case "boolean", "bool":
		return boolean
	default:
		return strings.ToUpper(name)
	}
}

// normalizeSQLType strips any size/precision suffix from a SQL type
// name and upper-cases it, so driver-reported names like "varchar(255)"
// or "NUMERIC(10,2)" map cleanly. Shared by every dialect's
// ColumnTypeToAQLType.
func normalizeSQLType(s string) string {
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = s[:i]
	}
	return strings.ToUpper(strings.TrimSpace(s))
}
