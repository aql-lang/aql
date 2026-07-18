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

// TestSiftOptionTypeErrors fires the wrong-typed-option error arm of every
// family option reader (str/boolean/integer/strict/typesMap/resolveFields).
func TestSiftOptionTypeErrors(t *testing.T) {
	bad := []string{
		// kv
		`{family:'kv' opts:{sep:1}}`,
		`{family:'kv' opts:{trim:1}}`,
		`{family:'kv' opts:{unquote:1}}`,
		`{family:'kv' opts:{names:1}}`,
		`{family:'kv' opts:{repeat:1}}`,
		`{family:'kv' opts:{vtype:1}}`,
		`{family:'kv' opts:{strict:1}}`,
		`{family:'kv' opts:{skip:'x'}}`,
		`{family:'kv' opts:{chop:'x'}}`,
		`{family:'kv' opts:{comment:1}}`,
		// blocks
		`{family:'blocks' opts:{sep:1}}`,
		`{family:'blocks' opts:{kvsep:1}}`,
		`{family:'blocks' opts:{table:1}}`,
		// columns
		`{family:'columns' opts:{header:1}}`,
		`{family:'columns' opts:{names:1}}`,
		`{family:'columns' opts:{label:1}}`,
		// dsv
		`{family:'dsv' opts:{sep:1}}`,
		`{family:'dsv' opts:{limit:'x'}}`,
		// fixed
		`{family:'fixed' opts:{trim:1 widths:[1]}}`,
		`{family:'fixed' opts:{cols:[[0 'x']]}}`,
		`{family:'fixed' opts:{cols:[[0]]}}`,
		// pattern
		`{family:'pattern' opts:{re:1}}`,
		`{family:'pattern' opts:{re:'(?P<a>x)' per:1}}`,
		`{family:'pattern' opts:{re:'(?P<a>x)' names:1}}`,
		`{family:'pattern' opts:{re:'(?P<a>x)' vtype:'nope'}}`,
		`{family:'pattern' opts:{re:'(?P<a>x)' strict:1}}`,
		// typesMap / resolveFields
		`{family:'dsv' opts:{types:{c1:1}}}`,
		`{family:'columns' opts:{fields:5}}`,
		`{family:'columns' opts:{fields:{a:'nope'}}}`,
		`{family:'columns' opts:{fields:{a:{type:'nope'}}}}`,
	}
	for _, spec := range bad {
		assertSiftErr(t, `import "aql:sift" Sift.parse `+spec+` "a:1"`, "sift_spec_error")
	}
}

// TestSiftPureHelpers covers the small pure helpers directly.
func TestSiftPureHelpers(t *testing.T) {
	if siftUnquote("x") != "x" || siftUnquote("'a'") != "a" || siftUnquote(`"a"`) != "a" || siftUnquote("'") != "'" {
		t.Error("siftUnquote")
	}
	if got := siftSplitN("a b c", 0); len(got) != 3 {
		t.Errorf("siftSplitN(0) = %v", got)
	}
	if got := siftSplitN("a b c", 2); len(got) != 2 || got[1] != "b c" {
		t.Errorf("siftSplitN(2) = %v", got)
	}
	if fixedSlice("abc", 5, 6) != "" || fixedSlice("abc", 2, 1) != "" || fixedSlice("abc", 0, 2) != "ab" {
		t.Error("fixedSlice bounds")
	}
	if !matchGlob("/a/*", "/a/b") || matchGlob("/a/*", "/x") || !matchGlob("/a", "/a") || matchGlob("/a", "/b") {
		t.Error("matchGlob")
	}
	if lastSlash("abc") != -1 || lastSlash("a/b") != 1 {
		t.Error("lastSlash")
	}
	// specToValue with neither opts nor doc.
	sv := specToValue(&siftSpec{name: "x", family: famKV, opts: native.NewNone()})
	if sv.String() != "{name:x family:kv}" {
		t.Errorf("specToValue bare = %s", sv.String())
	}
}

// TestSiftCallFnErrors covers callSiftFn's non-single-result and error arms and
// the str atom branch.
func TestSiftCallFnErrors(t *testing.T) {
	// A fn returning no value → callSiftFn's single-result guard fires.
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fn' fn:(fn [[s:String o:Map] [] []])} "x"`, "sift_parse_error")
	// A fn that raises → the raise propagates.
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fn' fn:(fn [[s:String o:Map] [Any] [raise boom "x"]])} "x"`, "boom")
	// str atom branch: an option value that is an Atom.
	assertSift(t, `import "aql:sift" (Sift.parse {family:'kv' opts:{sep:':' names:normalize/q}} "MemTotal: 4") dot mem-total`, "'4'")
}

// TestSiftDefineCheckMode drives siftDefineReturns (the check-mode install) and
// its skip/error arms directly.
func TestSiftDefineCheckMode(t *testing.T) {
	r := siftRegistry(t)
	st := siftHostStateFor(r)
	restore := r.Check.Begin()
	defer restore()
	spec := native.NewOrderedMap()
	spec.Set("family", native.NewAtom("kv"))
	name := native.NewAtom("ckmode")
	// A concrete name + concrete spec installs during check.
	if got := siftDefineReturns(st)([]native.Value{name, native.NewMap(spec)}, r); got != nil {
		t.Errorf("ReturnsFn should return nil, got %v", got)
	}
	if _, ok := st.specs["ckmode"]; !ok {
		t.Error("check-mode install did not register the kind")
	}
	// A non-concrete arg short-circuits (returns nil, installs nothing).
	if got := siftDefineReturns(st)([]native.Value{native.NewCarrier(native.TAtom)}, r); got != nil {
		t.Error("short arg should return nil")
	}
	// An install error is surfaced as a check diagnostic (name collides).
	kv := native.NewAtom("kv")
	_ = siftDefineReturns(st)([]native.Value{kv, native.NewMap(spec)}, r)
}

// TestSiftFamilyBranches hits the per-row branches of columns/dsv/fixed/pattern
// (blank lines, coercion failures, empty body, greedy limit, bad widths).
func TestSiftFamilyBranches(t *testing.T) {
	// columns: header-only input → empty table (len(body)==0).
	assertSift(t, `import "aql:sift" size (Sift.parse {family:'columns'} "H1 H2")`, "0")
	// columns: a blank line in the body is skipped.
	assertSift(t, `import "aql:sift" size (Sift.parse {family:'columns'} "H\n\n1\n2")`, "2")
	// columns: a coercion failure in a typed column raises.
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'columns' opts:{types:{h:'integer'}}} "h\nx"`, "sift_type_error")
	// columns: label mode with a typed remaining column.
	assertSift(t, `import "aql:sift" ((Sift.parse {family:'columns' opts:{label:'k' types:{u:'integer'}}} "u\nMem: 9") get 0) get 'u'`, "9")
	// dsv: blank line skipped; literal-sep greedy limit; coercion failure.
	assertSift(t, `import "aql:sift" size (Sift.parse {family:'dsv' opts:{sep:':'}} "a\n\nb")`, "2")
	assertSift(t, `import "aql:sift" ((Sift.parse {family:'dsv' opts:{sep:':' limit:2}} "a:b:c") get 0) get 'c2'`, "'b:c'")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'dsv' opts:{sep:':' types:{c1:'integer'}}} "x"`, "sift_type_error")
	// fixed: blank line skipped; coercion failure; widths not a list.
	assertSift(t, `import "aql:sift" size (Sift.parse {family:'fixed' opts:{widths:[1]}} "a\n\nb")`, "2")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fixed' opts:{widths:[1] types:{c1:'integer'}}} "x"`, "sift_type_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fixed' opts:{widths:5}} "x"`, "sift_spec_error")
	// pattern: per:'line' coercion failure; a blank/non-matching line under strict:'error'.
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'pattern' opts:{re:'(?P<n>\\S+)' per:'line' types:{n:'integer'}}} "x"`, "sift_type_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'pattern' opts:{re:'(?P<n>\\d+)' per:'line'}} "a\n5"`, "sift_parse_error")
	// resolveFields: the extended {type re} field form.
	assertSift(t, `import "aql:sift" ((Sift.parse {family:'dsv' opts:{sep:':' fields:{n:{type:'integer'}}}} "5") get 0) get 'n'`, "5")
	// readDetect: an argv whose basename is not in the table → None.
	assertSift(t, `import "aql:sift" Sift.detect ["nosuch" "x"]`, "none")
	// installKind: a fn kind's parse via the parselang macro (RegisterHostParser path).
	assertSift(t, `import "aql:sift" import "aql:parselang" Sift.define byname {family:'kv' opts:{sep:'='}} end  parse byname "a=1"`, "{a:'1'}")
}

// TestSiftCoverageArms sweeps the remaining reachable error/edge arms.
func TestSiftCoverageArms(t *testing.T) {
	// preprocess: trailing newline drop, skip>=len, chop>=len, interior blank.
	assertSift(t, `import "aql:sift" size (Sift.parse {family:'dsv' opts:{sep:':'}} "a\nb\n")`, "2")
	assertSift(t, `import "aql:sift" size (Sift.parse {family:'dsv' opts:{sep:':' skip:9}} "a\nb")`, "0")
	assertSift(t, `import "aql:sift" size (Sift.parse {family:'dsv' opts:{sep:':' chop:9}} "a\nb")`, "0")
	assertSift(t, `import "aql:sift" size (Sift.parse {family:'kv' opts:{sep:':'}} "a:1\n\nb:2")`, "2")
	// blocks: sep read error.
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'blocks' opts:{sep:1}} "a:1"`, "sift_spec_error")
	// columns: names/label read errors, empty body, interior blank, label coercion failure.
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'columns' opts:{names:1}} "a"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'columns' opts:{label:1}} "a"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'columns' opts:{label:'k' types:{u:'integer'}}} "u\nMem: x"`, "sift_type_error")
	// fixed: trim read error, other option errors.
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fixed' opts:{trim:1 widths:[1]}} "a"`, "sift_spec_error")
	// pattern: names/vtype/per read errors already covered; per:'line' coercion + strict:'skip' skip arms.
	assertSift(t, `import "aql:sift" size (Sift.parse {family:'pattern' opts:{re:'(?P<n>\\d+)' per:'line' types:{n:'integer'} strict:'skip'}} "5\nx")`, "1")
	assertSift(t, `import "aql:sift" (Sift.parse {family:'pattern' opts:{re:'(?P<n>\\S+)' types:{n:'integer'} strict:'skip'}} "x")`, "none")
	// specFromValue: missing family, opts-not-map, fn-key-missing, bad-fn, doc, detect cmd shapes, argv.
	assertSiftErr(t, `import "aql:sift" Sift.parse {opts:{sep:':'}} "a:1"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'kv' opts:5} "a:1"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fn'} "x"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fn' fn:5} "x"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'fn' fn:(fn [[s:String o:Integer] [Any] [s]])} "x"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.parse (fn [[bad:Integer] [Any] [bad]]) "x"`, "sift_spec_error")
	assertSiftErr(t, `import "aql:sift" Sift.define d {family:'kv' detect:{cmd:5}} end`, "sift_spec_error")
	assertSift(t, `import "aql:sift" Sift.define dc {family:'kv' detect:{cmd:{match:["z"] argv:["z" "-a"]}} doc:"d"} end  (Sift.spec dc) dot family`, "kv")
	// fields sugar onto empty opts (cloneOrdered nil path).
	assertSift(t, `import "aql:sift" ((Sift.parse {family:'dsv' fields:['a']} "x") get 0) get 'a'`, "'x'")
	// builtinParserKind collision on define.
	assertSiftErr(t, `import "aql:sift" Sift.define json {family:'kv'}`, "sift_kind_exists")
	// checkDetectConflicts: two kinds claiming one command basename.
	assertSiftErr(t, `import "aql:sift" Sift.define e1 {family:'kv' detect:{cmd:{match:["dup"]}}} end  Sift.define e2 {family:'kv' detect:{cmd:{match:["dup"]}}}`, "sift_detect_conflict")
	// siftDefineInstall: a bad spec surfaces its spec error.
	assertSiftErr(t, `import "aql:sift" Sift.define ok {family:'nope'} end`, "sift_spec_error")
	// siftParseHandler: a bad source ({file:…}).
	assertSiftErr(t, `import "aql:sift" Sift.parse {family:'kv'} {file:"x"}`, "parse_file_unsupported")
	// siftResolveSpec: a fn value passed directly to Sift.parse.
	assertSift(t, `import "aql:sift" Sift.parse (fn [[s:String o:Map] [String] [s]]) "hi"`, "'hi'")
	// specToValue: a kind WITH opts and doc round-trips through Sift.spec.
	assertSift(t, `import "aql:sift" Sift.define sd {family:'kv' opts:{sep:'='} doc:"the doc"} end  (Sift.spec sd) dot doc`, "'the doc'")
	assertSift(t, `import "aql:sift" Sift.define so {family:'kv' opts:{sep:'='}} end  ((Sift.spec so) dot opts) dot sep`, "'='")
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
