package native

import (
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	jsonic "github.com/jsonicjs/jsonic/go"
)

// Special path constants for stdio streams.
const (
	pathStdin  = "<stdin>"
	pathStdout = "<stdout>"
	pathStderr = "<stderr>"
)

// formatFromExt returns the format name based on the file extension.
// Returns empty string if the extension is not recognized.
func formatFromExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".csv":
		return "csv"
	case ".tsv":
		return "tsv"
	case ".json":
		return "json"
	case ".jsonic":
		return "jsonic"
	case ".txt":
		return "text"
	default:
		return ""
	}
}

// normalizeLineEndings replaces all \r\n and \r with \n.
func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// denormalizeLineEndings converts \n to the specified ending.
func denormalizeLineEndings(s string, nl string) string {
	switch nl {
	case "crlf":
		return strings.ReplaceAll(s, "\n", "\r\n")
	default:
		return s
	}
}

// applyNL applies line ending normalization based on the nl option.
func applyNL(content string, nl string) string {
	switch nl {
	case "lf":
		return normalizeLineEndings(content)
	case "crlf":
		return denormalizeLineEndings(normalizeLineEndings(content), "crlf")
	case "raw":
		return content
	default:
		return normalizeLineEndings(content)
	}
}

// parseFileOpts extracts options from an AQL map value.
// fmtExplicit is true if the user explicitly set the fmt option.
func parseFileOpts(opts Value) (enc, format, mode, nl string, fmtExplicit bool) {
	enc = "utf8"
	format = "text"
	mode = "write"
	nl = "lf"

	if !opts.Parent.Equal(TMap) || opts.Data == nil {
		return
	}
	m, _ := AsMap(opts)

	if s, ok := MapFieldString(m, "enc"); ok {
		enc = s
	}
	if s, ok := MapFieldString(m, "fmt"); ok {
		format = s
		fmtExplicit = true
	}
	if s, ok := MapFieldString(m, "mode"); ok {
		mode = s
	}
	if s, ok := MapFieldString(m, "nl"); ok {
		nl = s
	}

	return
}

// jsonicToValue converts a jsonic parse result to an AQL Value.
// This uses data context: all text becomes strings, not words.
func jsonicToValue(v any) (Value, error) {
	switch val := v.(type) {
	case nil:
		return NewTypeLiteral(TNone), nil
	case bool:
		return NewBoolean(val), nil
	case float64:
		if val == float64(int64(val)) && !math.IsInf(val, 0) && !math.IsNaN(val) {
			return NewInteger(int64(val)), nil
		}
		return NewDecimal(val), nil
	case string:
		return NewString(val), nil
	case []any:
		elems := make([]Value, len(val))
		for i, item := range val {
			e, err := jsonicToValue(item)
			if err != nil {
				return Value{}, err
			}
			elems[i] = e
		}
		return NewList(elems), nil
	case map[string]any:
		om := NewOrderedMap()
		for _, key := range sortedMapKeys(val) {
			child, err := jsonicToValue(val[key])
			if err != nil {
				return Value{}, err
			}
			om.Set(key, child)
		}
		return NewMap(om), nil
	case jsonic.Text:
		return NewString(val.Str), nil
	case jsonic.ListRef:
		return jsonicToValue(val.Val)
	case jsonic.MapRef:
		return jsonicToValue(val.Val)
	default:
		return Value{}, fmt.Errorf("unsupported jsonic type: %T", v)
	}
}

// sortedMapKeys returns map keys in sorted order for deterministic output.
func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// valueToJsonic converts an AQL Value to a jsonic-compatible string.
func valueToJsonic(v Value) string {
	switch {
	case v.Parent.Matches(TString):
		_as0, _ := AsString(v)
		return fmt.Sprintf("%q", _as0)
	case v.Parent.Matches(TDecimal):
		_as1, _ := AsDecimal(v)
		return strconv.FormatFloat(_as1, 'f', -1, 64)
	case v.Parent.Matches(TInteger):
		_as2, _ := AsInteger(v)
		return fmt.Sprintf("%d", _as2)
	case v.Parent.Matches(TBoolean):
		_as3, _ := AsBoolean(v)
		if _as3 {
			return "true"
		}
		return "false"
	case v.Parent.Equal(TNone):
		return "null"
	case v.Parent.Equal(TAtom):
		_as4, _ := AsAtom(v)
		return fmt.Sprintf("%q", _as4)
	case v.Parent.Equal(TList):
		if elems, err := AsMutableList(v); err == nil {
			parts := make([]string, len(elems))
			for i, e := range elems {
				parts[i] = valueToJsonic(e)
			}
			return "[" + strings.Join(parts, ",") + "]"
		}
		return "[]"
	case v.Parent.Equal(TMap):
		if om, err := AsMutableMap(v); err == nil {
			parts := make([]string, 0, om.Len())
			for _, k := range om.Keys() {
				val, _ := om.Get(k)
				parts = append(parts, fmt.Sprintf("%q:%s", k, valueToJsonic(val)))
			}
			return "{" + strings.Join(parts, ",") + "}"
		}
		return "{}"
	default:
		return fmt.Sprintf("%q", v.String())
	}
}

func doRead(r *Registry, path, enc, format, nl string) ([]Value, error) {
	var data []byte
	var err error

	if path == pathStdin {
		data, err = io.ReadAll(r.Input)
		if err != nil {
			return nil, fmt.Errorf("read: stdin: %w", err)
		}
	} else {
		data, err = EffectiveFileOps(r).ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
	}

	content := applyNL(string(data), nl)

	f, ok := HostFormats(r)[format]
	if !ok {
		return nil, r.AqlError("read_error", fmt.Sprintf("read: unknown format: %s", format), "read")
	}

	result, err := f.Decode(content)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	// Store table data in SQLite for formats that produce tables.
	if HostSQLite(r) != nil && len(result) == 1 {
		if td, ok := result[0].Data.(TableData); ok {
			// Derive table name from file path (basename without extension).
			baseName := path
			if idx := strings.LastIndex(baseName, "/"); idx >= 0 {
				baseName = baseName[idx+1:]
			}
			if idx := strings.LastIndex(baseName, "\\"); idx >= 0 {
				baseName = baseName[idx+1:]
			}
			if idx := strings.LastIndex(baseName, "."); idx >= 0 {
				baseName = baseName[:idx]
			}

			if err := HostSQLite(r).StoreTable(baseName, td); err != nil {
				return nil, fmt.Errorf("read: sqlite store: %w", err)
			}
			td.SQLite = true
			td.TableName = baseName
			result[0] = NewValueRaw(TList, td)
		}
	}

	return result, nil
}

func doWrite(r *Registry, path, content, enc, format, mode, nl string) ([]Value, error) {
	content = applyNL(content, nl)

	// Handle stdout/stderr special paths.
	if path == pathStdout || path == pathStderr {
		var w io.Writer
		if path == pathStdout {
			w = r.Output
		} else {
			w = r.ErrOutput
		}
		if _, err := fmt.Fprint(w, content); err != nil {
			return nil, fmt.Errorf("write: %w", err)
		}
		return []Value{NewString(path)}, nil
	}

	data := []byte(content)

	if mode == "append" {
		existing, err := EffectiveFileOps(r).ReadFile(path)
		if err == nil {
			data = append(existing, data...)
		}
	}

	if err := EffectiveFileOps(r).WriteFile(path, data, 0644); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	return []Value{NewString(path)}, nil
}
