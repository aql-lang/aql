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
		v, end, err := buildXmlElement(s, afterLA)
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

// buildXmlElement parses one element from s starting at i (the index just
// after the element's opening `<`) and returns the built Node/Xml value,
// the index just past the element's final `>`, and any error. On a
// well-formedness error it returns a usable stop index (> i where
// possible) so the matcher can still advance the cursor and surface the
// error through conversion. When the very first character is not a tag
// name, it returns end == i so the matcher declines.
func buildXmlElement(s string, i int) (eng.Value, int, error) {
	n := len(s)
	if i >= n || !isXmlNameStart(s[i]) {
		return eng.Value{}, i, fmt.Errorf("xml: expected a tag name after '<'")
	}
	nameStart := i
	for i < n && isXmlNameChar(s[i]) {
		i++
	}
	tag := s[nameStart:i]
	attr := eng.NewOrderedMap()

	// Opening-tag attributes, up to `>` or `/>`.
	for {
		for i < n && isXmlSpace(s[i]) {
			i++
		}
		if i >= n {
			return eng.Value{}, n, fmt.Errorf("xml: unterminated opening tag <%s>", tag)
		}
		if s[i] == '/' {
			if i+1 < n && s[i+1] == '>' {
				return eng.NewXmlElement(tag, attr, nil), i + 2, nil
			}
			return eng.Value{}, i + 1, fmt.Errorf("xml: expected '/>' to close <%s>", tag)
		}
		if s[i] == '>' {
			i++
			break
		}
		if !isXmlNameStart(s[i]) {
			return eng.Value{}, i + 1, fmt.Errorf("xml: invalid attribute name in <%s>", tag)
		}
		anStart := i
		for i < n && isXmlNameChar(s[i]) {
			i++
		}
		aname := s[anStart:i]
		for i < n && isXmlSpace(s[i]) {
			i++
		}
		aval := ""
		if i < n && s[i] == '=' {
			i++
			for i < n && isXmlSpace(s[i]) {
				i++
			}
			if i >= n || (s[i] != '"' && s[i] != '\'') {
				return eng.Value{}, i, fmt.Errorf("xml: attribute %q in <%s> must have a quoted value", aname, tag)
			}
			q := s[i]
			i++
			vStart := i
			for i < n && s[i] != q {
				i++
			}
			if i >= n {
				return eng.Value{}, n, fmt.Errorf("xml: unterminated value for attribute %q in <%s>", aname, tag)
			}
			aval = unescapeXml(s[vStart:i])
			i++ // past closing quote
		}
		attr.Set(aname, eng.NewString(aval))
	}

	// Element content, up to the matching `</tag>`.
	var cren []eng.Value
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			cren = append(cren, eng.NewString(unescapeXml(text.String())))
			text.Reset()
		}
	}
	for {
		if i >= n {
			return eng.Value{}, n, fmt.Errorf("xml: unterminated element <%s>", tag)
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
				return eng.Value{}, stop, fmt.Errorf("xml: malformed closing tag for <%s>", tag)
			}
			if cname != tag {
				return eng.Value{}, j + 1, fmt.Errorf("xml: mismatched closing tag </%s> for <%s>", cname, tag)
			}
			return eng.NewXmlElement(tag, attr, cren), j + 1, nil
		}
		if i+3 < n && s[i+1] == '!' && s[i+2] == '-' && s[i+3] == '-' {
			flush()
			k := i + 4
			for k+2 < n && !(s[k] == '-' && s[k+1] == '-' && s[k+2] == '>') {
				k++
			}
			if k+2 >= n {
				return eng.Value{}, n, fmt.Errorf("xml: unterminated comment in <%s>", tag)
			}
			i = k + 3
			continue
		}
		// Child element.
		flush()
		child, ni, err := buildXmlElement(s, i+1)
		if err != nil {
			return eng.Value{}, ni, err
		}
		cren = append(cren, child)
		i = ni
	}
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
