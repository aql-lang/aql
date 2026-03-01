package parser

import (
	"fmt"
	"strconv"
	"strings"

	jsonic "github.com/jsonicjs/jsonic/go"
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// typeNames maps well-known type names to their engine Type.
var typeNames = map[string]engine.Type{
	"any":     engine.TAny,
	"none":    engine.TNone,
	"number":  engine.TNumber,
	"string":  engine.TString,
	"boolean": engine.TBoolean,
	"list":    engine.TList,
	"map":     engine.TMap,
}

func boolPtr(b bool) *bool { return &b }

// Parse tokenizes the AQL source string into a slice of engine.Value.
// The input is treated as a top-level implicit list: jsonic.Parse handles
// the entire source. The TextInfo option distinguishes quoted strings from
// unquoted text (words).
//
// Context rules:
//   - Top level: unquoted text → words, quoted text → strings.
//   - Inside maps (including implicit): all text → scalar data.
//   - Inside lists at the top level: unquoted text → words (quotation).
//   - Inside lists inside maps: all text → scalar data.
func Parse(src string) ([]engine.Value, error) {
	// Ensure ( and ) are space-separated so they become distinct tokens
	// in the top-level implicit list.
	processed := preprocessParens(src)

	j := jsonic.Make(jsonic.Options{
		TextInfo: boolPtr(true),
		Value:    &jsonic.ValueOptions{Lex: boolPtr(false)},
	})

	result, err := j.Parse(processed)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	if result == nil {
		return nil, nil
	}

	// jsonic returns []any for both explicit lists [1,2,3] and implicit
	// lists (1 2 3). Check the source to disambiguate: if the entire
	// source is a single bracketed expression, it is one list value.
	switch val := result.(type) {
	case []any:
		if isSingleExplicitList(processed) {
			lv, err := convertWordList(val)
			if err != nil {
				return nil, err
			}
			return []engine.Value{lv}, nil
		}
		return convertTopLevel(val)
	default:
		v, err := convertTopLevelValue(val)
		if err != nil {
			return nil, err
		}
		return []engine.Value{v}, nil
	}
}

// preprocessParens inserts spaces around ( and ) that are outside quoted
// strings, so jsonic treats each parenthesis as a separate text token.
func preprocessParens(src string) string {
	// Quick check: skip allocation if no parens present.
	if !strings.ContainsAny(src, "()") {
		return src
	}

	var out strings.Builder
	out.Grow(len(src) + 10)
	inString := false
	var quote byte

	for i := 0; i < len(src); i++ {
		c := src[i]

		if inString {
			out.WriteByte(c)
			if c == '\\' && i+1 < len(src) {
				i++
				out.WriteByte(src[i])
			} else if c == quote {
				inString = false
			}
			continue
		}

		if c == '"' || c == '\'' || c == '`' {
			inString = true
			quote = c
			out.WriteByte(c)
			continue
		}

		if c == '(' || c == ')' {
			out.WriteByte(' ')
			out.WriteByte(c)
			out.WriteByte(' ')
			continue
		}

		out.WriteByte(c)
	}

	return out.String()
}

// isSingleExplicitList checks whether the source string represents a single
// explicit list literal [...]. It returns true only if the outermost balanced
// brackets span the entire source (ignoring trailing whitespace/comments).
func isSingleExplicitList(src string) bool {
	trimmed := strings.TrimSpace(src)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return false
	}

	depth := 0
	inString := false
	var quote byte

	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]

		if inString {
			if c == '\\' && i+1 < len(trimmed) {
				i++ // skip escaped char
			} else if c == quote {
				inString = false
			}
			continue
		}

		if c == '"' || c == '\'' || c == '`' {
			inString = true
			quote = c
			continue
		}

		if c == '[' || c == '{' {
			depth++
		} else if c == ']' || c == '}' {
			depth--
			if depth == 0 {
				// Check that everything after the closing bracket is
				// only whitespace or comments.
				return isTrailingOnly(trimmed[i+1:])
			}
		}
	}
	return false
}

// isTrailingOnly returns true if s contains only whitespace and line comments.
func isTrailingOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			continue
		}
		// Line comment: skip to end of line.
		if c == '#' || (c == '/' && i+1 < len(s) && s[i+1] == '/') {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		return false
	}
	return true
}

// convertTopLevel converts a top-level implicit list from jsonic into
// a slice of engine.Value using word context.
func convertTopLevel(items []any) ([]engine.Value, error) {
	values := make([]engine.Value, 0, len(items))
	for _, item := range items {
		v, err := convertTopLevelValue(item)
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, nil
}

// convertTopLevelValue converts a single value in word context.
// Unquoted text → word, quoted text → string.
func convertTopLevelValue(v any) (engine.Value, error) {
	switch val := v.(type) {
	case jsonic.Text:
		if val.Quote == "" {
			return parseWord(val.Str)
		}
		return engine.NewString(val.Str), nil

	case float64:
		return engine.NewInteger(int64(val)), nil

	case map[string]any:
		return convertMapData(val)

	case []any:
		return convertWordList(val)

	case bool:
		return engine.NewBoolean(val), nil

	case nil:
		return engine.NewTypeLiteral(engine.TNone), nil

	default:
		return engine.Value{}, fmt.Errorf("unsupported value type %T", v)
	}
}

// convertWordList converts a list in word context (top-level list).
// Square brackets at the top level are a quotation operator for program
// fragments: unquoted text → words, maps use data context.
func convertWordList(items []any) (engine.Value, error) {
	elems := make([]engine.Value, len(items))
	for i, item := range items {
		v, err := convertTopLevelValue(item)
		if err != nil {
			return engine.Value{}, err
		}
		elems[i] = v
	}
	return engine.NewList(elems), nil
}

// convertMapData converts a map in data context. All text values are
// scalar data regardless of quoting.
func convertMapData(m map[string]any) (engine.Value, error) {
	om := engine.NewOrderedMap()
	for _, key := range sortedKeys(m) {
		child, err := convertDataValue(m[key])
		if err != nil {
			return engine.Value{}, err
		}
		om.Set(key, child)
	}
	return engine.NewMap(om), nil
}

// convertDataValue converts a value in data context (inside maps and
// lists that are inside maps). All text → resolved scalar values
// (type names, booleans, or plain strings).
func convertDataValue(v any) (engine.Value, error) {
	switch val := v.(type) {
	case jsonic.Text:
		return resolveTextValue(val.Str), nil

	case float64:
		return engine.NewInteger(int64(val)), nil

	case map[string]any:
		return convertMapData(val)

	case []any:
		return convertDataList(val)

	case bool:
		return engine.NewBoolean(val), nil

	case nil:
		return engine.NewTypeLiteral(engine.TNone), nil

	default:
		return engine.Value{}, fmt.Errorf("unsupported value type %T", v)
	}
}

// convertDataList converts a list in data context (inside maps).
// All text → scalar data.
func convertDataList(items []any) (engine.Value, error) {
	elems := make([]engine.Value, len(items))
	for i, item := range items {
		v, err := convertDataValue(item)
		if err != nil {
			return engine.Value{}, err
		}
		elems[i] = v
	}
	return engine.NewList(elems), nil
}

// resolveTextValue converts a bare text string into the appropriate
// AQL value — type literal, boolean, or plain string.
func resolveTextValue(text string) engine.Value {
	if text == "true" {
		return engine.NewBoolean(true)
	}
	if text == "false" {
		return engine.NewBoolean(false)
	}
	if t, ok := typeNames[text]; ok {
		return engine.NewTypeLiteral(t)
	}
	return engine.NewString(text)
}

// sortedKeys returns the keys of a map in sorted order for deterministic output.
func sortedKeys(m map[string]any) []string {
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

// parseWord interprets an unquoted text token as an AQL word, handling
// modifier syntax: name/s (forceSuffix), name/p (forcePrefix), name/N (argCount),
// and combinations like name/1s or name/2p.
func parseWord(text string) (engine.Value, error) {
	name := text
	argCount := -1
	forcePrefix := false
	forceSuffix := false

	// Check for /... modifier suffix.
	if idx := strings.LastIndex(name, "/"); idx >= 0 && idx < len(name)-1 {
		mod := name[idx+1:]
		name = name[:idx]

		// Parse optional digits followed by optional 's' or 'p'.
		digits := ""
		rest := mod
		for i, c := range rest {
			if c >= '0' && c <= '9' {
				digits += string(c)
			} else {
				rest = rest[i:]
				break
			}
			if i == len(rest)-1 {
				rest = ""
			}
		}

		if digits != "" {
			n, err := strconv.Atoi(digits)
			if err == nil && n >= 0 {
				argCount = n
			}
		}

		switch rest {
		case "s":
			forceSuffix = true
		case "p":
			forcePrefix = true
		case "":
			// digits only, no mode flag
			if digits == "" {
				// No digits and no flag — not a valid modifier; restore name
				name = text
			}
		default:
			// Unrecognized modifier — treat entire token as plain word
			name = text
		}
	}

	if name == "" {
		return engine.Value{}, fmt.Errorf("empty word")
	}

	if forcePrefix || forceSuffix || argCount >= 0 {
		return engine.NewWordModified(name, argCount, forcePrefix, forceSuffix), nil
	}
	return engine.NewWord(name), nil
}
