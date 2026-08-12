package eng

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestRenderXmlTmplSrcMultiTokenHoles pins the source-form XmlInterp
// render (the comparative-analysis repair of the %v struct-dump leak)
// on the shapes the spec corpus doesn't reach: multi-token `${a b}`
// holes in an attribute value and in a child position, plus a nested
// child element. The multi-token space separator in both loops is the
// covered contract.
func TestRenderXmlTmplSrcMultiTokenHoles(t *testing.T) {
	tmpl := core.XmlTmpl{
		Tag: "p",
		Attr: []core.XmlAttrTmpl{{
			Name: "x",
			Parts: []core.InterpPart{
				{Lit: "n="},
				{Expr: []core.Value{core.NewInteger(1), core.NewWord("addq"), core.NewInteger(2)}},
			},
		}},
		Cren: []core.XmlCren{
			{Kind: core.XmlCrenLit, Lit: "t"},
			{Kind: core.XmlCrenExpr, Expr: []core.Value{core.NewWord("a"), core.NewWord("b")}},
			{Kind: core.XmlCrenChild, Child: &core.XmlTmpl{Tag: "q"}},
		},
	}
	v := core.NewXmlInterp(tmpl)
	got := v.String()
	want := `interp-xml(<p x="n=${1 word(addq) 2}">t${word(a) word(b)}<q/></p>)`
	if got != want {
		t.Errorf("render = %q, want %q", got, want)
	}

	// Negative pairing: an interp STRING with a multi-token expression
	// renders in source form too — never the raw payload struct.
	is := core.NewInterpString([]core.InterpPart{
		{Lit: "a "},
		{Expr: []core.Value{core.NewWord("x"), core.NewWord("addq"), core.NewInteger(1)}},
	})
	if s := is.String(); s != "interp('a ' ${word(x) word(addq) 1})" {
		t.Errorf("interp render = %q", s)
	}
	if strings.Contains(is.String(), "word()(") {
		t.Errorf("the struct-dump leak is back: %q", is.String())
	}
}
