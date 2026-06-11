package eng

import (
	"strconv"
	"strings"
)

// canonString renders a String payload as parseable AQL source — the
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

// Canon renders a stack of values as canonical AQL source — a string
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
// (commas are optional in AQL source and the default render omits
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

// CanonValue renders one value as canonical AQL source. See Canon.
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
			if t.Behavior == nil || t.Behavior == DefaultBehavior {
				continue
			}
			if _, ok := t.Behavior.(formatDelegatesToDefault); ok {
				continue
			}
			return t.Behavior.Format(v)
		}
	}
	switch {
	case IsNone(v):
		return "none"
	case v.Data == nil:
		if t := typeNodeOf(v); t != nil {
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
	case v.Parent.Equal(TAtom):
		s, _ := AsAtom(v)
		return s + "/q"
	case IsFlexList(v):
		// Round-trippable source form — a plain `[...]` would parse
		// back as an immutable List and lose the flexness.
		lst, _ := AsList(v)
		parts := make([]string, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			parts[i] = CanonValue(lst.Get(i))
		}
		return "(flex [" + strings.Join(parts, " ") + "])"
	case IsFlexMap(v):
		m, err := AsMap(v)
		if err != nil || m == nil {
			return v.String()
		}
		parts := make([]string, m.Len())
		for i, k := range m.Keys() {
			val, _ := m.Get(k)
			parts[i] = k + ":" + CanonValue(val)
		}
		return "(flex {" + strings.Join(parts, " ") + "})"
	case v.Parent.ConformsTo(TList) && v.Data != nil:
		lst, _ := AsList(v)
		parts := make([]string, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			parts[i] = CanonValue(lst.Get(i))
		}
		body := "[" + strings.Join(parts, " ") + "]"
		if v.Quoted {
			return "(quote " + body + ")"
		}
		return body
	case v.Parent.Equal(TMap) && v.Data != nil:
		m, err := AsMap(v)
		if err != nil || m == nil {
			return v.String()
		}
		parts := make([]string, m.Len())
		for i, k := range m.Keys() {
			val, _ := m.Get(k)
			parts[i] = k + ":" + CanonValue(val)
		}
		return "{" + strings.Join(parts, " ") + "}"
	case IsReach(v):
		return canonReach(v)
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

// canonReachToken renders one receiver/key token of a Reach as source:
// a Word as its bare name (m.a, not m.a/q), a ParenExpr / nested Reach
// recursively, anything else via CanonValue.
func canonReachToken(v Value) string {
	switch {
	case IsWord(v):
		w, _ := AsWord(v)
		return w.Name
	case IsParenExpr(v):
		toks, _ := AsParenExpr(v)
		return "(" + canonReachTokens(toks) + ")"
	case IsReach(v):
		return canonReach(v)
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
	var b strings.Builder
	b.WriteString("fn ")
	b.WriteString(fd.Name)
	b.WriteByte('[')
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
		for j, t := range sig.Body {
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
