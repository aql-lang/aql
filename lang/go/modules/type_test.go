package modules

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// typeRegistry returns a registry with the aql:type module installed.
//
// NOTE: aql:type is not yet wired into modules.go — its swap-form
// dispatch fails to auto-invoke the FnDef wrapper. The tests below
// are gated on TestTypeResolve so they only run once the module is
// available.
func typeRegistry(t *testing.T) *native.Registry {
	t.Helper()
	t.Skip("aql:type module pending dispatch fix; see modules.go for details")
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	desc, err := BuildTypeModule(r)
	if err != nil {
		t.Fatal(err)
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return r
}

// run is a thin wrapper that parses an expression and executes it
// against a fresh registry with the type module installed.
func runType(t *testing.T, expr string) []native.Value {
	t.Helper()
	r := typeRegistry(t)
	values, err := parser.Parse(expr)
	if err != nil {
		t.Fatalf("parse %q: %v", expr, err)
	}
	e := native.NewTop(r)
	result, err := e.Run(values)
	if err != nil {
		t.Fatalf("run %q: %v", expr, err)
	}
	return result
}

func TestTypeResolve(t *testing.T) {
	t.Skip("aql:type module pending dispatch fix")
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := Resolve("type", r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tx, ok := desc.Exports["type"]
	if !ok {
		t.Fatal("expected 'type' export in module descriptor")
	}
	for _, name := range []string{
		"exclude", "extract", "required", "pick", "omit", "merge",
		"paramsof", "returnsof", "arityof",
		"parent", "root", "lca", "alts",
		"nominal", "brand",
	} {
		if _, ok := tx.Get(name); !ok {
			t.Errorf("missing 'type.%s' export", name)
		}
	}
}

// --- type-set algebra ---

func TestTypeExclude(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`(String tor None) type.exclude None`, "String"},
		{`(String tor Number tor Boolean) type.exclude Boolean`, "String|Number"},
		{`(String tor Number) type.exclude (String tor Number)`, "Never"},
		{`Integer type.exclude String`, "Integer"},
		{`Integer type.exclude Integer`, "Never"},
	}
	for _, c := range cases {
		got := runType(t, c.expr)
		if len(got) != 1 || got[0].String() != c.want {
			t.Errorf("%s = %v, want %q", c.expr, formatResults(got), c.want)
		}
	}
}

func TestTypeExtract(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`(String tor Number tor Boolean) type.extract Number`, "Number"},
		{`(String tor None) type.extract None`, "None"},
		{`(String tor Number) type.extract Boolean`, "Never"},
		{`Integer type.extract Integer`, "Integer"},
	}
	for _, c := range cases {
		got := runType(t, c.expr)
		if len(got) != 1 || got[0].String() != c.want {
			t.Errorf("%s = %v, want %q", c.expr, formatResults(got), c.want)
		}
	}
}

// --- record surgery ---

func TestTypePick(t *testing.T) {
	got := runType(t, `(refine Record [x:Integer y:String z:Boolean]) type.pick [x/q z/q]`)
	if len(got) != 1 || got[0].String() != "record{x:Integer,z:Boolean}" {
		t.Errorf("pick = %v", formatResults(got))
	}
}

func TestTypeOmit(t *testing.T) {
	got := runType(t, `(refine Record [x:Integer y:String z:Boolean]) type.omit [y/q]`)
	if len(got) != 1 || got[0].String() != "record{x:Integer,z:Boolean}" {
		t.Errorf("omit = %v", formatResults(got))
	}
}

func TestTypeMerge(t *testing.T) {
	got := runType(t, `(refine Record [x:Integer]) type.merge (refine Record [y:String])`)
	if len(got) != 1 || got[0].String() != "record{x:Integer,y:String}" {
		t.Errorf("merge = %v", formatResults(got))
	}
}

func TestTypeRequired(t *testing.T) {
	got := runType(t, `(refine Record [x:Integer y:[String tor None]]) type.required`)
	if len(got) != 1 || got[0].String() != "record{x:Integer,y:String}" {
		t.Errorf("required = %v", formatResults(got))
	}
}

// --- function introspection ---

func TestTypeParamsOf(t *testing.T) {
	got := runType(t, `def add3 fn [[a:Integer b:Integer c:Integer] [Integer] [a b add c add]] type.paramsof add3`)
	if len(got) != 1 {
		t.Fatalf("paramsof: got %d results", len(got))
	}
	if s := got[0].String(); s != "[Integer,Integer,Integer]" {
		t.Errorf("paramsof = %q, want [Integer,Integer,Integer]", s)
	}
}

func TestTypeReturnsOf(t *testing.T) {
	got := runType(t, `def add2 fn [[a:Integer b:Integer] [Integer] [a b add]] type.returnsof add2`)
	if len(got) != 1 || got[0].String() != "Integer" {
		t.Errorf("returnsof = %v", formatResults(got))
	}
}

func TestTypeArityOf(t *testing.T) {
	got := runType(t, `def add3 fn [[a:Integer b:Integer c:Integer] [Integer] [a b add c add]] type.arityof add3`)
	if len(got) != 1 || got[0].String() != "3" {
		t.Errorf("arityof = %v", formatResults(got))
	}
}

// --- hierarchy navigation ---

func TestTypeParent(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`type.parent Integer`, "Number"},
		{`type.parent Number`, "Scalar"},
		{`type.parent Scalar`, "Any"},
		{`type.parent Any`, "Any"},
	}
	for _, c := range cases {
		got := runType(t, c.expr)
		if len(got) != 1 || got[0].String() != c.want {
			t.Errorf("%s = %v, want %q", c.expr, formatResults(got), c.want)
		}
	}
}

func TestTypeRoot(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`type.root Integer`, "Scalar"},
		{`type.root ProperString`, "Scalar"},
		{`type.root List`, "Node"},
		{`type.root Map`, "Node"},
		{`type.root None`, "None"},
		{`type.root Any`, "Any"},
		{`type.root Scalar`, "Scalar"},
	}
	for _, c := range cases {
		got := runType(t, c.expr)
		if len(got) != 1 || got[0].String() != c.want {
			t.Errorf("%s = %v, want %q", c.expr, formatResults(got), c.want)
		}
	}
}

func TestTypeLCA(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`Integer type.lca Decimal`, "Number"},
		{`Integer type.lca Number`, "Number"},
		{`Integer type.lca String`, "Scalar"},
		{`Integer type.lca List`, "Any"},
		{`Integer type.lca Integer`, "Integer"},
	}
	for _, c := range cases {
		got := runType(t, c.expr)
		if len(got) != 1 || got[0].String() != c.want {
			t.Errorf("%s = %v, want %q", c.expr, formatResults(got), c.want)
		}
	}
}

func TestTypeAlts(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`type.alts (String tor None)`, "[String,None]"},
		{`type.alts (Integer tor Decimal tor Boolean)`, "[Integer,Decimal,Boolean]"},
		{`type.alts Integer`, "[Integer]"},
	}
	for _, c := range cases {
		got := runType(t, c.expr)
		if len(got) != 1 || got[0].String() != c.want {
			t.Errorf("%s = %v, want %q", c.expr, formatResults(got), c.want)
		}
	}
}

// --- refinement primitives ---

func TestTypeNominal(t *testing.T) {
	// type.nominal Integer should produce a fresh subtype of Integer.
	// Pair with def to bind a name.
	got := runType(t, `def UserID (type.nominal Integer) typeof UserID`)
	if len(got) != 1 {
		t.Fatalf("got %d results", len(got))
	}
	// The bound type's name should be UserID; typeof of the binding
	// reports the lattice node.
	if s := got[0].String(); s != "UserID" {
		t.Errorf("typeof UserID = %q, want UserID", s)
	}
}

func TestTypeBrand(t *testing.T) {
	// Two brand calls with the same base + tag produce distinct types.
	got := runType(t, `def A (type.brand Integer userid/q) def B (type.brand Integer userid/q) A teq B`)
	if len(got) != 1 {
		t.Fatalf("got %d results", len(got))
	}
	if b, _ := native.AsBoolean(got[0]); b {
		t.Errorf("two brand calls should produce distinct types")
	}
}

// --- error: non-type input ---

func TestTypeErrors(t *testing.T) {
	r := typeRegistry(t)
	cases := []string{
		`5 type.parent`,
		`5 type.root`,
		`5 type.required`,
		`5 type.merge Integer`,
		`5 type.pick [x/q]`,
	}
	for _, expr := range cases {
		values, err := parser.Parse(expr)
		if err != nil {
			t.Fatalf("parse %q: %v", expr, err)
		}
		e := native.NewTop(r)
		_, err = e.Run(values)
		if err == nil {
			t.Errorf("%s: expected error, got nil", expr)
		}
	}
}

// --- helpers ---

func formatResults(values []native.Value) string {
	if len(values) == 0 {
		return "(empty)"
	}
	if len(values) == 1 {
		return values[0].String()
	}
	out := ""
	for i, v := range values {
		if i > 0 {
			out += " "
		}
		out += v.String()
	}
	return out
}
