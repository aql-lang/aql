package engine

import (
	"fmt"
	"strings"

	csvpkg "github.com/jsonicjs/csv/go"
	jsonic "github.com/jsonicjs/jsonic/go"
)

// Format handles encoding and decoding file content for a specific format.
type Format interface {
	Decode(content string) ([]Value, error)
	Encode(v Value) (string, error)
}

// TextFormat handles plain text (no parsing).
type TextFormat struct{}

func (f *TextFormat) Decode(content string) ([]Value, error) {
	return []Value{NewString(content)}, nil
}

func (f *TextFormat) Encode(v Value) (string, error) {
	if v.VType.Matches(TString) {
		return v.AsString(), nil
	}
	return v.String(), nil
}

// JSONFormat handles strict JSON via jsonic.Parse (standard JSON is valid jsonic).
type JSONFormat struct{}

func (f *JSONFormat) Decode(content string) ([]Value, error) {
	result, err := jsonic.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	v, err := jsonicToValue(result)
	if err != nil {
		return nil, err
	}
	return []Value{v}, nil
}

func (f *JSONFormat) Encode(v Value) (string, error) {
	return valueToJsonic(v), nil
}

// JsonicFormat handles relaxed JSON (unquoted keys, trailing commas, etc.).
type JsonicFormat struct{}

func (f *JsonicFormat) Decode(content string) ([]Value, error) {
	j := jsonic.Make()
	result, err := j.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("invalid jsonic: %w", err)
	}
	if result == nil {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	v, err := jsonicToValue(result)
	if err != nil {
		return nil, err
	}
	return []Value{v}, nil
}

func (f *JsonicFormat) Encode(v Value) (string, error) {
	return valueToJsonic(v), nil
}

// LinesFormat splits/joins on newlines, producing/consuming a list of strings.
type LinesFormat struct{}

func (f *LinesFormat) Decode(content string) ([]Value, error) {
	lines := strings.Split(content, "\n")
	elems := make([]Value, len(lines))
	for i, line := range lines {
		elems[i] = NewString(line)
	}
	return []Value{NewList(elems)}, nil
}

func (f *LinesFormat) Encode(v Value) (string, error) {
	if v.VType.Equal(TList) {
		if elems, ok := v.Data.([]Value); ok {
			parts := make([]string, len(elems))
			for i, e := range elems {
				if e.VType.Matches(TString) {
					parts[i] = e.AsString()
				} else {
					parts[i] = e.String()
				}
			}
			return strings.Join(parts, "\n"), nil
		}
	}
	return v.String(), nil
}

// CSVFormat parses CSV content into a table value.
// The first row is treated as column headers, producing a table type
// where each row is a record with those column names as fields.
type CSVFormat struct{}

func (f *CSVFormat) Decode(content string) ([]Value, error) {
	return decodeDelimited(content, ",")
}

func (f *CSVFormat) Encode(v Value) (string, error) {
	return encodeDelimited(v, ",")
}

// TSVFormat parses TSV (tab-separated) content into a table value.
type TSVFormat struct{}

func (f *TSVFormat) Decode(content string) ([]Value, error) {
	return decodeDelimited(content, "\t")
}

func (f *TSVFormat) Encode(v Value) (string, error) {
	return encodeDelimited(v, "\t")
}

// decodeDelimited parses CSV/TSV content into a table value.
// The first row is used as column headers. Each subsequent row becomes
// a record (map) keyed by those headers. The result is wrapped in a
// table type whose record schema has all-string fields.
func decodeDelimited(content string, sep string) ([]Value, error) {
	t := true
	j := csvpkg.MakeJsonic(csvpkg.CsvOptions{
		Header: &t,
		Field:  &csvpkg.FieldOptions{Separation: sep},
	})

	parsed, err := j.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("csv: %w", err)
	}

	records, ok := parsed.([]any)
	if !ok || len(records) == 0 {
		return []Value{NewList([]Value{})}, nil
	}

	// Extract column names from first line of content for deterministic order.
	var columns []string
	if first, ok := records[0].(map[string]any); ok {
		// Parse headers from the raw content's first line.
		headerLine := content
		if idx := strings.IndexAny(content, "\r\n"); idx >= 0 {
			headerLine = content[:idx]
		}
		columns = strings.Split(headerLine, sep)
		// Trim whitespace from header names.
		for i := range columns {
			columns[i] = strings.TrimSpace(columns[i])
		}
		// Filter to only keys that actually appear in the parsed result.
		filtered := make([]string, 0, len(columns))
		for _, col := range columns {
			if _, exists := first[col]; exists {
				filtered = append(filtered, col)
			}
		}
		columns = filtered
	}

	// Build the record type schema (all fields are string type).
	fields := NewOrderedMap()
	for _, col := range columns {
		fields.Set(col, NewTypeLiteral(TString))
	}
	recType := RecordTypeInfo{Fields: fields}

	// Convert each parsed record into an AQL map value.
	rows := make([]Value, 0, len(records))
	for _, rec := range records {
		m, ok := rec.(map[string]any)
		if !ok {
			continue
		}
		om := NewOrderedMap()
		for _, col := range columns {
			val, exists := m[col]
			if !exists {
				om.Set(col, NewString(""))
				continue
			}
			switch v := val.(type) {
			case string:
				om.Set(col, NewString(v))
			case float64:
				om.Set(col, NewInteger(int64(v)))
			case bool:
				om.Set(col, NewBoolean(v))
			default:
				om.Set(col, NewString(fmt.Sprintf("%v", v)))
			}
		}
		rows = append(rows, NewMap(om))
	}

	// Return a table value: list with TableData holding both schema and rows.
	return []Value{Value{VType: TList, Data: TableData{
		Record: recType,
		Rows:   rows,
	}}}, nil
}

// TableData holds a concrete table: its record schema plus the row data.
type TableData struct {
	Record    RecordTypeInfo
	Rows      []Value
	SQLite    bool   // true if data is backed by an in-memory SQLite table
	TableName string // name of the table in the SQLite store
}

// encodeDelimited converts a table value to CSV/TSV text.
func encodeDelimited(v Value, sep string) (string, error) {
	var rows []Value
	var columns []string

	switch data := v.Data.(type) {
	case TableData:
		columns = data.Record.Fields.Keys()
		rows = data.Rows
	case []Value:
		rows = data
		if len(rows) > 0 {
			if m, ok := rows[0].Data.(*OrderedMap); ok {
				columns = m.Keys()
			}
		}
	default:
		return v.String(), nil
	}

	if len(columns) == 0 {
		return "", nil
	}

	var sb strings.Builder
	// Header row.
	sb.WriteString(strings.Join(columns, sep))
	sb.WriteByte('\n')
	// Data rows.
	for _, row := range rows {
		m, ok := row.Data.(*OrderedMap)
		if !ok {
			continue
		}
		parts := make([]string, len(columns))
		for i, col := range columns {
			val, exists := m.Get(col)
			if !exists {
				parts[i] = ""
				continue
			}
			if val.VType.Matches(TString) {
				s := val.AsString()
				if strings.ContainsAny(s, sep+"\"\n\r") {
					s = "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
				}
				parts[i] = s
			} else {
				parts[i] = val.String()
			}
		}
		sb.WriteString(strings.Join(parts, sep))
		sb.WriteByte('\n')
	}

	return sb.String(), nil
}

// DefaultFormats returns the built-in format registry.
func DefaultFormats() map[string]Format {
	return map[string]Format{
		"text":   &TextFormat{},
		"json":   &JSONFormat{},
		"jsonic": &JsonicFormat{},
		"lines":  &LinesFormat{},
		"csv":    &CSVFormat{},
		"tsv":    &TSVFormat{},
	}
}
