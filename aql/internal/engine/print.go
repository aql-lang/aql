package engine

import (
	"fmt"
	"strings"
)

// registerPrint registers the "print" and "printstr" words.
// print consumes one value from the stack and writes its formatted
// representation to the registry's Output writer followed by a newline.
// printstr does the same but without a trailing newline.
//   - strings: printed as-is
//   - maps/lists: printed as JSON-like text
//   - tables: printed as a formatted table with column headers
func registerPrint(r *Registry) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		v := args[0]
		out := formatForPrint(v)
		fmt.Fprintln(r.Output, out)
		return nil, nil
	}

	r.Register("print",
		Signature{
			Args:    []Type{TAny},
			Handler: handler,
		},
	)

	handlerStr := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		v := args[0]
		out := formatForPrint(v)
		fmt.Fprint(r.Output, out)
		return nil, nil
	}

	r.Register("printstr",
		Signature{
			Args:    []Type{TAny},
			Handler: handlerStr,
		},
	)
}

// formatForPrint returns the print representation of a value.
func formatForPrint(v Value) string {
	// Type literals (Data==nil) — print the type name; None prints "null".
	if v.Data == nil {
		if v.VType.Equal(TNone) {
			return "null"
		}
		return v.VType.String()
	}

	// Table: formatted with headers and aligned columns.
	if v.IsTableType() {
		if td, ok := v.Data.(TableData); ok {
			return formatTable(td)
		}
		if qb, ok := v.Data.(QueryBuilder); ok {
			td, err := qb.Materialize()
			if err != nil {
				return "query(error:" + err.Error() + ")"
			}
			return formatTable(td)
		}
	}

	// String: printed as-is (no quotes).
	if v.VType.Matches(TString) {
		return v.AsString()
	}

	// Options type: use String() representation.
	if v.IsOptionsType() {
		return v.String()
	}

	// Map: JSON-like output.
	if v.VType.Equal(TMap) {
		return formatMapJSON(v)
	}

	// List: JSON-like output.
	if v.VType.Equal(TList) && v.Data != nil {
		return formatListJSON(v)
	}

	// Everything else: use valToString (integers, booleans, atoms).
	return valToString(v)
}

// formatMapJSON formats a map value as a JSON-like string.
func formatMapJSON(v Value) string {
	om, ok := v.Data.(*OrderedMap)
	if !ok {
		return "{}"
	}
	parts := make([]string, 0, om.Len())
	for _, k := range om.Keys() {
		val, _ := om.Get(k)
		parts = append(parts, fmt.Sprintf("%q: %s", k, formatValueJSON(val)))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// formatListJSON formats a list value as a JSON-like string.
func formatListJSON(v Value) string {
	elems := v.AsList()
	parts := make([]string, elems.Len())
	for i, e := range elems.Slice() {
		parts[i] = formatValueJSON(e)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// formatValueJSON formats any value for JSON-like output.
func formatValueJSON(v Value) string {
	if v.Data == nil {
		if v.VType.Equal(TNone) {
			return "null"
		}
		return v.VType.String()
	}
	switch {
	case v.VType.Matches(TString):
		return fmt.Sprintf("%q", v.AsString())
	case v.VType.Matches(TInteger):
		return fmt.Sprintf("%d", v.AsInteger())
	case v.VType.Matches(TBoolean):
		if v.AsBoolean() {
			return "true"
		}
		return "false"
	case v.VType.Equal(TNone):
		return "null"
	case v.VType.Equal(TMap):
		return formatMapJSON(v)
	case v.VType.Equal(TList):
		return formatListJSON(v)
	default:
		return v.String()
	}
}

// formatTable formats a TableData as an aligned text table with headers.
func formatTable(td TableData) string {
	columns := td.Record.Fields.Keys()
	if len(columns) == 0 {
		return "(empty table)"
	}

	// Collect all cell values as strings.
	cells := make([][]string, len(td.Rows))
	for i, row := range td.Rows {
		cells[i] = make([]string, len(columns))
		om := row.AsMap()
		for j, col := range columns {
			if val, ok := om.Get(col); ok {
				cells[i][j] = valToString(val)
			}
		}
	}

	// Calculate column widths.
	widths := make([]int, len(columns))
	for j, col := range columns {
		widths[j] = len(col)
	}
	for _, row := range cells {
		for j, cell := range row {
			if len(cell) > widths[j] {
				widths[j] = len(cell)
			}
		}
	}

	var b strings.Builder

	// Header row.
	for j, col := range columns {
		if j > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(padRight(col, widths[j]))
	}
	b.WriteByte('\n')

	// Separator row.
	for j := range columns {
		if j > 0 {
			b.WriteString("-+-")
		}
		b.WriteString(strings.Repeat("-", widths[j]))
	}
	b.WriteByte('\n')

	// Data rows.
	for _, row := range cells {
		for j, cell := range row {
			if j > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(padRight(cell, widths[j]))
		}
		b.WriteByte('\n')
	}

	return strings.TrimRight(b.String(), "\n")
}

// padRight pads s with spaces on the right to reach width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
