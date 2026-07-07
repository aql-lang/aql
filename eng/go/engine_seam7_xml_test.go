package eng

// Wave-7 coverage, part 7: buildXmlFromTmpl / evalXmlInterp arms, reached
// by constructing XmlTmpl skeletons directly. See design/TEST-SEAMS.10.md.

import (
	"strings"
	"testing"
)

// xmlReg builds a cov registry plus a carrier-bound name (dynv) and a
// flow-control-raising word (cbreak) for driving the dynamic / flow arms.
func xmlReg(t *testing.T) *Registry {
	t.Helper()
	r := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "cfailx",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return nil, &AqlError{Code: "runtime_error", Detail: "xboom"}
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
		r.RegisterNativeFunc(NativeFunc{
			Name: "cbreak",
			Signatures: []Signature{{
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
					reg.FlowCtrl = FlowBreak
					return []Value{NewInteger(0)}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
	})
	r.Defs.Push("dynv", NewCarrier(TAny))
	return r
}

func TestS7BuildXmlEmptyLiteralChild(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Cren: []XmlCren{{Kind: XmlCrenLit, Lit: ""}}}
	_, _, err := e.buildXmlFromTmpl(tmpl)
	if err != nil {
		t.Fatalf("empty literal child: %v", err)
	}
}

func TestS7BuildXmlNestedChildError(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	child := &XmlTmpl{Tag: "span", Cren: []XmlCren{{
		Kind: XmlCrenExpr, Expr: []Value{NewWord("cfailx"), NewInteger(1)},
	}}}
	tmpl := XmlTmpl{Tag: "p", Cren: []XmlCren{{Kind: XmlCrenChild, Child: child}}}
	if _, _, err := e.buildXmlFromTmpl(tmpl); err == nil {
		t.Fatal("a nested child template that errors must propagate")
	}
}

func TestS7BuildXmlNilChildSkipped(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Cren: []XmlCren{{Kind: XmlCrenChild, Child: nil}}}
	if _, _, err := e.buildXmlFromTmpl(tmpl); err != nil {
		t.Fatalf("nil child should be skipped, got %v", err)
	}
}

func TestS7BuildXmlAttrFlowCtrl(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Attr: []XmlAttrTmpl{{
		Name: "x", Parts: []InterpPart{{Expr: []Value{NewWord("cbreak")}}},
	}}}
	if _, _, err := e.buildXmlFromTmpl(tmpl); err != nil {
		t.Fatalf("attr flow-ctrl: %v", err)
	}
	if r.FlowCtrl != FlowBreak {
		t.Error("cbreak should have raised a flow-control signal")
	}
}

func TestS7BuildXmlChildExprFlowCtrl(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Cren: []XmlCren{{
		Kind: XmlCrenExpr, Expr: []Value{NewWord("cbreak")},
	}}}
	if _, _, err := e.buildXmlFromTmpl(tmpl); err != nil {
		t.Fatalf("child expr flow-ctrl: %v", err)
	}
	if r.FlowCtrl != FlowBreak {
		t.Error("cbreak should have raised a flow-control signal")
	}
}

func TestS7BuildXmlAttrDynamic(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Attr: []XmlAttrTmpl{{
		Name: "x", Parts: []InterpPart{{Expr: []Value{NewWord("dynv")}}},
	}}}
	_, dynamic, err := e.buildXmlFromTmpl(tmpl)
	if err != nil {
		t.Fatalf("attr dynamic: %v", err)
	}
	if !dynamic {
		t.Error("a carrier-valued attr part should mark the build dynamic")
	}
}

func TestS7BuildXmlNestedChildDynamic(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	child := &XmlTmpl{Tag: "span", Attr: []XmlAttrTmpl{{
		Name: "x", Parts: []InterpPart{{Expr: []Value{NewWord("dynv")}}},
	}}}
	tmpl := XmlTmpl{Tag: "p", Cren: []XmlCren{{Kind: XmlCrenChild, Child: child}}}
	_, dynamic, err := e.buildXmlFromTmpl(tmpl)
	if err != nil {
		t.Fatalf("nested child dynamic: %v", err)
	}
	if !dynamic {
		t.Error("a dynamic nested child should mark the parent build dynamic")
	}
}

func TestS7EvalXmlInterpNonTemplate(t *testing.T) {
	// A non-XmlInterp value fails AsXmlInterp → error arm.
	e := NewTop(xmlReg(t))
	if _, err := e.evalXmlInterp(NewInteger(5)); err == nil {
		t.Fatal("evalXmlInterp on a non-template value should error")
	}
}

func TestS7EvalInterpStringNonTemplate(t *testing.T) {
	// A non-InterpString value → AsInterpString errors → empty string.
	e := NewTop(xmlReg(t))
	out, err := e.evalInterpString(NewInteger(5))
	if err != nil {
		t.Fatalf("evalInterpString: %v", err)
	}
	if s, _ := AsString(out); s != "" {
		t.Errorf("non-template → %q, want empty string", s)
	}
}

// --- evalXmlInterp check-mode refusal --------------------------------------

func TestS7EvalXmlInterpCheckModeDynamicRefuses(t *testing.T) {
	r := xmlReg(t)
	done := r.Check.Begin()
	r.Check.Emit = NewEmitState()
	defer done()
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Attr: []XmlAttrTmpl{{
		Name: "x", Parts: []InterpPart{{Expr: []Value{NewWord("dynv")}}},
	}}}
	out, err := e.evalXmlInterp(NewXmlInterp(tmpl))
	if err != nil {
		t.Fatalf("evalXmlInterp: %v", err)
	}
	// A dynamic interpolation under an active recorder yields an Xml carrier.
	if !out.Carrier || !strings.Contains(out.Parent.Leaf(), "Xml") {
		t.Errorf("expected an Xml carrier, got %v", out)
	}
}
