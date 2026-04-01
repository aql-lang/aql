package engine

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

	if !opts.VType.Equal(TMap) || opts.Data == nil {
		return
	}
	m := opts.AsMap()

	if v, ok := m.Get("enc"); ok && v.VType.Matches(TString) {
		enc = v.AsString()
	}
	if v, ok := m.Get("fmt"); ok && v.VType.Matches(TString) {
		format = v.AsString()
		fmtExplicit = true
	}
	if v, ok := m.Get("mode"); ok && v.VType.Matches(TString) {
		mode = v.AsString()
	}
	if v, ok := m.Get("nl"); ok && v.VType.Matches(TString) {
		nl = v.AsString()
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
	case v.VType.Matches(TString):
		return fmt.Sprintf("%q", v.AsString())
	case v.VType.Matches(TDecimal):
		return strconv.FormatFloat(v.AsDecimal(), 'f', -1, 64)
	case v.VType.Matches(TInteger):
		return fmt.Sprintf("%d", v.AsInteger())
	case v.VType.Matches(TBoolean):
		if v.AsBoolean() {
			return "true"
		}
		return "false"
	case v.VType.Equal(TNone):
		return "null"
	case v.VType.Equal(TAtom):
		return fmt.Sprintf("%q", v.AsAtom())
	case v.VType.Equal(TList):
		if _, ok := v.Data.([]Value); ok {
			elems := v.AsList()
			parts := make([]string, elems.Len())
			for i, e := range elems.Slice() {
				parts[i] = valueToJsonic(e)
			}
			return "[" + strings.Join(parts, ",") + "]"
		}
		return "[]"
	case v.VType.Equal(TMap):
		if om, ok := v.Data.(*OrderedMap); ok {
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

// registerFileIO registers the read and write words.
func registerFileIO(r *Registry) {
	// read: [string] -> [string|list|map]
	readHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		path := args[0].AsString()
		format := formatFromExt(path)
		if format == "" {
			format = "text"
		}
		return doRead(r, path, "utf8", format, "lf")
	}

	// read: [string, map] -> [string|list|map]
	readOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		path := args[0].AsString()
		enc, format, _, nl, fmtExplicit := parseFileOpts(args[1])
		// If fmt was not explicitly set, use file extension.
		if !fmtExplicit {
			if extFmt := formatFromExt(path); extFmt != "" {
				format = extFmt
			}
		}
		return doRead(r, path, enc, format, nl)
	}

	r.Register("read",
		Signature{
			Args:    []Type{TString, TMap},
			Handler: readOptsHandler,
		},
		Signature{
			Args:    []Type{TString},
			Handler: readHandler,
		},
	)

	// write: [string, string] -> [string]
	writeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		path := args[0].AsString()
		content := args[1].AsString()
		return doWrite(r, path, content, "utf8", "text", "write", "lf")
	}

	// write: [string, string, map] -> [string]
	writeOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		path := args[0].AsString()
		content := args[1].AsString()
		enc, format, mode, nl, _ := parseFileOpts(args[2])
		return doWrite(r, path, content, enc, format, mode, nl)
	}

	// write: [string, any, map] -> [string] (for non-string data with fmt)
	writeAnyOptsHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		path := args[0].AsString()
		_, format, mode, nl, _ := parseFileOpts(args[2])
		if format == "text" {
			format = "jsonic"
		}
		content := valueToJsonic(args[1])
		return doWrite(r, path, content, "utf8", format, mode, nl)
	}

	r.Register("write",
		Signature{
			Args:    []Type{TString, TString, TMap},
			Handler: writeOptsHandler,
		},
		Signature{
			Args:    []Type{TString, TAny, TMap},
			Handler: writeAnyOptsHandler,
		},
		Signature{
			Args:    []Type{TString, TString},
			Handler: writeHandler,
		},
	)

	// stdin, stdout, stderr push special path strings for use with read/write.
	r.Register("stdin",
		Signature{
			Args: []Type{},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewString(pathStdin)}, nil
			},
		},
	)
	r.Register("stdout",
		Signature{
			Args: []Type{},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewString(pathStdout)}, nil
			},
		},
	)
	r.Register("stderr",
		Signature{
			Args: []Type{},
			Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewString(pathStderr)}, nil
			},
		},
	)
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
		data, err = r.EffectiveFileOps().ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
	}

	content := applyNL(string(data), nl)

	f, ok := r.Formats[format]
	if !ok {
		return nil, fmt.Errorf("read: unknown format: %s", format)
	}

	result, err := f.Decode(content)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	// Store table data in SQLite for formats that produce tables.
	if r.SQLite != nil && len(result) == 1 {
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

			if err := r.SQLite.StoreTable(baseName, td); err != nil {
				return nil, fmt.Errorf("read: sqlite store: %w", err)
			}
			td.SQLite = true
			td.TableName = baseName
			result[0] = newValue(TList, td)
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
		existing, err := r.EffectiveFileOps().ReadFile(path)
		if err == nil {
			data = append(existing, data...)
		}
	}

	if err := r.EffectiveFileOps().WriteFile(path, data, 0644); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	return []Value{NewString(path)}, nil
}
