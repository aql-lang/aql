package test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// The ini PARSER ships built-in with aql:parselang (backed by
// github.com/tabnas/ini/go). Unlike the calc host parser, no registration is
// needed — importing the module is enough for `parse ini <text>` to resolve.

const iniImp = `import "aql:parselang"  `

// TestParseLangIniBuiltin pins the decode: top-level entries, a {src:…}
// source, and a nested [section] whose recognised boolean decodes to a
// Boolean (not the string "true").
func TestParseLangIniBuiltin(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if got := runLast(t, a, iniImp+`(parse ini 'b = hello') get 'b'`); got != "hello" {
		t.Errorf("top-level value: got %v, want hello", got)
	}
	if got := runLast(t, a, iniImp+`(parse ini {src:'b = hello'}) get 'b'`); got != "hello" {
		t.Errorf("{src} form: got %v, want hello", got)
	}
	// The value is a real Boolean at the engine level (the module-parselang
	// spec pins `true`, and `typeof` reports Boolean); lang.Run renders it to
	// the Go string "true".
	if got := runLast(t, a, iniImp+`((parse ini 'a = 1\n[sec]\nx = true') get 'sec') get 'x'`); got != "true" {
		t.Errorf("section bool: got %v (%T), want true", got, got)
	}
	if got := runLast(t, a, iniImp+`typeof (((parse ini 'a = 1\n[sec]\nx = true') get 'sec') get 'x')`); got != "Boolean" {
		t.Errorf("section bool type: got %v, want Boolean", got)
	}
}

// TestParseLangIniDesugar proves `parse ini` is sugar for the standard
// ParseLang.parse_ini call.
func TestParseLangIniDesugar(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	sugar := runLast(t, a, iniImp+`(parse ini 'b = hi') get 'b'`)
	desugared := runLast(t, a, iniImp+`(ParseLang.parse_ini 'b = hi' {} end) get 'b'`)
	if sugar != desugared || sugar != "hi" {
		t.Fatalf("sugar=%v desugared=%v: parse must desugar to the standard call", sugar, desugared)
	}
}

// TestParseLangIniInKinds confirms ini is listed by ParseLang.kinds with no
// host registration — it is built in.
func TestParseLangIniInKinds(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := a.Run(iniImp + `ParseLang.kinds`)
	if err != nil {
		t.Fatalf("kinds: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %v", res)
	}
	if kinds := fmt.Sprintf("%v", res[0]); !strings.Contains(kinds, "ini") {
		t.Errorf("kinds %v should contain ini", kinds)
	}
}

// TestParseLangIniSourceErrors pins the framework's loud source failures for
// the built-in parser: a {file:…} source is deferred and a source map with no
// 'src' field is rejected.
func TestParseLangIniSourceErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"file deferred", iniImp + `parse ini {file:'cfg.ini'}`, "parse_file_unsupported"},
		{"bad source map", iniImp + `parse ini {nope:1}`, "parse_bad_source"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if _, err := a.Run(c.src); err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			} else if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}

// TestParseLangIniNotReRegisterable confirms ini occupies its kind slot: a
// host parser cannot claim the name once the module is built.
func TestParseLangIniNotReRegisterable(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if _, err := a.Run(iniImp); err != nil {
		t.Fatalf("import: %v", err)
	}
	spec := lang.ParseLangSpec{
		Name:    "ini",
		Returns: []*lang.Type{lang.TMap},
		Handler: calcParserSpec().Handler,
	}
	if err := a.RegisterParser(spec); err == nil {
		t.Fatal("expected error re-registering the built-in ini parser")
	} else if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("error %q should say already registered", err.Error())
	}
}
