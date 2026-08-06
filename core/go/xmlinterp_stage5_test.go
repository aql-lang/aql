package core

import (
	"testing"
)

// Stage-5 coverage for xml_interp.go: RebuildXmlFromTmpl, the
// hole-consuming twin of BuildXmlFromTmpl (OpInterpXml). Holes are
// consumed in build order — attributes first, then children
// left-to-right, depth-first — with the same assembly rules the build
// applies (attribute stringify, list one-level splice, text coalesce).

func xmlStage5Leaf(tag string) Value {
	return NewXmlElement(tag, nil, nil)
}

func TestXmlInterpStage5RebuildFullTraversal(t *testing.T) {
	inner := XmlTmpl{
		Tag:  "b",
		Cren: []XmlCren{{Kind: XmlCrenExpr, Expr: []Value{NewWord("y")}}},
	}
	tmpl := XmlTmpl{
		Tag: "div",
		Attr: []XmlAttrTmpl{
			{Name: "id", Parts: []InterpPart{{Lit: "p-"}, {Expr: []Value{NewWord("a")}}}},
		},
		Cren: []XmlCren{
			{Kind: XmlCrenLit, Lit: "hi "},
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("t")}},  // text hole: coalesces with "hi "
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("e")}},  // empty-string hole: no node
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("l")}},  // list hole: one-level splice
			{Kind: XmlCrenChild, Child: nil},                  // nil child: skipped
			{Kind: XmlCrenChild, Child: &inner},               // nested element, consumes a hole
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("x2")}}, // xml-valued hole
		},
	}
	holes := []Value{
		NewString("42"), // attr ${a}
		NewString("there"),
		NewString(""),
		NewList([]Value{xmlStage5Leaf("hr"), NewInteger(7)}),
		NewString("tail"), // inner <b> ${y}
		xmlStage5Leaf("img"),
	}

	v, used := RebuildXmlFromTmpl(tmpl, holes)
	if used != len(holes) {
		t.Fatalf("used = %d, want %d", used, len(holes))
	}
	el, err := AsXmlElement(v)
	if err != nil {
		t.Fatalf("AsXmlElement: %v", err)
	}
	if el.Tag != "div" {
		t.Errorf("tag = %q", el.Tag)
	}
	idv, ok := el.Attr.Get("id")
	if !ok {
		t.Fatal("id attribute missing")
	}
	if s, aerr := AsString(idv); aerr != nil || s != "p-42" {
		t.Errorf("id = %v (%v), want p-42", idv, aerr)
	}

	// Children: "hi there" (coalesced), <hr/>, "7" (splice text),
	// <b>tail</b> (recursed child), <img/> (xml hole).
	if len(el.Cren) != 5 {
		t.Fatalf("cren = %d nodes: %v", len(el.Cren), el.Cren)
	}
	if s, aerr := AsString(el.Cren[0]); aerr != nil || s != "hi there" {
		t.Errorf("cren[0] = %v (%v), want 'hi there'", el.Cren[0], aerr)
	}
	if hr, herr := AsXmlElement(el.Cren[1]); herr != nil || hr.Tag != "hr" {
		t.Errorf("cren[1] = %v (%v), want <hr/>", el.Cren[1], herr)
	}
	if s, aerr := AsString(el.Cren[2]); aerr != nil || s != "7" {
		t.Errorf("cren[2] = %v (%v), want '7'", el.Cren[2], aerr)
	}
	b, berr := AsXmlElement(el.Cren[3])
	if berr != nil || b.Tag != "b" {
		t.Fatalf("cren[3] = %v (%v), want <b>", el.Cren[3], berr)
	}
	if len(b.Cren) != 1 {
		t.Fatalf("inner cren = %v", b.Cren)
	}
	if s, aerr := AsString(b.Cren[0]); aerr != nil || s != "tail" {
		t.Errorf("inner text = %v (%v), want 'tail'", b.Cren[0], aerr)
	}
	if img, ierr := AsXmlElement(el.Cren[4]); ierr != nil || img.Tag != "img" {
		t.Errorf("cren[4] = %v (%v), want <img/>", el.Cren[4], ierr)
	}
}

func TestXmlInterpStage5RebuildExhaustedHoles(t *testing.T) {
	// With no holes left, both attribute exprs and child exprs are
	// skipped rather than consumed: the attribute keeps only its
	// literal segments and the child hole contributes nothing.
	tmpl := XmlTmpl{
		Tag: "i",
		Attr: []XmlAttrTmpl{
			{Name: "n", Parts: []InterpPart{{Lit: "x"}, {Expr: []Value{NewWord("q")}}}},
		},
		Cren: []XmlCren{
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("q")}},
		},
	}
	v, used := RebuildXmlFromTmpl(tmpl, nil)
	if used != 0 {
		t.Fatalf("used = %d, want 0", used)
	}
	el, err := AsXmlElement(v)
	if err != nil {
		t.Fatalf("AsXmlElement: %v", err)
	}
	nv, ok := el.Attr.Get("n")
	if !ok {
		t.Fatal("n attribute missing")
	}
	if s, aerr := AsString(nv); aerr != nil || s != "x" {
		t.Errorf("n = %v (%v), want 'x'", nv, aerr)
	}
	if len(el.Cren) != 0 {
		t.Errorf("cren = %v, want none", el.Cren)
	}
}
