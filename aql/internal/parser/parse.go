package parser

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	jsonic "github.com/jsonicjs/jsonic/go"
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// typeNames maps well-known type names to their engine Type.
var typeNames = map[string]engine.Type{
	"Any":       engine.TAny,
	"None":      engine.TNone,
	"Scalar":    engine.TScalar,
	"Number":    engine.TNumber,
	"Integer":   engine.TInteger,
	"Decimal":   engine.TDecimal,
	"String":    engine.TString,
	"Boolean":   engine.TBoolean,
	"Atom":      engine.TAtom,
	"Node":      engine.TNode,
	"List":      engine.TList,
	"Map":       engine.TMap,
	"Table":     engine.TTable,
	"Record":    engine.TRecord,
	"Object":    engine.TObject,
}

func boolPtr(b bool) *bool { return &b }

// isWhitespace returns true if the byte is a whitespace character.
func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// Parse tokenizes the AQL source string into a slice of engine.Value.
// The input is treated as a top-level implicit list: jsonic.Parse handles
// the entire source. The TextInfo option distinguishes quoted strings from
// unquoted text (words).
//
// Custom tokens are registered for (, ), and . so that they are lexed as
// separate tokens by jsonic. This replaces the earlier preprocessParens
// approach and string-based dot expansion, making the parser cleaner.
//
// Context rules:
//   - Top level: unquoted text → words, quoted text → strings.
//   - Inside maps (including implicit): all text → scalar data.
//   - Inside lists at the top level: unquoted text → words (quotation).
//   - Inside lists inside maps: all text → scalar data.
func Parse(src string) ([]engine.Value, error) {
	j := jsonic.Make(jsonic.Options{
		TextInfo: boolPtr(true),
		ListRef:  boolPtr(true),
		MapRef:   boolPtr(true),
		List:     &jsonic.ListOptions{Pair: boolPtr(true), Child: boolPtr(true)},
		Map:      &jsonic.MapOptions{Child: boolPtr(true)},
		Value:    &jsonic.ValueOptions{Lex: boolPtr(false)},
	})

	// Register ( ) . as separate fixed tokens so jsonic lexes them
	// independently, even when adjacent to other text (e.g. "(foo" → "(" + "foo").
	TinOP := j.Token("#OP", "(")
	TinCP := j.Token("#CP", ")")
	TinDT := j.Token("#DT", ".")

	// Add val rule alternates so the grammar recognizes these custom tokens
	// and produces Text marker values that the converter layer processes.
	//
	// For the dot token, we use source position to distinguish adjacent dots
	// (foo.bar → part of a dotted word) from space-separated dots
	// (foo . bar → standalone dot operator). Adjacent dots use Quote="adj"
	// so the converter can identify them.
	j.Rule("val", func(rs *jsonic.RuleSpec) {
		rs.Open = append([]*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{TinOP}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: "(", Quote: ""}
			}},
			{S: [][]jsonic.Tin{{TinCP}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: ")", Quote: ""}
			}},
			{S: [][]jsonic.Tin{{TinDT}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				si := r.O0.SI
				leftAdj := si > 0 && !isWhitespace(src[si-1])
				rightAdj := si+1 < len(src) && !isWhitespace(src[si+1])
				if leftAdj || rightAdj {
					// Adjacent dot: part of a dotted word (e.g. foo.bar, .bar, foo.)
					r.Node = jsonic.Text{Str: ".", Quote: "adj"}
				} else {
					// Standalone dot: the dot operator with spaces around it
					r.Node = jsonic.Text{Str: ".", Quote: ""}
				}
			}},
		}, rs.Open...)
	})

	// Intercept number tokens at lex time: wrap float64 values in numberVal
	// so the converter can distinguish "5" (integer) from "5.0" (decimal).
	j.Sub(func(tkn *jsonic.Token, rule *jsonic.Rule, ctx *jsonic.Context) {
		if tkn.Tin == jsonic.TinNR && strings.Contains(tkn.Src, ".") {
			tkn.Val = numberVal{Val: tkn.Val.(float64), Src: tkn.Src}
		}
	}, nil)

	result, err := j.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	if result == nil {
		return nil, nil
	}

	// With ListRef and MapRef enabled, jsonic returns ListRef/MapRef for
	// all lists and maps. ListRef.Implicit and MapRef.Implicit distinguish
	// implicit structures from explicit ones.
	switch val := result.(type) {
	case jsonic.ListRef:
		if val.Child != nil {
			tv, err := convertTypedList(val)
			if err != nil {
				return nil, err
			}
			return []engine.Value{tv}, nil
		}
		if !val.Implicit {
			// Explicit list [...]  — a single list value (quotation).
			lv, err := convertWordList(val.Val)
			if err != nil {
				return nil, err
			}
			return []engine.Value{lv}, nil
		}
		// Implicit list — top-level stack values.
		return convertTopLevel(val.Val)
	case jsonic.MapRef:
		if hasMapChild(val.Val) {
			tv, err := convertTypedMap(val.Val)
			if err != nil {
				return nil, err
			}
			return []engine.Value{tv}, nil
		}
		mv, err := convertMapData(val.Val)
		if err != nil {
			return nil, err
		}
		return []engine.Value{mv}, nil
	default:
		v, err := convertTopLevelValue(val)
		if err != nil {
			return nil, err
		}
		return []engine.Value{v}, nil
	}
}

// isDotMarker returns true if item is an adjacent dot marker (part of a
// dotted word like foo.bar). Standalone dots (with spaces) have Quote=""
// and are handled separately.
func isDotMarker(item any) bool {
	text, ok := item.(jsonic.Text)
	return ok && text.Str == "." && text.Quote == "adj"
}

// isStandaloneDot returns true if item is a standalone dot operator
// (space-separated, e.g. "foo . bar" or just ".").
func isStandaloneDot(item any) bool {
	text, ok := item.(jsonic.Text)
	return ok && text.Str == "." && text.Quote == ""
}

// isTextOrNumber returns true if item is an unquoted text token or a number,
// i.e. something that can appear as a segment in a dotted word.
func isTextOrNumber(item any) bool {
	if text, ok := item.(jsonic.Text); ok {
		return text.Quote == ""
	}
	switch item.(type) {
	case float64, numberVal:
		return true
	}
	return false
}

// itemToString returns the string representation of a dot segment item.
func itemToString(item any) string {
	switch v := item.(type) {
	case jsonic.Text:
		return v.Str
	case float64:
		if v == float64(int64(v)) && !math.IsInf(v, 0) && !math.IsNaN(v) {
			return strconv.FormatInt(int64(v), 10)
		}
		return fmt.Sprintf("%v", v)
	case numberVal:
		return v.Src
	default:
		return fmt.Sprintf("%v", v)
	}
}

// collectDotString scans items starting at start, collecting a dot-separated
// sequence of tokens (text/number separated by dot markers). Returns the
// joined dotted string and the number of items consumed.
//
// Examples:
//
//	Text"foo", dot, Text"bar"             → "foo.bar", 3
//	dot, Text"bar"                        → ".bar", 2
//	Text"!", dot                          → "!.", 2
//	Text"foo", dot, Number(0), dot, Text"x" → "foo.0.x", 5
func collectDotString(items []any, start int) (string, int) {
	var parts []string
	i := start

	// Handle leading dot.
	if isDotMarker(items[i]) {
		parts = append(parts, "") // empty first part → leading dot
		i++
	}

	for i < len(items) {
		// Expect text or number.
		if !isTextOrNumber(items[i]) {
			break
		}
		parts = append(parts, itemToString(items[i]))
		i++

		// Check for trailing dot.
		if i < len(items) && isDotMarker(items[i]) {
			i++ // consume dot, continue to next part
		} else {
			break
		}
	}

	// Handle trailing dot (e.g. "!." → parts=["!", ""])
	if i > start && isDotMarker(items[i-1]) {
		parts = append(parts, "")
	}

	return strings.Join(parts, "."), i - start
}

// convertTopLevelItems converts a list of jsonic items in word context,
// handling dot-separated sequences and parenthesis markers. This is the
// shared implementation for both convertTopLevel and convertWordList.
func convertTopLevelItems(items []any) ([]engine.Value, error) {
	values := make([]engine.Value, 0, len(items))
	for i := 0; i < len(items); i++ {
		// Standalone dot (space-separated) → the dot word.
		if isStandaloneDot(items[i]) {
			values = append(values, engine.NewWord("dot"))
			continue
		}

		// Adjacent dot marker: start of a leading-dot sequence (.bar, .bar.baz).
		if isDotMarker(items[i]) {
			if i+1 < len(items) && isTextOrNumber(items[i+1]) {
				dotStr, consumed := collectDotString(items, i)
				expanded, err := expandDottedWord(dotStr)
				if err != nil {
					return nil, err
				}
				values = append(values, expanded...)
				i += consumed - 1
				continue
			}
			// Adjacent dot with nothing after → treat as dot word.
			values = append(values, engine.NewWord("dot"))
			continue
		}

		// Unquoted text potentially followed by an adjacent dot.
		if text, ok := items[i].(jsonic.Text); ok && text.Quote == "" {
			if i+1 < len(items) && isDotMarker(items[i+1]) {
				// "!" followed by adjacent dot with nothing useful after → dotr.
				if text.Str == "!" && (i+2 >= len(items) || !isTextOrNumber(items[i+2])) {
					values = append(values, engine.NewWord("dotr"))
					i++ // skip the dot marker
					continue
				}
				// Regular dotted word: foo.bar, foo.bar.baz, etc.
				dotStr, consumed := collectDotString(items, i)
				expanded, err := expandDottedWord(dotStr)
				if err != nil {
					return nil, err
				}
				values = append(values, expanded...)
				i += consumed - 1
				continue
			}
		}

		v, err := convertTopLevelValue(items[i])
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, nil
}

// convertTopLevel converts a top-level implicit list from jsonic into
// a slice of engine.Value using word context.
func convertTopLevel(items []any) ([]engine.Value, error) {
	return convertTopLevelItems(items)
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

	case numberVal:
		return numberValToValue(val), nil

	case float64:
		return floatToValue(val), nil

	case jsonic.MapRef:
		if hasMapChild(val.Val) {
			return convertTypedMap(val.Val)
		}
		return convertMapData(val.Val)

	case map[string]any:
		// Raw map from list.pair syntax (e.g., [x:number] produces
		// map[string]any{"x": Text("number")} inside the list).
		if hasMapChild(val) {
			return convertTypedMap(val)
		}
		return convertMapData(val)

	case jsonic.ListRef:
		if val.Child != nil {
			return convertTypedList(val)
		}
		return convertWordList(val.Val)

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
	elems, err := convertTopLevelItems(items)
	if err != nil {
		return engine.Value{}, err
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

	case numberVal:
		return numberValToValue(val), nil

	case float64:
		return floatToValue(val), nil

	case jsonic.MapRef:
		if hasMapChild(val.Val) {
			return convertTypedMap(val.Val)
		}
		return convertMapData(val.Val)

	case map[string]any:
		if hasMapChild(val) {
			return convertTypedMap(val)
		}
		return convertMapData(val)

	case jsonic.ListRef:
		if val.Child != nil {
			return convertTypedList(val)
		}
		return convertDataList(val.Val)

	case bool:
		return engine.NewBoolean(val), nil

	case nil:
		return engine.NewTypeLiteral(engine.TNone), nil

	default:
		return engine.Value{}, fmt.Errorf("unsupported value type %T", v)
	}
}

// convertTypedList converts a ListRef with a Child into a typed list value.
// The child value is converted in data context (type names resolve to type literals).
func convertTypedList(lr jsonic.ListRef) (engine.Value, error) {
	childVal, err := convertDataValue(lr.Child)
	if err != nil {
		return engine.Value{}, err
	}
	return engine.NewTypedList(childVal), nil
}

// hasMapChild reports whether a jsonic map contains the "child$" key
// set by the map.child option (bare colon syntax {:value}).
func hasMapChild(m map[string]any) bool {
	_, ok := m["child$"]
	return ok
}

// convertTypedMap converts a map with a "child$" key into a typed map value.
// The child value is converted in data context (type names resolve to type literals).
func convertTypedMap(m map[string]any) (engine.Value, error) {
	childVal, err := convertDataValue(m["child$"])
	if err != nil {
		return engine.Value{}, err
	}
	return engine.NewTypedMap(childVal), nil
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

	// Type names resolve to type literals even in word context, so that
	// they retain their meaning inside quotations (e.g. [String,Decimal]).
	if t, ok := typeNames[name]; ok {
		return engine.NewTypeLiteral(t), nil
	}

	return engine.NewWord(name), nil
}

// expandDottedWord expands dot notation like "foo.a.b" into a sequence of
// engine values: [( foo a dot b dot )]. The first segment is emitted as a
// plain word, resolved by the engine (def lookup, registered function, or
// atom). Each subsequent segment extracts a key using the "dot" word.
// A standalone "." becomes the "dot" word.
// Leading dot (e.g. ".a.b") omits the first word and emits [a, dot, b, dot],
// operating on whatever value is already on the stack (no paren wrapping).
//
// The entire multi-token expansion is wrapped in parentheses so that it
// evaluates as a single sub-expression. This gives dot very high binding,
// letting suffix-precedence words like "list" consume the result:
// list foo.a → list ( foo a dot ).
func expandDottedWord(text string) ([]engine.Value, error) {
	// Standalone "." → just the dot word.
	if text == "." {
		return []engine.Value{engine.NewWord("dot")}, nil
	}

	// Standalone "!." → just the dotr word.
	if text == "!." {
		return []engine.Value{engine.NewWord("dotr")}, nil
	}

	parts := strings.Split(text, ".")
	var inner []engine.Value

	// First part (before first dot): emit as a plain word.
	// The engine resolves it naturally — def, registered function, or atom.
	leadingDot := parts[0] == ""
	if !leadingDot {
		w, err := parseWord(parts[0])
		if err != nil {
			return nil, err
		}
		inner = append(inner, w)
	}

	// Subsequent parts (after each dot): emit key then dot (prefix).
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		// Integer keys for list access.
		if n, err := strconv.ParseInt(part, 10, 64); err == nil {
			inner = append(inner, engine.NewInteger(n))
		} else {
			inner = append(inner, engine.NewWord(part))
		}
		inner = append(inner, engine.NewWordModified("dot", -1, true, false))
	}

	// Leading dot operates on whatever is already on the stack,
	// so don't wrap in parens (it needs the stack value).
	if leadingDot {
		return inner, nil
	}

	// Wrap in parentheses so the expression evaluates as a unit.
	result := make([]engine.Value, 0, len(inner)+2)
	result = append(result, engine.NewOpenParen())
	result = append(result, inner...)
	result = append(result, engine.NewWord(")"))
	return result, nil
}

// numberVal wraps a float64 with source text so we can distinguish
// integer literals (e.g. "5") from decimal literals (e.g. "5.0").
// Injected by the jsonic LexSub callback when the source contains a ".".
type numberVal struct {
	Val float64
	Src string
}

// floatToValue converts a JSON float64 to the appropriate AQL numeric value.
// Whole numbers become integers; fractional values become decimals.
func floatToValue(f float64) engine.Value {
	if f == float64(int64(f)) && !math.IsInf(f, 0) && !math.IsNaN(f) {
		return engine.NewInteger(int64(f))
	}
	return engine.NewDecimal(f)
}

// numberValToValue converts a numberVal (float64 + source) to the appropriate
// AQL numeric value. If the source text contains a ".", the value is always
// treated as a decimal — even for whole numbers like 5.0.
func numberValToValue(nv numberVal) engine.Value {
	if strings.Contains(nv.Src, ".") {
		return engine.NewDecimal(nv.Val)
	}
	return floatToValue(nv.Val)
}
