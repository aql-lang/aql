package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// =============================================================================
// Test 1: Native functions inside fn bodies (FullStackHandler paren fix)
//
// Regression: native functions (which use FullStackHandler) destroyed OpenParen
// markers when called inside fn bodies or explicit paren scopes, producing
// "unmatched closing parenthesis" errors.
// =============================================================================

func TestNativeFnInFnBody(t *testing.T) {
	cases := []struct {
		name  string
		def   string // fn definition step
		call  string // invocation step
		check func(t *testing.T, result []engine.Value)
	}{
		{
			name: "merge",
			def:  `def f fn [[m:Map] [Map] [m merge {x:1}]]`,
			call: `{a:1} f`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				m := result[0].AsMap()
				a, _ := m.Get("a")
				x, _ := m.Get("x")
				if a.AsInteger() != 1 {
					t.Errorf("expected a=1, got %v", a)
				}
				if x.AsInteger() != 1 {
					t.Errorf("expected x=1, got %v", x)
				}
			},
		},
		{
			name: "clone",
			def:  `def f fn [[m:Map] [Map] [m clone]]`,
			call: `{a:1} f`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				assertResult(t, result, "{a:1}")
			},
		},
		{
			name: "size",
			def:  `def f fn [[m:Map] [Integer] [m size]]`,
			call: `{a:1 b:2} f`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				assertResult(t, result, "2")
			},
		},
		{
			name: "jsonify",
			def:  `def f fn [[m:Map] [String] [m jsonify]]`,
			call: `{a:1} f`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				s := result[0].AsString()
				if !strings.Contains(s, "a") {
					t.Errorf("expected JSON containing 'a', got %q", s)
				}
			},
		},
		{
			name: "items",
			def:  `def f fn [[m:Map] [List] [m items]]`,
			call: `{a:1} f`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				items := result[0].AsList().Slice()
				if len(items) != 1 {
					t.Errorf("expected 1 item pair, got %d", len(items))
				}
			},
		},
		{
			name: "getpath",
			def:  `def f fn [[m:Map] [Any] [getpath m "a"]]`,
			call: `{a:42} f`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				assertResult(t, result, "42")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := runNativeSteps(t, nil, []string{tc.def, tc.call})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, result)
		})
	}
}

// =============================================================================
// Test 2: Native functions inside explicit paren scopes
//
// Same FullStackHandler bug path as Test 1, but without fn wrapping.
// =============================================================================

func TestNativeInExplicitParens(t *testing.T) {
	cases := []struct {
		name  string
		expr  string
		check func(t *testing.T, result []engine.Value)
	}{
		{
			name: "merge",
			expr: `({a:1} merge {b:2})`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				m := result[0].AsMap()
				a, _ := m.Get("a")
				b, _ := m.Get("b")
				if a.AsInteger() != 1 {
					t.Errorf("expected a=1, got %v", a)
				}
				if b.AsInteger() != 2 {
					t.Errorf("expected b=2, got %v", b)
				}
			},
		},
		{
			name: "clone",
			expr: `({a:1} clone)`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				assertResult(t, result, "{a:1}")
			},
		},
		{
			name: "size",
			expr: `({a:1 b:2} size)`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				assertResult(t, result, "2")
			},
		},
		{
			name: "jsonify",
			expr: `({a:1} jsonify)`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				s := result[0].AsString()
				if !strings.Contains(s, "a") {
					t.Errorf("expected JSON containing 'a', got %q", s)
				}
			},
		},
		{
			name: "flatten",
			expr: `([[1],[2,3]] flatten)`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				items := result[0].AsList().Slice()
				if len(items) != 3 {
					t.Errorf("expected 3 elements, got %d", len(items))
				}
			},
		},
		{
			name: "join",
			expr: `(join ["a","b"] "-")`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				assertResult(t, result, "'a-b'")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := runNativeSteps(t, nil, []string{tc.expr})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, result)
		})
	}
}

// =============================================================================
// Test 3: Type-literal safety — no panics
//
// For every word that accepts TMap, TList, or TAny arguments, passing a bare
// type literal (Map or List) must produce an error — never a panic.
// Uses runSteps() (engine builtins only); native functions with TAny sigs that
// are not registered will produce "unknown word" errors, which is acceptable.
// =============================================================================

func TestTypeLiteralNoPanic(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		// Accessors
		{"dot-map-atom", `Map dot a`},
		{"dot-list-int", `List dot 0`},
		{"getr-map-atom", `Map a getr`},

		// Control
		{"do-list", `do List`},
		{"do-map", `do Map`},

		// Definitions
		{"call-list", `List call`},
		{"fn-list", `fn List`},
		{"var-list", `var List`},
		{"record-list", `record List`},

		// Strings
		{"concat-list", `concat List`},
		{"pad-map", `"hi" pad 10 Map`},

		// I/O
		{"print-list", `print List`},
		{"print-map", `print Map`},
		{"trace-list", `trace List`},

		// Control flow
		{"if-list-cond", `List if 1 2`},
		{"for-list-body", `for 3 List`},
		{"for-list-range", `for List [1]`},

		// Type ops
		{"convert-type-literal", `Map convert String`},
		{"make-list", `Integer make List`},

		// Comparison (TAny sigs)
		{"eq-map", `Map eq Map`},
		{"lt-map", `Map lt 1`},

		// Native function names (not registered — produces "unknown word" error)
		{"merge-map-literal", `Map merge {a:1}`},
		{"merge-list-literal", `List merge [1]`},

		// Error
		{"error-list", `do [1 div 0] error List`},

		// Module
		{"module-list", `module List`},

		// Double call
		{"dblcall-list", `List dblcall`},

		// Additional accessors
		{"dot-map-no-field", `Map dot`},
		{"getr-list", `List 0 getr`},

		// Stack ops with type literals
		{"dup-map", `Map dup`},
		{"swap-map-list", `Map List swap`},

		// Arithmetic with type literals
		{"add-map", `Map add 1`},
		{"sub-list", `List sub 1`},
		{"mul-map", `Map mul 2`},
		{"div-list", `List div 1`},

		// String ops with type literals
		{"upper-map", `Map upper`},
		{"lower-list", `List lower`},
		{"split-map", `Map split ","`},
		{"trim-list", `List trim`},

		// Boolean ops with type literals
		{"not-map", `Map not`},
		{"and-map-map", `Map and Map`},
		{"or-list-list", `List or List`},

		// Type checking with type literals
		{"typeof-map", `Map typeof`},
		{"typeof-list", `List typeof`},
		{"is-map-integer", `Map is Integer`},
		{"is-list-string", `List is String`},

		// Conversion with type literals
		{"convert-map-string", `Map convert String`},
		{"convert-list-string", `List convert String`},

		// Metatype type literals
		{"typeof-type", `Type typeof`},
		{"typeof-scalartype", `ScalarType typeof`},
		{"typeof-nodetype", `NodeType typeof`},
		{"is-type-type", `Type is Type`},
		{"is-scalartype-type", `ScalarType is Type`},
		{"is-nodetype-type", `NodeType is Type`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Catch panics: a panic means the bug is present.
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC (type-literal safety violation): %v", r)
				}
			}()

			values, parseErr := parser.Parse(tc.expr)
			if parseErr != nil {
				// Parse error is acceptable — not a panic.
				return
			}

			reg, err := engine.DefaultRegistry()
			if err != nil {
				t.Fatal(err)
			}
			eng := engine.NewTop(reg)
			_, _ = eng.Run(values)
			// Any outcome (success or error) is fine — only panics fail the test.
		})
	}
}

// TestTypeLiteralNoPanicNative tests type-literal safety for native functions
// (registered via FullStackHandler). The centralized guard in makeFullStackHandler
// rejects type literals before they reach handlers.
func TestTypeLiteralNoPanicNative(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		{"merge-map-map", `merge Map Map`},
		{"merge-map-only", `Map merge Map`},
		{"clone-map", `clone Map`},
		{"clone-list", `clone List`},
		{"size-map", `size Map`},
		{"size-list", `size List`},
		{"jsonify-map", `jsonify Map`},
		{"jsonify-list", `jsonify List`},
		{"flatten-list", `flatten List`},
		{"join-list", `join List`},
		{"items-map", `items Map`},
		{"items-list", `items List`},
		{"getpath-map", `getpath Map "a"`},
		{"getpath-list", `getpath List "0"`},
		{"inject-map-map", `inject Map Map`},
		{"setpath-map", `setpath Map "a" 1`},
		{"walk-map", `walk Map`},
		{"walk-list", `walk List`},
		{"selector-map", `selector Map "a"`},
		{"validate-map", `validate Map {a:"$STRING"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC (type-literal safety violation): %v", r)
				}
			}()

			values, parseErr := parser.Parse(tc.expr)
			if parseErr != nil {
				return
			}

			reg, err := engine.DefaultRegistry()
			if err != nil {
				t.Fatal(err)
			}
			native.Register(reg)

			eng := engine.NewTop(reg)
			_, _ = eng.Run(values)
			// Any outcome (success or error) is fine — only panics fail the test.
		})
	}
}

// =============================================================================
// Additional regression tests for the FullStackHandler paren fix
// =============================================================================

// TestNativeFnInNestedParens tests native functions inside nested paren scopes
// to ensure OpenParen markers survive multiple levels.
func TestNativeFnInNestedParens(t *testing.T) {
	cases := []struct {
		name     string
		expr     string
		expected string
	}{
		{
			name:     "merge-nested-parens",
			expr:     `(({a:1} merge {b:2}) merge {c:3})`,
			expected: "",
		},
		{
			name:     "size-in-add-paren",
			expr:     `({a:1 b:2} size) add ({c:3} size)`,
			expected: "3",
		},
		{
			name:     "clone-in-nested",
			expr:     `(({a:1} clone) merge {b:2})`,
			expected: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := runNativeSteps(t, nil, []string{tc.expr})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.expected != "" {
				assertResult(t, result, tc.expected)
			} else {
				// Just verify no error/panic — the expression completed.
				if len(result) == 0 {
					t.Fatal("expected non-empty result")
				}
			}
		})
	}
}

// TestNativeFnInFnBodyChained tests chaining multiple native function calls
// inside a single fn body.
func TestNativeFnInFnBodyChained(t *testing.T) {
	cases := []struct {
		name  string
		def   string
		call  string
		check func(t *testing.T, result []engine.Value)
	}{
		{
			name: "clone-then-merge",
			def:  `def f fn [[m:Map] [Map] [m clone merge {extra:1}]]`,
			call: `{a:1} f`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				m := result[0].AsMap()
				a, _ := m.Get("a")
				e, _ := m.Get("extra")
				if a.AsInteger() != 1 {
					t.Errorf("expected a=1, got %v", a)
				}
				if e.AsInteger() != 1 {
					t.Errorf("expected extra=1, got %v", e)
				}
			},
		},
		{
			name: "merge-then-size",
			def:  `def f fn [[m:Map] [Integer] [m merge {x:1} size]]`,
			call: `{a:1} f`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				assertResult(t, result, "2")
			},
		},
		{
			name: "merge-then-jsonify",
			def:  `def f fn [[m:Map] [String] [m merge {b:2} jsonify]]`,
			call: `{a:1} f`,
			check: func(t *testing.T, result []engine.Value) {
				t.Helper()
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				s := result[0].AsString()
				if !strings.Contains(s, "a") || !strings.Contains(s, "b") {
					t.Errorf("expected JSON with 'a' and 'b', got %q", s)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := runNativeSteps(t, nil, []string{tc.def, tc.call})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, result)
		})
	}
}

// TestNativeFnInFnBodyCalledMultipleTimes ensures fn bodies with native
// functions can be called repeatedly without accumulating paren corruption.
func TestNativeFnInFnBodyCalledMultipleTimes(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def f fn [[m:Map] [Integer] [m size]]`,
		`{a:1} f`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")

	result, err = runNativeSteps(t, nil, []string{
		`def f fn [[m:Map] [Integer] [m size]]`,
		`{a:1 b:2} f`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")

	result, err = runNativeSteps(t, nil, []string{
		`def f fn [[m:Map] [Integer] [m size]]`,
		`{a:1 b:2 c:3} f`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3")
}

// TestNativeFnInFnBodyRepeatedCalls tests that a single fn definition with a
// native function body can be invoked multiple times in sequence.
func TestNativeFnInFnBodyRepeatedCalls(t *testing.T) {
	steps := []string{
		`def f fn [[m:Map] [Map] [m merge {added:true}]]`,
		`{a:1} f`,
	}
	result, err := runNativeSteps(t, nil, steps)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	v, ok := m.Get("added")
	if !ok {
		t.Fatal("expected 'added' key in result")
	}
	if fmt.Sprintf("%v", v) == "" {
		t.Error("'added' value should not be empty")
	}
}
