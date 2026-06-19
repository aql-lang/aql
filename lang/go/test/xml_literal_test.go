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

// TestXmlLiteralErrorsEndToEnd pins the loud-failure contract.
func TestXmlLiteralErrorsEndToEnd(t *testing.T) {
	for _, src := range []string{`<a></b>`, `<a>`, `<a x=1/>`} {
		a, err := lang.New()
		if err != nil {
			t.Fatalf("lang.New: %v", err)
		}
		if _, err := a.Run(src); err == nil {
			t.Errorf("%q: expected error, got none", src)
		}
	}
}
