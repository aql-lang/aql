package parser

import (
	"strings"

	"github.com/aql-lang/aql/eng/go"
	jsonic "github.com/jsonicjs/jsonic/go"
)

// parserTokens holds the custom jsonic token IDs registered for AQL grammar.
// Passed between grammar setup stages so token IDs are defined once.
type parserTokens struct {
	OP jsonic.Tin // (
	CP jsonic.Tin // )
	DT jsonic.Tin // .
	SC jsonic.Tin // ;
	QM jsonic.Tin // ?
	BG jsonic.Tin // !
	PI jsonic.Tin // |
	AR jsonic.Tin // => (lambda arrow → aliases the word `afn`)
	BT jsonic.Tin // ` (backtick)
	IS jsonic.Tin // ${ (interp start)
	TL jsonic.Tin // template literal segment
}

// setupBaseTokens registers the fixed AQL tokens and removes backtick from
// jsonic's string/multi chars so template strings are handled by custom rules.
func setupBaseTokens(j *jsonic.Jsonic) parserTokens {
	// Remove backtick from string chars so jsonic doesn't consume backtick
	// strings with the built-in string matcher. Template strings are handled
	// by custom tokens and rules below.
	delete(j.Config().StringChars, '`')
	delete(j.Config().MultiChars, '`')

	return parserTokens{
		OP: j.Token("#OP", "("),
		CP: j.Token("#CP", ")"),
		DT: j.Token("#DT", "."),
		SC: j.Token("#SC", ";"),
		QM: j.Token("#QM", "?"),
		BG: j.Token("#BG", "!"),
		PI: j.Token("#PI", "|"),
		AR: j.Token("#AR", "=>"),
		BT: j.Token("#BT", "`"),
		IS: j.Token("#IS", "${"),
		TL: j.Token("#TL"),
	}
}

// setupTemplateLiteralMatcher registers a high-priority lex matcher that
// produces #TL tokens for literal text inside template strings. Active only
// when rule.K["aql_tpl"] is set (inside a backtick-opened interp rule).
func setupTemplateLiteralMatcher(j *jsonic.Jsonic, t parserTokens) {
	j.AddMatcher("template_literal", 1000000, func(lex *jsonic.Lex, rule *jsonic.Rule) *jsonic.Token {
		if rule == nil {
			return nil
		}
		if _, ok := rule.K["aql_tpl"]; !ok {
			return nil
		}
		cursor := lex.Cursor()
		si := cursor.SI
		s := lex.Src
		if si >= len(s) {
			return nil
		}
		// Don't match if at ` or ${ — let fixed token matcher handle those.
		if s[si] == '`' {
			return nil
		}
		if s[si] == '$' && si+1 < len(s) && s[si+1] == '{' {
			return nil
		}
		// Scan forward collecting literal text until ` or ${ or end.
		start := si
		for si < len(s) {
			if s[si] == '`' {
				break
			}
			if s[si] == '$' && si+1 < len(s) && s[si+1] == '{' {
				break
			}
			// Process escape sequences in template literals.
			if s[si] == '\\' && si+1 < len(s) {
				si += 2
				continue
			}
			si++
		}
		if si == start {
			return nil
		}
		lit := s[start:si]
		// Process escape sequences.
		lit = processTemplateEscapes(lit)
		tkn := lex.Token("#TL", t.TL, lit, s[start:si])
		cursor.SI = si
		// Update row/col tracking.
		for _, ch := range s[start:si] {
			if ch == '\n' {
				cursor.RI++
				cursor.CI = 1
			} else {
				cursor.CI++
			}
		}
		return tkn
	})
}

// setupValRule extends the jsonic "val" rule with AQL-specific alternates:
// parens, template strings, close-paren markers, semicolons, ?, !, |, and dots.
func setupValRule(j *jsonic.Jsonic, t parserTokens) {
	j.Rule("val", func(rs *jsonic.RuleSpec) {
		rs.Open = append([]*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{t.OP}}, P: "paren"},
			// Backtick opens a template string → push to interp rule.
			{S: [][]jsonic.Tin{{t.BT}}, P: "interp"},
			// Bare ) outside a paren group: produce a marker so the engine
			// can report "unmatched closing parenthesis" at runtime.
			{S: [][]jsonic.Tin{{t.CP}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: ")", Quote: ""}
			}},
			{S: [][]jsonic.Tin{{t.SC}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				// Semicolon is an alias for the "end" keyword.
				r.Node = jsonic.Text{Str: "end", Quote: ""}
			}},
			// Question mark produces a "?" marker for optional param syntax.
			{S: [][]jsonic.Tin{{t.QM}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: "?", Quote: ""}
			}},
			// Bang: "!" token. The "!" "." sequence becomes getr in convertTopLevelItems.
			{S: [][]jsonic.Tin{{t.BG}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: "!", Quote: ""}
			}},
			// Pipe: "|" token. Used in fn signatures as forward barrier marker.
			{S: [][]jsonic.Tin{{t.PI}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: "|", Quote: ""}
			}},
			// Arrow: "=>" token. Lambda sugar — aliases the word `afn` so
			// `a => b` lexes as the same value sequence as `a afn b`.
			{S: [][]jsonic.Tin{{t.AR}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: "afn", Quote: ""}
			}},
			// Dot: "." token. Becomes get in convertTopLevelItems.
			{S: [][]jsonic.Tin{{t.DT}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Text{Str: ".", Quote: ""}
			}},
		}, rs.Open...)
	})

	// Capture source position: wrap every value node with the row/col of its
	// opening token, so the converter can stamp eng.Value.Pos for precise
	// error reporting (mirrors aontu's addsite). The BC fires on val-rule
	// close — after the Open alternates above have set r.Node (including the
	// marker Texts), so markers carry positions too. The wrapper is unwrapped
	// (deSite) at every node-type switch in parse.go and never leaks.
	j.Rule("val", func(rs *jsonic.RuleSpec) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if r.Node == nil || jsonic.IsUndefined(r.Node) {
				return
			}
			if _, already := r.Node.(sited); already {
				return
			}
			var pos eng.SrcPos
			if r.O0 != nil && !r.O0.IsNoToken() {
				pos = eng.SrcPos{Row: r.O0.RI, Col: r.O0.CI, Src: r.O0.Src}
			}
			r.Node = sited{Node: r.Node, Pos: pos}
		})
	})
}

// setupPairGrammar extends "pair" and "elem" rules for optional-field (?:)
// and computed-key ([key]:) syntax in both map and list contexts.
func setupPairGrammar(j *jsonic.Jsonic, t parserTokens) {
	// Shared helper: extract key from pair-open token.
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

	// --- Optional field syntax: key?:value in pair context ---

	j.Rule("pair", func(rs *jsonic.RuleSpec) {
		rs.Open = append([]*jsonic.AltSpec{
			// Match KEY ? — save key via K (propagated), push to pair.
			{S: [][]jsonic.Tin{jsonic.TinSetKEY, {t.QM}},
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

	// --- Computed key syntax: {[key]:value} in pair context ---

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

	// --- Shorthand field syntax: {foo} ≡ {foo: foo}, {foo/r} ≡ {foo: foo/r} ---

	j.Rule("pair", func(rs *jsonic.RuleSpec) {
		rs.Open = append(rs.Open, &jsonic.AltSpec{
			// A lone unquoted-text key with no following colon. Appended
			// LAST so it never shadows a real `key:value` pair: the base
			// KEY CL alt (and the prepended qm/ck alts) are tried first,
			// and only `{foo}` / `{foo/r}` — where no colon, `?`, or `[`
			// follows — fall through to here. The optional shorthand
			// `{foo?}` is handled by the qm path above (it consumes
			// `foo ?` and records Meta["qm"]); the value is synthesized in
			// convertMapData. No P/U["pair"]: nothing is pushed, so the
			// built-in pairval BC stays inert and the pair closes cleanly.
			S: [][]jsonic.Tin{{jsonic.TinTX}},
			A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				if raw, ok := r.O0.Val.(string); ok {
					r.U["aql_sh"] = raw
				}
			},
		})
	})

	// Record shorthand keys in MapRef.Meta["sh"] via pair.BC callback.
	j.Rule("pair", func(rs *jsonic.RuleSpec) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			raw, ok := r.U["aql_sh"].(string)
			if !ok || raw == "" {
				return
			}
			if mr, ok := r.Node.(jsonic.MapRef); ok {
				shList, _ := mr.Meta["sh"].([]string)
				shList = append(shList, raw)
				mr.Meta["sh"] = shList
				r.Node = mr // MapRef is a value type, reassign
			}
		})
	})

	// Record quoted keys in MapRef.Meta["qk"] via pair.BC callback. A
	// quoted key (`{'a/b': 1}`) arrives as a string token (TinST) rather
	// than bare text (TinTX); convertMapData can't tell them apart from the
	// plain string map afterwards, so it needs this flag to know the key
	// was an explicit literal — e.g. to allow a `/` in it that would be an
	// illegal word modifier on a bare key.
	j.Rule("pair", func(rs *jsonic.RuleSpec) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if r.O0.Tin != jsonic.TinST {
				return
			}
			key, ok := r.O0.Val.(string)
			if !ok || key == "" {
				return
			}
			if mr, ok := r.Node.(jsonic.MapRef); ok {
				qkSet, _ := mr.Meta["qk"].(map[string]bool)
				if qkSet == nil {
					qkSet = make(map[string]bool)
				}
				qkSet[key] = true
				mr.Meta["qk"] = qkSet
				r.Node = mr // MapRef is a value type, reassign
			}
		})
	})

	// --- Optional field syntax in list context: [x?:Integer] ---

	j.Rule("elem", func(rs *jsonic.RuleSpec) {
		rs.Open = append([]*jsonic.AltSpec{
			// Step 1: match KEY ? — save key, push to elem.
			{S: [][]jsonic.Tin{jsonic.TinSetKEY, {t.QM}},
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
}

// setupParenGrammar defines the "paren" and "pelem" rules that collect
// values between ( and ) into a parenGroup.
func setupParenGrammar(j *jsonic.Jsonic, t parserTokens) {
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
			{S: [][]jsonic.Tin{{t.CP}}, U: map[string]any{"closed": true}},
			// First element
			{P: "pelem"},
		}
		rs.Close = []*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{t.CP}}, U: map[string]any{"closed": true}},
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
			{S: [][]jsonic.Tin{{t.CP}}, B: 1},
			// End of source inside paren: auto-close (unclosed paren)
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
			// Comma: next element
			{S: [][]jsonic.Tin{{jsonic.TinCA}}, R: "pelem"},
			// Space-separated: next element (backtrack to re-read token)
			{R: "pelem", B: 1},
		}
	})
}

// setupInterpGrammar defines the "interp", "ielem", "iexpr", and "ieval"
// rules for template string interpolation (`hello ${name}`).
func setupInterpGrammar(j *jsonic.Jsonic, t parserTokens) {
	// Interp rule: collects template string parts between backticks.
	// K["aql_tpl"] is set in BO so the custom matcher produces #TL tokens
	// for literal text segments. Parts are accumulated in Node as an interpGroup.
	j.Rule("interp", func(rs *jsonic.RuleSpec) {
		rs.BO = []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = interpGroup{}
				// Set K so the custom matcher knows we're inside a template.
				// K propagates to child rules.
				r.K["aql_tpl"] = true
			},
		}
		rs.Open = []*jsonic.AltSpec{
			// Empty template: `` (immediate closing backtick)
			{S: [][]jsonic.Tin{{t.BT}}},
			// First element: push to ielem.
			{P: "ielem"},
		}
		rs.Close = []*jsonic.AltSpec{
			// Closing backtick ends the template.
			{S: [][]jsonic.Tin{{t.BT}}},
			// End of source: unterminated template string (auto-close).
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
		}
	})

	// Ielem rule: each element inside a template string.
	// Handles #TL (literal text) and #IS (interpolation start).
	// On close, appends its own Node to the parent interp's interpGroup.
	j.Rule("ielem", func(rs *jsonic.RuleSpec) {
		rs.BC = []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				// Collect the node value. For #TL, the action sets r.Node directly.
				// For #IS→iexpr, the child rule sets r.Node (iexprGroup) via push.
				node := r.Node
				if !jsonic.IsUndefined(r.Child.Node) {
					node = r.Child.Node
				}
				if jsonic.IsUndefined(node) {
					return
				}
				if r.Parent != nil && r.Parent != jsonic.NoRule {
					if grp, ok := r.Parent.Node.(interpGroup); ok {
						r.Parent.Node = append(grp, node)
					}
				}
			},
		}
		rs.Open = []*jsonic.AltSpec{
			// Literal text segment.
			{S: [][]jsonic.Tin{{t.TL}},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					// Store literal text as a Text with Quote="tl".
					if str, ok := r.O0.Val.(string); ok {
						r.Node = jsonic.Text{Str: str, Quote: "tl"}
					}
				}},
			// Interpolation start ${ — push to iexpr to collect the expression.
			{S: [][]jsonic.Tin{{t.IS}}, P: "iexpr"},
		}
		rs.Close = []*jsonic.AltSpec{
			// Closing backtick: backtrack so interp.Close can consume it.
			{S: [][]jsonic.Tin{{t.BT}}, B: 1},
			// End of source.
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
			// Next element (another literal or interpolation).
			{R: "ielem", B: 1},
		}
	})

	// Iexpr rule: collects expression values between ${ and }.
	// Pushes to val for each value, collects into a list.
	// Clears aql_tpl in BO so that expression content is parsed normally
	// (the custom matcher won't fire inside expressions).
	j.Rule("iexpr", func(rs *jsonic.RuleSpec) {
		rs.BO = []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = make([]any, 0)
				// Clear template mode so expressions parse normally.
				delete(r.K, "aql_tpl")
				// Increment dlist and dmap so val.Close won't create
				// implicit lists or maps inside interpolation expressions.
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
				// Wrap the collected values as an iexprGroup.
				if arr, ok := r.Node.([]any); ok {
					r.Node = iexprGroup(arr)
				}
			},
		}
		rs.Open = []*jsonic.AltSpec{
			// Empty expression: ${}
			{S: [][]jsonic.Tin{{jsonic.TinCB}}},
			// First expression value.
			{P: "ieval"},
		}
		rs.Close = []*jsonic.AltSpec{
			// Closing brace } ends the expression.
			{S: [][]jsonic.Tin{{jsonic.TinCB}}},
			// End of source inside expression.
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
		}
	})

	// Ieval rule: each value inside an interpolation expression.
	// Similar to pelem but closes on } instead of ).
	j.Rule("ieval", func(rs *jsonic.RuleSpec) {
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
			// } ends the expression (backtrack so iexpr.Close consumes it).
			{S: [][]jsonic.Tin{{jsonic.TinCB}}, B: 1},
			// End of source.
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
			// Comma: next expression value.
			{S: [][]jsonic.Tin{{jsonic.TinCA}}, R: "ieval"},
			// Space-separated: next expression value.
			{R: "ieval", B: 1},
		}
	})
}

// setupNumberSub registers a token subscriber that wraps decimal number
// tokens in a numberVal struct so the converter can distinguish "5" (integer)
// from "5.0" (decimal).
func setupNumberSub(j *jsonic.Jsonic) {
	j.Sub(func(tkn *jsonic.Token, rule *jsonic.Rule, ctx *jsonic.Context) {
		if tkn.Tin == jsonic.TinNR && strings.Contains(tkn.Src, ".") {
			tkn.Val = numberVal{Val: tkn.Val.(float64), Src: tkn.Src}
		}
	}, nil)
}
