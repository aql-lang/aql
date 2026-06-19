package native

import "fmt"

// MySQLDialect targets MySQL and MariaDB. Identifiers are backtick-
// quoted (same as SQLite), placeholders are positional "?", and the
// CAST target types differ from CREATE TABLE column types (MySQL only
// allows a restricted set in CAST).
type MySQLDialect struct{}

func (MySQLDialect) Name() string { return "mysql" }

// QuoteIdent reuses the backtick quoter — MySQL and SQLite share it.
func (MySQLDialect) QuoteIdent(name string) string { return quoteIdent(name) }

func (MySQLDialect) Placeholder(int) string { return "?" }

// LimitOffset: MySQL requires a LIMIT before an OFFSET. An offset with
// no limit uses the documented "very large LIMIT" idiom.
func (MySQLDialect) LimitOffset(limit, offset int, _ bool) string {
	switch {
	case limit < 0 && offset < 0:
		return ""
	case offset < 0:
		return fmt.Sprintf(" LIMIT %d", limit)
	case limit < 0:
		// Offset without limit: MySQL needs a limit, so use the max.
		return fmt.Sprintf(" LIMIT 18446744073709551615 OFFSET %d", offset)
	default:
		return fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}
}

func (MySQLDialect) AQLTypeToColumnType(t *Type) string {
	return columnTypeFor(t, "BIGINT", "DOUBLE", "DOUBLE", "TINYINT(1)", "TEXT")
}

// AQLTypenameToColumnType maps to MySQL CAST target types, which are a
// restricted set distinct from column types (e.g. CAST AS SIGNED, not
// CAST AS INTEGER; CAST AS CHAR, not CAST AS TEXT).
func (MySQLDialect) AQLTypenameToColumnType(name string) string {
	return columnTypeForName(name, "SIGNED", "DOUBLE", "CHAR", "SIGNED")
}

func (MySQLDialect) ColumnTypeToAQLType(sqlType string) *Type {
	switch normalizeSQLType(sqlType) {
	// BIGINT… are driver column types; SIGNED/UNSIGNED are the
	// integer CAST targets, recognized so cast result types resolve.
	case "BIGINT", "INT", "INTEGER", "SMALLINT", "TINYINT", "MEDIUMINT", "YEAR", "SIGNED", "UNSIGNED":
		return TInteger
	case "DOUBLE", "FLOAT", "DECIMAL", "NEWDECIMAL", "REAL":
		return TFloat
	case "BOOL", "BOOLEAN":
		return TBoolean
	default:
		return TString
	}
}

func (MySQLDialect) BooleanLiteral(b bool) string {
	if b {
		return "TRUE"
	}
	return "FALSE"
}

// ComparisonOperator: MySQL has the REGEXP keyword but no GLOB.
func (MySQLDialect) ComparisonOperator(name string) (string, bool) {
	switch name {
	case "regexp":
		return "REGEXP", true
	case "glob":
		return "", false
	default:
		op, ok := comparisonOps[name]
		return op, ok
	}
}

// Caps: MySQL COLLATE names (utf8mb4_*) do not map to SQLite's
// nocase/binary/rtrim.
func (MySQLDialect) Caps() DialectCaps { return DialectCaps{} }

func (MySQLDialect) IntrospectTablesSQL() string {
	return "SELECT table_name FROM information_schema.tables " +
		"WHERE table_schema = DATABASE() ORDER BY table_name"
}

func (MySQLDialect) IntrospectColumnsSQL(table string) string {
	return "SELECT column_name AS name, data_type AS type FROM information_schema.columns " +
		"WHERE table_schema = DATABASE() AND table_name = '" + escapeSQLLiteral(table) + "' ORDER BY ordinal_position"
}
