package engine

import (
	"fmt"
	"math"
	"strings"

	csvpkg "github.com/jsonicjs/csv/go"
	jsonic "github.com/jsonicjs/jsonic/go"
	multisource "github.com/jsonicjs/multisource/go"

	"github.com/aql-lang/aql/lang/internal/fileops"
)

// Format encodes and decodes file content for a named representation
// (text, json, csv, …). The host package owns this interface — aqleng
// has no knowledge of file formats. Word handlers look formats up via
// HostFormats(r) (see capabilities.go).
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
		_as0, _ := AsString(v)
		return _as0, nil
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
// When a Resolver is set, the multisource plugin is enabled so that
// @"path" references in .jsonic files are resolved and merged.
type JsonicFormat struct {
	Resolver multisource.Resolver
}

func (f *JsonicFormat) Decode(content string) ([]Value, error) {
	var j *jsonic.Jsonic
	if f.Resolver != nil {
		j = multisource.MakeJsonic(multisource.MultiSourceOptions{
			Resolver: f.Resolver,
		})
	} else {
		j = jsonic.Make()
	}
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

// MakeFileOpsResolver creates a multisource.Resolver backed by a FileOps
// implementation. This bridges the AQL file abstraction to multisource's
// path resolution so that @"path" references in .jsonic files work.
func MakeFileOpsResolver(ops fileops.FileOps) multisource.Resolver {
	return func(spec multisource.PathSpec, opts *multisource.MultiSourceOptions) multisource.Resolution {
		res := multisource.Resolution{
			PathSpec: spec,
		}

		// Build candidate paths with implicit extensions.
		candidates := []string{spec.Full}
		if spec.Kind == "" && opts != nil {
			for _, ext := range opts.ImplicitExt {
				candidates = append(candidates, spec.Full+"."+ext)
			}
		}

		for _, path := range candidates {
			data, err := ops.ReadFile(path)
			if err == nil {
				res.Src = string(data)
				res.Found = true
				res.Full = path

				// Determine kind from extension if not set.
				if res.Kind == "" {
					ext := ""
					for i := len(path) - 1; i >= 0; i-- {
						if path[i] == '.' {
							ext = path[i+1:]
							break
						}
					}
					res.Kind = ext
				}
				return res
			}
			res.Search = append(res.Search, path)
		}
		return res
	}
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
		if elems := AsMutableList(v); elems != nil {
			parts := make([]string, len(elems))
			for i, e := range elems {
				if e.VType.Matches(TString) {
					_as1, _ := AsString(e)
					parts[i] = _as1
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
	noObj := false
	noHdr := false
	records, err := csvpkg.Parse(content, csvpkg.CsvOptions{
		Object: &noObj,
		Header: &noHdr,
		Field:  &csvpkg.FieldOptions{Separation: sep},
	})
	if err != nil {
		return nil, fmt.Errorf("csv: %w", err)
	}

	if len(records) == 0 {
		return []Value{NewList([]Value{})}, nil
	}

	// First row is the header.
	headerRow, ok := records[0].([]any)
	if !ok || len(headerRow) == 0 {
		return []Value{NewList([]Value{})}, nil
	}

	columns := make([]string, len(headerRow))
	for i, h := range headerRow {
		columns[i] = strings.TrimSpace(fmt.Sprintf("%v", h))
	}

	// Build the record type schema (all fields are string type).
	fields := NewOrderedMap()
	for _, col := range columns {
		fields.Set(col, NewTypeLiteral(TString))
	}
	recType := RecordTypeInfo{Fields: fields}

	// Convert each data row into an AQL map value.
	rows := make([]Value, 0, len(records)-1)
	for _, rec := range records[1:] {
		arr, ok := rec.([]any)
		if !ok {
			continue
		}
		om := NewOrderedMap()
		for i, col := range columns {
			if i < len(arr) {
				switch v := arr[i].(type) {
				case string:
					om.Set(col, NewString(v))
				case float64:
					if v == float64(int64(v)) && !math.IsInf(v, 0) && !math.IsNaN(v) {
						om.Set(col, NewInteger(int64(v)))
					} else {
						om.Set(col, NewDecimal(v))
					}
				case bool:
					om.Set(col, NewBoolean(v))
				default:
					om.Set(col, NewString(fmt.Sprintf("%v", v)))
				}
			} else {
				om.Set(col, NewString(""))
			}
		}
		rows = append(rows, NewMap(om))
	}

	// Return a table value: list with TableData holding both schema and rows.
	return []Value{NewValueRaw(TList, TableData{
		Record: recType,
		Rows:   rows,
	})}, nil
}

// TableData is re-exported by aliases.go (defined in aqleng).

// encodeDelimited converts a table value to CSV/TSV text.
func encodeDelimited(v Value, sep string) (string, error) {
	var rows []Value
	var columns []string

	switch data := v.Data.(type) {
	case TableData:
		columns = data.Record.Fields.Keys()
		rows = data.Rows
	case ExtensionPayload:
		if qb, ok := data.Body.(QueryBuilder); ok {
			td, err := qb.Materialize()
			if err != nil {
				return "", fmt.Errorf("encode: %w", err)
			}
			columns = td.Record.Fields.Keys()
			rows = td.Rows
		} else {
			return v.String(), nil
		}
	case ListPayload:
		rows = data.Elems
		if len(rows) > 0 {
			if m := AsMutableMap(rows[0]); m != nil {
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
		m := AsMutableMap(row)
		if m == nil {
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
				s, _ := AsString(val)
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
