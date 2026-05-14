package eng

import (
	"fmt"
	"strings"
)

// PrintNatives consolidates the "print" and "printstr" words. print
// consumes one value from the stack and writes its formatted
// representation to the registry's Output writer followed by a
// newline. printstr does the same but without a trailing newline.
//   - strings: printed as-is
//   - maps/lists: printed as JSON-like text
//   - tables: printed as a formatted table with column headers
var PrintNatives = []NativeFunc{
	{
		Name:        "print",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: printHandler,
			Returns: []*Type{},
		}},
	},
	{
		Name:        "printstr",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: printstrHandler,
			Returns: []*Type{},
		}},
	},
}

func printHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v := args[0]
	out := FormatForPrint(v)
	fmt.Fprintln(r.Output, out)
	return nil, nil
}

func printstrHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v := args[0]
	out := FormatForPrint(v)
	fmt.Fprint(r.Output, out)
	return nil, nil
}

// FormatForPrint returns the print representation of a value.
func FormatForPrint(v Value) string {
	// The value `none` (the unique inhabitant of None, Data != nil
	// sentinel) prints as "none".
	if v.IsNone() {
		return "none"
	}
	// Type literals (Data==nil) — print as the leaf of the type path
	// (e.g. Integer → "Integer", List → "List"). Type names are
	// globally unique. The None type literal prints as "None".
	if v.Data == nil {
		return v.VType.Leaf()
	}

	// Table: formatted with headers and aligned columns.
	if v.IsTableType() {
		if td, ok := v.Data.(TableData); ok {
			return formatTable(td)
		}
		if mz, ok := v.Data.(Materializer); ok {
			td, err := mz.Materialize()
			if err != nil {
				return "query(error:" + err.Error() + ")"
			}
			return formatTable(td)
		}
	}

	// Error: print the error message.
	if v.IsError() {
		info, _ := v.AsError()
		return info.Message
	}

	// Dependent scalar: render the constraint, not the underlying base.
	// Must run before TString / TInteger / etc. matches because a
	// DepScalar's VType also matches its base via the lattice override.
	if v.IsDepScalar() {
		return ValToString(v)
	}

	// String: printed as-is (no quotes).
	if v.VType.Matches(TString) {
		_as0, _ := v.AsString()
		return _as0
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

	// Everything else: use ValToString (integers, booleans, atoms).
	return ValToString(v)
}

// formatMapJSON formats a map value as a JSON-like string.
func formatMapJSON(v Value) string {
	om := v.AsMutableMap()
	if om == nil {
		return "{}"
	}
	parts := make([]string, 0, om.Len())
	for _, k := range om.Keys() {
		val, _ := om.Get(k)
		parts = append(parts, fmt.Sprintf("%q: %s", k, FormatValueJSON(val)))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// formatListJSON formats a list value as a JSON-like string.
func formatListJSON(v Value) string {
	elems := v.AsList()
	parts := make([]string, elems.Len())
	for i, e := range elems.Slice() {
		parts[i] = FormatValueJSON(e)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// FormatValueJSON formats any value for JSON-like output.
func FormatValueJSON(v Value) string {
	// The value `none` and the None type literal both render as JSON
	// null — that's how JSON encodes the unit type / absent value.
	if v.IsNone() || (v.Data == nil && v.VType.Equal(TNone)) {
		return "null"
	}
	if v.Data == nil {
		// Type literal — render the leaf, quoted as a JSON string.
		return fmt.Sprintf("%q", v.VType.Leaf())
	}
	// DepScalar pre-empts the Matches(TString)/... dispatch so its
	// constraint payload renders via the DepScalar formatter rather
	// than crashing through AsString. Quote the form so it's a valid
	// JSON string.
	if s := renderDepScalar(v); s != "" {
		return fmt.Sprintf("%q", s)
	}
	switch {
	case v.VType.Matches(TString):
		_as1, _ := v.AsString()
		return fmt.Sprintf("%q", _as1)
	case v.VType.Matches(TInteger):
		_as2, _ := v.AsInteger()
		return fmt.Sprintf("%d", _as2)
	case v.VType.Matches(TBoolean):
		_as3, _ := v.AsBoolean()
		if _as3 {
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
				cells[i][j] = ValToString(val)
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
		b.WriteString(PadRight(col, widths[j]))
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
			b.WriteString(PadRight(cell, widths[j]))
		}
		b.WriteByte('\n')
	}

	return strings.TrimRight(b.String(), "\n")
}

// PadRight pads s with spaces on the right to reach width.
func PadRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
