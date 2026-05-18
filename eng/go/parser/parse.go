package parser

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/aql-lang/aql/eng/go"
	jsonic "github.com/jsonicjs/jsonic/go"
)

// typeNames is derived from the engine's canonical registry to prevent drift.
var typeNames = eng.TypeNameTable()

func boolPtr(b bool) *bool { return &b }

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

// Parse tokenizes the AQL source string into a slice of eng.Value.
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
func Parse(src string) ([]eng.Value, error) {
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
			return []eng.Value{tv}, nil
		}
		if !val.Implicit {
			// Explicit list [...]  — a single list value (quotation).
			lv, err := convertWordList(val.Val)
			if err != nil {
				return nil, err
			}
			return []eng.Value{lv}, nil
		}
		// Implicit list — top-level stack values.
		return convertTopLevel(val.Val)
	case jsonic.MapRef:
		if hasMapChild(val.Val) {
			tv, err := convertTypedMap(val.Val)
			if err != nil {
				return nil, err
			}
			return []eng.Value{tv}, nil
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
		return []eng.Value{mv}, nil
	case unclosedParen:
		return nil, eng.MakeAqlError("syntax_error", "unmatched opening parenthesis", "(", src, "")

	case parenGroup:
		// Single paren group at top level: expand to paren markers.
		return convertTopLevelItems([]any{val})

	case interpGroup:
		// Single template string at top level.
		iv, err := convertInterpGroup(val)
		if err != nil {
			return nil, err
		}
		return []eng.Value{iv}, nil

	default:
		v, err := convertTopLevelValue(val)
		if err != nil {
			return nil, err
		}
		return []eng.Value{v}, nil
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
func convertTopLevelItems(items []any) ([]eng.Value, error) {
	values := make([]eng.Value, 0, len(items))
	for i := 0; i < len(items); i++ {
		// "!" followed by "." → getr word.
		if isToken(items[i], "!") && i+1 < len(items) && isToken(items[i+1], ".") {
			values = append(values, eng.NewWord("getr"))
			i++ // skip the dot
			continue
		}

		// "." → get word.
		if isToken(items[i], ".") {
			values = append(values, eng.NewWord("get"))
			continue
		}

		// Unclosed paren: error at parse time.
		if up, ok := items[i].(unclosedParen); ok {
			_ = up
			return nil, eng.MakeAqlError("syntax_error", "unmatched opening parenthesis", "(", "", "")
		}

		// Paren group: expand to engine paren markers at top level.
		// Emit typed OpenParen / CloseParen values directly so the
		// engine can recognise them by VType identity (no stepWord
		// name-dispatch needed).
		if pg, ok := items[i].(parenGroup); ok {
			values = append(values, eng.NewOpenParen())
			inner, err := convertTopLevelItems([]any(pg))
			if err != nil {
				return nil, err
			}
			values = append(values, inner...)
			values = append(values, eng.NewCloseParen())
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
// a slice of eng.Value using word context.
func convertTopLevel(items []any) ([]eng.Value, error) {
	return convertTopLevelItems(items)
}

// convertTopLevelValue converts a single value in word context.
// Unquoted text → word, quoted text → string.
func convertTopLevelValue(v any) (eng.Value, error) {
	switch val := v.(type) {
	case jsonic.Text:
		if val.Quote == "" {
			return parseWord(val.Str)
		}
		return eng.NewString(val.Str), nil

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
		return eng.NewBoolean(val), nil

	case nil:
		// JSON null → Atom("null"). The unique inhabitant of None is
		// spelled `none` (a separate keyword); `null` is the JSON-null
		// atom at the value level.
		return eng.NewAtom("null"), nil

	default:
		return eng.Value{}, fmt.Errorf("unsupported value type %T", v)
	}
}

// convertWordList converts a list in word context (top-level list).
// The resulting list is marked for auto-evaluation: its contents will
// be executed at the end of Run unless quoted or consumed by a word.
func convertWordList(items []any) (eng.Value, error) {
	elems, err := convertTopLevelItems(items)
	if err != nil {
		return eng.Value{}, err
	}
	return eng.NewEvalList(elems), nil
}

// convertMapData converts a map in data context. All text values are
// scalar data regardless of quoting. When implicit is true the
// resulting OrderedMap is marked as coming from pair syntax (e.g.,
// [x:Integer] rather than {x:Integer}).
// Explicit maps are marked for auto-evaluation (Eval=true).
// The optional meta parameter receives MapRef.Meta for optional field detection.
func convertMapData(m map[string]any, implicit bool, meta ...map[string]any) (eng.Value, error) {
	om := eng.NewOrderedMap()
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
			return eng.Value{}, err
		}
		// Optional field: wrap value as (value or None).
		optional := qmSet[key]
		realKey := key
		if strings.HasSuffix(key, "?") {
			realKey = strings.TrimSuffix(key, "?")
			optional = true
		}
		if optional {
			child = eng.NewDisjunct([]eng.Value{
				child,
				eng.NewTypeLiteral(eng.TNone),
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
		return eng.NewEvalMap(om), nil
	}
	return eng.NewMap(om), nil
}

// convertDataValue converts a value in data context (inside maps).
// Quoted text → strings, unquoted text → words (executable).
func convertDataValue(v any) (eng.Value, error) {
	switch val := v.(type) {
	case jsonic.Text:
		if val.Quote != "" {
			// Quoted text (e.g. "hello") → string
			return eng.NewString(val.Str), nil
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
		return eng.Value{}, eng.MakeAqlError("syntax_error", "unmatched opening parenthesis", "(", "", "")

	case parenGroup:
		// Paren group in data context: convert items in word context
		// and wrap as a ParenExpr for inline evaluation by autoEvalMap.
		items, err := convertTopLevelItems([]any(val))
		if err != nil {
			return eng.Value{}, err
		}
		return eng.NewParenExpr(items), nil

	case bool:
		return eng.NewBoolean(val), nil

	case nil:
		// JSON null → Atom("null"). The unique inhabitant of None is
		// spelled `none` (a separate keyword); `null` is the JSON-null
		// atom at the value level.
		return eng.NewAtom("null"), nil

	default:
		return eng.Value{}, fmt.Errorf("unsupported value type %T", v)
	}
}

// convertTypedList converts a ListRef with a Child into a typed list value.
// The child value is converted in data context (type names resolve to type literals).
//
// When lr.Val carries concrete elements alongside the child constraint
// (`[v0 :T v1]`), each element is converted in data context and
// retained on the resulting Value's ChildTypeInfo.Elements; the
// runtime `is` validates them against Child on demand.
func convertTypedList(lr jsonic.ListRef) (eng.Value, error) {
	childVal, err := convertDataValue(lr.Child)
	if err != nil {
		return eng.Value{}, err
	}
	if len(lr.Val) == 0 {
		return eng.NewTypedList(childVal), nil
	}
	elems := make([]eng.Value, 0, len(lr.Val))
	for _, item := range lr.Val {
		ev, err := convertDataValue(item)
		if err != nil {
			return eng.Value{}, err
		}
		elems = append(elems, ev)
	}
	return eng.NewTypedListWithElements(childVal, elems), nil
}

// hasMapChild reports whether a jsonic map contains the "child$" key
// set by the map.child option (bare colon syntax {:value}).
func hasMapChild(m map[string]any) bool {
	_, ok := m["child$"]
	return ok
}

// convertTypedMap converts a map with a "child$" key into a typed
// map value. The child value is converted in data context (type
// names resolve to type literals).
//
// When the source carries concrete entries alongside the child
// constraint (`{k:v :T}`), each entry is converted in data context
// and retained on the resulting Value's ChildTypeInfo.Entries; the
// runtime `is` validates each entry's value against Child on demand.
func convertTypedMap(m map[string]any) (eng.Value, error) {
	childVal, err := convertDataValue(m["child$"])
	if err != nil {
		return eng.Value{}, err
	}
	// Collect non-`child$` entries as concrete values.
	keys := make([]string, 0, len(m))
	for k := range m {
		if k == "child$" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return eng.NewTypedMap(childVal), nil
	}
	sort.Strings(keys)
	entries := make([]eng.ChildEntry, 0, len(keys))
	for _, k := range keys {
		ev, err := convertDataValue(m[k])
		if err != nil {
			return eng.Value{}, err
		}
		entries = append(entries, eng.ChildEntry{Key: k, Value: ev})
	}
	return eng.NewTypedMapWithEntries(childVal, entries), nil
}

// convertDataList converts a list in data context (inside maps).
// Lists use word context and are marked for auto-evaluation.
func convertDataList(items []any) (eng.Value, error) {
	elems, err := convertTopLevelItems(items)
	if err != nil {
		return eng.Value{}, err
	}
	return eng.NewEvalList(elems), nil
}

// resolveTextValue converts a bare text string into the appropriate
// AQL value — type literal, boolean, or atom.
// Unquoted text is never a string; only quoted text produces strings.
func resolveTextValue(text string) eng.Value {
	if text == "true" {
		return eng.NewBoolean(true)
	}
	if text == "false" {
		return eng.NewBoolean(false)
	}
	if t, ok := typeNames[text]; ok {
		return eng.NewTypeLiteral(t)
	}
	if t, ok := eng.ResolveTypePath(text); ok {
		return eng.NewTypeLiteral(t)
	}
	return eng.NewAtom(text)
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
// name/q (quote → Atom), and combinations like name/1f, name/qs, name/f2.
// Modifiers stack in any order; f and s are mutually exclusive; the argCount
// digits form a single number; q produces an Atom and overrides the other
// modifiers (they are accepted syntactically but ignored).
func parseWord(text string) (eng.Value, error) {
	name := text
	argCount := -1
	forceStack := false
	forceForward := false
	quoteFlag := false

	// Check for /... modifier suffix.
	if idx := strings.LastIndex(name, "/"); idx >= 0 && idx < len(name)-1 {
		mod := name[idx+1:]
		baseName := name[:idx]

		// Scan modifier chars in any order: digits, 'f', 's', 'q'.
		// Each letter appears at most once; f/s are mutually exclusive;
		// digits run contiguously and form a single argCount value.
		valid := true
		seenDigits := false
		i := 0
		for i < len(mod) {
			c := mod[i]
			switch {
			case c >= '0' && c <= '9':
				if seenDigits {
					valid = false
				} else {
					j := i
					for j < len(mod) && mod[j] >= '0' && mod[j] <= '9' {
						j++
					}
					n, err := strconv.Atoi(mod[i:j])
					if err != nil || n < 0 {
						valid = false
					} else {
						argCount = n
						seenDigits = true
					}
					i = j
					continue
				}
			case c == 'f':
				if forceForward || forceStack {
					valid = false
				} else {
					forceForward = true
				}
			case c == 's':
				if forceForward || forceStack {
					valid = false
				} else {
					forceStack = true
				}
			case c == 'q':
				if quoteFlag {
					valid = false
				} else {
					quoteFlag = true
				}
			default:
				valid = false
			}
			if !valid {
				break
			}
			i++
		}

		if valid {
			name = baseName
		} else {
			// Unrecognized / malformed modifier — treat entire token as plain word.
			argCount = -1
			forceStack = false
			forceForward = false
			quoteFlag = false
		}
	}

	if name == "" {
		return eng.Value{}, fmt.Errorf("empty word")
	}

	// /q produces an Atom: equivalent to (quote name). Other modifiers
	// in the same suffix are accepted but ignored — the atom is data,
	// not a function call.
	if quoteFlag {
		return eng.NewAtom(name), nil
	}

	if forceStack || forceForward || argCount >= 0 {
		return eng.NewWordModified(name, argCount, forceStack, forceForward), nil
	}

	// `none` is the unique inhabitant of None — a value, not a type.
	// `null` is the JSON-null atom (handled in data context via `case
	// nil:`; here in word context bare `null` falls through as a Word
	// for engine-side resolution).
	if name == "none" {
		return eng.NewNone(), nil
	}

	// Reserved tape-syntax tokens emit typed marker values so the
	// engine recognises them by VType identity (parens, end / ';').
	// These would otherwise become plain Word values that the engine
	// would have to name-dispatch in stepWord.
	switch name {
	case "end":
		return eng.NewEnd(), nil
	case ")":
		return eng.NewCloseParen(), nil
	}

	// *Type names resolve to type literals even in word context, so that
	// they retain their meaning inside quotations (e.g. [String,Decimal]).
	if t, ok := typeNames[name]; ok {
		return eng.NewTypeLiteral(t), nil
	}
	if t, ok := eng.ResolveTypePath(name); ok {
		return eng.NewTypeLiteral(t), nil
	}

	return eng.NewWord(name), nil
}

// convertInterpGroup converts an interpGroup (produced by the interp/ielem/iexpr
// jsonic rules) into an engine InterpString value, or a plain string if there
// are no expression parts.
func convertInterpGroup(grp interpGroup) (eng.Value, error) {
	if len(grp) == 0 {
		return eng.NewString(""), nil
	}
	var parts []eng.InterpPart
	hasExpr := false
	for _, item := range grp {
		switch v := item.(type) {
		case jsonic.Text:
			// Template literal segment (Quote="tl").
			parts = append(parts, eng.InterpPart{Lit: v.Str})
		case iexprGroup:
			hasExpr = true
			exprVals, err := convertTopLevelItems([]any(v))
			if err != nil {
				return eng.Value{}, fmt.Errorf("interpolation expression error: %w", err)
			}
			parts = append(parts, eng.InterpPart{Expr: exprVals})
		default:
			return eng.Value{}, fmt.Errorf("unexpected interp part type %T", item)
		}
	}
	if !hasExpr {
		// No interpolations — just concatenate literals into a plain string.
		var buf strings.Builder
		for _, p := range parts {
			buf.WriteString(p.Lit)
		}
		return eng.NewString(buf.String()), nil
	}
	return eng.NewInterpString(parts), nil
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
func floatToValue(f float64) eng.Value {
	if f == float64(int64(f)) && !math.IsInf(f, 0) && !math.IsNaN(f) {
		return eng.NewInteger(int64(f))
	}
	return eng.NewDecimal(f)
}

// numberValToValue converts a numberVal (float64 + source) to the appropriate
// AQL numeric value. If the source text contains a ".", the value is always
// treated as a decimal — even for whole numbers like 5.0.
func numberValToValue(nv numberVal) eng.Value {
	if strings.Contains(nv.Src, ".") {
		return eng.NewDecimal(nv.Val)
	}
	return floatToValue(nv.Val)
}
