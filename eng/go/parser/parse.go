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

// sited wraps a converted jsonic node with the source position of its
// opening token, captured by the val-rule before-close callback (see
// grammar.go setupValRule). deSite unwraps it.
//
// Pos here is a pure diagnostic: an unknown (zero) SrcPos renders as
// "source position unknown" at error time rather than a guessed location,
// so the 0=unknown convention is deliberate and is NOT a "No Zero-Value
// Overload" violation (see eng/go/CLAUDE.md) — no decision reads a zero Pos.
type sited struct {
	Node any
	Pos  eng.SrcPos
}

// deSite unwraps a sited wrapper, returning the inner node and its
// source position. A non-sited node is returned unchanged with a zero
// (unknown) Pos. Idempotent and safe on any node — call it wherever a
// raw jsonic node is type-switched so the wrapper never leaks past the
// converter's dispatch points.
func deSite(v any) (any, eng.SrcPos) {
	if s, ok := v.(sited); ok {
		return s.Node, s.Pos
	}
	return v, eng.SrcPos{}
}

// withPos stamps a source position onto v, unless pos is unknown (zero
// Row), in which case v is returned untouched. The guard avoids both
// storing the unknown and clobbering a position a deeper converter
// already set on a nested value.
func withPos(v eng.Value, pos eng.SrcPos) eng.Value {
	if pos.Row != 0 {
		v.Pos = pos
	}
	return v
}

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

	// A single top-level scalar/paren/interp value arrives wrapped by the
	// val-rule BC; unwrap it so the cases below see a bare jsonic node, but
	// keep its position to stamp the single produced value. (Root containers
	// come from the list/map rule and are not sited; their elements carry
	// positions individually.)
	result, rootPos := deSite(result)

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
		return []eng.Value{withPos(iv, rootPos)}, nil

	default:
		// Single top-level scalar/word: stamp the position the root deSite
		// recovered (convertTopLevelValue sees a bare node, so it cannot).
		v, err := convertTopLevelValue(val)
		if err != nil {
			return nil, err
		}
		return []eng.Value{withPos(v, rootPos)}, nil
	}
}

// isToken checks if item is an unquoted text marker matching the given string.
// Quoted text (e.g. "." or "!") has Quote != "" and is handled as a string
// by convertTopLevelValue, so it never reaches the token checks.
func isToken(item any, tok string) bool {
	node, _ := deSite(item)
	text, ok := node.(jsonic.Text)
	return ok && text.Str == tok && text.Quote == ""
}

// convertTopLevelItems converts a list of jsonic items in word context,
// handling parenthesis markers and token sequences.
//
// Dotted access is sugar for `get`/`getr` and is GROUPED so it binds to
// its immediate receiver: a chain `recv.k1.k2` (or `recv!.k1`) wraps as a
// single paren group `( recv get k1 get k2 … )`. Grouping isolates the
// access from surrounding forward-collection, so `size m.x` means
// `size (m.x)`, not `(size m).x`. `get`/`getr` left-compose on the stack,
// so one flat group covers any chain length. The receiver and each key may
// themselves be paren groups, which covers `(expr).k` and the computed-key
// form `m.(expr)`.
//
//   - "." → get      "!" "." → getr
//
// A lone "." / "!." with no receiver still emits the bare word (and errors
// at runtime, as before). All other items convert to engine values
// directly.
func convertTopLevelItems(items []any) ([]eng.Value, error) {
	// Strip sited wrappers once into a bare-node slice plus a parallel
	// position slice. The token helpers (isToken/startsDot/isChainReceiver)
	// then see bare jsonic nodes, while the emitted operator-marker words
	// (get/getr) and primaries are still stamped from poss[i].
	poss := make([]eng.SrcPos, len(items))
	if anySited(items) {
		bare := make([]any, len(items))
		for i, it := range items {
			bare[i], poss[i] = deSite(it)
		}
		items = bare
	}

	values := make([]eng.Value, 0, len(items))
	for i := 0; i < len(items); i++ {
		// Dotted-access chain: a receiver primary followed by one or more
		// `.key` / `!.key` segments → one group `( recv get k … )`.
		if isChainReceiver(items[i]) && i+1 < len(items) && startsDot(items, i+1) {
			// A reach receiver must be something `get` can index into (a
			// Map, List, Store, Object, ModuleExport, …). A number has no
			// members, so a numeric literal before a `.` can never be a
			// valid reach — `1.2.3` (a malformed numeric literal), `1 . 2`,
			// or `5 . foo`. Reject it here with a clear message rather than
			// the runtime "no matching signature for get".
			if isNumberLiteral(items[i]) {
				return nil, numberReceiverError(poss[i])
			}
			// Build the access group into a temp slice so a trailing
			// `/`-modifier can be placed correctly: a WORD modifier
			// (usurp / stack-args / forward-args) is emitted BEFORE the
			// group so it forward-collects the result (the result must not
			// auto-dispatch first), while the Word/__DM marker (/N /r /q)
			// is emitted AFTER for execFnDefLiteral to peek.
			// Build a first-class Reach node (design/REACH.0.md Phase B):
			// receiver tokens + per-segment {op, literal-or-computed key}.
			var recv []eng.Value
			if err := emitPrimary(&recv, items[i], poss[i]); err != nil {
				return nil, err
			}
			var segs []eng.ReachSeg
			j := i + 1
			var pfx, sfx []eng.Value
		chain:
			for j < len(items) {
				var getr bool
				var keyItem any
				var kpos eng.SrcPos
				switch {
				case isToken(items[j], "!") && j+2 < len(items) && isToken(items[j+1], "."):
					getr, keyItem, kpos = true, items[j+2], poss[j+2]
					j += 3
				case isToken(items[j], ".") && j+1 < len(items):
					getr, keyItem, kpos = false, items[j+1], poss[j+1]
					j += 2
				default:
					break chain
				}
				// A `/`-modifier on the final key applies to the whole reach.
				if base, pre, suf, ok := groupModifier(keyItem); ok && base != "" {
					keyItem, pfx, sfx = jsonic.Text{Str: base}, pre, suf
				}
				seg := eng.ReachSeg{Getr: getr}
				if pg, ok := keyItem.(parenGroup); ok {
					// Computed key: m.(expr) — store the paren's tokens.
					inner, err := convertTopLevelItems([]any(pg))
					if err != nil {
						return nil, err
					}
					seg.Computed = true
					seg.KeyExpr = inner
				} else {
					// Literal key (word → atom-via-get/q, string, number).
					var tmp []eng.Value
					if err := emitPrimary(&tmp, keyItem, kpos); err != nil {
						return nil, err
					}
					if len(tmp) == 1 {
						seg.KeyLit = tmp[0]
					} else {
						seg.Computed = true
						seg.KeyExpr = tmp
					}
				}
				segs = append(segs, seg)
			}
			// A `$` receiver marks a RECEIVERLESS reach — a detached lens
			// (`$.name`), not a `$ get name` chain. It is inert (Eval=false):
			// it evaluates to itself (a Reach value) so it can be passed as a
			// reusable accessor (`people each $.name`) and applied/rebound to a
			// receiver later. `$` is reserved as a user word (see
			// ValidateWordName), so this never shadows a real binding.
			reachInfo := eng.ReachInfo{Receiver: recv, Segments: segs, Eval: true}
			if isDollarReceiver(recv) {
				reachInfo.Receiver, reachInfo.Eval = nil, false
			}
			pe := withPos(eng.NewReach(reachInfo), poss[i])
			// A standalone `/mod` token immediately after the path also
			// applies to the result (`(a.b)/mod`).
			if pfx == nil && sfx == nil && j < len(items) {
				if base, pre, suf, ok := groupModifier(items[j]); ok && base == "" {
					pfx, sfx = pre, suf
					j++
				}
			}
			for _, p := range pfx {
				values = append(values, withPos(p, poss[i]))
			}
			values = append(values, pe)
			for _, s := range sfx {
				values = append(values, withPos(s, poss[i]))
			}
			i = j - 1
			continue
		}

		// A "!." or "." with no receiver before it is the leading / prefix
		// dot form (e.g. `.5`, `.name`, `!.x`). It has nothing to access,
		// so reject it at parse time with a clear message instead of
		// emitting a receiverless get/getr that fails later as an obscure
		// signature error. The receiverless reach `$.name` is unaffected:
		// it carries an explicit `$` receiver and is built in the chain
		// branch above.
		if isToken(items[i], "!") && i+1 < len(items) && isToken(items[i+1], ".") {
			return nil, danglingDotError(poss[i])
		}
		if isToken(items[i], ".") {
			return nil, danglingDotError(poss[i])
		}

		// Unclosed paren: error at parse time.
		if up, ok := items[i].(unclosedParen); ok {
			_ = up
			return nil, eng.MakeAqlError("syntax_error", "unmatched opening parenthesis", "(", "", "")
		}

		// A standalone `/mod` token right after a primary (e.g. `(expr)/s`)
		// applies the modifier to that primary's result. Word modifiers are
		// emitted BEFORE the primary (so they forward-collect the result
		// before any auto-dispatch); the /r /q marker is emitted AFTER.
		if i+1 < len(items) {
			if base, pre, suf, ok := groupModifier(items[i+1]); ok && base == "" {
				for _, p := range pre {
					values = append(values, withPos(p, poss[i]))
				}
				if err := emitPrimary(&values, items[i], poss[i]); err != nil {
					return nil, err
				}
				for _, s := range suf {
					values = append(values, withPos(s, poss[i]))
				}
				i++
				continue
			}
		}

		// A value, or a paren group expanded to ( … ) markers.
		if err := emitPrimary(&values, items[i], poss[i]); err != nil {
			return nil, err
		}
	}
	return values, nil
}

// anySited reports whether any item is a sited wrapper, so the common
// data-context path (where items are never sited) skips re-allocation.
func anySited(items []any) bool {
	for _, it := range items {
		if _, ok := it.(sited); ok {
			return true
		}
	}
	return false
}

// startsDot reports whether items[j] begins a dot-access segment: a "."
// token, or a "!" immediately followed by ".".
func startsDot(items []any, j int) bool {
	if isToken(items[j], ".") {
		return true
	}
	return isToken(items[j], "!") && j+1 < len(items) && isToken(items[j+1], ".")
}

// isDollarReceiver reports whether a dot-chain receiver is the lone reserved
// `$` sentinel — the marker for a receiverless reach (`$.name` → a lens). The
// receiver emits as a single Word("$") (see emitPrimary / convertTopLevelValue).
func isDollarReceiver(recv []eng.Value) bool {
	if len(recv) != 1 || !eng.IsWord(recv[0]) {
		return false
	}
	w, _ := eng.AsWord(recv[0])
	return w.Name == "$"
}

// isChainReceiver reports whether an item can be the receiver of a dot
// chain — anything except the "." / "!" operator markers and an unclosed
// paren (i.e. a real value, word, or paren group).
func isChainReceiver(item any) bool {
	if _, ok := item.(unclosedParen); ok {
		return false
	}
	return !isToken(item, ".") && !isToken(item, "!")
}

// groupModifier decodes a `/`-modifier on a word token that should apply to
// a parenthesised / dotted-path RESULT. It returns the base text (empty for
// a standalone `/mod` token following the group) plus tokens to place around
// the group:
//
//	/u  → prefix `usurp`            /s  → prefix `stack-args`
//	/f  → prefix `forward-args`     /N  → prefix `force-arity N`
//	/r /q → suffix Word/__DM marker (leave the result inert/data)
//
// Word modifiers are PREFIXED (emitted before the group) so they
// forward-collect the result before it can auto-dispatch; the marker is
// SUFFIXED for execFnDefLiteral to peek. So `a.b/u` parses as
// `usurp (a get b)` and `a.b/2` as `force-arity 2 (a get b)`. ok=false for
// a plain word (no modifier).
func groupModifier(item any) (base string, prefix, suffix []eng.Value, ok bool) {
	node, _ := deSite(item)
	t, isText := node.(jsonic.Text)
	if !isText || t.Quote != "" {
		return "", nil, nil, false
	}
	b, argCount, fs, ff, q, r, u, valid := scanWordModifier(t.Str)
	if !valid {
		return "", nil, nil, false
	}
	switch {
	case u:
		return b, []eng.Value{eng.NewWord("usurp")}, nil, true
	case fs:
		return b, []eng.Value{eng.NewWord("stack-args")}, nil, true
	case ff:
		return b, []eng.Value{eng.NewWord("forward-args")}, nil, true
	case argCount >= 0:
		return b, []eng.Value{eng.NewWord("force-arity"), eng.NewInteger(int64(argCount))}, nil, true
	case r || q:
		return b, nil, []eng.Value{eng.NewDispatchMod(eng.DispatchModInfo{Ref: r, Quote: q})}, true
	}
	return "", nil, nil, false
}

// emitPrimary appends the converted form of a single primary item — a
// value, or a parenthesised group as a nested ParenExpr value — to dst. Used
// for the receiver, the keys of a dot chain, and ordinary top-level items.
// pos is the source position of item (caller has already deSited it).
//
// Word-context paren groups become a single ParenExpr value (paren-nesting
// Step 1, design/PAREN-REPRESENTATION.0.md), the same representation data
// context already uses. The engine evaluates it via evalParenExprResults
// (Step 2 at the pointer, Step 3 in a forward window).
func emitPrimary(dst *[]eng.Value, item any, pos eng.SrcPos) error {
	if pg, ok := item.(parenGroup); ok {
		inner, err := convertTopLevelItems([]any(pg))
		if err != nil {
			return err
		}
		*dst = append(*dst, withPos(eng.NewParenExpr(inner), pos))
		return nil
	}
	v, err := convertTopLevelValue(item)
	if err != nil {
		return err
	}
	// convertTopLevelValue deSites internally, but item arrived bare from
	// the caller's normalization, so re-stamp from the caller's pos.
	*dst = append(*dst, withPos(v, pos))
	return nil
}

// convertTopLevel converts a top-level implicit list from jsonic into
// a slice of eng.Value using word context.
func convertTopLevel(items []any) ([]eng.Value, error) {
	return convertTopLevelItems(items)
}

// convertTopLevelValue converts a single value in word context.
// Unquoted text → word, quoted text → string. The value is stamped with
// the source position captured by the val-rule BC (if any).
func convertTopLevelValue(v any) (eng.Value, error) {
	node, pos := deSite(v)
	res, err := convertTopLevelValueInner(node)
	if err != nil {
		return res, err
	}
	return withPos(res, pos), nil
}

func convertTopLevelValueInner(v any) (eng.Value, error) {
	switch val := v.(type) {
	case jsonic.Text:
		if val.Quote == "" {
			return parseWord(val.Str)
		}
		return eng.NewString(val.Str), nil

	case interpGroup:
		return convertInterpGroup(val)

	case numberVal:
		return numberValToValue(val)

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
		// An untyped-nil element is an EMPTY list slot — `[1,,2]`, `[,1]`,
		// a leading/repeated comma. (An explicit `null` arrives as the
		// jsonic.Text token "null", handled above, so this case only fires
		// for a genuinely empty element.) A repeated or leading comma is a
		// typo, not a value — reject it rather than fabricating a `null`.
		return eng.Value{}, emptyElementError()

	default:
		return eng.Value{}, fmt.Errorf("unsupported value type %T", v)
	}
}

// emptyElementError is raised when a list literal contains an empty
// element produced by a leading or repeated comma (`[1,,2]`, `[,1]`).
// AQL has no implicit "hole" value; commas are optional separators, so a
// missing element is a typo. Use `none` for an explicit empty value.
func emptyElementError() error {
	return &eng.AqlError{
		Code:   "syntax_error",
		Detail: "empty list element: remove the leading/repeated comma (write `none` for an explicit empty value)",
	}
}

// isNumberLiteral reports whether a (deSited) jsonic item is a numeric
// literal: an integer arrives from jsonic as a float64, a decimal as a
// numberVal (dot-bearing source wrapped by setupNumberSub).
func isNumberLiteral(item any) bool {
	switch item.(type) {
	case float64, numberVal:
		return true
	default:
		return false
	}
}

// numberReceiverError rejects a `.`-access whose receiver is a numeric
// literal. `get`'s receiver is always a container (Map / List / Store /
// Object / ModuleExport / …) — a number has no members — so `1.2.3` (a
// malformed numeric literal), `1 . 2`, or `5 . foo` can never be a valid
// reach. Caught at parse time instead of surfacing the runtime
// "no matching signature for get".
func numberReceiverError(pos eng.SrcPos) error {
	return &eng.AqlError{
		Code:   "syntax_error",
		Detail: "a number has no members to access with `.`",
		Hint:   "this looks like a malformed numeric literal (e.g. `1.2.3`) or `.`-access on a number — numbers have no fields or keys",
		Row:    pos.Row,
		Col:    pos.Col,
		Src:    pos.Src,
	}
}

// danglingDotError rejects a leading / receiverless `.` or `!.` — the
// prefix dot form (e.g. `.5`, `.name`, `!.x`). Member access must follow
// a value (`m.key`); a leading `.` is never valid. In particular `.5` is
// not a fraction — write `0.5`. The receiverless reach `$.name` is the
// supported way to write a detached accessor.
func danglingDotError(pos eng.SrcPos) error {
	return &eng.AqlError{
		Code:   "syntax_error",
		Detail: "`.` member access has no receiver",
		Hint:   "a `.`/`!.` access must follow a value (e.g. `m.key`); a leading `.` is not valid — write `0.5` for a fraction, or `$.key` for a detached accessor",
		Row:    pos.Row,
		Col:    pos.Col,
		Src:    pos.Src,
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
	var qkSet map[string]bool // quoted keys ({'k': v} syntax)
	var shList []string       // shorthand keys ({foo} / {foo/r} syntax)
	if len(meta) > 0 && meta[0] != nil {
		qmSet, _ = meta[0]["qm"].(map[string]bool)
		ckSet, _ = meta[0]["ck"].(map[string]bool)
		qkSet, _ = meta[0]["qk"].(map[string]bool)
		shList, _ = meta[0]["sh"].([]string)
	}

	// Word modifiers (`/r`, `/q`, `/f`, `/s`, `/N`) are legal only on
	// shorthand entries — `{foo/r}` ≡ `{foo: foo/r}` — where the token is
	// also the value, so the modifier qualifies that value. On a bare
	// `key: value` pair the modifier would only ever land on the KEY,
	// which is meaningless (a map key is a plain name). Reject it rather
	// than silently keeping `f/r` as a literal key. A quoted key
	// (`{'f/r': …}`) or computed key (`{[f/r]: …}`) is an explicit string
	// literal — the slash is data, not a modifier — so those are exempt.
	// Bare explicit keys (with and without an optional `?`) arrive in m;
	// shorthand (sh) and value-less optional (qm) tokens are handled below
	// where the modifier legitimately rides along on the synthesized value.
	for key := range m {
		if qkSet[key] || ckSet[key] {
			continue
		}
		if _, _, _, _, _, _, _, hasMod := scanWordModifier(key); hasMod {
			return eng.Value{}, fmt.Errorf(
				"[aql/illegal_key]: word modifier not allowed on map key %q (modifiers qualify values, not keys; quote the key as '%s' for a literal slash)",
				key, key)
		}
	}

	// Shorthand entries: `{foo}` ≡ `{foo: foo}`, `{foo/r}` ≡ `{foo: foo/r}`,
	// `{foo?}` ≡ `{foo?: foo}`, `{foo/r?}` ≡ `{foo?: foo/r}`. The key is
	// always the base name (modifiers stay on the value only); the value
	// is the full word token. synth maps each synthesized base key to that
	// token. Two sources: explicit shorthand tokens (Meta["sh"]) and
	// optional keys (Meta["qm"]) that never received an explicit value.
	//
	// optBase collects the base name of every optional key so optionality
	// is looked up by base name below — qmSet is keyed by the raw token
	// (e.g. `f/r`), which no longer matches the synthesized base key.
	synth := make(map[string]string)
	optBase := make(map[string]bool, len(qmSet))
	for _, raw := range shList {
		synth[wordBaseName(raw)] = raw
	}
	for key := range qmSet {
		base := wordBaseName(strings.TrimSuffix(key, "?"))
		optBase[base] = true
		if _, ok := m[key]; !ok {
			synth[base] = key
		}
	}

	// Iterate the sorted union of value-map keys and synthesized keys.
	keySet := make(map[string]bool, len(m)+len(synth))
	union := make(map[string]any, len(m)+len(synth))
	for k, v := range m {
		union[k] = v
		keySet[k] = true
	}
	for k := range synth {
		if !keySet[k] {
			union[k] = nil
			keySet[k] = true
		}
	}

	for _, key := range sortedKeys(union) {
		var child eng.Value
		var err error
		if _, inMap := m[key]; inMap {
			child, err = convertDataValue(m[key])
		} else {
			child, err = parseWord(synth[key])
		}
		if err != nil {
			return eng.Value{}, err
		}
		// Optional field: `?:T` means "None or absent, universally" —
		// desugared to `disjunct(T, None, Absent)`. The Absent
		// alternative carries the "may be missing" half of the rule
		// (the map unifier synthesises Absent when a key is absent);
		// the None alternative carries the "may be explicitly None"
		// half. No separate metadata needed — the optionality lives
		// entirely in the type.
		optional := qmSet[key] || optBase[key]
		realKey := key
		if strings.HasSuffix(key, "?") {
			realKey = strings.TrimSuffix(key, "?")
			optional = true
		}
		if optional {
			child = eng.NewDisjunct([]eng.Value{
				child,
				eng.NewTypeLiteral(eng.TNone),
				eng.NewTypeLiteral(eng.TAbsent),
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
// Quoted text → strings, unquoted text → words (executable). The value is
// stamped with the source position captured by the val-rule BC (if any).
func convertDataValue(v any) (eng.Value, error) {
	node, pos := deSite(v)
	res, err := convertDataValueInner(node)
	if err != nil {
		return res, err
	}
	return withPos(res, pos), nil
}

func convertDataValueInner(v any) (eng.Value, error) {
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
		return numberValToValue(val)

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
		// An untyped-nil element is an EMPTY list slot — `[1,,2]`, `[,1]`,
		// a leading/repeated comma. (An explicit `null` arrives as the
		// jsonic.Text token "null", handled above, so this case only fires
		// for a genuinely empty element.) A repeated or leading comma is a
		// typo, not a value — reject it rather than fabricating a `null`.
		return eng.Value{}, emptyElementError()

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
	switch text {
	case "inf":
		return eng.NewFloat(math.Inf(1))
	case "-inf":
		return eng.NewFloat(math.Inf(-1))
	case "nan":
		return eng.NewFloat(math.NaN())
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

// scanWordModifier parses the optional `/...` modifier suffix of an
// unquoted word token: name/f (forceForward), name/s (forceStack),
// name/N (argCount), name/q (quote → Atom), name/r (ref → bound value),
// name/u (usurp → reversed-sig wrapper), and combinations like name/1f,
// name/qs, name/f2, name/ur. Modifiers stack in any order; f and s are
// mutually exclusive; q is mutually exclusive with both r and u (an atom
// has no binding to reference or usurp); u may combine with r (name/ur);
// the argCount digits form a single number. When the token has no `/` or a
// malformed modifier suffix, valid is false, base is the whole text, and
// every flag is at its zero value (argCount -1).
func scanWordModifier(text string) (base string, argCount int, forceStack, forceForward, quoteFlag, refFlag, usurpFlag, valid bool) {
	argCount = -1

	idx := strings.LastIndex(text, "/")
	if idx < 0 || idx >= len(text)-1 {
		return text, argCount, false, false, false, false, false, false
	}
	mod := text[idx+1:]
	baseName := text[:idx]

	// Scan modifier chars in any order: digits, 'f', 's', 'q', 'r', 'u'.
	// Each letter appears at most once; f/s are mutually exclusive; q is
	// mutually exclusive with r and u; digits run contiguously and form a
	// single argCount value.
	valid = true
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
			if quoteFlag || refFlag || usurpFlag {
				valid = false
			} else {
				quoteFlag = true
			}
		case c == 'r':
			if refFlag || quoteFlag {
				valid = false
			} else {
				refFlag = true
			}
		case c == 'u':
			if usurpFlag || quoteFlag {
				valid = false
			} else {
				usurpFlag = true
			}
		default:
			valid = false
		}
		if !valid {
			break
		}
		i++
	}

	if !valid {
		// Unrecognized / malformed modifier — treat entire token as plain word.
		return text, -1, false, false, false, false, false, false
	}
	return baseName, argCount, forceStack, forceForward, quoteFlag, refFlag, usurpFlag, true
}

// wordBaseName returns the base name of an unquoted word token, stripping
// a valid `/...` modifier suffix (foo/r → foo, foo → foo). Used to derive
// the key of a shorthand map entry, whose value keeps the full token.
func wordBaseName(text string) string {
	base, _, _, _, _, _, _, _ := scanWordModifier(text)
	return base
}

// parseWord interprets an unquoted text token as an AQL word, handling
// the modifier syntax decoded by scanWordModifier. q produces an Atom and
// overrides the other modifiers; u emits a usurp-word and r emits a
// ref-word, both of which short-circuit the rest.
func parseWord(text string) (eng.Value, error) {
	name, argCount, forceStack, forceForward, quoteFlag, refFlag, usurpFlag, _ := scanWordModifier(text)

	if name == "" {
		return eng.Value{}, fmt.Errorf("empty word")
	}

	// /q produces an Atom: equivalent to (quote name). Other modifiers
	// in the same suffix are accepted but ignored — the atom is data,
	// not a function call.
	if quoteFlag {
		return eng.NewAtom(name), nil
	}

	// /u emits a usurp-word that resolves the name to its bound Function
	// value and wraps it with reversed signature arg order. Legal only for
	// function words (illegal_ref at run time otherwise). It may combine
	// with /r: /u alone dispatches the wrapper, /ur leaves it as data.
	// Argument-shape modifiers don't apply (the wrapper supplies its own).
	if usurpFlag {
		return eng.NewWordUsurp(name, refFlag), nil
	}

	// /r emits a ref-word that, when reached at the pointer, resolves the
	// name to its bound Function value without invoking. /r is legal only
	// for function words; a non-fn binding raises illegal_ref at run time
	// (the parser accepts the syntax — the binding kind isn't known until
	// resolution). Argument-shape modifiers don't apply because ref
	// bypasses dispatch entirely; they're accepted syntactically but
	// ignored.
	if refFlag {
		return eng.NewWordRef(name), nil
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

	// IEEE-754 special-value literals, following the AQL reserved-literal
	// convention (lowercase, parser-emitted, like true / false / none):
	// `inf` / `-inf` / `nan` produce the corresponding Float. They render
	// back to these same tokens (see FormatFloat), so print∘parse is
	// identity. `inf negate` is the long form for -inf.
	switch name {
	case "inf":
		return eng.NewFloat(math.Inf(1)), nil
	case "-inf":
		return eng.NewFloat(math.Inf(-1)), nil
	case "nan":
		return eng.NewFloat(math.NaN()), nil
	}

	// Reserved tape-syntax tokens emit typed marker values so the
	// engine recognises them by Parent identity (parens, end / ';').
	// These would otherwise become plain Word values that the engine
	// would have to name-dispatch in stepWord.
	switch name {
	case "end":
		return eng.NewEnd(), nil
	case ")":
		return eng.NewCloseParen(), nil
	}

	// *Type names resolve to type literals even in word context, so that
	// they retain their meaning inside quotations (e.g. [String,Float]).
	if t, ok := typeNames[name]; ok {
		return eng.NewTypeLiteral(t), nil
	}
	if t, ok := eng.ResolveTypePath(name); ok {
		return eng.NewTypeLiteral(t), nil
	}

	// A base-prefixed integer token whose magnitude jsonic could not lex
	// as a number (>= 2^63) arrives here as bare text. Parse it exactly:
	// `-0x8000000000000000` (= int64 min) succeeds, while a truly
	// out-of-range magnitude becomes a clean [aql/integer_overflow]
	// instead of an opaque undefined_word. (In-range base-prefixed
	// literals never reach this path — jsonic lexes them as numbers.)
	if isBasePrefixedInteger(name) {
		if strings.IndexByte(name, '_') >= 0 && !validUnderscores(name) {
			return eng.Value{}, &eng.AqlError{Code: "syntax_error", Src: name,
				Detail: "misplaced `_` in numeric literal: " + name,
				Hint:   "`_` is a single digit-separator — use one between digits"}
		}
		n, err := strconv.ParseInt(stripUnderscores(name), 0, 64)
		if err == nil {
			return eng.NewInteger(n), nil
		}
		if isRangeError(err) {
			return eng.Value{}, integerLiteralOverflowError(name, 0, 0)
		}
		// Other parse errors (e.g. an invalid digit) fall through to the
		// numeric-shape classification below.
	}

	// A digit-led token whose magnitude OVERFLOWS binary64 (e.g. 1e309)
	// reached here because jsonic could not lex it. Report a clear
	// float-overflow error instead of an opaque "undefined word". We use
	// ParseFloat's *range* error as the discriminator: it means the token
	// is a well-formed number that is merely out of range — never a
	// digit-led WORD (e.g. `2dup`, which fails ParseFloat with a *syntax*
	// error and falls through to be resolved as a word).
	if isDigitLed(name) {
		if f, err := strconv.ParseFloat(stripUnderscores(name), 64); isRangeError(err) || math.IsInf(f, 0) {
			return eng.Value{}, floatLiteralOverflowError(name)
		}
	}

	return eng.NewWord(name), nil
}

// isDigitLed reports whether s starts (after an optional sign) with a
// decimal digit — a necessary precondition for the numeric-overflow
// check. Note a digit-led token is NOT necessarily a number: `2dup` and
// friends are valid words.
func isDigitLed(s string) bool {
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	return len(s) > 0 && s[0] >= '0' && s[0] <= '9'
}

// isRangeError reports whether err is a strconv range (overflow) error.
func isRangeError(err error) bool {
	ne, ok := err.(*strconv.NumError)
	return ok && ne.Err == strconv.ErrRange
}

// floatLiteralOverflowError reports a floating-point literal whose
// magnitude overflows binary64 to ±infinity (e.g. 1e309).
func floatLiteralOverflowError(src string) error {
	return &eng.AqlError{
		Code:   "float_overflow",
		Detail: "floating-point literal out of range: " + src + " overflows to infinity",
		Src:    src,
		Hint:   "the Float range is about ±1.8e308; write the `inf` literal for infinity",
	}
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
		item, _ := deSite(item) // interp parts are not normally sited; guard anyway
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

// numberVal wraps a float64 with its source text and source position so
// we can (1) distinguish integer literals (e.g. "5") from decimal
// literals (e.g. "5.0"), (2) parse integers from their exact digits, and
// (3) locate an out-of-range integer literal in the source. Injected by
// the jsonic Sub callback for every number token (setupNumberSub).
type numberVal struct {
	Val      float64
	Src      string
	Row, Col int // 1-based source position of the literal (0 = unknown)
}

// floatToValue converts a JSON float64 to the appropriate AQL numeric value.
// Whole numbers become integers; fractional values become decimals.
func floatToValue(f float64) eng.Value {
	if f == float64(int64(f)) && !math.IsInf(f, 0) && !math.IsNaN(f) {
		return eng.NewInteger(int64(f))
	}
	return eng.NewFloat(f)
}

// numberValToValue converts a numberVal (float64 + source) to the
// appropriate AQL numeric value.
//
//   - A source containing "." is always a Float (even whole-valued, e.g.
//     5.0), using the float64 jsonic produced.
//   - A *plain decimal integer* source (optional leading '-', then digits
//     and '_' separators only — no base prefix, no exponent) is parsed
//     from its exact digits with strconv.ParseInt. This is the fix for
//     WAT Exhibit K: routing through float64 silently corrupts any
//     integer above 2^53 (e.g. 9007199254740993 → ...992) and turns
//     near-int64-max literals into Floats. An out-of-int64-range literal
//     now raises [aql/integer_overflow] instead of silently degrading.
//   - Everything else (scientific notation like 1e3, or a base prefix
//     like 0x10 / 0o17 / 0b101) keeps the existing float64-derived path,
//     which already matches jsonic's own base interpretation.
//
// See design/INTEGER-OVERFLOW-STRATEGY.0.md.
func numberValToValue(nv numberVal) (eng.Value, error) {
	// `_` is a single digit-separator only: it must sit between two
	// digits (no leading, trailing, or repeated underscores). `1__0` and
	// `1_` are rejected.
	if strings.IndexByte(nv.Src, '_') >= 0 && !validUnderscores(nv.Src) {
		return eng.Value{}, underscoreError(nv)
	}
	if strings.Contains(nv.Src, ".") {
		// A leading `.` (with or without a sign) is not a valid number —
		// `.5`, `-.5`, `+.5` are rejected; write `0.5` / `-0.5`. The bare
		// `.5` is caught earlier as a stray `.` token; the signed forms
		// reach here because jsonic lexes them as one number token.
		if isLeadingDotFloat(nv.Src) {
			return eng.Value{}, leadingDotNumberError(nv)
		}
		// Parse the Float from its exact source digits for a guaranteed
		// correctly-rounded (round-ties-to-even) decimal→binary64
		// conversion, rather than trusting the float64 jsonic produced.
		// strconv.ParseFloat is IEEE-correct; fall back to jsonic's value
		// only if the (jsonic-validated) token somehow fails to reparse.
		if f, err := strconv.ParseFloat(stripUnderscores(nv.Src), 64); err == nil {
			return eng.NewFloat(f), nil
		}
		return eng.NewFloat(nv.Val), nil
	}
	if isPlainDecimalInteger(nv.Src) {
		n, err := strconv.ParseInt(stripUnderscores(nv.Src), 10, 64)
		if err != nil {
			return eng.Value{}, integerLiteralOverflowError(nv.Src, nv.Row, nv.Col)
		}
		return eng.NewInteger(n), nil
	}
	if isBasePrefixedInteger(nv.Src) {
		// base 0 auto-detects the 0x / 0o / 0b prefix; parsing the exact
		// digits keeps full precision above 2^53 and range-checks like
		// the decimal path (out of int64 range → integer_overflow).
		n, err := strconv.ParseInt(stripUnderscores(nv.Src), 0, 64)
		if err != nil {
			return eng.Value{}, integerLiteralOverflowError(nv.Src, nv.Row, nv.Col)
		}
		return eng.NewInteger(n), nil
	}
	return floatToValue(nv.Val), nil
}

// isAlnum reports whether c is an ASCII letter or digit (the chars that
// may flank a `_` digit-separator: decimal/hex digits and the x/o/b/e of
// a prefix or exponent).
func isAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// validUnderscores reports whether every `_` in src is a single separator
// flanked by alphanumeric characters — no leading, trailing, or repeated
// underscores. `1_000` and `0xFF_FF` pass; `1__0`, `1_`, `_1` fail.
func validUnderscores(src string) bool {
	for i := 0; i < len(src); i++ {
		if src[i] != '_' {
			continue
		}
		if i == 0 || i == len(src)-1 || !isAlnum(src[i-1]) || !isAlnum(src[i+1]) {
			return false
		}
	}
	return true
}

// isLeadingDotFloat reports whether src is a `.`-leading number (after an
// optional sign): `.5`, `-.5`, `+.5`. These need a digit before the dot.
func isLeadingDotFloat(src string) bool {
	s := src
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	return len(s) > 0 && s[0] == '.'
}

// isPlainDecimalInteger reports whether src is a base-10 integer literal:
// an optional leading sign ('-' or '+') followed by one or more decimal
// digits, with '_' permitted as a digit separator. It deliberately
// rejects base prefixes (0x/0o/0b — handled by isBasePrefixedInteger) and
// exponents (1e3). Leading zeros stay decimal (jsonic treats 010 as 10,
// not octal), which base-10 ParseInt also does.
func isPlainDecimalInteger(src string) bool {
	if src == "" {
		return false
	}
	i := 0
	if src[0] == '-' || src[0] == '+' {
		i = 1
	}
	if i == len(src) {
		return false
	}
	sawDigit := false
	for ; i < len(src); i++ {
		c := src[i]
		switch {
		case c >= '0' && c <= '9':
			sawDigit = true
		case c == '_':
			// separator; allowed between digits
		default:
			return false
		}
	}
	return sawDigit
}

// isBasePrefixedInteger reports whether src is a hex / octal / binary
// integer literal — an optional sign then a 0x / 0o / 0b prefix. These are
// parsed exactly (strconv.ParseInt base 0) rather than via float64, so
// values above 2^53 keep full precision instead of silently rounding.
func isBasePrefixedInteger(src string) bool {
	s := src
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	if len(s) < 3 || s[0] != '0' {
		return false
	}
	switch s[1] {
	case 'x', 'X', 'o', 'O', 'b', 'B':
		return true
	}
	return false
}

// stripUnderscores removes '_' digit separators so strconv.ParseInt
// (base 10) accepts a source jsonic lexed with separators (e.g. 1_000).
func stripUnderscores(src string) string {
	if !strings.Contains(src, "_") {
		return src
	}
	return strings.ReplaceAll(src, "_", "")
}

// integerLiteralOverflowError reports an integer literal (decimal or
// base-prefixed) that does not fit in int64, located at the literal's
// source position when known (row/col 0 = unknown). Until Integer becomes
// arbitrary-precision (Phase 1 of design/INTEGER-OVERFLOW-STRATEGY.0.md),
// this is a hard parse error rather than a silent fall-back to Float.
func integerLiteralOverflowError(src string, row, col int) error {
	return &eng.AqlError{
		Code:   "integer_overflow",
		Detail: "integer literal out of range: " + src + " exceeds the Integer range (-9223372036854775808..9223372036854775807)",
		Row:    row,
		Col:    col,
		Src:    src,
		Hint:   "this value exceeds the 64-bit signed Integer range; use a Float (e.g. add a decimal point) for an approximate magnitude",
	}
}

// underscoreError reports a misused `_` digit-separator (leading, trailing,
// or repeated) in a numeric literal.
func underscoreError(nv numberVal) error {
	return &eng.AqlError{
		Code:   "syntax_error",
		Detail: "misplaced `_` in numeric literal: " + nv.Src,
		Row:    nv.Row,
		Col:    nv.Col,
		Src:    nv.Src,
		Hint:   "`_` is a single digit-separator — use one between digits, e.g. `1_000` (not `1__0` or `1_`)",
	}
}

// leadingDotNumberError reports a `.`-leading numeric literal (`-.5`,
// `+.5`); a number needs a digit before the decimal point.
func leadingDotNumberError(nv numberVal) error {
	return &eng.AqlError{
		Code:   "syntax_error",
		Detail: "numeric literal has no digit before `.`: " + nv.Src,
		Row:    nv.Row,
		Col:    nv.Col,
		Src:    nv.Src,
		Hint:   "write a leading zero, e.g. `-0.5` instead of `-.5`",
	}
}
