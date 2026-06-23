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

// TestXmlParseAlignment pins Increment 4: `parse xml` yields a Node/Xml
// element — the same shape an embedded literal produces — so it renders
// back to well-formed XML and is the same type. See XML-LITERAL.0.md §5.6.
func TestXmlParseAlignment(t *testing.T) {
	const imp = `import "aql:parselang"  `
	cases := []struct {
		src  string
		want any
	}{
		{`typeof (parse xml '<a/>')`, "Xml"},
		{`parse xml '<a x="1"/>'`, `<a x="1"/>`},
		{`parse xml '<p>hi</p>'`, "<p>hi</p>"},
		{`parse xml '<r><a>1</a></r>'`, "<r><a>1</a></r>"},
		// a parsed element and the equivalent literal are deeply equal.
		{`(parse xml '<a x="1"><b/></a>') deq <a x="1"><b/></a>`, "true"},
	}
	for _, c := range cases {
		a, err := lang.New()
		if err != nil {
			t.Fatalf("lang.New: %v", err)
		}
		if got := runLast(t, a, imp+c.src); got != c.want {
			t.Errorf("%q: got %v (%T), want %v", c.src, got, got, c.want)
		}
	}
}

// TestXmlAccessors covers Increment 5: the stored fields via dotted
// access (tag/attr/cren) and the computed query words elem/text/xml-attr.
func TestXmlAccessors(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		// stored fields via dotted access
		{`(<a x="1" y="2"/>).tag`, "a"},
		{`(<a x="1"/>).attr.x`, "1"},
		{`(<p>hi</p>).cren size`, int64(1)},
		// xml-elem: element children only (text nodes dropped)
		{`(xml-elem <a>t<b/>u<c/></a>) size`, int64(2)},
		// xml-text: concatenated subtree text content
		{`xml-text <a>x<b>y</b>z</a>`, "xyz"},
		// xml-attr: a present attribute, and a missing one → none
		{`xml-attr 'x' <a x="1"/>`, "1"},
		{`typeof (xml-attr 'z' <a x="1"/>)`, "None"},
		// the words work on a parsed element too (same Node/Xml shape)
		{`import "aql:parselang"  xml-text (parse xml '<a>hi<b>!</b></a>')`, "hi!"},
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

// TestXmlReviewFixes pins the PR-review corrections (PR #159):
// structural eq/deq, FlexXml children in interpolation, one-level child
// splice, attribute-name validation, and duplicate-attribute rejection.
func TestXmlReviewFixes(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		// eq is container identity; deq is structural.
		{`def x <a/> x eq x`, "true"},
		{`(<a/>) eq (<a/>)`, "false"},
		{`(<a x="1"><b/></a>) deq (<a x="1"><b/></a>)`, "true"},
		// a FlexXml child interpolates as an element, not escaped text.
		{`def b (flex <b/>) <a>${b}</a>`, "<a><b/></a>"},
		// the child-hole splice is ONE level: a nested list stays one value.
		{`<a>${[<b/> <c/>]}</a>`, "<a><b/><c/></a>"},
		{`<a>${[[1 2]]}</a>`, "<a>[1 2]</a>"},
		// a closure over a ${} expression captures the outer binding.
		{`def mk ([x:String] => [([] => [<p>${x}</p>])]) def f (mk "bob") f`, "<p>bob</p>"},
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

	// Negatives: invalid FlexXml attribute name, duplicate literal attribute.
	for _, src := range []string{`set 'bad name' '1' (flex <a/>)`, `<a x="1" x="2"/>`} {
		a, err := lang.New()
		if err != nil {
			t.Fatalf("lang.New: %v", err)
		}
		if _, err := a.Run(src); err == nil {
			t.Errorf("%q: expected error, got none", src)
		}
	}

	// Check mode must not strip a Word/__XI literal to a payload-less
	// carrier (it stays evaluable): `aql check` succeeds and infers Xml.
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := a.Check(`def x "y" <p>${x}</p>`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 || res.Stack[0] != "Xml" {
		t.Errorf("check: expected [Xml], got %v", res.Stack)
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
