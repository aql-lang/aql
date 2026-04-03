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

// parenGroup represents items collected between ( and ) by the jsonic grammar.
// At the top level, these are expanded to engine paren markers. In data context
// (map values), they become ParenExpr values for inline evaluation.
type parenGroup []any

// unclosedParen wraps a parenGroup that was auto-closed at EOF (no matching `)`).
// The converter produces an error for these.
type unclosedParen struct{ items []any }

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

	// Register ( ) . ; ? ! | as separate fixed tokens so jsonic lexes them
	// independently, even when adjacent to other text (e.g. "(foo" → "(" + "foo").
	TinOP := j.Token("#OP", "(")
	TinCP := j.Token("#CP", ")")
	TinDT := j.Token("#DT", ".")
	TinSC := j.Token("#SC", ";")
	TinQM := j.Token("#QM", "?")
	TinBG := j.Token("#BG", "!")
	TinPI := j.Token("#PI", "|")

	// Add val rule alternates so the grammar recognizes these custom tokens
	// and produces Text marker values that the converter layer processes.
	//
	// Parens push to the "paren" rule which collects items into a parenGroup.
	// The close paren `)` is handled by the paren/pelem rules, not val.
	//
	// For the dot token, we use source position to distinguish adjacent dots
	// (foo.bar → part of a dotted word) from space-separated dots
	// (foo . bar → standalone dot operator). Adjacent dots use Quote="adj"
	// so the converter can identify them.
	j.Rule("val", func(rs *jsonic.RuleSpec) {
		rs.Open = append([]*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{TinOP}}, P: "paren"},
			// Bare ) outside a paren group: produce a marker so the engine
			// can report "unmatched closing parenthesis" at runtime.
			{S: [][]jsonic.Tin{{TinCP}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: ")", Quote: ""}
			}},
			{S: [][]jsonic.Tin{{TinSC}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				// Semicolon is an alias for the "end" keyword.
				r.Node = jsonic.Text{Str: "end", Quote: ""}
			}},
			// Question mark produces a "?" marker for optional param syntax.
			{S: [][]jsonic.Tin{{TinQM}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: "?", Quote: ""}
			}},
			// Bang: "!" token. The "!" "." sequence becomes getr in convertTopLevelItems.
			{S: [][]jsonic.Tin{{TinBG}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: "!", Quote: ""}
			}},
			// Pipe: "|" token. Used in fn signatures as forward barrier marker.
			{S: [][]jsonic.Tin{{TinPI}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: "|", Quote: ""}
			}},
			// Dot: "." token. Becomes get in convertTopLevelItems.
			{S: [][]jsonic.Tin{{TinDT}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: ".", Quote: ""}
			}},
		}, rs.Open...)
	})

	// Optional field syntax: key?:value in pair context.
	// Two alternates on pair.Open:
	//   [KEY, QM] — matches "a ?", saves key via pairkey action,
	//               sets K["aql_qm"]=true (propagated to child), pushes to pair.
	//   [CL]      — matches ":" when K["aql_qm"]=true, sets U["pair"]=true,
	//               pushes to val for the value.
	// A pair.BC callback records optional keys in MapRef.Meta["qm"]
	// so convertMapData can wrap them in (value or None).
	pairkey := func(r *jsonic.Rule, ctx *jsonic.Context) {
		keyToken := r.O0
		var key string
		if keyToken.Tin == jsonic.TinST || keyToken.Tin == jsonic.TinTX {
			key, _ = keyToken.Val.(string)
		} else {
			key = keyToken.Src
		}
		r.U["key"] = key
	}

	j.Rule("pair", func(rs *jsonic.RuleSpec) {
		rs.Open = append([]*jsonic.AltSpec{
			// Match KEY ? — save key via K (propagated), push to pair.
			{S: [][]jsonic.Tin{jsonic.TinSetKEY, {TinQM}},
				P: "pair", K: map[string]any{"aql_qm": true},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					pairkey(r, ctx)
					// Propagate key to inner pair via K.
					r.K["key"] = r.U["key"]
				}},
			// Match : when aql_qm flag is set — copy key from K, proceed as pair value.
			{S: [][]jsonic.Tin{{jsonic.TinCL}},
				C: func(r *jsonic.Rule, ctx *jsonic.Context) bool {
					_, ok := r.K["aql_qm"]
					return ok
				},
				P: "val", U: map[string]any{"pair": true},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					r.U["key"] = r.K["key"]
				}},
		}, rs.Open...)
	})

	// Record optional keys in MapRef.Meta via pair.BC callback.
	j.Rule("pair", func(rs *jsonic.RuleSpec) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if _, ok := r.K["aql_qm"]; !ok {
				return
			}
			key, _ := r.U["key"].(string)
			if key == "" {
				return
			}
			// Store optional key in MapRef.Meta["qm"].
			if mr, ok := r.Node.(jsonic.MapRef); ok {
				qmSet, _ := mr.Meta["qm"].(map[string]bool)
				if qmSet == nil {
					qmSet = make(map[string]bool)
				}
				qmSet[key] = true
				mr.Meta["qm"] = qmSet
				r.Node = mr // MapRef is a value type, reassign
			}
		})
	})

	// Computed key syntax: {[key]:value} in pair context.
	// Three-step approach:
	//   [OS]       — matches "[", sets K["aql_ck"]=true, pushes to pair.
	//   [KEY, CS]  — matches "key ]" when aql_ck, saves key, pushes to pair.
	//   [CL]       — matches ":" when aql_ck, copies key, pushes to val.
	// A pair.BC callback records the computed key in MapRef.Meta["ck"]
	// so autoEvalMap can evaluate the key expression at runtime.
	j.Rule("pair", func(rs *jsonic.RuleSpec) {
		rs.Open = append([]*jsonic.AltSpec{
			// Step 1: match [ — set computed key flag, push to pair.
			{S: [][]jsonic.Tin{{jsonic.TinOS}},
				P: "pair", K: map[string]any{"aql_ck": true}},
			// Step 2: match KEY ] when aql_ck — save key, push to pair.
			{S: [][]jsonic.Tin{jsonic.TinSetKEY, {jsonic.TinCS}},
				C: func(r *jsonic.Rule, ctx *jsonic.Context) bool {
					_, ok := r.K["aql_ck"]
					return ok
				},
				P: "pair",
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					pairkey(r, ctx)
					r.K["key"] = r.U["key"]
				}},
			// Step 3: match : when aql_ck and key is set — proceed as pair value.
			{S: [][]jsonic.Tin{{jsonic.TinCL}},
				C: func(r *jsonic.Rule, ctx *jsonic.Context) bool {
					_, hasCK := r.K["aql_ck"]
					_, hasKey := r.K["key"]
					return hasCK && hasKey
				},
				P: "val", U: map[string]any{"pair": true},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					r.U["key"] = r.K["key"]
				}},
		}, rs.Open...)
	})

	// Record computed keys in MapRef.Meta via pair.BC callback.
	j.Rule("pair", func(rs *jsonic.RuleSpec) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if _, ok := r.K["aql_ck"]; !ok {
				return
			}
			key, _ := r.U["key"].(string)
			if key == "" {
				return
			}
			// Store computed key in MapRef.Meta["ck"].
			if mr, ok := r.Node.(jsonic.MapRef); ok {
				ckSet, _ := mr.Meta["ck"].(map[string]bool)
				if ckSet == nil {
					ckSet = make(map[string]bool)
				}
				ckSet[key] = true
				mr.Meta["ck"] = ckSet
				r.Node = mr
			}
		})
	})

	// Optional field syntax in list context: [x?:Integer]
	// Same two-step approach as pair rule:
	//   [KEY, QM] — matches "x ?", saves key via K, pushes to elem.
	//   [CL]      — matches ":" when K["aql_qm"], proceeds as list pair.
	// The inner elem's BC handles the pair creation normally.
	j.Rule("elem", func(rs *jsonic.RuleSpec) {
		rs.Open = append([]*jsonic.AltSpec{
			// Step 1: match KEY ? — save key, push to elem.
			{S: [][]jsonic.Tin{jsonic.TinSetKEY, {TinQM}},
				P: "elem", K: map[string]any{"aql_qm": true},
				U: map[string]any{"done": true},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					pairkey(r, ctx)
					r.K["key"] = r.U["key"]
				}},
			// Step 2: match : when aql_qm is set — proceed as list pair.
			{S: [][]jsonic.Tin{{jsonic.TinCL}},
				C: func(r *jsonic.Rule, ctx *jsonic.Context) bool {
					_, ok := r.K["aql_qm"]
					return ok
				},
				P: "val",
				N: map[string]int{"pk": 1, "dmap": 1},
				U: map[string]any{"done": true, "pair": true, "list": true},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					r.U["key"] = r.K["key"].(string) + "?"
				}},
		}, rs.Open...)
	})

	// Propagate the outer elem's updated Node to grandparent (list rule).
	// The inner elem's BC[1] already updated the outer elem's Node via
	// r.Parent.Node, but the outer elem's BC[0] is skipped (done=true),
	// so we need explicit propagation to the list rule.
	j.Rule("elem", func(rs *jsonic.RuleSpec) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if _, ok := r.K["aql_qm"]; !ok {
				return
			}
			// Only propagate from the OUTER elem (which has done=true).
			done, _ := r.U["done"].(bool)
			if !done {
				return
			}
			if r.Parent != nil && r.Parent != jsonic.NoRule {
				r.Parent.Node = r.Node
			}
		})
	})

	// Paren rule: collects values between ( and ) into a parenGroup.
	// Works like a simplified list rule but closes on ) instead of ].
	j.Rule("paren", func(rs *jsonic.RuleSpec) {
		rs.BO = []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = make([]any, 0)
			},
			// Increment dlist and dmap so val.Close won't create
			// implicit lists or maps inside paren groups.
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if v, ok := r.N["dlist"]; ok {
					r.N["dlist"] = v + 1
				} else {
					r.N["dlist"] = 1
				}
				if v, ok := r.N["dmap"]; ok {
					r.N["dmap"] = v + 1
				} else {
					r.N["dmap"] = 1
				}
			},
		}
		rs.BC = []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if arr, ok := r.Node.([]any); ok {
					r.Node = parenGroup(arr)
				}
			},
		}
		rs.AC = []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if r.U["closed"] != true {
					if pg, ok := r.Node.(parenGroup); ok {
						r.Node = unclosedParen{items: []any(pg)}
					}
				}
			},
		}
		rs.Open = []*jsonic.AltSpec{
			// Empty parens: ()
			{S: [][]jsonic.Tin{{TinCP}}, U: map[string]any{"closed": true}},
			// First element
			{P: "pelem"},
		}
		rs.Close = []*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{TinCP}}, U: map[string]any{"closed": true}},
			// End of source: auto-close (unclosed paren)
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
		}
	})

	// Pelem (paren element): each item inside a paren group.
	// Pushes to val for each value, appends to parent paren's list.
	j.Rule("pelem", func(rs *jsonic.RuleSpec) {
		rs.BC = []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if !jsonic.IsUndefined(r.Child.Node) {
					if arr, ok := r.Node.([]any); ok {
						r.Node = append(arr, r.Child.Node)
						if r.Parent != nil && r.Parent != jsonic.NoRule {
							r.Parent.Node = r.Node
						}
					}
				}
			},
		}
		rs.Open = []*jsonic.AltSpec{
			{P: "val"},
		}
		rs.Close = []*jsonic.AltSpec{
			// ) ends the paren group (backtrack so paren.Close consumes it)
			{S: [][]jsonic.Tin{{TinCP}}, B: 1},
			// End of source inside paren: auto-close (unclosed paren)
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
			// Comma: next element
			{S: [][]jsonic.Tin{{jsonic.TinCA}}, R: "pelem"},
			// Space-separated: next element (backtrack to re-read token)
			{R: "pelem", B: 1},
		}
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
