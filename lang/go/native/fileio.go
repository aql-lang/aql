package native

import (
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"

	"github.com/aql-lang/aql/eng/go/parser"
	jsonic "github.com/tabnas/jsonic/go"
)

// Special path constants for stdio streams.
const (
	pathStdin  = "<stdin>"
	pathStdout = "<stdout>"
	pathStderr = "<stderr>"
)

// DefaultExtensions returns the built-in file-extension→format-name map
// (lowercase keys, no leading dot). The mapping is many-to-one — several
// extensions may share one format (cfg/conf/ini→ini, yml/yaml→yaml) — and
// host-overridable via SetHostExtensions / RegisterFormat. An extension
// absent from the map decodes as plain text.
func DefaultExtensions() map[string]string {
	return map[string]string{
		"csv":      "csv",
		"tsv":      "tsv",
		"json":     "json",
		"jsonic":   "jsonic",
		"txt":      "text",
		"ini":      "ini",
		"cfg":      "ini",
		"conf":     "ini",
		"cnf":      "ini",
		"yml":      "yaml",
		"yaml":     "yaml",
		"toml":     "toml",
		"xml":      "xml",
		"json5":    "json5",
		"jsonc":    "jsonc",
		"zon":      "zon",
		"md":       "markdown",
		"markdown": "markdown",
		"rss":      "feed",
		"atom":     "feed",
	}
}

// formatFromExt returns the format name for path's extension, consulting
// the host extension registry (HostExtensions). Returns "" when the
// extension is unmapped or no registry is installed; the caller falls back
// to text.
func formatFromExt(r *Registry, path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	if ext == "" {
		return ""
	}
	if m := HostExtensions(r); m != nil {
		if f, ok := m[ext]; ok {
			return f
		}
	}
	return ""
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

// parseFileOpts extracts options from an AQL map value. fmtExplicit is
// true if the user explicitly set the fmt option. parserOpts holds every
// remaining (non-reserved) key, forwarded to an opts-aware decoder — e.g.
// `read foo.csv {fmt:'csv' field:{separation:';'}}` yields
// parserOpts={field:{separation:';'}}. It is nil when no extra keys exist.
func parseFileOpts(opts Value) (enc, format, mode, nl string, fmtExplicit bool, parserOpts map[string]any) {
	enc = "utf8"
	format = "text"
	mode = "write"
	nl = "lf"

	if !opts.Parent.Equal(TMap) || !IsConcrete(opts) {
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

	// Surface any non-reserved keys as parser options for the decoder.
	if raw, ok := ValueToAny(opts).(map[string]any); ok {
		delete(raw, "enc")
		delete(raw, "fmt")
		delete(raw, "mode")
		delete(raw, "nl")
		if len(raw) > 0 {
			parserOpts = raw
		}
	}

	return
}

// jsonicToValue converts a jsonic parse result to an AQL Value.
// This uses data context: all text becomes strings, not words.
func jsonicToValue(v any) (Value, error) {
	// A number-Sub-wrapped token (from parser.SafeParseData) carries its
	// exact source text, so "42.0" stays a Float and "42" an Integer (and
	// large integers keep full precision / raise integer_overflow). The
	// bare float64 case below is the fallback for parsers built without
	// the Sub (SafeParse / direct float64 callers), where the source text
	// is already gone and a whole-valued float collapses to Integer.
	if ev, ok, err := parser.ConvertParsedNumber(v); ok {
		return ev, err
	}
	switch val := v.(type) {
	case nil:
		return NewTypeLiteral(TNone), nil
	case bool:
		return NewBoolean(val), nil
	case float64:
		if val == float64(int64(val)) && !math.IsInf(val, 0) && !math.IsNaN(val) {
			return NewInteger(int64(val)), nil
		}
		return NewFloat(val), nil
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
// Thin alias for the shared sortedAnyMapKeys (transform.go), kept for the
// file-local callers.
func sortedMapKeys(m map[string]any) []string {
	return sortedAnyMapKeys(m)
}

// valueToJsonic converts an AQL Value to a compact json/jsonic-compatible
// string. It is a thin wrapper over the canonical walk-based json encoder
// (emit.go) — the single emit code path — keeping its name and signature for
// the existing callers/tests. Compact, double-quoted keys (the json profile),
// byte-for-byte identical to the previous hand-rolled implementation.
func valueToJsonic(v Value) string {
	s, _ := encodeJSON(v, nil)
	return s
}

func doRead(r *Registry, path, enc, format, nl string, opts map[string]any) ([]Value, error) {
	var data []byte
	var err error

	if path == pathStdout || path == pathStderr {
		return nil, r.AqlError("read_error", "read: cannot read from an output stream", "read")
	}

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

	var result []Value
	if d, ok := f.(DecodeOpter); ok && len(opts) > 0 {
		result, err = d.DecodeOpts(content, opts)
	} else {
		result, err = f.Decode(content)
	}
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
