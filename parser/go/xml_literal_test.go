package parser

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// parseOne parses src and requires exactly one resulting value.
func parseOneXml(t *testing.T, src string) core.Value {
	t.Helper()
	vals, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", src, err)
	}
	if len(vals) != 1 {
		t.Fatalf("Parse(%q): got %d values, want 1: %v", src, len(vals), vals)
	}
	return vals[0]
}

func TestXmlLiteralBasic(t *testing.T) {
	cases := []struct {
		src    string
		render string
	}{
		{`<a/>`, `<a/>`},
		{`<a x="1"/>`, `<a x="1"/>`},
		{`<p>hi</p>`, `<p>hi</p>`},
		{`<a><b/></a>`, `<a><b/></a>`},
		{`<a x="1" y="2">t<b/>u</a>`, `<a x="1" y="2">t<b/>u</a>`},
		{`<p>a &lt; b &amp; c</p>`, `<p>a &lt; b &amp; c</p>`},
		// An empty element normalises to the self-closing form.
		{`<br></br>`, `<br/>`},
	}
	for _, c := range cases {
		v := parseOneXml(t, c.src)
		if !v.Is(core.TXml) {
			t.Errorf("%q: value is not Node/Xml (got %s)", c.src, core.ValueType(v).Path())
			continue
		}
		if got := v.String(); got != c.render {
			t.Errorf("%q: render = %q, want %q", c.src, got, c.render)
		}
	}
}

func TestXmlLiteralStructure(t *testing.T) {
	v := parseOneXml(t, `<a x="1" y="hi">text<b/></a>`)
	x, err := core.AsXmlElement(v)
	if err != nil {
		t.Fatalf("AsXmlElement: %v", err)
	}
	str := func(v core.Value) string { s, _ := core.AsString(v); return s }
	if x.Tag != "a" {
		t.Errorf("tag = %q, want a", x.Tag)
	}
	if xv, ok := x.Attr.Get("x"); !ok || str(xv) != "1" {
		t.Errorf("attr x missing/wrong: %v", xv)
	}
	if xv, ok := x.Attr.Get("y"); !ok || str(xv) != "hi" {
		t.Errorf("attr y missing/wrong: %v", xv)
	}
	if len(x.Cren) != 2 {
		t.Fatalf("cren len = %d, want 2", len(x.Cren))
	}
	if str(x.Cren[0]) != "text" {
		t.Errorf("cren[0] = %v, want text node", x.Cren[0])
	}
	if !x.Cren[1].Is(core.TXml) {
		t.Errorf("cren[1] is not an element: %v", x.Cren[1])
	}
}

func TestXmlLiteralEquality(t *testing.T) {
	a := parseOneXml(t, `<a x="1"><b/></a>`)
	b := parseOneXml(t, `<a x="1"><b/></a>`)
	c := parseOneXml(t, `<a x="2"><b/></a>`)
	if !core.TXml.Behavior().Equal(a, b) {
		t.Errorf("equal elements compared unequal")
	}
	if core.TXml.Behavior().Equal(a, c) {
		t.Errorf("different attrs compared equal")
	}
}

// TestXmlLiteralDoesNotBreakGenerics is the disambiguation regression:
// `<` after a capitalised name stays generics, `<` in value position is XML.
func TestXmlLiteralGenericsUntouched(t *testing.T) {
	// Generics use-site: must parse, and must NOT be a single Node/Xml.
	vals, err := Parse(`Box<Integer>`)
	if err != nil {
		t.Fatalf("Box<Integer> parse error: %v", err)
	}
	for _, v := range vals {
		if v.Is(core.TXml) {
			t.Fatalf("Box<Integer> produced a Node/Xml value: %v", vals)
		}
	}
}

// TestXmlLiteralFreezeVsInterp pins the parse-time split: a literal with
// no ${} freezes to a constant Node/Xml; one with any ${} hole becomes a
// deferred Word/__XI skeleton the engine evaluates at runtime.
func TestXmlLiteralFreezeVsInterp(t *testing.T) {
	// No interpolation → constant Node/Xml.
	for _, src := range []string{`<a/>`, `<a x="1">t<b/></a>`} {
		v := parseOneXml(t, src)
		if !v.Is(core.TXml) {
			t.Errorf("%q: want Node/Xml (frozen), got %s", src, core.ValueType(v).Path())
		}
	}
	// Any ${} hole (text, attribute, or child position) → deferred skeleton.
	for _, src := range []string{
		`<p>${x}</p>`,
		`<a href=${u}/>`,
		`<a class="b ${k}"/>`,
		`<ul>${items}</ul>`,
		`<a><b>${x}</b></a>`,
	} {
		v := parseOneXml(t, src)
		if !v.Is(core.TXmlInterp) {
			t.Errorf("%q: want Word/__XI (deferred), got %s", src, core.ValueType(v).Path())
		}
	}
}

func TestXmlLiteralInterpErrors(t *testing.T) {
	for _, src := range []string{`<p>${x</p>`, `<a x=${y/>`} {
		if _, err := Parse(src); err == nil {
			t.Errorf("%q: expected error, got none", src)
		}
	}
}

func TestXmlLiteralErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`<a></b>`, "mismatched"},
		{`<a>`, "unterminated"},
		{`<a x="1">`, "unterminated"},
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		if err == nil {
			t.Errorf("%q: expected error, got none", c.src)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: error %q does not contain %q", c.src, err.Error(), c.want)
		}
	}
}
