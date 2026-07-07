package native

import (
	"strings"
	"testing"
)

// Wave-3 coverage for native_behave.go (the behave word, the sig
// validators, and the userBehavior capability wrapper) and reify.go
// (class hydration from Nodes and JSON text). The program shapes mirror
// lang/go/test/behave.tsv and lang/spec/module-struct.tsv, run here in
// the native package so the handlers count toward its self-coverage.

func w3Behave(t *testing.T, src string) (string, error) {
	t.Helper()
	return w3Run(t, w3TypeReg(), src)
}

func w3BehaveWant(t *testing.T, src, want string) {
	t.Helper()
	got, err := w3Behave(t, src)
	if err != nil {
		t.Fatalf("%q: unexpected error: %v", src, err)
	}
	if got != want {
		t.Errorf("%q = %q, want %q", src, got, want)
	}
}

func w3BehaveErr(t *testing.T, src, substr string) {
	t.Helper()
	_, err := w3Behave(t, src)
	if err == nil {
		t.Fatalf("%q: expected error containing %q, got none", src, substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("%q: error %q does not contain %q", src, err.Error(), substr)
	}
}

// ---- behave compare ----

func TestW3BehaveCompare(t *testing.T) {
	base := `def Person class {age:Integer}
behave compare/q (fn [[Person Person] [Integer] [(a 'age' get) (b 'age' get) sub]])
def alice (make Person {age:30})
def bob (make Person {age:25})
`
	w3BehaveWant(t, base+`alice bob lt`, `false`)
	w3BehaveWant(t, base+`bob alice lt`, `true`)
	w3BehaveWant(t, base+`alice alice lte`, `true`)
}

func TestW3BehaveCompareRejections(t *testing.T) {
	w3BehaveErr(t, `def P class {age:Integer} behave compare/q (fn [[P Integer] [Integer] [42]])`,
		"both params must be the same type")
	w3BehaveErr(t, `def P class {age:Integer} behave compare/q (fn [[P] [Integer] [42]])`,
		"must take 2 args")
	w3BehaveErr(t, `def P class {age:Integer} behave compare/q (fn [[P P] [String] ['x']])`,
		"return Integer")
	w3BehaveErr(t, `behave compare/q (fn [[Integer Integer] [Integer] [0]])`,
		"builtin")
	w3BehaveErr(t, `behave bogus/q (fn [[Integer Integer] [Integer] [0]])`,
		"unknown behavior")
	// A body error propagates through the Comparer.
	w3BehaveErr(t, `def F class {x:Integer}
behave compare/q (fn [[F F] [Integer] [quux]])
def v1 (make F {x:1}) def v2 (make F {x:2}) v1 v2 lt`, "quux")
	// A body whose runtime result is not an Integer is loud.
	w3BehaveErr(t, `def G class {x:Integer}
behave compare/q (fn [[G G] [Integer] [drop drop 'nope']])
def v1 (make G {x:1}) def v2 (make G {x:2}) v1 v2 lt`, "")
	// The fn arg must be a Function value (direct call: dispatch gates
	// non-functions away).
	r := w3TypeReg()
	if _, err := behaveHandler([]Value{NewAtom("compare"), NewInteger(5)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "must be a Function") {
		t.Errorf("behave with non-fn: got %v", err)
	}
}

// ---- behave canon ----

func TestW3BehaveCanon(t *testing.T) {
	w3BehaveWant(t, `def Person class {name:String age:Integer}
behave canon/q (fn [[Person] [String] [(a 'name' get)]])
make Person {name:'Alice' age:30}`, `Alice`)
	// String-form behavior name takes the TString sig.
	w3BehaveWant(t, `def Q class {name:String}
behave "canon" (fn [[Q] [String] [(a 'name' get)]])
make Q {name:'Zed'}`, `Zed`)
	// All slots coexist: compare still works after canon installed.
	w3BehaveWant(t, `def Item class {n:Integer}
behave compare/q (fn [[Item Item] [Integer] [(a 'n' get) (b 'n' get) sub]])
behave canon/q (fn [[Item] [String] [(a 'n' get) ' (item)' add]])
def i1 (make Item {n:1}) def i9 (make Item {n:9})
i1 i9 lt`, `true`)
	// Match / Equal delegate through the wrapper after install.
	w3BehaveWant(t, `def R class {x:Integer}
behave canon/q (fn [[R] [String] ['r']])
def v (make R {x:1})
(v is R) (v eq v)`, `true true`)
}

func TestW3BehaveCanonRejections(t *testing.T) {
	w3BehaveErr(t, `def P class {name:String} behave canon/q (fn [[P P] [String] ['x']])`,
		"1 arg")
	w3BehaveErr(t, `def P class {name:String} behave canon/q (fn [[P] [Integer] [42]])`,
		"return String")
	// A canon body that errors at render time surfaces as an inline
	// canon-error marker, not a crash.
	got, err := w3Behave(t, `def P class {x:Integer}
behave canon/q (fn [[P] [String] [quux]])
make P {x:1}`)
	if err != nil {
		t.Fatalf("canon-error render: %v", err)
	}
	if !strings.Contains(got, "canon-error") {
		t.Errorf("canon body error should render inline, got %q", got)
	}
	// A canon body that returns a non-String at run time is the same shape.
	got, err = w3Behave(t, `def P2 class {x:Integer}
behave canon/q (fn [[P2] [String] [drop 42]])
make P2 {x:1}`)
	if err != nil {
		t.Fatalf("canon non-string render: %v", err)
	}
	if !strings.Contains(got, "canon-error") {
		t.Errorf("canon non-String body should render canon-error, got %q", got)
	}
}

// ---- behave nodify ----

func TestW3BehaveNodify(t *testing.T) {
	// Default: values with no nodify behavior pass through.
	w3BehaveWant(t, `5 nodify`, `5`)
	w3BehaveWant(t, `nodify {a:1}`, `{a:1}`)
	// A custom projection renames and drops fields.
	w3BehaveWant(t, `def Person class {name:String age:Integer secret:String}
behave nodify/q (fn [[Person] [Any] [{n: (a 'name' get), age: (a 'age' get)}]])
make Person {name:'Alice' age:30 secret:'hush'} nodify`, `{age:30 n:'Alice'}`)
	// Scalar collapse.
	w3BehaveWant(t, `def Money class {cents:Integer}
behave nodify/q (fn [[Money] [Any] [(a 'cents' get)]])
make Money {cents:1234} nodify`, `1234`)
	// Sig rejections.
	w3BehaveErr(t, `def F class {x:Integer} behave nodify/q (fn [[F F] [Any] [42]])`,
		"1 arg")
	// A body error propagates.
	w3BehaveErr(t, `def E class {x:Integer}
behave nodify/q (fn [[E] [Any] [quux]])
make E {x:1} nodify`, "quux")
}

// ---- behave unify ----

func TestW3BehaveUnify(t *testing.T) {
	w3BehaveWant(t, `def Item refine Integer
behave unify/q (fn [[a:Item b:Item] [Any] [a b add]])
def x:Item 3
def y:Item 4
x unify y`, `7 true`)
	// None signals unification failure.
	w3BehaveWant(t, `def Item refine Integer
behave unify/q (fn [[a:Item b:Item] [Any] [None]])
def x:Item 3
def y:Item 4
x unify y`, `'~unify-fail' false`)
	// Output-type validation: a value outside the target family is loud.
	got, err := w3Behave(t, `def Item refine Integer
behave unify/q (fn [[a:Item b:Item] [Any] ["bogus-string"]])
def x:Item 3
def y:Item 4
x unify y`)
	if err != nil {
		t.Fatalf("unify bogus output: %v", err)
	}
	if !strings.HasSuffix(got, `false`) {
		t.Errorf("unify with out-of-family body result should fail, got %q", got)
	}
	// A body error is a unify failure, not a crash.
	got, err = w3Behave(t, `def Item refine Integer
behave unify/q (fn [[a:Item b:Item] [Any] [quux]])
def x:Item 3
def y:Item 4
x unify y`)
	if err != nil {
		t.Fatalf("unify body error: %v", err)
	}
	if !strings.HasSuffix(got, `false`) {
		t.Errorf("unify with erroring body should fail, got %q", got)
	}
}

func TestW3BehaveUnifyRejections(t *testing.T) {
	w3BehaveErr(t, `def Item refine Integer behave unify/q (fn [[a:Item] [Item] [a]])`,
		"must take 2 args")
	w3BehaveErr(t, `def Item refine Integer def Other refine Integer behave unify/q (fn [[a:Item b:Other] [Item] [a]])`,
		"same type")
	w3BehaveErr(t, `def Item refine Integer behave unify/q (fn [[a:Item b:Item] [String] [a]])`,
		"return type must match")
}

// ---- reify: Node and JSON-text hydration ----

func TestW3ReifyNode(t *testing.T) {
	w3BehaveWant(t, `def Point class {x:1, y:2} def p (reify Point {x:9}) p typeof`, `Point`)
	w3BehaveWant(t, `def Point class {x:1, y:2} (reify Point {x:9}) dot y`, `2`)
	w3BehaveErr(t, `def Point class {x:1} reify Point {z:5}`, "unknown field")
	w3BehaveErr(t, `reify Integer {x:1}`, "must be a class")
}

func TestW3ReifyJSONText(t *testing.T) {
	w3BehaveWant(t, `def Point class {x:1} (reify Point '{"x": 9}') dot x`, `9`)
	w3BehaveErr(t, `def Point class {x:1} reify Point '{bad'`, "invalid JSON")
	w3BehaveErr(t, `def Point class {x:1} reify Point '[1, 2]'`, "expected a JSON object")
	w3BehaveErr(t, `def Point class {x:1} reify Point '{"$bogus": 1}'`, "unknown metadata key")
	w3BehaveErr(t, `def Point class {x:1} reify Point '{"$class": 5}'`, "$class must be a string")
	w3BehaveErr(t, `def Point class {x:1} reify Point '{"$class": "Other"}'`, "does not match target class")
}

func TestW3ReifyUnion(t *testing.T) {
	base := `def Circle class {r:1.0} def Square class {side:1.0} def Shape (Circle tor Square) `
	w3BehaveWant(t, base+`(reify Shape (jsonify (make Square {side:4.0}))) typeof`, `Square`)
	w3BehaveErr(t, base+`reify Shape {r:2.0}`, "needs a $class")
	w3BehaveErr(t, base+`reify Shape '{"$class": "Blob"}'`, "not a member of the union")
}

func TestW3ReifySchemaRecovery(t *testing.T) {
	// A JSON integer lands as the Float the predicate field declares.
	w3BehaveWant(t, `def Radius (Float gte 0.0) def Circle class {r:Radius} (reify Circle '{"r": 5}') dot r`, `5.0`)
	// The predicate still runs strictly after recovery.
	w3BehaveErr(t, `def Radius (Float gte 0.0) def Circle class {r:Radius} reify Circle '{"r": -1.5}'`, "does not satisfy")
	// A string field declared Atom recovers to an Atom.
	w3BehaveWant(t, `def Tagged class {kind:Atom} ((reify Tagged '{"kind": "circle"}') dot kind) typeof`, `Atom`)
}

func TestW3ReifyNestedAndLeaves(t *testing.T) {
	// Nested class hydrates recursively — from a Node and from JSON text.
	w3BehaveWant(t, `def Inner class {v:1} def Outer class {i:Inner} ((reify Outer {i:{v:9}}) dot i) dot v`, `9`)
	w3BehaveWant(t, `def Inner class {v:1} def Outer class {i:Inner} ((reify Outer '{"i": {"v": 9}}') dot i) dot v`, `9`)
	// JSON leaves: bool, list, float, null.
	w3BehaveWant(t, `def P class {on:Boolean} (reify P '{"on": true}') dot on`, `true`)
	w3BehaveWant(t, `def P class {xs:List} (reify P '{"xs": [1, 2]}') dot xs`, `[1 2]`)
	w3BehaveWant(t, `def P class {f:1.5} (reify P '{"f": 2.5}') dot f`, `2.5`)
	// The jsonify→reify round trip is structurally exact.
	w3BehaveWant(t, `def P class {x:1} def p (make P {x:5}) (reify P (jsonify p)) deq p`, `true`)
}
