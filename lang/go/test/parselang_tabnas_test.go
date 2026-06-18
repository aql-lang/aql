package test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// The tabnas parser family (github.com/tabnas/*) all ships built-in with
// aql:parselang, reached the same way as ini: import the module, then
// `parse <kind> '<text>'`. These tests cover the user-facing sugar for each
// kind plus the kinds listing and a couple of failure modes.

// TestParseLangTabnasFamily pins one decode per built-in parser kind through
// the `parse <kind>` sugar, across the three top-level shapes (object, list,
// scalar field).
func TestParseLangTabnasFamily(t *testing.T) {
	cases := []struct {
		kind, expr string
		want       any
	}{
		{"json", `(parse json '{"a":1,"b":"hi"}') get 'b'`, "hi"},
		{"jsonic", `(parse jsonic '{a:1, b:"hi"}') get 'b'`, "hi"},
		{"json5", `(parse json5 '{a:1, b:0x10}') get 'b'`, int64(16)},
		{"jsonc", `(parse jsonc '{"a":1, "b":2}') get 'b'`, int64(2)},
		{"csv", `((parse csv 'a,b\n1,2') get 0) get 1`, "b"},
		{"toml", `(parse toml 'b = "hi"') get 'b'`, "hi"},
		{"yaml", `(parse yaml 'b: hi') get 'b'`, "hi"},
		{"xml", `(parse xml '<r><a>1</a></r>') get 'name'`, "r"},
		{"zon", `(parse zon '.{ .a = 1, .b = "hi" }') get 'b'`, "hi"},
		{"markdown", `((parse markdown '# T\n\nhello world') get 0) get 0`, "hello world"},
		{"feed", `(parse feed '<rss version="2.0"><channel><title>T</title></channel></rss>') get 'format'`, "atom"},
	}
	imp := `"aql:parselang" import end  `
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if got := runLast(t, a, imp+c.expr); got != c.want {
				t.Errorf("%s: got %v (%T), want %v", c.kind, got, got, c.want)
			}
		})
	}
}

// TestParseLangTabnasDesugar proves the sugar matches the desugared standard
// call for a representative kind from each shape family.
func TestParseLangTabnasDesugar(t *testing.T) {
	imp := `"aql:parselang" import end  `
	cases := []struct{ kind, sugar, desugared string }{
		{"json", `(parse json '{"a":1}') get 'a'`, `(ParseLang.parse_json '{"a":1}' {} end) get 'a'`},
		{"yaml", `(parse yaml 'b: hi') get 'b'`, `(ParseLang.parse_yaml 'b: hi' {} end) get 'b'`},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			sugar := runLast(t, a, imp+c.sugar)
			desugared := runLast(t, a, imp+c.desugared)
			if sugar != desugared {
				t.Fatalf("%s: sugar=%v desugared=%v must agree", c.kind, sugar, desugared)
			}
		})
	}
}

// TestParseLangTabnasKinds confirms every built-in kind is listed by
// ParseLang.kinds with no host registration.
func TestParseLangTabnasKinds(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := a.Run(`"aql:parselang" import end  ParseLang.kinds`)
	if err != nil {
		t.Fatalf("kinds: %v", err)
	}
	listed := fmt.Sprintf("%v", res[0])
	for _, kind := range []string{
		"ini", "json", "jsonic", "json5", "jsonc", "csv",
		"toml", "yaml", "xml", "zon", "markdown", "feed",
	} {
		if !strings.Contains(listed, kind) {
			t.Errorf("ParseLang.kinds %v should contain %q", listed, kind)
		}
	}
}

// TestParseLangTabnasSyntaxErrors pins the loud failure: a malformed source
// raises parse_syntax_error rather than returning a silent none.
func TestParseLangTabnasSyntaxErrors(t *testing.T) {
	imp := `"aql:parselang" import end  `
	cases := []struct{ name, src string }{
		{"json", imp + `parse json '{not valid'`},
		{"toml", imp + `parse toml '= = ='`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if _, err := a.Run(c.src); err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			} else if !strings.Contains(err.Error(), "parse_syntax_error") {
				t.Fatalf("%s: error %q should be parse_syntax_error", c.name, err.Error())
			}
		})
	}
}
