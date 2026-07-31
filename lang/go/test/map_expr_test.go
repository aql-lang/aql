package test

import (
	"github.com/boru-lang/boru/lang/go/native"
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go/parser"
)

// runExpr parses and runs a multi-line BORU expression with a fresh registry.
func runExpr(t *testing.T, expr string) ([]native.Value, error) {
	t.Helper()
	values, err := parser.Parse(expr)
	if err != nil {
		return nil, err
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	eng := native.NewTop(reg)
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
	assertResult(t, result, "{a:1 b:2}")
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
	assertResult(t, result, "[{a:1} {b:2}]")
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
	assertResult(t, result, "{a:10 b:11 c:'lit'}")
}

// --- Inside function bodies ---

func TestMapExprInFnBody(t *testing.T) {
	result, err := runExpr(t, `def x 1 def f word [do {a:x}] f`)
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
	assertResult(t, result, "{a:{b:1} c:2}")
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
		"mod.boru": `def val 42
export "M" {x:val}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`M.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestMapExprModuleExportMultipleDefs(t *testing.T) {
	files := map[string]string{
		"mod.boru": `def a 10 def b 20
export "M" {x:a, y:b}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`M.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10")
}

func TestMapExprModuleExportMultipleDefsSecondKey(t *testing.T) {
	files := map[string]string{
		"mod.boru": `def a 10 def b 20
export "M" {x:a, y:b}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`M.y`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "20")
}

func TestMapExprModuleExportParen(t *testing.T) {
	files := map[string]string{
		"mod.boru": `def bse 10
export "M" {x:(bse add 5)}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`M.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "15")
}

func TestMapExprModuleExportNested(t *testing.T) {
	files := map[string]string{
		"mod.boru": `def v 99
export "M" {top:{deep:v}}`,
	}
	// Access nested: get outer map, then get inner key.
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`M.top`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{deep:99}")
}

func TestMapExprModuleExportNestedDeep(t *testing.T) {
	files := map[string]string{
		"mod.boru": `def v 99
export "M" {top:{deep:v}}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
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
		"mod.boru": `def double fn [[n:Integer] Integer [n add n]]
export "M" {double:double/r}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
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
		"mod.boru": `export "M" {x:"foo"}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`def foo 99`,
		`import "./mod.boru"`,
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
		"a.boru": `export "A" {val:42}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./a.boru"`,
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
		"a.boru": `export "A" {val:42}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./a.boru"`,
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
		"inner.boru": `def n 7
export "Inner" {val:n}`,
		"outer.boru": `import "./inner.boru"
def doubled ((Inner.val) add (Inner.val))
export "Outer" {result:doubled}`,
	}
	result, err := runModuleSteps(t, files, []string{
		`import "./outer.boru"`,
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
	assertResult(t, result, "{a:{b:1} c:2}")
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
	m, _ := native.AsMap(result[0])
	av, _ := m.Get("a")
	bv, _ := m.Get("b")
	cv, _ := m.Get("c")
	avi, _ := native.AsInteger(av)
	bvi, _ := native.AsInteger(bv)
	cvs, _ := native.AsString(cv)
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
	assertResult(t, result, "{a:true b:false}")
}

func TestMapExprNumberValueUnchanged(t *testing.T) {
	result, err := runExpr(t, `def x 99 {a:42, b:x}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:42 b:99}")
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
		"mod.boru": `def bval 100
def incr fn [[n:Integer] Integer [n add 1]]
export "M" {bval:bval, incr:incr/r}`,
	}

	// Test 1: explicit map with module value
	result, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`def b (M.bval)`,
		`{x:b}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{x:100}")

	// Test 2: explicit map with paren expression
	result, err = runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`def b (M.bval)`,
		`{x:(b add 5)}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{x:105}")

	// Test 3: map inside a list
	result, err = runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`def b (M.bval)`,
		`[{val:b}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "[{val:100}]")

	// Test 4: nested map
	result, err = runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`def b (M.bval)`,
		`{top:{deep:b}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{top:{deep:100}}")
}

// --- Shorthand map syntax: {foo} ≡ {foo: foo} ---

func TestMapExprShorthandBasic(t *testing.T) {
	result, err := runExpr(t, `def foo 1 {foo}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{foo:1}")
}

func TestMapExprShorthandMatchesExplicit(t *testing.T) {
	sh, err := runExpr(t, `def foo 10 def bar 20 {foo a:1 bar}`)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := runExpr(t, `def foo 10 def bar 20 {foo:foo a:1 bar:bar}`)
	if err != nil {
		t.Fatal(err)
	}
	if formatStack(sh) != formatStack(ex) {
		t.Errorf("shorthand = %q, want same as explicit = %q",
			formatStack(sh), formatStack(ex))
	}
}

func TestMapExprShorthandNested(t *testing.T) {
	result, err := runExpr(t, `def foo 5 {a:{foo}}`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:{foo:5}}")
}

func TestMapExprShorthandRefModifier(t *testing.T) {
	// {f/r} captures the fn reference under key "f"; calling it dispatches.
	result, err := runExpr(t,
		`def f fn [[a:Integer b:Integer] [Integer] [a add b]] ({f/r}).f 2 3`)
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "5")
}

func TestMapExprShorthandUnboundErrors(t *testing.T) {
	// An unbound shorthand name errors exactly like {a:foo} would.
	_, err := runExpr(t, `{foo}`)
	if err == nil {
		t.Fatalf("expected undefined_word error for unbound shorthand")
	}
	assertErrorContains(t, err, "undefined", "foo")
}

// TestDoMapListValueRespectsQuoting pins voxgig DX report T4: a list
// value inside a `do` map evaluates unquoted words as code, but leaves
// quoted strings and atoms as DATA — so a stored value whose text
// happens to name a registered word (`"if"`, `"get"`, `"do"`) is kept,
// not dispatched. Before the fix, the promote-strings-to-words step
// dispatched them, forcing the boxing workaround the report describes
// (and breaking even that).
func TestDoMapListValueRespectsQuoting(t *testing.T) {
	cases := []struct{ name, src, want string }{
		// Data preserved — quoted strings that name words stay as strings.
		{"word-name string if", `do {val:["if"]}`, `{val:'if'}`},
		{"word-name string get", `do {a:["get"]}`, `{a:'get'}`},
		{"word-name string do", `do {a:["do"]}`, `{a:'do'}`},
		{"non-word string", `do {a:["hello"]}`, `{a:'hello'}`},
		// Code still runs — unquoted words evaluate as before.
		{"unquoted word runs", `do {a:[add 1 2]}`, `{a:3}`},
		{"unquoted bound name resolves", `def x 5 do {a:[x]}`, `{a:5}`},
		{"plain value list passes through", `do {a:[1 2 3]}`, `{a:[1 2 3]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runExpr(t, tc.src)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.src, err)
			}
			if got := formatStack(out); got != tc.want {
				t.Errorf("%s\n got: %s\nwant: %s", tc.src, got, tc.want)
			}
		})
	}
}

// suppress unused import warning
var _ = strings.Join
