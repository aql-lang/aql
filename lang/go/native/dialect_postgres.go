package native

import (
	"fmt"
	"strconv"
	"strings"
)

// PostgresDialect targets PostgreSQL (and wire-compatible engines).
// Identifiers are double-quoted, bind placeholders are positional
// ($1, $2, …), LIMIT/OFFSET are independent, and regexp is the `~`
// operator.
type PostgresDialect struct{}

func (PostgresDialect) Name() string { return "postgres" }

func (PostgresDialect) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (PostgresDialect) Placeholder(n int) string { return "$" + strconv.Itoa(n) }

// LimitOffset uses the standard, independent LIMIT and OFFSET clauses —
// Postgres accepts a bare OFFSET with no LIMIT.
func (PostgresDialect) LimitOffset(limit, offset int, _ bool) string {
	var b strings.Builder
	if limit >= 0 {
		fmt.Fprintf(&b, " LIMIT %d", limit)
	}
	if offset >= 0 {
		fmt.Fprintf(&b, " OFFSET %d", offset)
	}
	return b.String()
}

func (PostgresDialect) AQLTypeToColumnType(t *Type) string {
	return columnTypeFor(t, "BIGINT", "DOUBLE PRECISION", "DOUBLE PRECISION", "BOOLEAN", "TEXT")
}

func (PostgresDialect) AQLTypenameToColumnType(name string) string {
	return columnTypeForName(name, "BIGINT", "DOUBLE PRECISION", "TEXT", "BOOLEAN")
}

func (PostgresDialect) ColumnTypeToAQLType(sqlType string) *Type {
	switch normalizeSQLType(sqlType) {
	case "INT8", "BIGINT", "INT4", "INTEGER", "INT", "INT2", "SMALLINT", "SERIAL", "BIGSERIAL":
		return TInteger
	case "FLOAT8", "DOUBLE PRECISION", "FLOAT4", "REAL", "NUMERIC", "DECIMAL", "MONEY":
		return TFloat
	case "BOOL", "BOOLEAN":
		return TBoolean
	default:
		return TString
	}
}

func (PostgresDialect) BooleanLiteral(b bool) string {
	if b {
		return "TRUE"
	}
	return "FALSE"
}

// ComparisonOperator renders regexp as Postgres's `~` operator and has
// no GLOB.
func (PostgresDialect) ComparisonOperator(name string) (string, bool) {
	switch name {
	case "regexp":
		return "~", true
	case "glob":
		return "", false
	default:
		op, ok := comparisonOps[name]
		return op, ok
	}
}

// Caps: Postgres COLLATE names (ICU/libc locales) do not correspond to
// SQLite's nocase/binary/rtrim, so no AQL collation is offered.
func (PostgresDialect) Caps() DialectCaps { return DialectCaps{} }
