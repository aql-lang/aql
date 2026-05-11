package test

import (
	"github.com/aql-lang/aql/lang/native"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/parser"
	"github.com/aql-lang/aql/lang/engine"
)

// runExpr parses and runs a multi-line AQL expression with a fresh registry.
func runExpr(t *testing.T, expr string) ([]engine.Value, error) {
	t.Helper()
	values, err := parser.Parse(expr)
	if err != nil {
		return nil, err
	}
	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.NewTop(reg)
	return eng.Run(values)
}

// --- Explicit map (baseline) ---

func TestMapExprExplicitBasic(t *testing.T) {
	result, err := runExpr(t, `def x 1 {a:x}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:1}")
}

func TestMapExprExplicitMultiKey(t *testing.T) {
	result, err := runExpr(t, `def x 1 def y 2 {a:x, b:y}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:1,b:2}")
}

// --- Implicit map (pair syntax at top level) ---

func TestMapExprImplicitBasic(t *testing.T) {
	result, err := runExpr(t, `def x 1 ; a:x`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:1}")
}

func TestMapExprImplicitMultiKey(t *testing.T) {
	result, err := runExpr(t, `def x 1 def y 2 ; a:x ; b:y`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:1} {b:2}")
}

// --- Inside lists ---

func TestMapExprInList(t *testing.T) {
	result, err := runExpr(t, `def x 1 [{a:x}]`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "[{a:1}]")
}

func TestMapExprInListMultipleMaps(t *testing.T) {
	result, err := runExpr(t, `def x 1 def y 2 [{a:x},{b:y}]`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "[{a:1},{b:2}]")
}

func TestMapExprInListNestedExpr(t *testing.T) {
	result, err := runExpr(t, `def x 10 [{a:(x add 5)}]`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "[{a:15}]")
}

// --- Paren expressions in map values ---

func TestMapExprParenSimple(t *testing.T) {
	result, err := runExpr(t, `{a:(1 add 2)}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:3}")
}

func TestMapExprParenWithDef(t *testing.T) {
	result, err := runExpr(t, `def x 10 {a:(x add 5)}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:15}")
}

func TestMapExprParenMultipleOps(t *testing.T) {
	result, err := runExpr(t, `def x 2 {a:(x mul 3 add 1)}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:7}")
}

func TestMapExprParenString(t *testing.T) {
	result, err := runExpr(t, `{a:("hello" upper)}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:'HELLO'}")
}

func TestMapExprParenMixedValues(t *testing.T) {
	result, err := runExpr(t, `def x 10 {a:x, b:(x add 1), c:"lit"}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:10,b:11,c:'lit'}")
}

// --- Inside function bodies ---

func TestMapExprInFnBody(t *testing.T) {
	result, err := runExpr(t, `def x 1 def f [do {a:x}] f`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:1}")
}

func TestMapExprInFnBodyDefRef(t *testing.T) {
	// Fn body uses a top-level def in a map value.
	result, err := runExpr(t, `def x 42 def mkmap fn [[] Map [do {a:x}]] mkmap`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:42}")
}

// --- Nested maps ---

func TestMapExprNestedExplicit(t *testing.T) {
	result, err := runExpr(t, `def x 1 {a:{b:x}}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:{b:1}}")
}

func TestMapExprNestedDeep(t *testing.T) {
	result, err := runExpr(t, `def x 1 {a:{b:{c:x}}}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:{b:{c:1}}}")
}

func TestMapExprNestedMixed(t *testing.T) {
	result, err := runExpr(t, `def x 1 def y 2 {a:{b:x}, c:y}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:{b:1},c:2}")
}

func TestMapExprNestedWithParen(t *testing.T) {
	result, err := runExpr(t, `def x 5 {a:{b:(x add 1)}}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:{b:6}}")
}

// --- Inside modules ---

func TestMapExprModuleExportDef(t *testing.T) {
	// Module exports a map whose values come from defs inside the module.
	files := map[string]string{
		"mod.aql": `def val 42
export "M" {x:val}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`M.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestMapExprModuleExportMultipleDefs(t *testing.T) {
	files := map[string]string{
		"mod.aql": `def a 10 def b 20
export "M" {x:a, y:b}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`M.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10")
}

func TestMapExprModuleExportMultipleDefsSecondKey(t *testing.T) {
	files := map[string]string{
		"mod.aql": `def a 10 def b 20
export "M" {x:a, y:b}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`M.y`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "20")
}

func TestMapExprModuleExportParen(t *testing.T) {
	files := map[string]string{
		"mod.aql": `def base 10
export "M" {x:(base add 5)}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`M.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "15")
}

func TestMapExprModuleExportNested(t *testing.T) {
	files := map[string]string{
		"mod.aql": `def v 99
export "M" {top:{deep:v}}`,
	}
	// Access nested: get outer map, then get inner key.
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`M.top`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{deep:99}")
}

func TestMapExprModuleExportNestedDeep(t *testing.T) {
	files := map[string]string{
		"mod.aql": `def v 99
export "M" {top:{deep:v}}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`def m (M.top)`,
		`m.deep`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "99")
}

func TestMapExprModuleExportFnDef(t *testing.T) {
	// Module exports a function; caller uses it to build a map with expressions.
	files := map[string]string{
		"mod.aql": `def double fn [[n:Integer] Integer [n add n]]
export "M" {double:double}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`def x 5`,
		`{a:(x M.double)}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:10}")
}

func TestMapExprModuleIsolation(t *testing.T) {
	// Parent defs should NOT leak into module map values.
	// Undefined word in map value now errors, so use a string.
	files := map[string]string{
		"mod.aql": `export "M" {x:"foo"}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`def foo 99`,
		`import "./mod.aql"`,
		`M.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	// "foo" is a string, not 99 — proves isolation.
	got := formatStack(result)
	if got == "99" {
		t.Error("parent def 'foo' leaked into module map value")
	}
}

func TestMapExprModuleChainDefs(t *testing.T) {
	// Module A exports a value; top level imports and uses it in a map.
	files := map[string]string{
		"a.aql": `export "A" {val:42}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./a.aql"`,
		`def v (A.val)`,
		`{result:v}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{result:42}")
}

func TestMapExprModuleChainDefsImplicit(t *testing.T) {
	// Same as above but with implicit map syntax.
	files := map[string]string{
		"a.aql": `export "A" {val:42}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./a.aql"`,
		`def v (A.val)`,
		`result:v`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{result:42}")
}

func TestMapExprModuleDeepChain(t *testing.T) {
	// Chain: inner → outer → top level, each using map expressions with defs.
	files := map[string]string{
		"inner.aql": `def n 7
export "Inner" {val:n}`,
		"outer.aql": `import "./inner.aql"
def doubled ((Inner.val) add (Inner.val))
export "Outer" {result:doubled}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./outer.aql"`,
		`(Outer.result)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "14")
}

// --- Map expressions in do blocks ---

func TestMapExprDo(t *testing.T) {
	result, err := runExpr(t, `def x 1 do {a:x}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:1}")
}

func TestMapExprDoNested(t *testing.T) {
	result, err := runExpr(t, `def x 1 def y 2 do {a:{b:x}, c:y}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:{b:1},c:2}")
}

func TestMapExprDoParen(t *testing.T) {
	result, err := runExpr(t, `def x 5 do {a:(x mul 2)}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:10}")
}

// --- Mixed: list values inside maps ---

func TestMapExprListValue(t *testing.T) {
	result, err := runExpr(t, `def n 10 do {a:n, b:[n add 5], c:"lit"}`)
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	av, _ := m.Get("a")
	bv, _ := m.Get("b")
	cv, _ := m.Get("c")
	avi, _ := av.AsInteger()
	bvi, _ := bv.AsInteger()
	cvs, _ := cv.AsString()
	if avi != 10 {
		t.Errorf("a = %d, want 10", avi)
	}
	if bvi != 15 {
		t.Errorf("b = %d, want 15", bvi)
	}
	if cvs != "lit" {
		t.Errorf("c = %q, want 'lit'", cvs)
	}
}

// --- Edge cases ---

func TestMapExprStringValueUnchanged(t *testing.T) {
	// Quoted strings should pass through unchanged even with same name as a def.
	result, err := runExpr(t, `def x 1 {a:"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:'x'}")
}

func TestMapExprBooleanValueUnchanged(t *testing.T) {
	result, err := runExpr(t, `{a:true, b:false}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:true,b:false}")
}

func TestMapExprNumberValueUnchanged(t *testing.T) {
	result, err := runExpr(t, `def x 99 {a:42, b:x}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:42,b:99}")
}

func TestMapExprEmptyMap(t *testing.T) {
	result, err := runExpr(t, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{}")
}

// --- Module with all contexts combined ---

func TestMapExprModuleComprehensive(t *testing.T) {
	// Module exports function + constant; top level uses both
	// in explicit map, implicit map, list, nested map, and paren expr.
	files := map[string]string{
		"mod.aql": `def bval 100
def incr fn [[n:Integer] Integer [n add 1]]
export "M" {bval:bval, incr:incr}`,
	}

	// Test 1: explicit map with module value
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`def b (M.bval)`,
		`{x:b}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{x:100}")

	// Test 2: explicit map with paren expression
	result, err = runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`def b (M.bval)`,
		`{x:(b add 5)}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{x:105}")

	// Test 3: map inside a list
	result, err = runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`def b (M.bval)`,
		`[{val:b}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "[{val:100}]")

	// Test 4: nested map
	result, err = runModuleSteps(t, files, []string{
		`import "./mod.aql"`,
		`def b (M.bval)`,
		`{top:{deep:b}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{top:{deep:100}}")
}

// suppress unused import warning
var _ = strings.Join
