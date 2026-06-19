package test

import (
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// TestXmlLiteralEndToEnd runs embedded XML literals through the full
// lang pipeline (parse → engine → result projection). A Node/Xml result
// projects via String(), i.e. it renders back to well-formed XML.
func TestXmlLiteralEndToEnd(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		{`<a/>`, "<a/>"},
		{`<a x="1"/>`, `<a x="1"/>`},
		{`<p>hello</p>`, "<p>hello</p>"},
		{`<a><b/></a>`, "<a><b/></a>"},
		{`<ul><li>x</li><li>y</li></ul>`, "<ul><li>x</li><li>y</li></ul>"},
		// bound, then referenced — survives as a value.
		{`def page <html><body/></html> page`, "<html><body/></html>"},
		// XML literal as a list element.
		{`[<a/> <b/>] size`, int64(2)},
		// typeof / is: Node/Xml is a real type reachable by its leaf name.
		{`typeof <a/>`, "Xml"},
		{`(<a/>) is Xml`, "true"},
		{`(<a/>) is Map`, "false"},
	}
	for _, c := range cases {
		a, err := lang.New()
		if err != nil {
			t.Fatalf("lang.New: %v", err)
		}
		if got := runLast(t, a, c.src); got != c.want {
			t.Errorf("%q: got %v (%T), want %v", c.src, got, got, c.want)
		}
	}
}

// TestXmlLiteralInterpolation runs ${}-interpolated XML literals through
// the full pipeline. The literal carries a deferred skeleton (Word/__XI)
// that the engine evaluates against live bindings to a Node/Xml. See
// design/XML-LITERAL.0.md §4.
func TestXmlLiteralInterpolation(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		// text interpolation against a binding
		{`def name "bob" <p>hello ${name}</p>`, "<p>hello bob</p>"},
		// scalar hole stringifies into a text node
		{`def n 42 <p>n=${n}</p>`, "<p>n=42</p>"},
		// attribute value: bare ${} and mixed literal+${}
		{`def u "/x" <a href=${u}/>`, `<a href="/x"/>`},
		{`def k "big" <a class="btn ${k}"/>`, `<a class="btn big"/>`},
		// a single Node/Xml hole becomes one child element
		{`def b <b/> <a>${b}</a>`, "<a><b/></a>"},
		// a List hole splices each element as a child
		{`def items [<li>x</li> <li>y</li>] <ul>${items}</ul>`, "<ul><li>x</li><li>y</li></ul>"},
		// interpolation nested inside a child element
		{`def x "hi" <a><b>${x}</b></a>`, "<a><b>hi</b></a>"},
		// an interpolated literal is still a Node/Xml
		{`def x "y" typeof <p>${x}</p>`, "Xml"},
		// expression holes, not just bindings
		{`<p>${1 add 2}</p>`, "<p>3</p>"},
	}
	for _, c := range cases {
		a, err := lang.New()
		if err != nil {
			t.Fatalf("lang.New: %v", err)
		}
		if got := runLast(t, a, c.src); got != c.want {
			t.Errorf("%q: got %v (%T), want %v", c.src, got, got, c.want)
		}
	}
}

// TestXmlFlexEndToEnd covers the mutable Node/Xml/FlexXml variant:
// `flex` makes a mutable copy, `append`/`set` mutate in place, `node`
// converts back to an immutable Node/Xml. See design/XML-LITERAL.0.md §5.
func TestXmlFlexEndToEnd(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		// flex round-trips through rendering, and reports its own type.
		{`flex <a x="1"><b/></a>`, `<a x="1"><b/></a>`},
		{`typeof (flex <a/>)`, "FlexXml"},
		{`typeof <a/>`, "Xml"},
		// append a child element in place (nested so the call is one result).
		{`append <li>y</li> (append <li>x</li> (flex <ul/>))`, "<ul><li>x</li><li>y</li></ul>"},
		// a List of children splices.
		{`append [<li>x</li> <li>y</li>] (flex <ul/>)`, "<ul><li>x</li><li>y</li></ul>"},
		// set an attribute in place (DOM setAttribute).
		{`set 'class' 'card' (flex <a/>)`, `<a class="card"/>`},
		// node converts a mutated flex tree back to an immutable Node/Xml.
		{`node (append <b/> (flex <a/>))`, "<a><b/></a>"},
		{`typeof (node (flex <a/>))`, "Xml"},
	}
	for _, c := range cases {
		a, err := lang.New()
		if err != nil {
			t.Fatalf("lang.New: %v", err)
		}
		if got := runLast(t, a, c.src); got != c.want {
			t.Errorf("%q: got %v (%T), want %v", c.src, got, got, c.want)
		}
	}
}

// TestXmlLiteralErrorsEndToEnd pins the loud-failure contract.
func TestXmlLiteralErrorsEndToEnd(t *testing.T) {
	for _, src := range []string{`<a></b>`, `<a>`, `<a x=1/>`, `<p>${x</p>`} {
		a, err := lang.New()
		if err != nil {
			t.Fatalf("lang.New: %v", err)
		}
		if _, err := a.Run(src); err == nil {
			t.Errorf("%q: expected error, got none", src)
		}
	}
}
