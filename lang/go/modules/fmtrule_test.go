package modules

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// TestFmtKindHandler drives every classification arm of Fmt.kind: an
// untagged Map, a Map tagged with a String $kind, a Map tagged with an Atom
// $kind, a Map whose $kind is non-scalar (falls back to "map"), a List, and
// a scalar leaf.
func TestFmtKindHandler(t *testing.T) {
	kind := func(v native.Value) string {
		t.Helper()
		out, err := fmtKindHandler([]native.Value{v}, nil, nil, nil)
		if err != nil {
			t.Fatalf("Fmt.kind: %v", err)
		}
		a, aerr := out[0].AsConcreteAtom()
		if aerr != nil {
			t.Fatalf("Fmt.kind: result not an atom: %v", aerr)
		}
		return a
	}

	untagged := native.NewOrderedMap()
	untagged.Set("a", native.NewInteger(1))
	if got := kind(native.NewMap(untagged)); got != "map" {
		t.Errorf("untagged map = %q, want map", got)
	}

	strTag := native.NewOrderedMap()
	strTag.Set("$kind", native.NewString("entry"))
	if got := kind(native.NewMap(strTag)); got != "entry" {
		t.Errorf("string-tagged map = %q, want entry", got)
	}

	atomTag := native.NewOrderedMap()
	atomTag.Set("$kind", native.NewAtom("header"))
	if got := kind(native.NewMap(atomTag)); got != "header" {
		t.Errorf("atom-tagged map = %q, want header", got)
	}

	// A non-scalar $kind value is not a usable tag — fall back to "map".
	badTag := native.NewOrderedMap()
	badTag.Set("$kind", native.NewList([]native.Value{native.NewInteger(1)}))
	if got := kind(native.NewMap(badTag)); got != "map" {
		t.Errorf("non-scalar-tagged map = %q, want map", got)
	}

	if got := kind(native.NewList([]native.Value{native.NewInteger(1)})); got != "list" {
		t.Errorf("list = %q, want list", got)
	}
	if got := kind(native.NewInteger(5)); got != "scalar" {
		t.Errorf("scalar = %q, want scalar", got)
	}
}

// TestFmtChildrenHandler drives every arm of Fmt.children: a Map (one entry
// node per key, skipping the reserved $kind tag), a List (its elements), and
// a scalar (no children).
func TestFmtChildrenHandler(t *testing.T) {
	children := func(v native.Value) native.ReadList {
		t.Helper()
		out, err := fmtChildrenHandler([]native.Value{v}, nil, nil, nil)
		if err != nil {
			t.Fatalf("Fmt.children: %v", err)
		}
		lst, lerr := native.AsList(out[0])
		if lerr != nil {
			t.Fatalf("Fmt.children: result not a list: %v", lerr)
		}
		return lst
	}

	// A tagged Map: the $kind tag is skipped; each other key becomes an
	// entry node {$kind:'entry' key:<k> value:<v>}.
	m := native.NewOrderedMap()
	m.Set("$kind", native.NewAtom("record"))
	m.Set("a", native.NewInteger(1))
	m.Set("b", native.NewInteger(2))
	kids := children(native.NewMap(m))
	if kids.Len() != 2 {
		t.Fatalf("map children = %d, want 2 (the $kind tag skipped)", kids.Len())
	}
	entry, eerr := native.AsMap(kids.Get(0))
	if eerr != nil {
		t.Fatalf("entry not a map: %v", eerr)
	}
	if k, _ := entry.Get("$kind"); func() bool { a, _ := k.AsConcreteAtom(); return a != "entry" }() {
		t.Errorf("entry $kind not 'entry'")
	}
	if kv, _ := entry.Get("key"); func() bool { s, _ := kv.AsConcreteString(); return s != "a" }() {
		t.Errorf("first entry key not 'a'")
	}
	if vv, _ := entry.Get("value"); func() bool { n, _ := vv.AsConcreteInteger(); return n != 1 }() {
		t.Errorf("first entry value not 1")
	}

	// A List yields its elements verbatim.
	if kids := children(native.NewList([]native.Value{native.NewInteger(7), native.NewInteger(8)})); kids.Len() != 2 {
		t.Errorf("list children = %d, want 2", kids.Len())
	}
	// A scalar has no children.
	if kids := children(native.NewInteger(5)); kids.Len() != 0 {
		t.Errorf("scalar children = %d, want 0", kids.Len())
	}
}

// TestFmtDeclarativeFormatter proves the whole point of the rule vocabulary:
// a complete formatter written as a declarative boru rule table + a one-line
// apply driver, dispatching by Fmt.kind, recursing through Fmt.children, and
// laying the result out with the Fmt.render document algebra. The program
// formats a nested value {a:1 b:{c:2}}; the `b` entry's Map value recurses,
// so the nested `c` appears — end-to-end dispatch + recursion + layout.
//
// It runs via NewTop(reg).Run (no static pre-flight): the driver applies a
// fn fetched from the rule table — a dynamic dispatch that ALSO compiles
// (Stage-3 fn-value dispatch, the whole-frame OpCallDynFrame replay); the
// compiled twin with byte-parity is TestFmtDeclarativeFormatterCompiledParity
// (fmt_compiled_parity_test.go).
func TestFmtDeclarativeFormatter(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)

	const prog = `import "boru:fmt"
def fmtapply fn [nd:Any Any [nd (rules get (Fmt.kind nd)) apply]]
def rules {
  scalar: (nd:Any => (canon nd))
  entry:  (nd:Any => ({fmt:'concat' parts:[nd.key ':' (fmtapply nd.value)]}))
  map:    (nd:Any => ({fmt:'group' body:{fmt:'concat' parts:((Fmt.children nd) each [fmtapply])}}))
}
Fmt.render 72 (fmtapply {a:1 b:{c:2}})`

	values, perr := parser.Parse(prog)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	stack, rerr := native.NewTop(reg).Run(values)
	if rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	if len(stack) != 1 {
		t.Fatalf("run: stack depth %d, want 1", len(stack))
	}
	got, cerr := stack[0].AsConcreteString()
	if cerr != nil {
		t.Fatalf("result not a concrete string: %v", cerr)
	}
	// Flat layout (fits 72): entries joined, the nested map recursed into.
	if want := "a:1b:c:2"; got != want {
		t.Errorf("declarative format = %q, want %q", got, want)
	}
}
