package eng

import (
	"strings"
	"testing"
)

// TestRenderXmlTmplSrcMultiTokenHoles pins the source-form XmlInterp
// render (the comparative-analysis repair of the %v struct-dump leak)
// on the shapes the spec corpus doesn't reach: multi-token `${a b}`
// holes in an attribute value and in a child position, plus a nested
// child element. The multi-token space separator in both loops is the
// covered contract.
func TestRenderXmlTmplSrcMultiTokenHoles(t *testing.T) {
	tmpl := XmlTmpl{
		Tag: "p",
		Attr: []XmlAttrTmpl{{
			Name: "x",
			Parts: []InterpPart{
				{Lit: "n="},
				{Expr: []Value{NewInteger(1), NewWord("addq"), NewInteger(2)}},
			},
		}},
		Cren: []XmlCren{
			{Kind: XmlCrenLit, Lit: "t"},
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("a"), NewWord("b")}},
			{Kind: XmlCrenChild, Child: &XmlTmpl{Tag: "q"}},
		},
	}
	v := NewXmlInterp(tmpl)
	got := v.String()
	want := `interp-xml(<p x="n=${1 word(addq) 2}">t${word(a) word(b)}<q/></p>)`
	if got != want {
		t.Errorf("render = %q, want %q", got, want)
	}

	// Negative pairing: an interp STRING with a multi-token expression
	// renders in source form too — never the raw payload struct.
	is := NewInterpString([]InterpPart{
		{Lit: "a "},
		{Expr: []Value{NewWord("x"), NewWord("addq"), NewInteger(1)}},
	})
	if s := is.String(); s != "interp('a ' ${word(x) word(addq) 1})" {
		t.Errorf("interp render = %q", s)
	}
	if strings.Contains(is.String(), "word()(") {
		t.Errorf("the struct-dump leak is back: %q", is.String())
	}
}
