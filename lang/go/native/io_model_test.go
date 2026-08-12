package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Direct arm coverage for the io read/write check-mode models
// (readReturns / writeReturns in native_misc.go). The corpus drives the
// mainline shapes end to end (a made Pathon routing text / bytes /
// positioned reads); these tests pin the decline arms and the format
// table the rows cannot reach.

// modelPathon builds a concrete Pathon the way the runtime does —
// through the scalar make handler — so the model sees the same payload
// shape check mode surfaces.
func modelPathon(t *testing.T, r *Registry, path string) Value {
	t.Helper()
	out, err := core.MakeScalarHandler([]Value{NewTypeLiteral(TPathon), NewString(path)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("make Pathon %q = (%v, %v)", path, out, err)
	}
	return out[0]
}

// wantOneCarrier asserts the residual is exactly one non-dynamic
// carrier of typ.
func wantOneCarrier(t *testing.T, out []Value, typ *Type, label string) {
	t.Helper()
	if len(out) != 1 || out[0].Dynamic || !out[0].Parent.Equal(typ) {
		t.Fatalf("%s residual = %v, want one %s carrier", label, out, typ.Leaf())
	}
}

// wantDynAny asserts the model declined to the declared dynamic Any.
func wantDynAny(t *testing.T, out []Value, label string) {
	t.Helper()
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(TAny) {
		t.Fatalf("%s residual = %v, want one dynamic Any carrier", label, out)
	}
}

func TestReadReturnsRouting(t *testing.T) {
	r := mirrorReg(t)
	rf1 := readReturns(false)
	rf2 := readReturns(true)

	// 1-arg: extension-routed formats. txt maps to text (String);
	// an unmapped extension falls back to text; csv/tsv decode to a
	// table-shaped List; a payload-shaped format (json) declines.
	wantOneCarrier(t, rf1([]Value{modelPathon(t, r, "mem://a.txt")}, r), TString, "txt read")
	wantOneCarrier(t, rf1([]Value{modelPathon(t, r, "mem://a.bin")}, r), TString, "unmapped-ext read")
	wantOneCarrier(t, rf1([]Value{modelPathon(t, r, "mem://a.csv")}, r), TList, "csv read")
	wantDynAny(t, rf1([]Value{modelPathon(t, r, "mem://a.json")}, r), "json read")

	// 1-arg declines: nil registry, a Pathon carrier (computed target),
	// a concrete non-Pathon (reachable through union-combo expansion).
	wantDynAny(t, rf1([]Value{modelPathon(t, r, "mem://a.txt")}, nil), "nil-registry read")
	wantDynAny(t, rf1([]Value{NewCarrier(TPathon)}, r), "carrier-target read")
	wantDynAny(t, rf1([]Value{NewInteger(1)}, r), "non-Pathon read")
	wantDynAny(t, rf1(nil, r), "no-args read")

	// Opts: the binary bypass ({enc:'bytes'} or 'binary' reads Bytes),
	// the positioned slice ({offset}/{length} reads the decoded text),
	// an explicit fmt override, and the extension fallback.
	p := modelPathon(t, r, "mem://a.txt")
	wantOneCarrier(t, rf2([]Value{p, mapOfKV("enc", NewString("bytes"))}, r), TBytes, "enc bytes")
	wantOneCarrier(t, rf2([]Value{p, mapOfKV("enc", NewString("binary"))}, r), TBytes, "enc binary")
	wantOneCarrier(t, rf2([]Value{p, mapOfKV("offset", NewInteger(1))}, r), TString, "positioned offset")
	wantOneCarrier(t, rf2([]Value{p, mapOfKV("length", NewInteger(3))}, r), TString, "positioned length")
	wantOneCarrier(t, rf2([]Value{modelPathon(t, r, "mem://a.json"), mapOfKV("fmt", NewString("lines"))}, r), TList, "explicit fmt beats ext")
	wantOneCarrier(t, rf2([]Value{modelPathon(t, r, "mem://a.tsv"), NewMap(NewOrderedMap())}, r), TList, "tsv ext with empty opts")

	// Opts declines: a computed opts map (the runtime value decides the
	// format, e.g. a carrier {enc:…}), a short arg slice.
	wantDynAny(t, rf2([]Value{p, NewCarrier(TMap)}, r), "carrier opts")
	wantDynAny(t, rf2([]Value{p}, r), "missing opts")
}

func TestWriteReturnsIdentity(t *testing.T) {
	r := mirrorReg(t)
	rf := writeReturns()

	// A concrete Pathon target is handed back VERBATIM — the runtime's
	// returnPath contract — so the construction stays concrete through
	// a write-then-read chain.
	p := modelPathon(t, r, "mem://w.txt")
	out := rf([]Value{p, NewString("hello")}, r)
	if len(out) != 1 || !core.IsConcrete(out[0]) {
		t.Fatalf("concrete write residual = %v, want the Pathon back", out)
	}
	if !core.ValuesEqual(out[0], p) {
		t.Fatalf("write residual = %v, want the input Pathon %v", out[0], p)
	}

	// Declines keep the declared fresh carrier: a computed target, a
	// concrete non-Pathon, a short arg slice.
	wantOneCarrier(t, rf([]Value{NewCarrier(TPathon), NewString("x")}, r), TPathon, "carrier-target write")
	wantOneCarrier(t, rf([]Value{NewInteger(1), NewString("x")}, r), TPathon, "non-Pathon write")
	wantOneCarrier(t, rf(nil, r), TPathon, "no-args write")
}

// mapOfKV builds a one-entry concrete map value.
func mapOfKV(k string, v Value) Value {
	m := NewOrderedMap()
	m.Set(k, v)
	return NewMap(m)
}
