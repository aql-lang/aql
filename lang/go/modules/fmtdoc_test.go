package modules

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/formatter"
	"github.com/boru-lang/boru/lang/go/native"
)

func dstr(s string) native.Value { return native.NewString(s) }
func dint(n int64) native.Value  { return native.NewInteger(n) }

func dlist(vs ...native.Value) native.Value { return native.NewList(vs) }

func dmap(kv ...native.Value) native.Value {
	m := native.NewOrderedMap()
	for i := 0; i+1 < len(kv); i += 2 {
		k, _ := kv[i].AsConcreteString()
		m.Set(k, kv[i+1])
	}
	return native.NewMap(m)
}

// TestBuildDocPositive covers every doc node kind (text/line/hardline/
// concat/group/indent and the bare-string shorthand) through one composite
// tree rendered flat and broken, plus a hard line.
func TestBuildDocPositive(t *testing.T) {
	inner := dmap(dstr("fmt"), dstr("group"), dstr("body"),
		dmap(dstr("fmt"), dstr("concat"), dstr("parts"),
			dlist(dstr("a"), dmap(dstr("fmt"), dstr("line")), dstr("b"))))
	doc := dmap(dstr("fmt"), dstr("group"), dstr("body"),
		dmap(dstr("fmt"), dstr("concat"), dstr("parts"), dlist(
			dstr("outer("),
			dmap(dstr("fmt"), dstr("indent"), dstr("by"), dint(2), dstr("body"),
				dmap(dstr("fmt"), dstr("concat"), dstr("parts"),
					dlist(dmap(dstr("fmt"), dstr("line")), inner))),
			dmap(dstr("fmt"), dstr("line")),
			dstr(")"),
		)))
	d, err := buildDoc(doc)
	if err != nil {
		t.Fatalf("buildDoc: %v", err)
	}
	if got, want := formatter.RenderDoc(80, d), "outer( a b )"; got != want {
		t.Errorf("flat: got %q, want %q", got, want)
	}
	if got, want := formatter.RenderDoc(8, d), "outer(\n  a b\n)"; got != want {
		t.Errorf("break: got %q, want %q", got, want)
	}

	// text node + hard line.
	d2, err := buildDoc(dmap(dstr("fmt"), dstr("concat"), dstr("parts"), dlist(
		dmap(dstr("fmt"), dstr("text"), dstr("s"), dstr("a")),
		dmap(dstr("fmt"), dstr("hardline")),
		dstr("b"),
	)))
	if err != nil {
		t.Fatalf("buildDoc text/hardline: %v", err)
	}
	if got, want := formatter.RenderDoc(80, d2), "a\nb"; got != want {
		t.Errorf("hardline: got %q, want %q", got, want)
	}
}

// TestBuildDocErrors covers every rejection branch of buildDoc and its
// map-reading helpers.
func TestBuildDocErrors(t *testing.T) {
	cases := []struct {
		name string
		v    native.Value
	}{
		{"non string non map", dint(5)},
		{"missing fmt", dmap(dstr("x"), dstr("y"))},
		{"non-string fmt", dmap(dstr("fmt"), dint(1))},
		{"text missing s", dmap(dstr("fmt"), dstr("text"))},
		{"concat missing parts", dmap(dstr("fmt"), dstr("concat"))},
		{"concat non-list parts", dmap(dstr("fmt"), dstr("concat"), dstr("parts"), dstr("x"))},
		{"concat part error", dmap(dstr("fmt"), dstr("concat"), dstr("parts"), dlist(dint(5)))},
		{"group missing body", dmap(dstr("fmt"), dstr("group"))},
		{"group body error", dmap(dstr("fmt"), dstr("group"), dstr("body"), dint(5))},
		{"indent missing by", dmap(dstr("fmt"), dstr("indent"), dstr("body"), dstr("x"))},
		{"indent non-int by", dmap(dstr("fmt"), dstr("indent"), dstr("by"), dstr("x"), dstr("body"), dstr("y"))},
		{"indent missing body", dmap(dstr("fmt"), dstr("indent"), dstr("by"), dint(2))},
		{"unknown kind", dmap(dstr("fmt"), dstr("bogus"))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := buildDoc(c.v); err == nil {
				t.Errorf("%s: expected an error", c.name)
			}
		})
	}
}

// TestRenderDocHandler covers the handler's success and its two error arms
// (a non-concrete width, and a doc that fails to build).
func TestRenderDocHandler(t *testing.T) {
	doc := dmap(dstr("fmt"), dstr("group"), dstr("body"),
		dmap(dstr("fmt"), dstr("concat"), dstr("parts"),
			dlist(dstr("a"), dmap(dstr("fmt"), dstr("line")), dstr("b"))))

	out, err := renderDocHandler([]native.Value{dint(40), doc}, nil, nil, nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if s, _ := out[0].AsConcreteString(); s != "a b" {
		t.Errorf("render flat: got %q", s)
	}

	if _, err := renderDocHandler([]native.Value{native.NewTypeLiteral(native.TInteger), doc}, nil, nil, nil); err == nil {
		t.Error("non-concrete width should error")
	}
	if _, err := renderDocHandler([]native.Value{dint(40), dint(5)}, nil, nil, nil); err == nil {
		t.Error("invalid doc should error")
	}
}
