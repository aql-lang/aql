package native

import (
	"fmt"
	"strconv"
	"strings"
)

// The aontu parser — a Go implementation of the data-producing core of
// rjrodger's aontu (github.com/rjrodger/aontu), a CUE-inspired
// unification-based configuration dialect. aontu has no Go port, so unlike
// the tabnas family (see tabnas.go) this is a self-contained lexer + parser
// + unifier. It is wired in as a built-in `parse` kind by aql:parselang
// (see modules/parselang.go), so `import "aql:parselang"` then
// `parse aontu '<text>'` yields a Node data structure of Maps and Lists —
// the resolved configuration, exactly like the JSON family.
//
// Supported core (the subset that resolves to plain data):
//
//   - maps: top-level `a:1 b:2`, explicit braces `{a:1, b:2}`, and the
//     colon-chain `a:b:c:1` → {a:{b:{c:1}}}.
//   - lists: `[1, 2, 3]` or whitespace-separated `[1 2 3]`, mixed element
//     types.
//   - scalars: integers (incl. negative), floats, bare strings, quoted
//     strings, `true` / `false`, `null`.
//   - comments: `#` and `//` to end of line, and `/* … */` blocks.
//   - unification `&` and duplicate-key merge: two maps deep-merge, equal
//     scalars collapse, a conflict is a loud error.
//   - kinds (schema words) `string` / `number` / `integer` (`int`) /
//     `boolean` (`bool`) / `top`: a kind unifies with a matching scalar to
//     yield the scalar; a mismatch conflicts; a kind left with no concrete
//     value is an "incomplete" error.
//   - references: absolute `$.a.b` (with list indexing `$.a.1`) resolve
//     from the document root; relative `.k` resolves the key in the nearest
//     enclosing map scope (climbing outward).
//
// Not yet supported (documented limits, raised loudly if used): the `+`,
// `*` and `|` operators, optional `?` keys, the `.$KEY` self-key reference,
// and explicit map sealing.

// AontuParse parses aontu source text into generic Go data (map[string]any,
// []any, and scalars), or an error. opts is accepted for interface
// uniformity with the tabnas parser family (TabnasParser); aontu has no
// options yet, so it is ignored. It is the decoder the aql:parselang
// `aontu` kind is built from.
func AontuParse(src string, _ map[string]any) (any, error) {
	toks, err := lexAontu(src)
	if err != nil {
		return nil, err
	}
	p := &aontuParser{toks: toks}
	root, err := p.parseDocument()
	if err != nil {
		return nil, err
	}
	ev := &aontuEval{root: root, memo: map[*aTerm]any{}}
	out, err := ev.resolve(&aTerm{conj: []aAtom{root}}, []*aObj{}, map[*aTerm]bool{})
	if err != nil {
		return nil, err
	}
	if err := aontuCheckComplete(out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---- lexer ----------------------------------------------------------------

const (
	tEOF = iota
	tLBrace
	tRBrace
	tLBrack
	tRBrack
	tColon
	tAmp
	tString // quoted string literal (already unescaped)
	tAtom   // bare run: number / keyword / kind / bare string
	tRef    // $… (absolute) or .… (relative) path reference
)

type aToken struct {
	kind int
	text string   // tString/tAtom payload
	abs  bool     // tRef: true for $… , false for .…
	segs []string // tRef: path segments
}

// aontuSep reports the bytes that separate tokens (whitespace and comma);
// commas are pure separators in aontu, equivalent to whitespace.
func aontuSep(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ','
}

// aontuStructural reports the single-byte structural tokens that always end
// a bare atom.
func aontuStructural(c byte) bool {
	switch c {
	case '{', '}', '[', ']', ':', '&', '#', '"', '\'':
		return true
	}
	return false
}

func lexAontu(src string) ([]aToken, error) {
	var toks []aToken
	i, n := 0, len(src)
	for i < n {
		c := src[i]
		switch {
		case aontuSep(c):
			i++
		case c == '#':
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '/':
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			i += 2
			for i+1 < n && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			if i+1 >= n {
				return nil, fmt.Errorf("aontu: unterminated block comment")
			}
			i += 2
		case c == '{':
			toks = append(toks, aToken{kind: tLBrace})
			i++
		case c == '}':
			toks = append(toks, aToken{kind: tRBrace})
			i++
		case c == '[':
			toks = append(toks, aToken{kind: tLBrack})
			i++
		case c == ']':
			toks = append(toks, aToken{kind: tRBrack})
			i++
		case c == ':':
			toks = append(toks, aToken{kind: tColon})
			i++
		case c == '&':
			toks = append(toks, aToken{kind: tAmp})
			i++
		case c == '"' || c == '\'':
			s, ni, err := lexQuoted(src, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, aToken{kind: tString, text: s})
			i = ni
		case c == '$':
			j := i + 1
			for j < n && isRefChar(src[j]) {
				j++
			}
			toks = append(toks, aToken{kind: tRef, abs: true, segs: refSegs(src[i+1 : j])})
			i = j
		case c == '.':
			j := i + 1
			for j < n && isRefChar(src[j]) {
				j++
			}
			if j == i+1 {
				return nil, fmt.Errorf("aontu: bare '.' is not a valid reference")
			}
			toks = append(toks, aToken{kind: tRef, abs: false, segs: refSegs(src[i+1 : j])})
			i = j
		default:
			j := i
			for j < n && !aontuSep(src[j]) && !aontuStructural(src[j]) {
				j++
			}
			toks = append(toks, aToken{kind: tAtom, text: src[i:j]})
			i = j
		}
	}
	toks = append(toks, aToken{kind: tEOF})
	return toks, nil
}

// isRefChar reports the characters that may appear inside a reference path
// (segment names, the dot separators, and `$` for the self-key form).
func isRefChar(c byte) bool {
	return c == '.' || c == '_' || c == '$' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// refSegs splits a reference body ("a.b.1" or ".a") into its non-empty path
// segments. A leading dot (relative refs already strip the first dot at the
// call site, but absolute `$.a` keeps one) is dropped by the empty filter.
func refSegs(body string) []string {
	var out []string
	for _, s := range strings.Split(body, ".") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// lexQuoted scans a single- or double-quoted string starting at src[i],
// handling the standard JSON-ish escapes. It returns the unescaped contents
// and the index past the closing quote.
func lexQuoted(src string, i int) (string, int, error) {
	quote := src[i]
	i++
	var b strings.Builder
	for i < len(src) {
		c := src[i]
		switch c {
		case quote:
			return b.String(), i + 1, nil
		case '\\':
			if i+1 >= len(src) {
				return "", 0, fmt.Errorf("aontu: unterminated string escape")
			}
			switch src[i+1] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"':
				b.WriteByte('"')
			case '\'':
				b.WriteByte('\'')
			case '\\':
				b.WriteByte('\\')
			case '/':
				b.WriteByte('/')
			default:
				b.WriteByte(src[i+1])
			}
			i += 2
		default:
			b.WriteByte(c)
			i++
		}
	}
	return "", 0, fmt.Errorf("aontu: unterminated string literal")
}

// ---- AST ------------------------------------------------------------------

// aTerm is a conjunction: a sequence of atoms that must unify to one value.
// Duplicate map keys and the `&` operator both append to a term's conjuncts.
type aTerm struct{ conj []aAtom }

// aAtom is one of *aObj, *aArr, aScalar, aKind, aRef.
type aAtom interface{}

type aObj struct {
	keys   []string
	fields map[string]*aTerm
}

func newObj() *aObj { return &aObj{fields: map[string]*aTerm{}} }

// add unifies value t into key k, creating the field or appending conjuncts
// to an existing one (duplicate-key merge).
func (o *aObj) add(k string, t *aTerm) {
	if cur, ok := o.fields[k]; ok {
		cur.conj = append(cur.conj, t.conj...)
		return
	}
	o.keys = append(o.keys, k)
	o.fields[k] = t
}

type aArr struct{ items []*aTerm }

type aScalar struct{ v any } // int64, float64, string, bool, nil

type aKind struct{ name string } // canonical: string/number/integer/boolean/top

type aRef struct {
	abs  bool
	segs []string
}

// ---- parser ---------------------------------------------------------------

type aontuParser struct {
	toks []aToken
	pos  int
}

func (p *aontuParser) peek() aToken  { return p.toks[p.pos] }
func (p *aontuParser) next() aToken  { t := p.toks[p.pos]; p.pos++; return t }
func (p *aontuParser) at(k int) bool { return p.toks[p.pos].kind == k }

// entryAhead reports whether the upcoming tokens form a `key :` pair (a map
// entry), used to disambiguate a value-level `&` from an entry separator and
// to detect colon-chains.
func (p *aontuParser) entryAhead() bool {
	t := p.toks[p.pos]
	return (t.kind == tAtom || t.kind == tString) && p.toks[p.pos+1].kind == tColon
}

// parseDocument parses the whole source: a `[`/`{` opener yields a list/map
// value, otherwise the top level is an implicit map body.
func (p *aontuParser) parseDocument() (*aObj, error) {
	if p.at(tLBrack) || p.at(tLBrace) {
		// A bare top-level list or braced map: wrap it under no key by
		// returning a single-conjunct synthetic — but the document root must
		// be an *aObj for reference resolution, so only a braced map is a
		// valid top here; a bare list document is rejected (aontu documents
		// are maps).
		if p.at(tLBrace) {
			p.next()
			return p.parseStructBody(tRBrace)
		}
		return nil, fmt.Errorf("aontu: a document must be a map, not a top-level list")
	}
	return p.parseStructBody(tEOF)
}

// parseStructBody parses map entries until the closing token `end` (tRBrace
// or tEOF). Entries are separated by whitespace/comma; a `&` between entries
// is an entry separator (the unify is realised by duplicate-key merge).
func (p *aontuParser) parseStructBody(end int) (*aObj, error) {
	obj := newObj()
	for {
		for p.at(tAmp) { // skip entry-level separators
			p.next()
		}
		if p.at(end) {
			if end != tEOF {
				p.next()
			}
			return obj, nil
		}
		if p.at(tEOF) {
			return nil, fmt.Errorf("aontu: unexpected end of input, missing '}'")
		}
		key, err := p.parseKey()
		if err != nil {
			return nil, err
		}
		if !p.at(tColon) {
			return nil, fmt.Errorf("aontu: expected ':' after key %q", key)
		}
		p.next()
		val, err := p.parseEntryValue()
		if err != nil {
			return nil, err
		}
		obj.add(key, val)
	}
}

func (p *aontuParser) parseKey() (string, error) {
	t := p.peek()
	if t.kind == tAtom || t.kind == tString {
		p.next()
		return t.text, nil
	}
	return "", fmt.Errorf("aontu: expected a map key, got %s", tokenDesc(t))
}

// parseEntryValue parses the value after `key:`. A colon-chain (`b:c:1`)
// nests another map; otherwise it is an ordinary value.
func (p *aontuParser) parseEntryValue() (*aTerm, error) {
	if p.entryAhead() {
		k, err := p.parseKey()
		if err != nil {
			return nil, err
		}
		p.next() // consume the ':'
		inner, err := p.parseEntryValue()
		if err != nil {
			return nil, err
		}
		o := newObj()
		o.add(k, inner)
		return &aTerm{conj: []aAtom{o}}, nil
	}
	return p.parseValue()
}

// parseValue parses a value and any value-level `&` unifications. It stops
// before a `&` that introduces a new map entry (key followed by ':').
func (p *aontuParser) parseValue() (*aTerm, error) {
	first, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	conj := []aAtom{first}
	for p.at(tAmp) {
		save := p.pos
		p.next()
		if p.entryAhead() {
			p.pos = save // leave the '&' for the struct loop
			break
		}
		nxt, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		conj = append(conj, nxt)
	}
	return &aTerm{conj: conj}, nil
}

func (p *aontuParser) parsePrimary() (aAtom, error) {
	t := p.peek()
	switch t.kind {
	case tLBrace:
		p.next()
		return p.parseStructBody(tRBrace)
	case tLBrack:
		p.next()
		return p.parseList()
	case tString:
		p.next()
		return aScalar{v: t.text}, nil
	case tRef:
		p.next()
		if len(t.segs) == 0 {
			return nil, fmt.Errorf("aontu: empty reference")
		}
		for _, s := range t.segs {
			if strings.HasPrefix(s, "$") {
				return nil, fmt.Errorf("aontu: the .$KEY self-key reference is not yet supported")
			}
		}
		return aRef{abs: t.abs, segs: t.segs}, nil
	case tAtom:
		p.next()
		return classifyAtom(t.text), nil
	default:
		return nil, fmt.Errorf("aontu: unexpected %s", tokenDesc(t))
	}
}

func (p *aontuParser) parseList() (*aArr, error) {
	arr := &aArr{}
	for {
		if p.at(tRBrack) {
			p.next()
			return arr, nil
		}
		if p.at(tEOF) {
			return nil, fmt.Errorf("aontu: unexpected end of input, missing ']'")
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr.items = append(arr.items, v)
	}
}

// classifyAtom maps a bare token to a scalar, keyword, or kind. Unrecognised
// tokens are bare strings.
func classifyAtom(s string) aAtom {
	switch s {
	case "true":
		return aScalar{v: true}
	case "false":
		return aScalar{v: false}
	case "null":
		return aScalar{v: nil}
	case "string", "number", "top":
		return aKind{name: s}
	case "integer", "int":
		return aKind{name: "integer"}
	case "boolean", "bool":
		return aKind{name: "boolean"}
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return aScalar{v: i}
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return aScalar{v: f}
	}
	return aScalar{v: s}
}

func tokenDesc(t aToken) string {
	switch t.kind {
	case tEOF:
		return "end of input"
	case tLBrace:
		return "'{'"
	case tRBrace:
		return "'}'"
	case tLBrack:
		return "'['"
	case tRBrack:
		return "']'"
	case tColon:
		return "':'"
	case tAmp:
		return "'&'"
	case tString:
		return fmt.Sprintf("string %q", t.text)
	case tAtom:
		return fmt.Sprintf("%q", t.text)
	case tRef:
		return "reference"
	}
	return "token"
}

// ---- evaluation (unification + reference resolution) ----------------------

// aontuKind is the sentinel a lone (unresolved) kind resolves to; it is the
// only value aontuCheckComplete rejects.
type aontuKind struct{ name string }

type aontuEval struct {
	root *aObj
	memo map[*aTerm]any
}

// resolve reduces a term to a concrete Go value by resolving and unifying
// every conjunct. scopes is the chain of enclosing maps (innermost last),
// used for relative references; active guards against reference cycles.
func (e *aontuEval) resolve(t *aTerm, scopes []*aObj, active map[*aTerm]bool) (any, error) {
	if v, ok := e.memo[t]; ok {
		return v, nil
	}
	if active[t] {
		return nil, fmt.Errorf("aontu: cyclic reference")
	}
	active[t] = true
	defer delete(active, t)

	var acc any
	have := false
	for _, atom := range t.conj {
		v, err := e.resolveAtom(atom, scopes, active)
		if err != nil {
			return nil, err
		}
		if !have {
			acc, have = v, true
			continue
		}
		merged, err := unifyAontu(acc, v)
		if err != nil {
			return nil, err
		}
		acc = merged
	}
	if !have {
		acc = nil
	}
	e.memo[t] = acc
	return acc, nil
}

func (e *aontuEval) resolveAtom(atom aAtom, scopes []*aObj, active map[*aTerm]bool) (any, error) {
	switch a := atom.(type) {
	case aScalar:
		return a.v, nil
	case aKind:
		return aontuKind(a), nil
	case *aObj:
		out := map[string]any{}
		inner := append(scopes, a)
		for _, k := range a.keys {
			v, err := e.resolve(a.fields[k], inner, active)
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, nil
	case *aArr:
		out := make([]any, len(a.items))
		for i, it := range a.items {
			v, err := e.resolve(it, scopes, active)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case aRef:
		return e.resolveRef(a, scopes, active)
	default:
		return nil, fmt.Errorf("aontu: internal: unknown atom %T", atom)
	}
}

// resolveRef resolves a reference by locating its base term (the document
// root for absolute refs, or the nearest enclosing scope holding the first
// segment for relative refs), resolving it, then indexing the resulting Go
// value by the remaining path segments.
func (e *aontuEval) resolveRef(r aRef, scopes []*aObj, active map[*aTerm]bool) (any, error) {
	var base *aTerm
	var baseScopes []*aObj
	head := r.segs[0]
	if r.abs {
		t, ok := e.root.fields[head]
		if !ok {
			return nil, fmt.Errorf("aontu: cannot resolve reference $.%s", strings.Join(r.segs, "."))
		}
		base, baseScopes = t, []*aObj{e.root}
	} else {
		for k := len(scopes) - 1; k >= 0; k-- {
			if t, ok := scopes[k].fields[head]; ok {
				base, baseScopes = t, scopes[:k+1]
				break
			}
		}
		if base == nil {
			return nil, fmt.Errorf("aontu: cannot resolve reference .%s", strings.Join(r.segs, "."))
		}
	}
	v, err := e.resolve(base, baseScopes, active)
	if err != nil {
		return nil, err
	}
	for _, seg := range r.segs[1:] {
		switch cur := v.(type) {
		case map[string]any:
			nv, ok := cur[seg]
			if !ok {
				return nil, fmt.Errorf("aontu: cannot resolve reference path segment %q", seg)
			}
			v = nv
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(cur) {
				return nil, fmt.Errorf("aontu: cannot index list with %q", seg)
			}
			v = cur[idx]
		default:
			return nil, fmt.Errorf("aontu: cannot index %T with %q", v, seg)
		}
	}
	return v, nil
}

// unifyAontu unifies two already-resolved Go values, the heart of aontu's
// `&`/duplicate-key semantics: maps deep-merge, equal-length lists unify
// element-wise, a kind validates against a scalar, and any other mismatch is
// a conflict.
func unifyAontu(a, b any) (any, error) {
	if ka, ok := a.(aontuKind); ok {
		return unifyKindValue(ka, b)
	}
	if kb, ok := b.(aontuKind); ok {
		return unifyKindValue(kb, a)
	}
	am, aIsMap := a.(map[string]any)
	bm, bIsMap := b.(map[string]any)
	if aIsMap && bIsMap {
		return unifyMaps(am, bm)
	}
	al, aIsList := a.([]any)
	bl, bIsList := b.([]any)
	if aIsList && bIsList {
		if len(al) != len(bl) {
			return nil, fmt.Errorf("aontu: cannot unify lists of length %d and %d", len(al), len(bl))
		}
		out := make([]any, len(al))
		for i := range al {
			u, err := unifyAontu(al[i], bl[i])
			if err != nil {
				return nil, err
			}
			out[i] = u
		}
		return out, nil
	}
	if aIsMap != bIsMap || aIsList != bIsList {
		return nil, fmt.Errorf("aontu: cannot unify %s with %s", aontuTypeName(a), aontuTypeName(b))
	}
	if scalarEqual(a, b) {
		return a, nil
	}
	return nil, fmt.Errorf("aontu: conflict: %v & %v", a, b)
}

func unifyMaps(am, bm map[string]any) (any, error) {
	out := make(map[string]any, len(am)+len(bm))
	for k, v := range am {
		out[k] = v
	}
	for k, v := range bm {
		if cur, ok := out[k]; ok {
			u, err := unifyAontu(cur, v)
			if err != nil {
				return nil, err
			}
			out[k] = u
			continue
		}
		out[k] = v
	}
	return out, nil
}

// unifyKindValue unifies a kind with another resolved value: kind∧kind
// narrows (or conflicts), kind∧scalar validates the scalar, and `top`
// admits anything.
func unifyKindValue(k aontuKind, v any) (any, error) {
	if k.name == "top" {
		return v, nil
	}
	if kv, ok := v.(aontuKind); ok {
		return unifyKinds(k, kv)
	}
	if kindMatches(k.name, v) {
		return v, nil
	}
	return nil, fmt.Errorf("aontu: %v is not a %s", v, k.name)
}

func unifyKinds(a, b aontuKind) (any, error) {
	if a.name == b.name {
		return a, nil
	}
	if a.name == "top" {
		return b, nil
	}
	if b.name == "top" {
		return a, nil
	}
	// integer is the narrower of {number, integer}.
	if (a.name == "number" && b.name == "integer") || (a.name == "integer" && b.name == "number") {
		return aontuKind{name: "integer"}, nil
	}
	return nil, fmt.Errorf("aontu: cannot unify kinds %s and %s", a.name, b.name)
}

func kindMatches(name string, v any) bool {
	switch name {
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		switch v.(type) {
		case int64, float64:
			return true
		}
		return false
	case "integer":
		if _, ok := v.(int64); ok {
			return true
		}
		if f, ok := v.(float64); ok {
			return f == float64(int64(f))
		}
		return false
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "top":
		return true
	}
	return false
}

func scalarEqual(a, b any) bool {
	if af, ok := toFloat(a); ok {
		if bf, ok := toFloat(b); ok {
			return af == bf
		}
		return false
	}
	return a == b
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func aontuTypeName(v any) string {
	switch v.(type) {
	case map[string]any:
		return "map"
	case []any:
		return "list"
	case string:
		return "string"
	case bool:
		return "boolean"
	case int64, float64:
		return "number"
	case nil:
		return "null"
	}
	return fmt.Sprintf("%T", v)
}

// aontuCheckComplete walks the resolved value and rejects any lone kind that
// never unified with a concrete value — aontu's "incomplete" error.
func aontuCheckComplete(v any) error {
	switch val := v.(type) {
	case aontuKind:
		return fmt.Errorf("aontu: incomplete value (kind %q has no concrete value)", val.name)
	case map[string]any:
		for _, k := range val {
			if err := aontuCheckComplete(k); err != nil {
				return err
			}
		}
	case []any:
		for _, it := range val {
			if err := aontuCheckComplete(it); err != nil {
				return err
			}
		}
	}
	return nil
}
