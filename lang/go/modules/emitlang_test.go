package modules

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// runEmit runs a full AQL program against a production-shaped registry (the
// module resolver wired, the parser installed) and returns the canonical
// render of the result, or the error.
func runEmit(t *testing.T, src string) (string, error) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	values, perr := parser.Parse(src)
	if perr != nil {
		return "", perr
	}
	out, rerr := native.New(reg).Run(values)
	if rerr != nil {
		return "", rerr
	}
	return native.Canon(out), nil
}

const emitImp = `import "aql:emitlang"  `

// TestEmitBuiltins pins the canonical walk-based emitters end-to-end through
// the `emit` word: auto json, explicit kinds, opts, and the desugared form.
func TestEmitBuiltins(t *testing.T) {
	cases := []struct{ src, want string }{
		// auto (natural format)
		{emitImp + `emit {a:1 b:[2 3]}`, `'{"a":1,"b":[2,3]}'`},
		{emitImp + `emit [1 2 3]`, `'[1,2,3]'`},
		{emitImp + `emit "hi"`, `'"hi"'`},
		// explicit json + opts
		{emitImp + `emit json {a:1}`, `'{"a":1}'`},
		{emitImp + `emit json {pretty:true} {a:1}`, "'{\\n  \"a\": 1\\n}'"},
		// jsonic bare keys
		{emitImp + `emit jsonic {a:1}`, `'{a:1}'`},
		// yaml block
		{emitImp + `emit yaml {a:1}`, `'a: 1'`},
		// csv list of records
		{emitImp + `emit csv [{a:1 b:2}]`, `'a,b\n1,2\n'`},
		// toml / ini map shapes
		{emitImp + `emit toml {server:{host:"x"}}`, `'[server]\nhost = "x"'`},
		{emitImp + `emit ini {db:{user:"a"}}`, `'[db]\nuser=a'`},
		// xml round-trip
		{emitImp + `emit xml <r><c>1</c></r>`, `'<r><c>1</c></r>'`},
		// desugared standard call is equivalent
		{emitImp + `EmitLang.emit_json {a:1} {} end`, `'{"a":1}'`},
		// bare variable evaluates as data, not a kind
		{emitImp + `def m {a:1}  emit m`, `'{"a":1}'`},
		// an opts map held in a variable is opts, not a kind attempt
		{emitImp + `def o {pretty:true} end  emit o {a:1}`, "'{\\n  \"a\": 1\\n}'"},
		// a negative indent is clamped to 0 rather than panicking
		{emitImp + `emit json {indent:-1} {a:1}`, "'{\\n\"a\": 1\\n}'"},
		// a non-bare TOML key is quoted so it round-trips
		{emitImp + `emit toml {"a b":1}`, `'"a b" = 1'`},
	}
	for _, c := range cases {
		got, err := runEmit(t, c.src)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.src, got, c.want)
		}
	}
}

// TestEmitNegatives pins the error contract: unknown kinds, unsupported
// shapes, and register validation — the negative half of the test discipline.
func TestEmitNegatives(t *testing.T) {
	cases := []struct{ src, wantCode string }{
		{emitImp + `emit nope {a:1}`, "emit_unknown_lang"},
		{`emit json {a:1}`, "emit_unknown_lang"}, // no import → kind unknown
		{emitImp + `emit toml [1 2]`, "emit_unsupported"},
		{emitImp + `emit xml {a:1}`, "emit_unsupported"},
		{emitImp + `emit ini {a:{b:{c:1}}}`, "emit_unsupported"},
		// csv/tsv require a tabular shape — a bare Map or scalar is rejected
		{emitImp + `emit csv {a:1}`, "emit_unsupported"},
		{emitImp + `emit csv 1`, "emit_unsupported"},
		// toml cannot represent None (no null) — it errors, never corrupts
		{emitImp + `emit toml {a:None}`, "emit_unsupported"},
		{emitImp + `EmitLang.register Bad (fn [[value:Any opts:Map] [String] ["x"]])`, "emit_bad_name"},
		{emitImp + `EmitLang.register bad (fn [[a:Integer] [Integer] [a]])`, "emit_bad_signature"},
		// the value parameter must be Any (an emitter must accept every value)
		{emitImp + `EmitLang.register narrow (fn [[value:Integer opts:Map] [String] ["x"]])`, "emit_bad_signature"},
		// the return type must be String (emit is value→string)
		{emitImp + `EmitLang.register numret (fn [[value:Any opts:Map] [Integer] [1]])`, "emit_bad_signature"},
		{emitImp + `EmitLang.register emit_x (fn [[value:Any opts:Map] [String] ["x"]])`, "emit_bad_name"},
		{emitImp + `EmitLang.register json (fn [[value:Any opts:Map] [String] ["x"]])`, "emit_kind_exists"},
	}
	for _, c := range cases {
		_, err := runEmit(t, c.src)
		if err == nil {
			t.Errorf("%s: expected error %q, got none", c.src, c.wantCode)
			continue
		}
		if !strings.Contains(err.Error(), c.wantCode) {
			t.Errorf("%s: error %q does not contain %q", c.src, err.Error(), c.wantCode)
		}
	}
}

// TestEmitRegisterRoundTrip pins a custom emitter installed via
// EmitLang.register and used through `emit <name>`.
func TestEmitRegisterRoundTrip(t *testing.T) {
	src := emitImp + `EmitLang.register up (fn [[value:Any opts:Map] [String] ["UP"]]) end  emit up {a:1}`
	got, err := runEmit(t, src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `'UP'` {
		t.Fatalf("got %q, want 'UP'", got)
	}
}

// TestEmitKindsOrder pins EmitLang.kinds to the fixed EmitKinds() order.
func TestEmitKindsOrder(t *testing.T) {
	got, err := runEmit(t, emitImp+`EmitLang.kinds`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "[json/q jsonic/q yaml/q csv/q tsv/q toml/q ini/q xml/q]"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestEmitKindEncodeNoPanic feeds type-literals and non-concrete values to
// every EmitKind.Encode and asserts it returns (a string or an error) rather
// than panicking — the Panic Prevention discipline applied to the canonical
// emitter core.
func TestEmitKindEncodeNoPanic(t *testing.T) {
	bad := []native.Value{
		native.NewTypeLiteral(native.TMap),
		native.NewTypeLiteral(native.TList),
		native.NewTypeLiteral(native.TString),
		native.NewTypeLiteral(native.TAny),
		native.NewTypeLiteral(native.TXml),
		native.NewTypedMap(native.NewTypeLiteral(native.TString)),
		native.NewDynamicCarrier(native.TMap),
		native.NewDynamicCarrier(native.TList),
	}
	for _, k := range native.EmitKinds() {
		for i, v := range bad {
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Fatalf("emit %s case %d: Encode panicked: %v", k.Name, i, rec)
					}
				}()
				// Both nil opts and an empty opts map must be panic-safe.
				_, _ = k.Encode(v, nil)
				_, _ = k.Encode(v, map[string]any{})
			}()
		}
	}
}

// TestNaturalEmitKindNoPanic asserts NaturalEmitKind never panics on
// type-literal / non-concrete inputs and resolves the documented natural
// targets for concrete values.
func TestNaturalEmitKindNoPanic(t *testing.T) {
	for _, v := range []native.Value{
		native.NewTypeLiteral(native.TMap),
		native.NewTypeLiteral(native.TAny),
		native.NewDynamicCarrier(native.TList),
	} {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("NaturalEmitKind panicked: %v", rec)
				}
			}()
			native.NaturalEmitKind(v)
		}()
	}
	// Concrete naturals.
	if name, ok := native.NaturalEmitKind(native.NewMap(native.NewOrderedMap())); !ok || name != "json" {
		t.Errorf("Map natural = %q,%v; want json,true", name, ok)
	}
	if name, ok := native.NaturalEmitKind(native.NewList(nil)); !ok || name != "json" {
		t.Errorf("List natural = %q,%v; want json,true", name, ok)
	}
	// A table from read/CSVFormat.Decode is a TList carrying a TableData
	// payload (its Parent does NOT conform to TTable) — its natural format must
	// still resolve to csv, not the generic-list json.
	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	row := native.NewOrderedMap()
	row.Set("name", native.NewString("alice"))
	table := native.NewValueRaw(native.TList, native.TableData{
		Record: native.RecordTypeInfo{Fields: fields},
		Rows:   []native.Value{native.NewMap(row)},
	})
	if name, ok := native.NaturalEmitKind(table); !ok || name != "csv" {
		t.Errorf("TableData natural = %q,%v; want csv,true", name, ok)
	}
}
