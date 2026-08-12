package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestFlexXmlRoundTrip exercises the kernel flex primitives for the
// Node/Xml ↔ Node/Xml/FlexXml pair: FlexDeepCopy makes an independent
// mutable copy (no aliasing of the immutable source), in-place mutation
// is observable, NodeDeepCopy converts back to an immutable element, and
// the Xml Behavior renders / compares both payload shapes uniformly.
// See design/XML-LITERAL.0.md §5.
func TestFlexXmlRoundTrip(t *testing.T) {
	mkAttr := func() *core.OrderedMap {
		m := core.NewOrderedMap()
		m.Set("x", core.NewString("1"))
		return m
	}
	xml := core.NewXmlElement("a", mkAttr(), []core.Value{core.NewXmlElement("b", nil, nil)})

	fx, err := core.FlexDeepCopy(xml)
	if err != nil {
		t.Fatalf("FlexDeepCopy: %v", err)
	}
	if !core.IsFlexXml(fx) {
		t.Fatalf("flex result is not FlexXml: %s", core.ValueType(fx).Path())
	}
	if got := fx.String(); got != `<a x="1"><b/></a>` {
		t.Errorf("flex render = %q", got)
	}

	// Mutating the flex copy must NOT touch the immutable source.
	fd, err := core.AsFlexXml(fx)
	if err != nil {
		t.Fatalf("AsFlexXml: %v", err)
	}
	fd.Cren = append(fd.Cren, core.NewXmlElement("c", nil, nil))
	fd.Attr.Set("x", core.NewString("9"))
	if got := xml.String(); got != `<a x="1"><b/></a>` {
		t.Errorf("source mutated through flex copy: %q", got)
	}

	// node converts the mutated tree back to an immutable Node/Xml.
	nv, err := core.NodeDeepCopy(fx)
	if err != nil {
		t.Fatalf("NodeDeepCopy: %v", err)
	}
	if !nv.Is(core.TXml) || core.IsFlexXml(nv) {
		t.Errorf("node result is not an immutable Node/Xml: %s", core.ValueType(nv).Path())
	}
	if got := nv.String(); got != `<a x="9"><b/><c/></a>` {
		t.Errorf("node render = %q", got)
	}

	// Behavior.Equal sees a flex copy and its source element as equal
	// (flexness is a mutability mode, not value identity).
	xml2 := core.NewXmlElement("a", mkAttr(), []core.Value{core.NewXmlElement("b", nil, nil)})
	fx2, err := core.FlexDeepCopy(xml2)
	if err != nil {
		t.Fatalf("FlexDeepCopy(xml2): %v", err)
	}
	if !core.TXml.Behavior().Equal(fx2, xml2) {
		t.Errorf("flex copy not Behavior-equal to its source element")
	}
}
