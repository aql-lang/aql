package modules

import (
	"errors"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// w8Fmt is a minimal test Format: Decode returns preset output/err.
type w8Fmt struct {
	out    []native.Value
	decErr error
}

func (f w8Fmt) Decode(_ string) ([]native.Value, error) { return f.out, f.decErr }
func (f w8Fmt) Encode(_ native.Value) (string, error)   { return "", nil }

// w8OptFmt additionally implements DecodeOpter.
type w8OptFmt struct{ w8Fmt }

func (f w8OptFmt) DecodeOpts(_ string, _ map[string]any) ([]native.Value, error) {
	return f.out, f.decErr
}

// TestW8FormatParseHandler drives formatParseHandler's src (297), DecodeOpter
// (302), decode-error (307) and empty-output (312) arms.
func TestW8FormatParseHandler(t *testing.T) {
	r := mcovReg(t)
	opts := s7bMap()

	// 297: non-concrete src.
	h := formatParseHandler("x", w8Fmt{})
	if _, err := h([]native.Value{w8LitS(), opts}, nil, nil, r); err == nil {
		t.Error("formatParseHandler: non-concrete src should error")
	}

	// 302: the DecodeOpter branch (happy).
	ho := formatParseHandler("x", w8OptFmt{w8Fmt{out: []native.Value{native.NewInteger(7)}}})
	out, err := ho([]native.Value{native.NewString("s"), opts}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("DecodeOpts path: got %v, %v", out, err)
	}

	// 307: decode error.
	he := formatParseHandler("x", w8Fmt{decErr: errors.New("boom")})
	if _, err := he([]native.Value{native.NewString("s"), opts}, nil, nil, r); err == nil {
		t.Error("formatParseHandler: decode error should propagate")
	}

	// 312: empty output → None.
	hn := formatParseHandler("x", w8Fmt{out: nil})
	out, err = hn([]native.Value{native.NewString("s"), opts}, nil, nil, r)
	if err != nil || len(out) != 1 || !native.IsNoneShape(out[0]) {
		t.Errorf("formatParseHandler: empty decode should yield None, got %v (%v)", out, err)
	}
}

// TestW8InstallHostParserDuplicate drives installHostParser's duplicate-key
// refusal (325).
func TestW8InstallHostParserDuplicate(t *testing.T) {
	r := mcovReg(t)
	exports := native.NewOrderedMap()
	spec := ParseLangSpec{Name: "dupw8", Handler: func(a []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return nil, nil
	}}
	if err := installHostParser(exports, r, spec); err != nil {
		t.Fatalf("first install should succeed: %v", err)
	}
	if err := installHostParser(exports, r, spec); err == nil {
		t.Error("second install of the same name should error")
	}
}

// TestW8PureParseFoldFallback drives pureParseFoldReturns's fallback arm (378):
// a non-2-arg call yields declared-Returns carriers.
func TestW8PureParseFoldFallback(t *testing.T) {
	r := mcovReg(t)
	fn := pureParseFoldReturns([]*native.Type{native.TAny}, func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return nil, nil
	})
	out := fn([]native.Value{native.NewInteger(1)}, r) // wrong arity → fallback
	if len(out) != 1 {
		t.Fatalf("fallback should carry one declared return, got %v", out)
	}
}

// TestW8ParseFoldableValue drives parseFoldableValue's carrier / list / map arms.
func TestW8ParseFoldableValue(t *testing.T) {
	// 399: a carrier is never foldable.
	if parseFoldableValue(native.NewCarrier(native.TAny)) {
		t.Error("a carrier is not foldable")
	}
	// list happy (403/408/409/413).
	if !parseFoldableValue(native.NewList([]native.Value{native.NewInteger(1), native.NewString("x")})) {
		t.Error("a concrete scalar list is foldable")
	}
	// list with a non-foldable element (409 → return false).
	if parseFoldableValue(native.NewList([]native.Value{native.NewCarrier(native.TAny)})) {
		t.Error("a list with a carrier element is not foldable")
	}
	// map happy (416 → loop → 425 return true).
	mOK := native.NewOrderedMap()
	mOK.Set("a", native.NewInteger(1))
	if !parseFoldableValue(native.NewMap(mOK)) {
		t.Error("a concrete scalar map is foldable")
	}
	// map with a non-foldable value (421 → return false).
	mBad := native.NewOrderedMap()
	mBad.Set("a", native.NewCarrier(native.TAny))
	if parseFoldableValue(native.NewMap(mBad)) {
		t.Error("a map with a carrier value is not foldable")
	}
	// list-typed value whose payload AsList refuses (405 → return false).
	if parseFoldableValue(eng.NewExtension(native.TList, "w8")) {
		t.Error("a list value AsList cannot read is not foldable")
	}
	// map-typed value whose payload AsMap refuses (416 → return false).
	if parseFoldableValue(eng.NewExtension(native.TMap, "w8")) {
		t.Error("a map value AsMap cannot read is not foldable")
	}
}

// TestW8ResolveParseSourceBad drives resolveParseSource's non-String/non-map
// refusal (456) and its empty-map arm (441): a TMap-typed value AsMap refuses.
func TestW8ResolveParseSourceBad(t *testing.T) {
	r := mcovReg(t)
	if _, err := resolveParseSource(native.NewInteger(5), r); err == nil {
		t.Error("resolveParseSource: an Integer source should error")
	}
	// 441: a concrete TMap-typed value AsMap reads as nil.
	if _, err := resolveParseSource(eng.NewExtension(native.TMap, "w8"), r); err == nil {
		t.Error("resolveParseSource: an unreadable source map should error")
	}
}

// TestW8RegisterFormatParserNoFormats drives RegisterFormatParser's
// missing-formats-capability arm (parselang.go:277): a bare registry with no
// formats capability installed.
func TestW8RegisterFormatParserNoFormats(t *testing.T) {
	bare, err := native.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if native.HostFormats(bare) != nil {
		t.Skip("bare registry unexpectedly has a formats capability")
	}
	rerr := RegisterFormatParser(bare, "w8fmt", w8Fmt{})
	if rerr == nil {
		t.Error("RegisterFormatParser without a formats capability should error")
	}
}

// TestW8ParseRegisterValidate drives parseRegisterValidate's name (492),
// non-function (499) and bad-signature (511) arms.
func TestW8ParseRegisterValidate(t *testing.T) {
	r := mcovReg(t)
	fn := mcovFilterFn()

	// 492: non-concrete name.
	if _, err := parseRegisterValidate([]native.Value{native.NewTypeLiteral(native.TAtom), fn}, r); err == nil {
		t.Error("parseRegisterValidate: non-concrete name should error")
	}
	// 499: not a function value.
	if _, err := parseRegisterValidate([]native.Value{native.NewAtom("k"), native.NewInteger(5)}, r); err == nil {
		t.Error("parseRegisterValidate: non-function should error")
	}
	// 511: a function whose leading params are not [String|Any, Map].
	badSig := native.NewFunction(native.FnDefInfo{
		Name: "p",
		Signatures: []native.FnSig{{
			Params: []native.FnParam{{Type: native.TInteger}, {Type: native.TInteger}},
		}},
	})
	if _, err := parseRegisterValidate([]native.Value{native.NewAtom("k"), badSig}, r); err == nil {
		t.Error("parseRegisterValidate: wrong param types should error")
	}
}

// TestW8ParseRegisterReturnsGuards drives parseRegisterReturns's early-return
// guards (558 / 561).
func TestW8ParseRegisterReturnsGuards(t *testing.T) {
	r := mcovReg(t)
	exports := native.NewOrderedMap()
	idents := map[string]registerIdent{}
	fn := parseRegisterReturns(exports, idents)

	// 558: fewer than two args.
	if out := fn([]native.Value{native.NewTypeLiteral(native.TAtom)}, r); out != nil {
		t.Errorf("parseRegisterReturns: <2 args should return nil, got %v", out)
	}
	// 561: args[1] is not a function value.
	if out := fn([]native.Value{native.NewAtom("k"), native.NewInteger(5)}, r); out != nil {
		t.Errorf("parseRegisterReturns: non-function should return nil, got %v", out)
	}
}

// (parselang-deferred-dispatch was deleted with Parse.register: grammar
// parsers are Parse.parser Function VALUES dispatched through
// parselang-fn-dispatch, whose arms TestFnDispatchArms drives.)

// TestW8AontuAndTabnasSrcErrors drives the src refusal arms of the aontu (666)
// and tabnas (687) built-in parser handlers.
func TestW8AontuAndTabnasSrcErrors(t *testing.T) {
	r := mcovReg(t)
	opts := s7bMap()

	aontu := aontuParserSpec()
	if _, err := aontu.Handler([]native.Value{w8LitS(), opts}, nil, nil, r); err == nil {
		t.Error("aontu handler: non-concrete src should error")
	}

	specs := tabnasParserSpecs()
	if len(specs) == 0 {
		t.Fatal("expected built-in tabnas parser specs")
	}
	if _, err := specs[0].Handler([]native.Value{w8LitS(), opts}, nil, nil, r); err == nil {
		t.Error("tabnas handler: non-concrete src should error")
	}
}

// TestW8HostStateNoCreate drives parseLangHostStateFor's no-create arm (220):
// a registry with no state returns nil when create=false.
func TestW8HostStateNoCreate(t *testing.T) {
	r := mcovReg(t)
	if s := parseLangHostStateFor(r, false); s != nil {
		t.Error("parseLangHostStateFor(create=false) on a fresh registry should be nil")
	}
}
