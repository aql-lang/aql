package native

import (
	"fmt"
	"strconv"
	"strings"
)

// MSSQLDialect targets Microsoft SQL Server (2012+). Identifiers use
// square brackets, placeholders are "@pN", and paging uses
// OFFSET..FETCH — which requires an ORDER BY, synthesized when the
// query has none. SQL Server has neither GLOB nor REGEXP.
type MSSQLDialect struct{}

func (MSSQLDialect) Name() string { return "sqlserver" }

func (MSSQLDialect) QuoteIdent(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

func (MSSQLDialect) Placeholder(n int) string { return "@p" + strconv.Itoa(n) }

// LimitOffset renders OFFSET..FETCH. That construct requires an ORDER
// BY, so when the query has none we synthesize a stable no-op ordering.
// OFFSET is mandatory whenever FETCH is present, so a limit-only query
// still emits "OFFSET 0 ROWS".
func (MSSQLDialect) LimitOffset(limit, offset int, hasOrderBy bool) string {
	if limit < 0 && offset < 0 {
		return ""
	}
	var b strings.Builder
	if !hasOrderBy {
		b.WriteString(" ORDER BY (SELECT NULL)")
	}
	off := offset
	if off < 0 {
		off = 0
	}
	fmt.Fprintf(&b, " OFFSET %d ROWS", off)
	if limit >= 0 {
		fmt.Fprintf(&b, " FETCH NEXT %d ROWS ONLY", limit)
	}
	return b.String()
}

func (MSSQLDialect) AQLTypeToColumnType(t *Type) string {
	return columnTypeFor(t, "BIGINT", "FLOAT", "FLOAT", "BIT", "NVARCHAR(MAX)")
}

func (MSSQLDialect) AQLTypenameToColumnType(name string) string {
	return columnTypeForName(name, "BIGINT", "FLOAT", "NVARCHAR(MAX)", "BIT")
}

func (MSSQLDialect) ColumnTypeToAQLType(sqlType string) *Type {
	switch normalizeSQLType(sqlType) {
	case "BIGINT", "INT", "SMALLINT", "TINYINT":
		return TInteger
	case "FLOAT", "REAL", "DECIMAL", "NUMERIC", "MONEY", "SMALLMONEY":
		return TFloat
	case "BIT":
		return TBoolean
	default:
		return TString
	}
}

// BooleanLiteral: SQL Server has no boolean literals; BIT uses 1/0.
func (MSSQLDialect) BooleanLiteral(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// ComparisonOperator: SQL Server supports LIKE but neither GLOB nor
// REGEXP.
func (MSSQLDialect) ComparisonOperator(name string) (string, bool) {
	switch name {
	case "glob", "regexp":
		return "", false
	default:
		op, ok := comparisonOps[name]
		return op, ok
	}
}

// Caps: SQL Server COLLATE names (SQL_Latin1_General_*) do not map to
// SQLite's nocase/binary/rtrim.
func (MSSQLDialect) Caps() DialectCaps { return DialectCaps{} }

func (MSSQLDialect) IntrospectTablesSQL() string {
	return "SELECT table_name FROM information_schema.tables " +
		"WHERE table_type = 'BASE TABLE' ORDER BY table_name"
}

func (MSSQLDialect) IntrospectColumnsSQL(table string) string {
	return "SELECT column_name AS name, data_type AS type FROM information_schema.columns " +
		"WHERE table_name = '" + escapeSQLLiteral(table) + "' ORDER BY ordinal_position"
}
