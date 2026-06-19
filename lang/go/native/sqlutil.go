package native

import "strings"

// quoteIdent quotes a SQL identifier with backticks. Backticks rather
// than the SQL-standard double quotes because of SQLite's legacy
// double-quoted-string fallback: a double-quoted name that matches no
// column is silently reinterpreted as a string literal, so a typo'd
// column in a where clause matches every row instead of erroring.
// Backtick-quoted names are always identifiers — an unknown column is
// a hard "no such column" error.
func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// joinQuoted joins identifiers with commas, each quoted.
func joinQuoted(names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = quoteIdent(n)
	}
	return strings.Join(parts, ", ")
}

// firstColumnStrings returns the first column of every row in td as a
// string slice. Used by ListTables-style introspection queries that
// project a single name column.
func firstColumnStrings(td TableData) []string {
	cols := td.Record.Fields.Keys()
	if len(cols) == 0 {
		return nil
	}
	first := cols[0]
	out := make([]string, 0, len(td.Rows))
	for _, row := range td.Rows {
		m, err := AsMap(row)
		if err != nil {
			continue
		}
		if v, ok := m.Get(first); ok {
			out = append(out, ValToString(v))
		}
	}
	return out
}
