package eng_test

// Standalone-suite tail probes, external half (design/
// ENG-COVERAGE-PARITY.0.md stage 4): the fn-spec parameter matrix
// (ParseFnParams / ResolveSigType) and the predicate-type runner.
// Rows run through the specfix registry; the pattern shapes the
// surface syntax can't spell (pre-evaluated literal elements) call
// ParseFnDef with constructed value lists — the same shapes lang's
// richer spec assembly hands it.

import (
	"strings"
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
)

func TestFnSpecParamMatrix(t *testing.T) {
	rows := []struct {
		input   string
		want    string
		wantErr string
	}{
		// Optional param via a None-bearing disjunct: both arities.
		{input: "def f fn [[x:(Integer tor None)] [Integer] [1]] f 5", want: "1"},
		{input: "def f fn [[x:(Integer tor None)] [Integer] [1]] f", want: "1"},

		// Named literal patterns; the annotation value fixes the kind.
		{input: "def f fn [[x:5] [Integer] [3]] f 5", want: "3"},
		{input: "def f fn [[x:2.5] [Integer] [4]] f 2.5", want: "4"},
		{input: "def bt true def f fn [[x:bt] [Integer] [1]] f true",
			wantErr: "must start with an uppercase letter"},

		// Structural annotations: map pattern, typed-list child, empty list.
		{input: "def f fn [[x:{a:Integer}] [Integer] [5]] f {a:9}", want: "5"},
		{input: "def f fn [[x:[:Integer]] [Integer] [6]] f [1 2]", want: "6"},
		{input: "def f fn [[x:[]] [Integer] [7]] f []", want: "7"},

		// Named types: a def-installed alias is nominal — the raw base
		// value does not dispatch into it.
		{input: "def Big Integer def f fn [[x:Big] [Integer] [8]] f 3",
			wantErr: "no signature matches"},
		{input: "def Rec (refine Record [{a:Integer}]) def f fn [[x:Rec] [Integer] [9]] f {a:1}",
			want: "9"},
		{input: "def f fn [[x:P] [Integer] [10]] f p", want: "10"},

		// Builtin names in the returns slot, including the None mismatch.
		{input: "def f fn [[x:Integer] [None] [11]] f 1",
			wantErr: "expected None, got Integer"},
		{input: "def f fn [[x:Integer] [Boolean] [true]] f 1", want: "true"},
		{input: "def f fn [[x:Integer] [Map] [{a:1}]] f 1", want: "{a:1}"},
		{input: "def f fn [[x:Integer] [List] [[1]]] f 1", want: "[1]"},
		{input: "def f fn [[x:Integer] [Function] [(fn [[y:Integer] [Integer] [1]])]] f 1",
			want: "fn [[y:Integer][Integer][1]]"},

		// A dotted annotation with no bound module export is an error.
		{input: "def f fn [[x:a.b] [Integer] [1]]",
			wantErr: "dotted annotation must reach a type literal"},
	}
	for _, row := range rows {
		t.Run(row.input, func(t *testing.T) {
			values, err := parser.Parse(row.input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			out, runErr := eng.NewTop(specfixProbeRegistry(t)).Run(values)
			if row.wantErr != "" {
				if runErr == nil || !strings.Contains(runErr.Error(), row.wantErr) {
					t.Errorf("want error containing %q, got %v (out %s)", row.wantErr, runErr, eng.Canon(out))
				}
				return
			}
			if runErr != nil {
				t.Fatalf("unexpected error: %v", runErr)
			}
			if got := eng.Canon(out); got != row.want {
				t.Errorf("got %s, want %s", got, row.want)
			}
		})
	}
}

// TestParseFnDefConstructedSpecs drives the parameter arms only a
// pre-evaluated spec list reaches: unnamed positional literal
// patterns of every scalar kind.
func TestParseFnDefConstructedSpecs(t *testing.T) {
	r, err := standaloneRegistry()
	if err != nil {
		t.Fatal(err)
	}
	body := eng.NewList([]Value{eng.NewInteger(1)})
	rets := eng.NewList([]Value{eng.NewTypeLiteral(eng.TInteger)})
	mk := func(params ...Value) []Value {
		return []Value{eng.NewList(params), rets, body}
	}
	for _, c := range []struct {
		name  string
		param Value
	}{
		{"integer", eng.NewInteger(7)},
		{"boolean", eng.NewBoolean(true)},
		{"string", eng.NewString("pat")},
		{"bare-type-node", eng.NewTypeLiteral(eng.TInteger)},
	} {
		if _, perr := eng.ParseFnDef(r, mk(c.param)); perr != nil {
			t.Errorf("%s positional pattern: %v", c.name, perr)
		}
	}
	// Named pairs with pre-evaluated annotation values (the shapes the
	// surface syntax resolves as type names instead).
	bt := eng.NewOrderedMap()
	bt.Set("x", eng.NewBoolean(true))
	if _, perr := eng.ParseFnDef(r, mk(eng.NewMap(bt))); perr != nil {
		t.Errorf("named boolean pattern: %v", perr)
	}
	st := eng.NewOrderedMap()
	st.Set("x", eng.NewAtom("tag"))
	if _, perr := eng.ParseFnDef(r, mk(eng.NewMap(st))); perr != nil {
		t.Errorf("named atom pattern: %v", perr)
	}
	// An unresolvable element shape is the invalid-parameter arm.
	if _, perr := eng.ParseFnDef(r, mk(eng.NewError(errProbe{}))); perr == nil {
		t.Error("error-value param must be rejected")
	}
}

type Value = eng.Value

type errProbe struct{}

func (errProbe) Error() string { return "probe" }

func TestRunPredicateArms(t *testing.T) {
	r := specfixProbeRegistry(t)
	build := func(src string) Value {
		t.Helper()
		vals, err := parser.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		out, err := eng.NewTop(r).Run(vals)
		if err != nil || len(out) != 1 {
			t.Fatalf("build %q: %v %v", src, out, err)
		}
		return out[0]
	}
	pred := build("fn [[x:Integer] [Boolean] [x is Integer]]")

	if _, ok, err := r.RunPredicate(pred, eng.NewInteger(5)); !ok || err != nil {
		t.Errorf("integer candidate must match: ok=%v err=%v", ok, err)
	}
	// Candidate outside the declared input type: no match, no run.
	if _, ok, err := r.RunPredicate(pred, eng.NewString("s")); ok || err != nil {
		t.Errorf("string candidate must not match: ok=%v err=%v", ok, err)
	}
	// Non-fn constraint.
	if _, _, err := r.RunPredicate(eng.NewInteger(1), eng.NewInteger(5)); err == nil {
		t.Error("non-fn constraint must error")
	}
	// A two-param fn is not a predicate shape.
	two := build("fn [[a:Integer b:Integer] [Boolean] [true]]")
	if _, _, err := r.RunPredicate(two, eng.NewInteger(5)); err == nil {
		t.Error("two-param constraint must error")
	}
	// A predicate returning false rejects the candidate.
	no := build("fn [[x:Integer] [Boolean] [false]]")
	if _, ok, err := r.RunPredicate(no, eng.NewInteger(5)); ok || err != nil {
		t.Errorf("false predicate must reject: ok=%v err=%v", ok, err)
	}
	// A non-Boolean result is a VALUE-mode predicate: the result is
	// the transformed binding.
	xform := build("fn [[x:Integer] [Integer] [x addq 1]]")
	out, ok, err := r.RunPredicate(xform, eng.NewInteger(5))
	if err != nil || !ok {
		t.Fatalf("value-mode predicate: ok=%v err=%v", ok, err)
	}
	if got := eng.Canon([]Value{out}); got != "6" {
		t.Errorf("value-mode predicate result = %s, want 6", got)
	}
	// A predicate producing None rejects.
	noneOut := build("fn [[x:Integer] [Any] [{a:1} get b]]")
	if _, ok, err := r.RunPredicate(noneOut, eng.NewInteger(5)); ok || err != nil {
		t.Errorf("None-producing predicate must reject: ok=%v err=%v", ok, err)
	}
	// A raising predicate surfaces the error.
	boom := build("fn [[x:Integer] [Any] [refine 5]]")
	if _, _, err := r.RunPredicate(boom, eng.NewInteger(5)); err == nil {
		t.Error("raising predicate must surface its error")
	}
	// Check-mode short-circuit: matched without running the body.
	done := r.Check.Begin()
	if _, ok, err := r.RunPredicate(no, eng.NewInteger(5)); !ok || err != nil {
		t.Errorf("check-mode must short-circuit to match: ok=%v err=%v", ok, err)
	}
	done()
}

// TestCallBoruArms drives the direct fn-call API: list-argument
// handling, body def-leak recovery, and unnamed-arg result trimming.
func TestCallBoruArms(t *testing.T) {
	r := specfixProbeRegistry(t)
	build := func(src string) Value {
		t.Helper()
		vals, err := parser.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		out, err := eng.NewTop(r).Run(vals)
		if err != nil || len(out) != 1 {
			t.Fatalf("build %q: %v %v", src, out, err)
		}
		return out[0]
	}
	sigOf := func(v Value) *eng.FnSig {
		t.Helper()
		info, ok := v.Data.(eng.FnDefInfo)
		if !ok || len(info.Signatures) == 0 {
			t.Fatalf("not a fn value: %s", v.String())
		}
		return &info.Signatures[0]
	}

	// A list argument passes through unquoted.
	lst := build("fn [[x:[:Integer]] [Integer] [x lengthq]]")
	out, err := r.CallBoru(sigOf(lst), []Value{eng.NewList([]Value{eng.NewInteger(1), eng.NewInteger(2)})}, nil)
	if err != nil || eng.Canon(out) != "2" {
		t.Errorf("list arg call = %s / %v, want 2", eng.Canon(out), err)
	}

	// A body that defs without undef: the leak-recovery arm cleans up.
	leak := build("fn [[x:Integer] [Integer] [def zz 9 x]]")
	out, err = r.CallBoru(sigOf(leak), []Value{eng.NewInteger(4)}, nil)
	if err != nil || eng.Canon(out) != "4" {
		t.Errorf("def-leaking call = %s / %v, want 4", eng.Canon(out), err)
	}
	if _, bound := r.Defs.Top("zz"); bound {
		t.Error("leaked def must be cleaned up after the call")
	}

	// An unnamed positional param with a body producing an extra value:
	// the trim arm drops the unconsumed input from the residual bottom.
	extra := build("fn [Integer [Integer] [7]]")
	out, err = r.CallBoru(sigOf(extra), []Value{eng.NewInteger(4)}, nil)
	if err != nil || eng.Canon(out) != "7" {
		t.Errorf("unnamed-extra call = %s / %v, want 7", eng.Canon(out), err)
	}
}

func TestRunPredicateShapeArms(t *testing.T) {
	r := specfixProbeRegistry(t)
	// A Function-parented value with no FnDefInfo payload.
	if _, _, err := r.RunPredicate(eng.NewValueRaw(eng.TFunction, eng.IntPayload{N: 1}), eng.NewInteger(5)); err == nil {
		t.Error("non-FnDefInfo constraint must error")
	}
	// A predicate whose body yields two values.
	vals, err := parser.Parse("fn [[x:Integer] [Any] [1 2]]")
	if err != nil {
		t.Fatal(err)
	}
	out, err := eng.NewTop(r).Run(vals)
	if err != nil || len(out) != 1 {
		t.Fatalf("build: %v %v", out, err)
	}
	if _, _, err := r.RunPredicate(out[0], eng.NewInteger(5)); err == nil {
		t.Error("two-value predicate body must error")
	}
}

// TestParseFnDefReturnSlots drives the return-list resolution arms
// with pre-evaluated values: literal patterns in the returns slot,
// typed-map children, and the empty returns list.
func TestParseFnDefReturnSlots(t *testing.T) {
	r, err := standaloneRegistry()
	if err != nil {
		t.Fatal(err)
	}
	body := eng.NewList([]Value{eng.NewInteger(1)})
	params := eng.NewList([]Value{eng.NewTypeLiteral(eng.TInteger)})
	mkRets := func(rets ...Value) []Value {
		return []Value{params, eng.NewList(rets), body}
	}
	for _, c := range []struct {
		name string
		ret  Value
	}{
		{"boolean-literal", eng.NewBoolean(true)},
		{"string-literal", eng.NewString("s")},
		{"integer-literal", eng.NewInteger(5)},
		{"atom-literal", eng.NewAtom("tag")},
		{"typed-map", eng.NewTypedMap(eng.NewTypeLiteral(eng.TInteger))},
		{"typed-list", eng.NewTypedList(eng.NewTypeLiteral(eng.TInteger))},
		{"record", func() Value {
			om := eng.NewOrderedMap()
			om.Set("a", eng.NewTypeLiteral(eng.TInteger))
			return eng.NewRecordType(om)
		}()},
	} {
		if _, perr := eng.ParseFnDef(r, mkRets(c.ret)); perr != nil {
			t.Errorf("%s return slot: %v", c.name, perr)
		}
	}
	// Empty returns list.
	if _, perr := eng.ParseFnDef(r, mkRets()); perr != nil {
		t.Errorf("empty returns: %v", perr)
	}
}
