package parser

import (
	"fmt"
	"strings"

	"github.com/aql-lang/aql/eng/go"
	jsonic "github.com/tabnas/jsonic/go"
)

// xmlElemVal carries an embedded XML literal from the xml_literal lex
// matcher to the converter. The matcher builds the immutable Node/Xml
// value (V) directly — one parse, in the lexer — so the converter just
// hands V through. A build failure rides in Err so the converter can
// surface a clean error rather than a generic jsonic parse error.
type xmlElemVal struct {
	V   eng.Value
	Err error
}

// setupXmlGrammar wires the embedded XML literal rule. The val.Open `<`
// alternate (setupValRule) pushes "xml"; this rule's BO arms the
// xml_literal matcher, whose single #XML token carries the whole
// element. Generics' angle sugar consumes `<` only as a val.Close
// suffix after a capitalised name, so the two never compete for `<`.
// See design/XML-LITERAL.0.md §3.
func setupXmlGrammar(j *jsonic.Jsonic, t parserTokens) {
	j.Rule("xml", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBO(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				// Arm the matcher for the next token (the element body).
				r.K["aql_xml"] = true
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{t.XML}}, A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				// Disarm immediately: the lookahead token the Close
				// alternate lexes must not scan a following sibling
				// `<…>` as part of this element.
				delete(r.K, "aql_xml")
				r.Node = r.O0.Val
			}},
		})
		setClose(rs, []*jsonic.AltSpec{
			// The #XML token already consumed the entire element; close
			// without consuming the lookahead (the dotchain technique).
			{B: 1},
		})
	})
}

// setupXmlMatcher registers the lex matcher that, when armed by the xml
// rule's BO (rule.K["aql_xml"]), scans the whole balanced element that
// follows the already-consumed `<` and emits one #XML token carrying the
// built Node/Xml value. It runs ONLY when armed, so it never touches the
// generics `<` (which is lexed outside the xml rule) or any other `<`.
func setupXmlMatcher(j *jsonic.Jsonic, t parserTokens) {
	addMatcher(j, "xml_literal", 1000003, func(lex *jsonic.Lex, rule *jsonic.Rule) *jsonic.Token {
		if rule == nil {
			return nil
		}
		if armed, _ := rule.K["aql_xml"].(bool); !armed {
			return nil
		}
		cursor := lex.Cursor()
		s := lex.Src
		afterLA := cursor.SI // position just past the `<` the val.Open alt consumed
		tmpl, end, err := buildXmlTmpl(s, afterLA)
		if err != nil {
			// Absorb the rest of the source so leftover tokens don't
			// trigger a confusing jsonic error before conversion surfaces
			// this (more specific) one.
			end = len(s)
		}
		if end <= afterLA {
			// No progress (`<` at end of source): decline and let normal
			// lexing report the unexpected character.
			return nil
		}
		// No interpolation anywhere → freeze to a constant Node/Xml (the
		// Increment-1 path). Otherwise emit the skeleton; the engine
		// evaluates it to a Node/Xml at runtime. See evalXmlInterp.
		var v eng.Value
		if err == nil {
			if cv, ok := freezeXmlTmpl(tmpl); ok {
				v = cv
			} else {
				v = eng.NewXmlInterp(*tmpl)
			}
		}
		la := afterLA - 1
		if la < 0 {
			la = 0
		}
		raw := s[la:end]
		tkn := lex.Token("#XML", t.XML, xmlElemVal{V: v, Err: err}, raw)
		for k := afterLA; k < end; k++ {
			if s[k] == '\n' {
				cursor.RI++
				cursor.CI = 1
			} else {
				cursor.CI++
			}
		}
		cursor.SI = end
		return tkn
	})
}

func isXmlNameStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isXmlNameChar(c byte) bool {
	return isXmlNameStart(c) || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == ':'
}

func isXmlSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// buildXmlTmpl parses one element from s starting at i (the index just
// after the element's opening `<`) and returns its skeleton (XmlTmpl), the
// index just past the element's final `>`, and any error. The skeleton
// captures literal text and `${expr}` holes uniformly; the matcher then
// either freezes a hole-free skeleton to a constant Node/Xml or emits it
// for runtime evaluation (see freezeXmlTmpl / evalXmlInterp). On a
// well-formedness error it returns a usable stop index (> i where possible)
// so the matcher can still advance the cursor and surface the error through
// conversion. When the very first character is not a tag name, it returns
// end == i so the matcher declines.
func buildXmlTmpl(s string, i int) (*eng.XmlTmpl, int, error) {
	n := len(s)
	if i >= n || !isXmlNameStart(s[i]) {
		return nil, i, fmt.Errorf("xml: expected a tag name after '<'")
	}
	nameStart := i
	for i < n && isXmlNameChar(s[i]) {
		i++
	}
	t := &eng.XmlTmpl{Tag: s[nameStart:i]}

	// Opening-tag attributes, up to `>` or `/>`.
	for {
		for i < n && isXmlSpace(s[i]) {
			i++
		}
		if i >= n {
			return nil, n, fmt.Errorf("xml: unterminated opening tag <%s>", t.Tag)
		}
		if s[i] == '/' {
			if i+1 < n && s[i+1] == '>' {
				return t, i + 2, nil
			}
			return nil, i + 1, fmt.Errorf("xml: expected '/>' to close <%s>", t.Tag)
		}
		if s[i] == '>' {
			i++
			break
		}
		if !isXmlNameStart(s[i]) {
			return nil, i + 1, fmt.Errorf("xml: invalid attribute name in <%s>", t.Tag)
		}
		anStart := i
		for i < n && isXmlNameChar(s[i]) {
			i++
		}
		aname := s[anStart:i]
		for i < n && isXmlSpace(s[i]) {
			i++
		}
		// Default (valueless) attribute → empty string, like strict XML.
		parts := []eng.InterpPart{{Lit: ""}}
		if i < n && s[i] == '=' {
			i++
			for i < n && isXmlSpace(s[i]) {
				i++
			}
			if i < n && s[i] == '$' && i+1 < n && s[i+1] == '{' {
				// Bare interpolation as the whole value: `name=${expr}`.
				toks, end, err := scanInterp(s, i)
				if err != nil {
					return nil, end, err
				}
				parts = []eng.InterpPart{{Expr: toks}}
				i = end
			} else if i < n && (s[i] == '"' || s[i] == '\'') {
				vparts, end, err := scanAttrValue(s, i+1, s[i], aname, t.Tag)
				if err != nil {
					return nil, end, err
				}
				parts = vparts
				i = end
			} else {
				return nil, i, fmt.Errorf("xml: attribute %q in <%s> must have a quoted value", aname, t.Tag)
			}
		}
		t.Attr = append(t.Attr, eng.XmlAttrTmpl{Name: aname, Parts: parts})
	}

	// Element content, up to the matching `</tag>`.
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			t.Cren = append(t.Cren, eng.XmlCren{Kind: eng.XmlCrenLit, Lit: unescapeXml(text.String())})
			text.Reset()
		}
	}
	for {
		if i >= n {
			return nil, n, fmt.Errorf("xml: unterminated element <%s>", t.Tag)
		}
		// ${expr} interpolation hole in content (child position).
		if s[i] == '$' && i+1 < n && s[i+1] == '{' {
			flush()
			toks, end, err := scanInterp(s, i)
			if err != nil {
				return nil, end, err
			}
			t.Cren = append(t.Cren, eng.XmlCren{Kind: eng.XmlCrenExpr, Expr: toks})
			i = end
			continue
		}
		if s[i] != '<' {
			text.WriteByte(s[i])
			i++
			continue
		}
		// s[i] == '<'
		if i+1 < n && s[i+1] == '/' {
			flush()
			j := i + 2
			for j < n && isXmlSpace(s[j]) {
				j++
			}
			cnStart := j
			for j < n && isXmlNameChar(s[j]) {
				j++
			}
			cname := s[cnStart:j]
			for j < n && isXmlSpace(s[j]) {
				j++
			}
			if j >= n || s[j] != '>' {
				stop := j + 1
				if stop > n {
					stop = n
				}
				return nil, stop, fmt.Errorf("xml: malformed closing tag for <%s>", t.Tag)
			}
			if cname != t.Tag {
				return nil, j + 1, fmt.Errorf("xml: mismatched closing tag </%s> for <%s>", cname, t.Tag)
			}
			return t, j + 1, nil
		}
		if i+3 < n && s[i+1] == '!' && s[i+2] == '-' && s[i+3] == '-' {
			flush()
			k := i + 4
			for k+2 < n && !(s[k] == '-' && s[k+1] == '-' && s[k+2] == '>') {
				k++
			}
			if k+2 >= n {
				return nil, n, fmt.Errorf("xml: unterminated comment in <%s>", t.Tag)
			}
			i = k + 3
			continue
		}
		// Child element.
		flush()
		child, ni, err := buildXmlTmpl(s, i+1)
		if err != nil {
			return nil, ni, err
		}
		t.Cren = append(t.Cren, eng.XmlCren{Kind: eng.XmlCrenChild, Child: child})
		i = ni
	}
}

// scanAttrValue parses a quoted attribute value beginning at i (just past
// the opening quote q) into InterpParts — literal segments plus `${expr}`
// holes — returning the index just past the closing quote.
func scanAttrValue(s string, i int, q byte, aname, tag string) ([]eng.InterpPart, int, error) {
	n := len(s)
	var parts []eng.InterpPart
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			parts = append(parts, eng.InterpPart{Lit: unescapeXml(lit.String())})
			lit.Reset()
		}
	}
	for i < n {
		if s[i] == q {
			flush()
			if len(parts) == 0 {
				parts = []eng.InterpPart{{Lit: ""}}
			}
			return parts, i + 1, nil
		}
		if s[i] == '$' && i+1 < n && s[i+1] == '{' {
			flush()
			toks, end, err := scanInterp(s, i)
			if err != nil {
				return nil, end, err
			}
			parts = append(parts, eng.InterpPart{Expr: toks})
			i = end
			continue
		}
		lit.WriteByte(s[i])
		i++
	}
	return nil, n, fmt.Errorf("xml: unterminated value for attribute %q in <%s>", aname, tag)
}

// scanInterp consumes a `${ … }` interpolation beginning at i (where
// s[i:i+2] == "${"), returning the parsed expression tokens, the index
// just past the closing `}`, and any error. The matching brace is found by
// depth counting that skips over quoted strings (', ", `), so a `}` inside
// a string does not close the hole early.
func scanInterp(s string, i int) ([]eng.Value, int, error) {
	n := len(s)
	j := i + 2
	depth := 1
	for j < n {
		switch s[j] {
		case '\'', '"', '`':
			q := s[j]
			j++
			for j < n {
				if s[j] == '\\' && j+1 < n {
					j += 2
					continue
				}
				if s[j] == q {
					j++
					break
				}
				j++
			}
		case '{':
			depth++
			j++
		case '}':
			depth--
			if depth == 0 {
				inner := strings.TrimSpace(s[i+2 : j])
				toks, err := Parse(inner)
				if err != nil {
					return nil, j + 1, fmt.Errorf("xml: in ${...}: %w", err)
				}
				return toks, j + 1, nil
			}
			j++
		default:
			j++
		}
	}
	return nil, n, fmt.Errorf("xml: unterminated ${...} interpolation")
}

// freezeXmlTmpl returns a constant Node/Xml value for a skeleton with no
// `${expr}` holes anywhere (ok == true), or ok == false when any hole is
// present — in which case the matcher emits the skeleton for runtime
// evaluation. The frozen value is byte-identical to the Increment-1
// constant build.
func freezeXmlTmpl(t *eng.XmlTmpl) (eng.Value, bool) {
	attr := eng.NewOrderedMap()
	for _, a := range t.Attr {
		s, ok := staticParts(a.Parts)
		if !ok {
			return eng.Value{}, false
		}
		attr.Set(a.Name, eng.NewString(s))
	}
	var cren []eng.Value
	for _, c := range t.Cren {
		switch c.Kind {
		case eng.XmlCrenLit:
			cren = append(cren, eng.NewString(c.Lit))
		case eng.XmlCrenChild:
			cv, ok := freezeXmlTmpl(c.Child)
			if !ok {
				return eng.Value{}, false
			}
			cren = append(cren, cv)
		case eng.XmlCrenExpr:
			return eng.Value{}, false
		}
	}
	return eng.NewXmlElement(t.Tag, attr, cren), true
}

// staticParts concatenates InterpParts when none is an expression hole.
func staticParts(parts []eng.InterpPart) (string, bool) {
	var b strings.Builder
	for _, p := range parts {
		if p.Expr != nil {
			return "", false
		}
		b.WriteString(p.Lit)
	}
	return b.String(), true
}

var xmlUnescaper = strings.NewReplacer(
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", `"`,
	"&apos;", "'",
	"&amp;", "&",
)

func unescapeXml(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	return xmlUnescaper.Replace(s)
}
