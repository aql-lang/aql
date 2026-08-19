package core

import (
	"strconv"
	"strings"
)

// canonString renders a String payload as parseable boru source — the
// round-trip half of the canon contract. The plain single-quoted form
// is kept verbatim for ordinary content (the form every spec row and
// doc example pins); content containing a single quote switches to
// double quotes; content with both quote kinds, backslashes, or
// control characters falls back to single quotes with backslash
// escapes. Whatever is emitted re-parses to the same string —
// previously `it's` rendered as 'it's', which re-parsed wrongly.
func canonString(s string) string {
	hasSingle := strings.ContainsRune(s, '\'')
	hasEscape := strings.ContainsAny(s, "\\\n\t\r")
	switch {
	case !hasSingle && !hasEscape:
		return "'" + s + "'"
	case !strings.ContainsRune(s, '"') && !hasEscape:
		return `"` + s + `"`
	default:
		var b strings.Builder
		b.WriteByte('\'')
		for _, r := range s {
			switch r {
			case '\\':
				b.WriteString(`\\`)
			case '\'':
				b.WriteString(`\'`)
			case '\n':
				b.WriteString(`\n`)
			case '\t':
				b.WriteString(`\t`)
			case '\r':
				b.WriteString(`\r`)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteByte('\'')
		return b.String()
	}
}

// Canon renders a stack of values as canonical boru source — a string
// that, when parsed and evaluated, reproduces the input stack. Where it
// diverges from Value.String:
//
//   - atoms render as `name/q` (bare `name` would parse as a word
//     lookup, not as an atom value; the /q suffix is the preferred
//     short form over `(quote name)`)
//   - quoted lists render as `(quote [...])` so the Quoted flag survives
//     a round-trip (the /q suffix is only defined for words)
//
// Lists and maps are space-separated in both Canon and Value.String
// (commas are optional in boru source and the default render omits
// them); the atom and quoted-list rules above are what keep Canon
// distinct from Value.String.
//
// Values without a known canonical form (runtime markers, errors,
// foreign types) fall back to Value.String.
func Canon(stack []Value) string {
	parts := make([]string, len(stack))
	for i, v := range stack {
		parts[i] = CanonValue(v)
	}
	return strings.Join(parts, " ")
}

// CanonValue renders one value as canonical boru source. See Canon.
func CanonValue(v Value) string {
	// Behavior-driven dispatch for user-defined types: if a non-
	// builtin type in v.Parent's parent chain has a non-default
	// Behavior, route through it. This is how user-installed canon
	// bodies (`behave canon/q (fn [[T] [String] [body]])`) flow
	// into eng.Canon.
	//
	// Built-in Behaviors (listFormatBehavior, mapFormatBehavior,
	// dateFormatBehavior, …) are deliberately skipped here — they
	// produce Value.String's debug form (e.g. time-domain renderings,
	// bare atoms) which doesn't match Canon's source-shape conventions
	// (e.g. `name/q` atoms, quoted strings). CanonValue's own switch
	// below preserves those.
	if v.Data != nil && v.Parent != nil {
		for t := v.Parent; t != nil; t = t.Parent {
			if t.Origin == OriginBuiltin {
				continue
			}
			if t.Behavior() == nil || t.Behavior() == DefaultBehavior {
				continue
			}
			if delegatesFormat(t.Behavior()) {
				continue
			}
			return t.Behavior().Format(v)
		}
	}
	switch {
	case IsNone(v):
		return "none"
	case IsCloseParen(v):
		// A STRAY `)` — one the parser could not pair, so it survives as a
		// value rather than closing a group. Canon had no arm, so the
		// payload-less fallthrough rendered it as the empty string and
		// `1 )` canon'd to `1 `, dropping the token silently. Value.String
		// has always rendered it `)`. Same defect the End marker had; an
		// unmatched OPENING paren never reaches here, because that is a
		// parse error rather than a value.
		return ")"
	case IsDispatchMod(v):
		// The `/v` / `/q` group modifier the parser emits AFTER a paren or
		// dotted-path group. Canon had no arm for it, so each engine fell
		// through to a DIFFERENT debug spelling — Go to
		// `word()({false true})` (the word arm reading a DispatchModInfo as
		// a WordInfo) and TS to `word(undefined)`. Neither is source, and
		// they disagreed, which is what the parity probe caught on `m.k/q`.
		if info, ok := AsDispatchMod(v); ok {
			if info.Val {
				return "/v"
			}
			return "/q"
		}
		return "/q" //covergate:allow IsDispatchMod is exactly `Parent == TDispatchMod`, and NewDispatchMod is the only constructor of that type — the payload is always a DispatchModInfo
	case IsEnd(v):
		// `end` is the WORD and `;` is its synonym (REFERENCE.md:415), so
		// the canonical source for either is `end`. This arm was missing,
		// and the payload-less fallthrough below rendered the marker as the
		// empty string — canon of `1 ;` came out `1 ` and reparsed as a bare
		// `1`, silently dropping the collection barrier. Value.String has
		// always rendered it `end` (value.go); only canon disagreed.
		return "end"
	case v.Data == nil:
		if t := TypeNodeOf(v); t != nil {
			if name := TypeNameByID(t.ID); name != "" {
				return name
			}
			return t.Leaf()
		}
		return "none"
	case v.IsDepScalar():
		return v.String()
	case v.Parent.ConformsTo(TBigInteger):
		n, _ := AsBigInteger(v)
		return FormatBigInteger(n)
	case v.Parent.ConformsTo(TBigDecimal):
		d, _ := AsBigDecimal(v)
		return FormatBigDecimal(d)
	case v.Parent.ConformsTo(TInteger):
		n, _ := AsInteger(v)
		return strconv.FormatInt(n, 10)
	case v.Parent.ConformsTo(TFloat):
		f, _ := AsFloat(v)
		return FormatFloat(f)
	case v.Parent.ConformsTo(TString):
		s, _ := AsString(v)
		return canonString(s)
	case v.Parent.ConformsTo(TBoolean):
		b, _ := AsBoolean(v)
		if b {
			return "true"
		}
		return "false"
	case v.Parent.ConformsTo(TAtom):
		s, _ := AsAtom(v)
		return s + "/q"
	case IsBoundedType(v):
		// `Type of [B]` canons as the suffix sugar B/t (the shortest
		// round-trippable spelling — the /q-for-atoms convention).
		if n, err := AsBoundedType(v); err == nil {
			return n.Leaf() + "/t"
		}
		return v.String()
	case IsWeakFlexMap(v):
		// STABLE SNAPSHOT RENDER, not a semantic round-trip: re-parsing
		// stores literal entries whose referents are dead on arrival.
		// The Node column's first descriptive canon, declared as such
		// (design/FLEX-ATTRS.1.md §4.7).
		m, err := AsMap(v)
		if err != nil || m == nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return v.String()
		}
		return "(make WeakFlexMap {" + joinEntries(m, canonChild) + "})"
	case IsWeakFlexList(v):
		lst, _ := AsList(v)
		parts := make([]string, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			parts[i] = canonChild(lst.Get(i))
		}
		return "(make WeakFlexList [" + strings.Join(parts, " ") + "])"
	case IsWeakFlexXml(v):
		return "(make WeakFlexXml " + v.String() + ")"
	case IsFlexList(v):
		// Round-trippable source form — a plain `[...]` would parse
		// back as an immutable List and lose the flexness.
		lst, _ := AsList(v)
		parts := make([]string, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			parts[i] = canonChild(lst.Get(i))
		}
		return "(flex [" + strings.Join(parts, " ") + "])"
	case IsFlexMap(v):
		m, err := AsMap(v)
		if err != nil || m == nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return v.String()
		}
		return "(flex {" + joinEntries(m, canonChild) + "})"
	case v.Parent.ConformsTo(TList) && v.Data != nil:
		lst, _ := AsList(v)
		parts := make([]string, 0, lst.Len()+1)
		// A TYPED list keeps its element-type tag: `[:Integer 1 2]` is the
		// source syntax (REFERENCE.md:228) and the tag is part of the value
		// — `deq` ignoring it (REFERENCE.md:1224) is only meaningful because
		// it is there. AsList returns the ELEMENTS alone, so canon used to
		// render `[1 2]` and `[:Integer]` collapsed to `[]`: a canonical
		// render that does not parse back as the same value.
		if ct, ok := v.Data.(ChildTypeInfo); ok {
			parts = append(parts, ":"+canonTypeTag(ct.Child))
		}
		for i := 0; i < lst.Len(); i++ {
			parts = append(parts, canonChild(lst.Get(i)))
		}
		body := "[" + strings.Join(parts, " ") + "]"
		if v.Quoted {
			return "(quote " + body + ")"
		}
		return body
	case v.Parent.Equal(TMap) && v.Data != nil:
		// A typed MAP keeps its value-type tag, for the same reason a typed
		// list keeps its element tag. Canon was inconsistent in both
		// directions here: a bare `{:String}` fell past the AsMap guard to
		// Value.String and leaked the debug spelling `{:word(String)}`,
		// while `{:Integer a:1}` reached joinEntries and lost the tag
		// entirely as `{a:1}`.
		if ct, ok := v.Data.(ChildTypeInfo); ok {
			parts := make([]string, 0, len(ct.Entries)+1)
			parts = append(parts, ":"+canonTypeTag(ct.Child))
			for _, e := range ct.Entries {
				parts = append(parts, e.Key+":"+canonChild(e.Value))
			}
			return "{" + strings.Join(parts, " ") + "}"
		}
		m, err := AsMap(v)
		if err != nil || m == nil {
			return v.String()
		}
		return "{" + joinEntries(m, canonChild) + "}"
	case IsReach(v):
		return canonReach(v)
	case IsWord(v) && hasWordModifiers(v):
		// A word carries its `/`-modifiers in the payload, and dropping
		// them made canon say something the source did not (NUR059): both
		// `foo/v` and `foo/2` rendered `word(foo)`, so re-parsing the canon
		// yielded a different program.
		//
		// Scoped to words that HAVE a modifier. A bare word keeps its
		// existing `word(foo)` spelling: rendering every word bare is a
		// far larger change (175 of the corpus's 724 rows) and a different
		// question — `word(foo)` denotes a word VALUE, while bare `foo`
		// re-parses as a word that will be DISPATCHED — which this record
		// does not decide. The modifier-bearing form follows the atom
		// convention the record cites as already correct (`foo/q`).
		w, _ := AsWord(v)
		return w.Name + canonWordModifiers(w)
	case IsSugar(v):
		// Sugar markers have a surface spelling of their own; without one
		// they fell through to the debug dump `sugar(angle Box [word(…)])`
		// (NUR059).
		if info, ok := AsSugar(v); ok {
			if out, handled := canonSugar(info); handled {
				return out
			}
		}
		return v.String()
	case IsParenExpr(v):
		// `(1 add 2)` rather than `paren([1 word(add) 2])` (NUR059). The
		// body renders through canonReachTokens, which keeps words bare —
		// inside a group they are CODE, not atom data.
		toks, _ := AsParenExpr(v)
		return "(" + canonReachTokens(toks) + ")"
	case isFnDefValue(v):
		// A function value participates in the total order (cmp/sort),
		// so its canon form must DISCRIMINATE between distinct fns —
		// two same-shaped-but-different-body predicates must not collapse
		// to one string. Render the name plus each sig's params, returns,
		// and body. Deliberately excludes Registry and Captured: a fn's
		// closure environment is not part of its identity for ordering,
		// and dumping it would spill the module exports map (the leak
		// formatFnDef fixed for String()). See canonFnDef.
		fd, _ := v.Data.(FnDefInfo)
		return canonFnDef(fd)
	default:
		return v.String()
	}
}

// canonChild renders a value that sits INSIDE a composite canon (a map value,
// a list element, a record/childtype field). A disjunct's `A tor B` renders
// with whitespace, so concatenated into a container it fragments into extra
// word-separated tokens (`{x:Integer tor String}` reads as broken map syntax)
// and breaks the source round-trip; grouping it in parens keeps the compound
// canon re-parseable (`{x:(Integer tor String)}`). A TOP-LEVEL disjunct canon
// is left ungrouped — it re-parses as-is AND it is a comparison ordering key
// (compareStructural), so a leading `(` there would silently reorder unions.
// The TS engine mirrors this (eng/ts/src/canon.ts::canonChild).
func canonChild(v Value) string {
	if IsDisjunct(v) {
		return "(" + CanonValue(v) + ")"
	}
	return CanonValue(v)
}

// canonTypeTag renders a container's element-type tag in SOURCE form. The
// parser is type-name-opaque (ADR-012 rule 4), so the tag on an unevaluated
// literal is still a bare Word — `[:Integer]` holds word(Integer), not the
// Integer type node — and rendering it through the generic value path would
// leak the debug spelling `word(Integer)` into what is meant to be source.
func canonTypeTag(v Value) string {
	if IsWord(v) {
		// The modifier suffix rides along (NUR059): an angle argument can
		// be a modifier-bearing word (`Box<a/v>`, `Box<a/2>`), and emitting
		// the bare name would drop it — the same silent loss this record
		// exists to remove, reintroduced one level down. A tag that carries
		// no modifier renders exactly as before.
		w, _ := AsWord(v)
		return w.Name + canonWordModifiers(w)
	}
	return canonChild(v)
}

// canonReachToken renders one receiver/key token of a Reach as source:
// a Word as its bare name (m.a, not m.a/q), a ParenExpr / nested Reach
// recursively, anything else via CanonValue.
func canonReachToken(v Value) string {
	switch {
	case IsWord(v):
		// Bare NAME plus any modifier suffix (NUR059). A reach SEGMENT
		// never carries one — the parser peels a trailing `/mod` off the
		// final key and applies it to the whole reach — so segments render
		// exactly as before (`m.a`, not `m.a/q`). A word inside a PAREN
		// body can carry one, and `(x/v)` losing its `/v` would be the
		// same defect this record removes.
		w, _ := AsWord(v)
		return w.Name + canonWordModifiers(w)
	case IsParenExpr(v):
		toks, _ := AsParenExpr(v)
		return "(" + canonReachTokens(toks) + ")"
	case IsReach(v):
		return canonReach(v)
	case IsEnd(v):
		// The end marker renders as `;` inside a reach, never as `end`.
		// A bare `end` in reach-token position is a WORD — a field name in
		// a segment, an ordinary word in a paren body (NUR066) — so
		// spelling the marker `end` here would re-parse as that word and
		// silently change the program. `;` re-parses to this marker.
		return ";"
	default:
		return CanonValue(v)
	}
}

// canonReachTokens renders a token sequence (receiver / computed-key / paren
// body) as source, each token via canonReachToken so words stay bare.
func canonReachTokens(toks []Value) string {
	parts := make([]string, len(toks))
	for i, t := range toks {
		parts[i] = canonReachToken(t)
	}
	return strings.Join(parts, " ")
}

// canonReach renders a Reach back to its dotted surface — m.a.b, m!.x,
// m.'k', m.(expr), (expr).k — the read∘print round-trip (design/REACH.10.md
// §6). A Quoted (codequote-captured) reach wraps in (codequote …) so it
// round-trips, mirroring the list quote convention.
func canonReach(v Value) string {
	info, err := AsReach(v)
	if err != nil {
		return v.String()
	}
	var b strings.Builder
	switch len(info.Receiver) {
	case 0:
		// receiverless reach (a lens): the reserved `$` sentinel receiver,
		// so `read ∘ print` round-trips ($.name parses back to a lens).
		b.WriteString("$")
	case 1:
		b.WriteString(canonReachToken(info.Receiver[0]))
	default:
		b.WriteString("(" + canonReachTokens(info.Receiver) + ")")
	}
	for _, seg := range info.Segments {
		if seg.Getr {
			b.WriteString("!.")
		} else {
			b.WriteString(".")
		}
		if seg.Computed {
			b.WriteString("(" + canonReachTokens(seg.KeyExpr) + ")")
		} else {
			b.WriteString(canonReachToken(seg.KeyLit))
		}
	}
	if v.Quoted {
		return "(codequote " + b.String() + ")"
	}
	return b.String()
}

// canonFnDef renders a function value's discriminating canonical form:
// its name plus the params / returns / body of each authored signature.
// Used only by CanonValue (the ordering / structural-compare surface),
// so unlike formatFnDef it must distinguish fns that String() renders
// identically. It never touches FnDefInfo.Registry or .Captured.
func canonFnDef(fd FnDefInfo) string {
	// NO NAME (NUR031). Canon used to render `fn NAME[…]` keyed on the
	// binding the function happened to be reached through, so `def a (f/v)`
	// and `def b (f/v)` — two bindings of ONE function — canon'd
	// differently. That made canon name-DEPENDENT, which ADR-015 cannot
	// live with: a canon keyed on a name cannot round-trip, because
	// re-parsing it under a different binding yields a different rendering
	// of the same function.
	//
	// The anonymous fn literal is already valid source, re-parses to a
	// structurally identical function, and is name-independent by
	// construction rather than by a naming scheme. With the structural deq
	// below, `parse(canon(f/v)) deq f/v` holds.
	var b strings.Builder
	b.WriteString("fn [")
	sigs := fd.OwnSigs()
	for i := range sigs {
		if i > 0 {
			b.WriteByte(' ')
		}
		sig := &sigs[i]
		b.WriteByte('[')
		for j, p := range sig.Params {
			if j > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(p.Name)
			if p.Type != nil {
				b.WriteByte(':')
				b.WriteString(p.Type.String())
			}
		}
		b.WriteString("][")
		for j, r := range sig.Returns {
			if j > 0 {
				b.WriteByte(' ')
			}
			if r != nil {
				b.WriteString(r.String())
			}
		}
		b.WriteString("][")
		for j, t := range sig.Body() {
			if j > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(CanonValue(t))
		}
		b.WriteString("]")
	}
	b.WriteByte(']')
	return b.String()
}

// canonWordModifiers renders the `/`-suffix a WordInfo carries, or "" when
// it carries none (NUR059).
//
// Canon's contract is that its output re-parses to the same value, and
// these modifiers used to break it SILENTLY rather than loudly: a word
// with a modifier fell through to `Value.String`'s debug form, which
// spells every word `word(foo)` — so `foo/v` and `foo/2` both canon'd as
// `word(foo)` and the modifier was not merely mis-spelled but DROPPED.
// Re-parsing that canon yields a different program.
//
// The letters may be written in any order at the call site, so canon picks
// one CANONICAL order — digits, then f|s, then u, then r — and emits it
// consistently. `scanWordModifier` accepts that order, so the render
// re-parses; a stable order also keeps canon a usable sort key, which the
// total order relies on.
//
// `/q` is not here: a quoted word is an ATOM by the time it is a value
// (parseWord returns one before the modifier switch is reached), and the
// Atom arm already renders `name/q`.
// hasWordModifiers reports whether v is a Word carrying any `/`-suffix.
func hasWordModifiers(v Value) bool {
	w, err := AsWord(v)
	return err == nil && canonWordModifiers(w) != ""
}

func canonWordModifiers(w WordInfo) string {
	var b strings.Builder
	if w.ArgCount >= 0 {
		b.WriteString(strconv.Itoa(w.ArgCount))
	}
	switch {
	case w.ForceForward:
		b.WriteByte('f')
	case w.ForceStack:
		b.WriteByte('s')
	}
	if w.ForceUsurp {
		b.WriteByte('u')
	}
	if w.ForceVal {
		b.WriteByte('v')
	}
	if b.Len() == 0 {
		return ""
	}
	return "/" + b.String()
}

// canonSugar renders a sugar marker back to the surface syntax it came
// from (NUR059). Without this the marker fell through to `Value.String`,
// so `[:Box<Integer>]` canon'd as `[:sugar(angle Box [word(Integer)])]` —
// a debug dump no parser accepts.
//
// ONLY the angle form is handled, and the restraint is measured rather
// than cautious. The modifier kinds (usurp / stack-args / forward-args /
// force-arity) are lowered from a `/`-suffix canonWordModifiers already
// renders on the word itself, so reaching them here means a bare marker
// the user cannot write. The other three were TRIED and withdrawn: a
// per-row fixpoint check (does the new render re-parse to itself?) showed
// `+m'src'` rendering `+m<src>`, which re-parses with a stray `>` because
// SugarInfo does not retain the source delimiter, and `w/t` rendering
// `[w/q]/t`, which does not parse at all. A spelling that does not
// round-trip is worse than the debug form, because it looks like source.
func canonSugar(info SugarInfo) (string, bool) {
	switch info.Kind {
	case SugarAngle:
		parts := make([]string, len(info.Items))
		for i, it := range info.Items {
			parts[i] = canonTypeTag(it)
		}
		return info.Name + "<" + strings.Join(parts, " ") + ">", true
	}
	return "", false
}
