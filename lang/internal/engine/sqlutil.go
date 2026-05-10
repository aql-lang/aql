package engine

import "strings"

// quoteIdent quotes a SQL identifier with double quotes.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// joinQuoted joins identifiers with commas, each quoted.
func joinQuoted(names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = quoteIdent(n)
	}
	return strings.Join(parts, ", ")
}
