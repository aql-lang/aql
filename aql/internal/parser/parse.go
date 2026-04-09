package parser

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	jsonic "github.com/jsonicjs/jsonic/go"
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// typeNames is derived from the engine's canonical registry to prevent drift.
var typeNames = engine.TypeNameTable()

func boolPtr(b bool) *bool { return &b }

// isWhitespace returns true if the byte is a whitespace character.
func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// parenGroup represents items collected between ( and ) by the jsonic grammar.
// At the top level, these are expanded to engine paren markers. In data context
// (map values), they become ParenExpr values for inline evaluation.
type parenGroup []any

// unclosedParen wraps a parenGroup that was auto-closed at EOF (no matching `)`).
// The converter produces an error for these.
type unclosedParen struct{ items []any }

// interpGroup represents the parts collected between backticks by the interp
// grammar rule. Each element is either a jsonic.Text{Quote:"tl"} (literal
// segment) or an iexprGroup (interpolated expression).
type interpGroup []any

// iexprGroup represents the values collected between ${ and } by the iexpr
// grammar rule. Contains raw jsonic values that will be converted to engine
// values by the converter.
type iexprGroup []any

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

	// Stage 1: Lex setup — register tokens and custom matchers.
	t := setupBaseTokens(j)
	setupTemplateLiteralMatcher(j, t)

	// Stage 2: Grammar setup — extend rules for AQL syntax.
	setupValRule(j, t)
	setupPairGrammar(j, t)
	setupParenGrammar(j, t)
	setupInterpGrammar(j, t)
	setupNumberSub(j)

	// Stage 3: Parse and convert to engine values.
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
		mv, err := convertMapData(val.Val, val.Implicit, val.Meta)
		if err != nil {
			return nil, err
		}
		// Top-level implicit maps (e.g. entire input is "a:x") must be
		// auto-evaluated so expressions in values resolve.
		if val.Implicit && !mv.Eval {
			mv.Eval = true
		}
		return []engine.Value{mv}, nil
	case unclosedParen:
		return nil, engine.MakeAqlError("syntax_error", "unmatched opening parenthesis", "(", src, "")

	case parenGroup:
		// Single paren group at top level: expand to paren markers.
		return convertTopLevelItems([]any{val})

	case interpGroup:
		// Single template string at top level.
		iv, err := convertInterpGroup(val)
		if err != nil {
			return nil, err
		}
		return []engine.Value{iv}, nil

	default:
		v, err := convertTopLevelValue(val)
		if err != nil {
			return nil, err
		}
		return []engine.Value{v}, nil
	}
}

// isToken checks if item is an unquoted text marker matching the given string.
// Quoted text (e.g. "." or "!") has Quote != "" and is handled as a string
// by convertTopLevelValue, so it never reaches the token checks.
func isToken(item any, tok string) bool {
	text, ok := item.(jsonic.Text)
	return ok && text.Str == tok && text.Quote == ""
}

// convertTopLevelItems converts a list of jsonic items in word context,
// handling parenthesis markers and token sequences. The . and ! tokens
// are converted to "get" and "getr" words respectively:
//   - "." → get
//   - "!" "." → getr (the ! is consumed together with the following .)
//
// All other items are converted to engine values directly.
func convertTopLevelItems(items []any) ([]engine.Value, error) {
	values := make([]engine.Value, 0, len(items))
	for i := 0; i < len(items); i++ {
		// "!" followed by "." → getr word.
		if isToken(items[i], "!") && i+1 < len(items) && isToken(items[i+1], ".") {
			values = append(values, engine.NewWord("getr"))
			i++ // skip the dot
			continue
		}

		// "." → get word.
		if isToken(items[i], ".") {
			values = append(values, engine.NewWord("get"))
			continue
		}

		// Unclosed paren: error at parse time.
		if up, ok := items[i].(unclosedParen); ok {
			_ = up
			return nil, engine.MakeAqlError("syntax_error", "unmatched opening parenthesis", "(", "", "")
		}

		// Paren group: expand to engine paren markers at top level.
		if pg, ok := items[i].(parenGroup); ok {
			values = append(values, engine.NewWord("("))
			inner, err := convertTopLevelItems([]any(pg))
			if err != nil {
				return nil, err
			}
			values = append(values, inner...)
			values = append(values, engine.NewWord(")"))
			continue
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

	case interpGroup:
		return convertInterpGroup(val)

	case numberVal:
		return numberValToValue(val), nil

	case float64:
		return floatToValue(val), nil

	case jsonic.MapRef:
		if hasMapChild(val.Val) {
			return convertTypedMap(val.Val)
		}
		mv, err := convertMapData(val.Val, val.Implicit, val.Meta)
		if err != nil {
			return mv, err
		}
		// In word context (top level), implicit maps from pair syntax
		// (e.g. a:x) must be auto-evaluated so expressions resolve.
		if val.Implicit && !mv.Eval {
			mv.Eval = true
		}
		return mv, nil

	case map[string]any:
		// Raw map from list.pair syntax (e.g., [x:number] produces
		// map[string]any{"x": Text("number")} inside the list).
		if hasMapChild(val) {
			return convertTypedMap(val)
		}
		mv, err := convertMapData(val, true)
		if err != nil {
			return mv, err
		}
		// In word context (top level), implicit maps from pair syntax
		// must be auto-evaluated so expressions resolve.
		mv.Eval = true
		return mv, nil

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
// The resulting list is marked for auto-evaluation: its contents will
// be executed at the end of Run unless quoted or consumed by a word.
func convertWordList(items []any) (engine.Value, error) {
	elems, err := convertTopLevelItems(items)
	if err != nil {
		return engine.Value{}, err
	}
	return engine.NewEvalList(elems), nil
}

// convertMapData converts a map in data context. All text values are
// scalar data regardless of quoting. When implicit is true the
// resulting OrderedMap is marked as coming from pair syntax (e.g.,
// [x:Integer] rather than {x:Integer}).
// Explicit maps are marked for auto-evaluation (Eval=true).
// The optional meta parameter receives MapRef.Meta for optional field detection.
func convertMapData(m map[string]any, implicit bool, meta ...map[string]any) (engine.Value, error) {
	om := engine.NewOrderedMap()
	if implicit {
		om.Implicit = true
	}
	// Extract metadata from MapRef.Meta.
	var qmSet map[string]bool // optional keys (? syntax)
	var ckSet map[string]bool // computed keys ([key] syntax)
	if len(meta) > 0 && meta[0] != nil {
		qmSet, _ = meta[0]["qm"].(map[string]bool)
		ckSet, _ = meta[0]["ck"].(map[string]bool)
	}
	for _, key := range sortedKeys(m) {
		child, err := convertDataValue(m[key])
		if err != nil {
			return engine.Value{}, err
		}
		// Optional field: wrap value as (value or None).
		optional := qmSet[key]
		realKey := key
		if strings.HasSuffix(key, "?") {
			realKey = strings.TrimSuffix(key, "?")
			optional = true
		}
		if optional {
			child = engine.NewDisjunct([]engine.Value{
				child,
				engine.NewTypeLiteral(engine.TNone),
			})
		}
		om.Set(realKey, child)
	}
	// Propagate computed keys to OrderedMap.Meta for autoEvalMap.
	if len(ckSet) > 0 {
		if om.Meta == nil {
			om.Meta = make(map[string]any)
		}
		om.Meta["ck"] = ckSet
	}
	// Explicit maps (from {...} syntax) are marked for auto-evaluation.
	// Implicit maps (from pair syntax [x:Integer]) are structural and not evaluated.
	if !implicit {
		return engine.NewEvalMap(om), nil
	}
	return engine.NewMap(om), nil
}

// convertDataValue converts a value in data context (inside maps).
// Quoted text → strings, unquoted text → words (executable).
func convertDataValue(v any) (engine.Value, error) {
	switch val := v.(type) {
	case jsonic.Text:
		if val.Quote != "" {
			// Quoted text (e.g. "hello") → string
			return engine.NewString(val.Str), nil
		}
		// Unquoted text → word (same as top-level word context).
		// This allows map values like {r:rv} to evaluate rv.
		return parseWord(val.Str)

	case interpGroup:
		return convertInterpGroup(val)

	case numberVal:
		return numberValToValue(val), nil

	case float64:
		return floatToValue(val), nil

	case jsonic.MapRef:
		if hasMapChild(val.Val) {
			return convertTypedMap(val.Val)
		}
		return convertMapData(val.Val, val.Implicit, val.Meta)

	case map[string]any:
		if hasMapChild(val) {
			return convertTypedMap(val)
		}
		return convertMapData(val, true)

	case jsonic.ListRef:
		if val.Child != nil {
			return convertTypedList(val)
		}
		return convertDataList(val.Val)

	case unclosedParen:
		return engine.Value{}, engine.MakeAqlError("syntax_error", "unmatched opening parenthesis", "(", "", "")

	case parenGroup:
		// Paren group in data context: convert items in word context
		// and wrap as a ParenExpr for inline evaluation by autoEvalMap.
		items, err := convertTopLevelItems([]any(val))
		if err != nil {
			return engine.Value{}, err
		}
		return engine.NewParenExpr(items), nil

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
// Lists use word context and are marked for auto-evaluation.
func convertDataList(items []any) (engine.Value, error) {
	elems, err := convertTopLevelItems(items)
	if err != nil {
		return engine.Value{}, err
	}
	return engine.NewEvalList(elems), nil
}

// resolveTextValue converts a bare text string into the appropriate
// AQL value — type literal, boolean, or atom.
// Unquoted text is never a string; only quoted text produces strings.
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
	if t, ok := engine.ResolveTypePath(text); ok {
		return engine.NewTypeLiteral(t)
	}
	return engine.NewAtom(text)
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
// modifier syntax: name/f (forceForward), name/s (forceStack), name/N (argCount),
// and combinations like name/1f or name/2s.
func parseWord(text string) (engine.Value, error) {
	name := text
	argCount := -1
	forceStack := false
	forceForward := false

	// Check for /... modifier suffix.
	if idx := strings.LastIndex(name, "/"); idx >= 0 && idx < len(name)-1 {
		mod := name[idx+1:]
		name = name[:idx]

		// Parse optional digits followed by optional 'f' or 's'.
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
		case "f":
			forceForward = true
		case "s":
			forceStack = true
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

	if forceStack || forceForward || argCount >= 0 {
		return engine.NewWordModified(name, argCount, forceStack, forceForward), nil
	}

	// Type names resolve to type literals even in word context, so that
	// they retain their meaning inside quotations (e.g. [String,Decimal]).
	if t, ok := typeNames[name]; ok {
		return engine.NewTypeLiteral(t), nil
	}
	if t, ok := engine.ResolveTypePath(name); ok {
		return engine.NewTypeLiteral(t), nil
	}

	return engine.NewWord(name), nil
}

// convertInterpGroup converts an interpGroup (produced by the interp/ielem/iexpr
// jsonic rules) into an engine InterpString value, or a plain string if there
// are no expression parts.
func convertInterpGroup(grp interpGroup) (engine.Value, error) {
	if len(grp) == 0 {
		return engine.NewString(""), nil
	}
	var parts []engine.InterpPart
	hasExpr := false
	for _, item := range grp {
		switch v := item.(type) {
		case jsonic.Text:
			// Template literal segment (Quote="tl").
			parts = append(parts, engine.InterpPart{Lit: v.Str})
		case iexprGroup:
			hasExpr = true
			exprVals, err := convertTopLevelItems([]any(v))
			if err != nil {
				return engine.Value{}, fmt.Errorf("interpolation expression error: %w", err)
			}
			parts = append(parts, engine.InterpPart{Expr: exprVals})
		default:
			return engine.Value{}, fmt.Errorf("unexpected interp part type %T", item)
		}
	}
	if !hasExpr {
		// No interpolations — just concatenate literals into a plain string.
		var buf strings.Builder
		for _, p := range parts {
			buf.WriteString(p.Lit)
		}
		return engine.NewString(buf.String()), nil
	}
	return engine.NewInterpString(parts), nil
}

// processTemplateEscapes processes escape sequences in template literal text.
func processTemplateEscapes(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case 'r':
				buf.WriteByte('\r')
			case '\\':
				buf.WriteByte('\\')
			case '`':
				buf.WriteByte('`')
			case '$':
				buf.WriteByte('$')
			default:
				// Unknown escape: keep as-is.
				buf.WriteByte('\\')
				buf.WriteByte(next)
			}
			i++ // skip the escaped char
		} else {
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
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
