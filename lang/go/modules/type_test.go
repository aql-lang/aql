package modules

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// typeRegistry returns a registry with the aql:type module installed.
func typeRegistry(t *testing.T) *native.Registry {
	t.Helper()
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
		// Subtype semantics (TypeScript `Exclude<T,U>`): removing a
		// supertype drops every subtype that participates.
		{`(Integer tor String) type.exclude Number`, "String"},
		{`(Integer tor Decimal tor Boolean) type.exclude Number`, "Boolean"},
		{`Integer type.exclude Number`, "Never"},
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
		// Subtype semantics (TypeScript `Extract<T,U>`): extracting a
		// supertype keeps every numeric subtype in the disjunct.
		{`(Integer tor Decimal tor String) type.extract Number`, "Integer|Decimal"},
		{`(Integer tor String) type.extract Number`, "Integer"},
		{`Integer type.extract Number`, "Integer"},
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
	// Use a lambda literal so the Function value lands on the stack
	// directly (no auto-invoke ambiguity that a bare def-bound name
	// would cause).
	got := runType(t, `([a:Integer b:Integer c:Integer] => [a]) type.paramsof`)
	if len(got) != 1 {
		t.Fatalf("paramsof: got %d results", len(got))
	}
	if s := got[0].String(); s != "[Integer,Integer,Integer]" {
		t.Errorf("paramsof = %q, want [Integer,Integer,Integer]", s)
	}
}

func TestTypeReturnsOf(t *testing.T) {
	// Anonymous lambdas have Returns=[Any] (the conservative default
	// for afn-produced fns). Use that as the smoke check.
	got := runType(t, `([a:Integer b:Integer] => [a]) type.returnsof`)
	if len(got) != 1 || got[0].String() != "Any" {
		t.Errorf("returnsof = %v", formatResults(got))
	}
}

func TestTypeArityOf(t *testing.T) {
	got := runType(t, `([a:Integer b:Integer c:Integer] => [a]) type.arityof`)
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
	// type.nominal mints a fresh refine prefab. The prefab is created
	// in the module's sub-registry lattice; pairing with `def` in the
	// outer registry currently fails because the prefab isn't in the
	// outer lattice (cross-registry minting limitation — see the
	// design doc).
	//
	// Smoke-test only: verify the call returns a type-body value
	// (so `is Type` is true) without trying to bind it via `def`.
	got := runType(t, `(type.nominal Integer) is Type`)
	if len(got) != 1 {
		t.Fatalf("got %d results", len(got))
	}
	b, _ := native.AsBoolean(got[0])
	if !b {
		t.Errorf("(type.nominal Integer) is Type = false, want true")
	}
}

func TestTypeBrand(t *testing.T) {
	// Smoke-test: brand returns a type body. Same cross-registry
	// limitation as TestTypeNominal — can't verify distinct identity
	// via def-binding here.
	got := runType(t, `(Integer type.brand userid/q) is Type`)
	if len(got) != 1 {
		t.Fatalf("got %d results", len(got))
	}
	b, _ := native.AsBoolean(got[0])
	if !b {
		t.Errorf("(Integer type.brand userid/q) is Type = false, want true")
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
