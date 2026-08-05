package eng

import "testing"

// White-box arms of rebuildXmlFromTmpl (OpInterpXml, §9.2c) that the
// source-level rows cannot reach comfortably: a nil child template (the
// defensive mirror of buildXmlFromTmpl's guard), the empty-text coalesce
// return, and the attribute literal-part arm; plus RecordInterpXml's
// inactive-state decline.
func TestRebuildXmlFromTmplArms(t *testing.T) {
	tmpl := XmlTmpl{
		Tag: "p",
		Attr: []XmlAttrTmpl{{Name: "a", Parts: []InterpPart{
			{Lit: "v"},
			{Expr: []Value{NewWord("x")}},
		}}},
		Cren: []XmlCren{
			{Kind: XmlCrenChild, Child: nil},
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("x")}},
		},
	}
	out, used := RebuildXmlFromTmpl(tmpl, []Value{NewString("7"), NewString("")})
	if used != 2 {
		t.Fatalf("used = %d, want 2", used)
	}
	if got := out.String(); got != `<p a="v7"/>` {
		t.Fatalf("rebuild = %s, want <p a=\"v7\"/> (literal+hole attr, empty text hole)", got)
	}

	if (&EmitState{}).RecordInterpXml(XmlTmpl{}, []Value{NewInteger(1)}, Value{}, SrcPos{}) {
		t.Fatal("an inactive EmitState must decline RecordInterpXml")
	}
}
