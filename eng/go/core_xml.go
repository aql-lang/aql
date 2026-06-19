package eng

import "strings"

// xmlBehavior is the TypeBehavior for Node/Xml. It renders an element
// back to well-formed XML (Format), and compares two elements
// structurally (Equal). Match stays nominal — DefaultBehavior delegates
// to ConformsTo, so a Node/Xml value matches the Xml / Node / Any slots.
//
// Node/Xml is parser-emitted (embedded `<tag>…</tag>` literals), so the
// type itself is kernel-declared in builtinDecls; this file only
// attaches the Behavior, mirroring coretype_list_map_behaviors.go.
// See design/XML-LITERAL.0.md.
type xmlBehavior struct{}

func (xmlBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }

func (xmlBehavior) Format(v Value) string {
	x, ok := v.Data.(XmlElementPayload)
	if !ok {
		// Bare type literal (Data == nil) or unexpected payload.
		return "Xml"
	}
	var b strings.Builder
	formatXmlInto(&b, x)
	return b.String()
}

func (xmlBehavior) Equal(a, b Value) bool {
	xa, okA := a.Data.(XmlElementPayload)
	xb, okB := b.Data.(XmlElementPayload)
	if okA != okB {
		return false
	}
	if !okA {
		// Both bare type literals: equal iff same lattice node.
		return DefaultBehavior.Equal(a, b)
	}
	return xmlElementsEqual(xa, xb)
}

// formatXmlInto serialises one element. Empty-cren elements self-close
// (`<tag/>`); otherwise `<tag attrs>children</tag>`. Attribute values
// and text are entity-escaped.
func formatXmlInto(b *strings.Builder, x XmlElementPayload) {
	b.WriteByte('<')
	b.WriteString(x.Tag)
	if x.Attr != nil {
		for _, k := range x.Attr.Keys() {
			val, _ := x.Attr.Get(k)
			b.WriteByte(' ')
			b.WriteString(k)
			b.WriteString(`="`)
			b.WriteString(escapeXmlAttr(xmlChildText(val)))
			b.WriteByte('"')
		}
	}
	if len(x.Cren) == 0 {
		b.WriteString("/>")
		return
	}
	b.WriteByte('>')
	for _, c := range x.Cren {
		if cx, ok := c.Data.(XmlElementPayload); ok {
			formatXmlInto(b, cx)
			continue
		}
		b.WriteString(escapeXmlText(xmlChildText(c)))
	}
	b.WriteString("</")
	b.WriteString(x.Tag)
	b.WriteByte('>')
}

// xmlChildText extracts the raw string of a text/scalar child without
// the quoting that Value.String adds to a String value.
func xmlChildText(v Value) string {
	if sp, ok := v.Data.(StrPayload); ok {
		return sp.S
	}
	return v.String()
}

func xmlElementsEqual(a, b XmlElementPayload) bool {
	if a.Tag != b.Tag || len(a.Cren) != len(b.Cren) {
		return false
	}
	ak, bk := orderedMapLen(a.Attr), orderedMapLen(b.Attr)
	if ak != bk {
		return false
	}
	if a.Attr != nil {
		for _, k := range a.Attr.Keys() {
			av, _ := a.Attr.Get(k)
			bv, ok := b.Attr.Get(k)
			if !ok || xmlChildText(av) != xmlChildText(bv) {
				return false
			}
		}
	}
	for i := range a.Cren {
		ca, cb := a.Cren[i], b.Cren[i]
		cax, aIsEl := ca.Data.(XmlElementPayload)
		cbx, bIsEl := cb.Data.(XmlElementPayload)
		if aIsEl != bIsEl {
			return false
		}
		if aIsEl {
			if !xmlElementsEqual(cax, cbx) {
				return false
			}
			continue
		}
		if xmlChildText(ca) != xmlChildText(cb) {
			return false
		}
	}
	return true
}

func orderedMapLen(m *OrderedMap) int {
	if m == nil {
		return 0
	}
	return m.Len()
}

var xmlTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
var xmlAttrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

func escapeXmlText(s string) string { return xmlTextEscaper.Replace(s) }
func escapeXmlAttr(s string) string { return xmlAttrEscaper.Replace(s) }

func init() {
	TXml.Behavior = xmlBehavior{}
}
