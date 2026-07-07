package parser

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go"
)

// TestXmlWave3AcceptedForms pins well-formed variants of the embedded XML
// literal that render back deterministically.
func TestXmlWave3AcceptedForms(t *testing.T) {
	cases := []struct{ src, render string }{
		// Multi-line content (newline tracking in the matcher).
		{"<a>\n</a>", "<a>\n</a>"},
		// Whitespace around `=` in attributes.
		{`<a x ="1"/>`, `<a x="1"/>`},
		{`<a x= "1"/>`, `<a x="1"/>`},
		// Valueless (default) attribute → empty string, like strict XML.
		{`<a checked/>`, `<a checked=""/>`},
		// An explicit empty attribute value round-trips.
		{`<a x=""/>`, `<a x=""/>`},
		// Entities in attribute values survive the escape round-trip.
		{`<a x="a&amp;b"/>`, `<a x="a&amp;b"/>`},
		// Comments are stripped.
		{`<a><!-- c --></a>`, `<a/>`},
		// Whitespace inside the closing tag.
		{`<a></ a>`, `<a/>`},
		{`<a></a >`, `<a/>`},
	}
	for _, c := range cases {
		vals := mustParseWave3(t, c.src)
		if len(vals) != 1 || !vals[0].Is(eng.TXml) {
			t.Errorf("%q: expected one Node/Xml, got %v", c.src, vals)
			continue
		}
		if got := vals[0].String(); got != c.render {
			t.Errorf("%q: render %q, want %q", c.src, got, c.render)
		}
	}
}

// TestXmlWave3InterpolationForms pins the deferred (Word/__XI) skeleton for
// interpolation holes in tricky positions.
func TestXmlWave3InterpolationForms(t *testing.T) {
	for _, src := range []string{
		`<a x="p${k}s"/>`,       // hole with literal prefix AND suffix in one attr
		`<p>${x cat "}"}</p>`,   // `}` inside a string must not close the hole
		`<p>${ {a:1} . a }</p>`, // nested braces via depth counting
		"<p>${`x`}</p>",         // backtick string inside the hole
		`<p>${"a\"}"}</p>`,      // escaped quote inside a string in the hole
	} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 || !vals[0].Is(eng.TXmlInterp) {
			t.Errorf("%q: expected one deferred Word/__XI skeleton, got %v", src, vals)
		}
	}
}

func TestXmlWave3DataContext(t *testing.T) {
	// An XML literal as a map value stays a constant Node/Xml…
	vals := mustParseWave3(t, `{a:<b/>}`)
	if len(vals) != 1 {
		t.Fatalf("{a:<b/>}: got %d values, want 1", len(vals))
	}
	m, err := eng.AsMap(vals[0])
	if err != nil {
		t.Fatalf("{a:<b/>}: AsMap: %v", err)
	}
	v, ok := m.Get("a")
	if !ok || !v.Is(eng.TXml) {
		t.Errorf("{a:<b/>}: value %v is not a Node/Xml", v)
	}
	// …and a malformed one surfaces its build error through the map path.
	wantParseErrWave3(t, `{a:<b}`, "xml:")
}

// TestXmlWave3Errors pins every malformed-literal diagnostic: each input
// FAILS with the specific xml error, never a panic.
func TestXmlWave3Errors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`<a x=y/>`, "must have a quoted value"},
		{`<a 1="2"/>`, "invalid attribute name"},
		{`<a x="1" x="2"/>`, `duplicate attribute "x"`},
		{`<a /x`, "expected '/>' to close <a>"},
		{`<a`, "unterminated opening tag <a>"},
		{`< a></a>`, "expected a tag name after '<'"},
		{`<a><1/></a>`, "expected a tag name after '<'"},
		{`<a></a`, "malformed closing tag for <a>"},
		{`<a><!-- x`, "unterminated comment in <a>"},
		{`<a x="abc`, `unterminated value for attribute "x"`},
		{`<a x="a${b"/>`, "unterminated ${...} interpolation"},
		// Parse errors INSIDE a ${...} hole, in attribute (quoted and
		// bare) and content position.
		{`<a x="${1__0}"/>`, "xml: in ${...}:"},
		{`<a x=${1__0}/>`, "xml: in ${...}:"},
		{`<p>${1__0}</p>`, "xml: in ${...}:"},
	}
	for _, c := range cases {
		_, err := parseWave3(t, c.src)
		if err == nil {
			t.Errorf("%q: expected an error, got none", c.src)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: error %q does not contain %q", c.src, err.Error(), c.want)
		}
	}
	// A bare `<` at end of source: the matcher declines and normal lexing
	// reports the unexpected character.
	wantParseErrWave3(t, "<", "")
}
