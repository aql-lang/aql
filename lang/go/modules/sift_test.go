package modules

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// siftRegistry returns a registry with the aql:sift, aql:parselang, and
// aql:string-util modules resolvable (via the full resolver) plus a parser, so
// source-string programs exercise sift end-to-end.
func siftRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)
	return r
}

func runSiftSrc(t *testing.T, r *native.Registry, src string) ([]native.Value, error) {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return native.NewTop(r).Run(values)
}

func assertSift(t *testing.T, src, want string) {
	t.Helper()
	r := siftRegistry(t)
	res, err := runSiftSrc(t, r, src)
	if err != nil {
		t.Fatalf("%q: unexpected error: %v", src, err)
	}
	if len(res) != 1 {
		t.Fatalf("%q: expected 1 result, got %d", src, len(res))
	}
	if got := res[0].String(); got != want {
		t.Errorf("%q = %s, want %s", src, got, want)
	}
}

func assertSiftErr(t *testing.T, src, wantCode string) {
	t.Helper()
	r := siftRegistry(t)
	_, err := runSiftSrc(t, r, src)
	if err == nil {
		t.Fatalf("%q: expected error %s, got none", src, wantCode)
	}
	ae, ok := err.(*native.AqlError)
	if !ok || ae.Code != wantCode {
		t.Errorf("%q: expected %s, got %v", src, wantCode, err)
	}
}

// --- Module structure ---

func TestSiftModuleExports(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildSiftModule(r)
	if err != nil {
		t.Fatal(err)
	}
	ns := desc.Exports["Sift"]
	if ns == nil {
		t.Fatal("no Sift namespace")
	}
	for _, w := range []string{"define", "parse", "kinds", "families", "spec", "detect", "check"} {
		if _, ok := ns.Get(w); !ok {
			t.Errorf("missing export %q", w)
		}
	}
	// Re-import is idempotent (families are not re-registered).
	if _, err := BuildSiftModule(r); err != nil {
		t.Fatalf("re-import: %v", err)
	}
}

// --- coerce: every type and every failure arm ---

func TestSiftCoerce(t *testing.T) {
	cases := []struct {
		vtype, cell, want string
		wantErr           bool
	}{
		{"", "x", "'x'", false},
		{"string", "x", "'x'", false},
		{"integer", "42", "42", false},
		{"integer", "x", "", true},
		{"integer", "", "none", false},
		{"float", "1.5", "1.5", false},
		{"float", "x", "", true},
		{"boolean", "yes", "true", false},
		{"boolean", "off", "false", false},
		{"boolean", "x", "", true},
		{"percent", "55%", "55.0", false},
		{"percent", "x%", "", true},
		{"size", "10", "10", false},
		{"size", "4 kB", "4096", false},
		{"size", "2MiB", "2097152", false},
		{"size", "1 zz", "", true},
		{"size", "xx", "", true},
		{"auto", "42", "42", false},
		{"auto", "1.5", "1.5", false},
		{"auto", "true", "true", false},
		{"auto", "false", "false", false},
		{"auto", "hi", "'hi'", false},
		{"nope", "1", "", true},
	}
	for _, c := range cases {
		got, err := coerce(c.vtype, c.cell)
		if c.wantErr {
			if err == nil {
				t.Errorf("coerce(%q,%q) expected error", c.vtype, c.cell)
			}
			continue
		}
		if err != nil {
			t.Errorf("coerce(%q,%q): %v", c.vtype, c.cell, err)
			continue
		}
		if got.String() != c.want {
			t.Errorf("coerce(%q,%q) = %s, want %s", c.vtype, c.cell, got.String(), c.want)
		}
	}
}

func TestSiftNormalizeName(t *testing.T) {
	cases := map[string]string{
		"MemTotal":        "mem-total",
		"HugePages_Total": "huge-pages-total",
		"%CPU":            "cpu",
		"1K-blocks":       "f-1k-blocks",
		"Mounted":         "mounted",
		"":                "",
		"---":             "",
	}
	for raw, want := range cases {
		if got := normalizeName(raw); got != want {
			t.Errorf("normalizeName(%q) = %q, want %q", raw, got, want)
		}
	}
	// Collision de-duplication via applyNames.
	seen := map[string]int{}
	if got := applyNames("A", "normalize", seen); got != "a" {
		t.Errorf("first = %q", got)
	}
	if got := applyNames("A", "normalize", seen); got != "a-2" {
		t.Errorf("collision = %q", got)
	}
	if got := applyNames("Keep", "keep", seen); got != "Keep" {
		t.Errorf("keep = %q", got)
	}
}

func TestSiftTypeLiteral(t *testing.T) {
	for _, c := range []struct{ t, want string }{
		{"integer", "Integer"}, {"size", "Integer"},
		{"float", "Float"}, {"percent", "Float"},
		{"boolean", "Boolean"}, {"string", "String"}, {"", "String"},
	} {
		if got := typeLiteralFor(c.t).String(); got != c.want {
			t.Errorf("typeLiteralFor(%q) = %s, want %s", c.t, got, c.want)
		}
	}
}

func TestSiftIsKnownType(t *testing.T) {
	for _, k := range []string{"string", "integer", "float", "boolean", "percent", "size", "auto"} {
		if !isKnownType(k) {
			t.Errorf("%q should be known", k)
		}
	}
	if isKnownType("nope") {
		t.Error("nope should be unknown")
	}
}

// --- the read Format path (Go-only; not reachable from parse) ---

func TestSiftReadFormat(t *testing.T) {
	r := siftRegistry(t)
	spec := &siftSpec{name: "kv", family: famKV, opts: native.NewNone()}
	f := &siftReadFormat{spec: spec, reg: r}
	// Decode (no opts).
	out, err := f.Decode("a: 1\nb: 2")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(out) != 1 || out[0].String() != "{a:'1' b:'2'}" {
		t.Errorf("Decode = %v", out)
	}
	// DecodeOpts with a String option and a nested type map.
	out, err = f.DecodeOpts("a: 1", map[string]any{"sep": ":", "vtype": "integer"})
	if err != nil {
		t.Fatalf("DecodeOpts: %v", err)
	}
	if out[0].String() != "{a:1}" {
		t.Errorf("DecodeOpts = %s", out[0].String())
	}
	// A malformed option surfaces as an error.
	if _, err := f.DecodeOpts("a: 1", map[string]any{"vtype": "nope"}); err == nil {
		t.Error("expected DecodeOpts error for bad vtype")
	}
	// Encode is read-only; ReadOnly is true.
	if _, err := f.Encode(native.NewNone()); err == nil {
		t.Error("expected read-only Encode error")
	}
	if !f.ReadOnly() {
		t.Error("ReadOnly should be true")
	}
}

func TestSiftAnyToValue(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "none"},
		{"s", "'s'"},
		{true, "true"},
		{int(3), "3"},
		{int64(4), "4"},
		{float64(2), "2"},
		{float64(2.5), "2.5"},
		{[]any{int64(1), "x"}, "[1 'x']"},
		{map[string]any{"k": int64(9)}, "{k:9}"},
		{struct{}{}, "'{}'"},
	}
	for _, c := range cases {
		if got := anyToValue(c.in).String(); got != c.want {
			t.Errorf("anyToValue(%#v) = %s, want %s", c.in, got, c.want)
		}
	}
	if anyMapToValue(nil).String() != "none" {
		t.Error("empty anyMapToValue should be None")
	}
}

// --- error arms reached through AQL source ---

func TestSiftErrorArms(t *testing.T) {
	// Malformed option types across families.
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'kv' opts:{sep:1}} "a:1"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'kv' opts:{trim:1}} "a:1"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'kv' opts:{skip:'x'}} "a:1"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'kv' opts:{types:{a:'nope'}}} "a:1"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'dsv' opts:{fields:5}} "a"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fixed' opts:{cols:5}} "a"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fixed' opts:{cols:[5]}} "a"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fixed' opts:{widths:['x']}} "a"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fixed' opts:{widths:[3] fields:['a' 'b']}} "abc"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'pattern' opts:{re:'x'}} "x"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'pattern'} "x"`, "sift_spec_error")
	// detect: bad shapes.
	assertSiftErr(t, `import "aql:sift" Sift.define d {family:'kv' detect:{path:5}} end`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.define d {family:'kv' detect:{cmd:{match:5}}} end`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.define d {family:'kv' detect:5} end`, "sift_spec_error")
	// spec: not a map / not a function.
	assertSiftErr(t, `import "aql:sift" Sift.parse 5 "x"`, "sift_spec_error")
	// fields sugar conflict.
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'dsv' fields:['a'] opts:{fields:['b']}} "x"`, "sift_spec_error")
}

func TestSiftMisc(t *testing.T) {
	// fields sugar (top-level) folds onto opts and drives dsv naming.
	assertSift(t, `import "aql:sift" ((Sift.parse {family:'dsv' fields:['u' 'p'] opts:{sep:':'}} "a:b") get 0) get 'u'`, "'a'")
	// A fields map with a typed value.
	assertSift(t, `import "aql:sift" ((Sift.parse {family:'dsv' fields:{n:'integer'} opts:{sep:':'}} "5") get 0) get 'n'`, "5")
	// unquote on a single-quoted value.
	assertSift(t, `import "aql:sift" (Sift.parse {family:'kv' opts:{sep:'=' unquote:true}} "a='v'") dot a`, "'v'")
	// blocks table mode field union with a missing field → None.
	assertSift(t, `import "aql:sift" ((Sift.parse {family:'blocks' opts:{table:true}} "a: 1\nb: 2\n\na: 3") get 1) get 'b'`, "none")
	// detect argv with a slash-prefixed argv0 (basename match).
	assertSift(t, `import "aql:sift" Sift.define q {family:'columns' detect:{cmd:{match:["ps"]}}} end  Sift.detect ["/bin/ps" "aux"]`, "q")
	// detect a path glob (trailing *).
	assertSift(t, `import "aql:sift" Sift.define g {family:'kv' detect:{path:["/sys/*"]}} end  Sift.detect "/sys/x"`, "g")
}
