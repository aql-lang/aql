package core

import (
	"strings"
	"testing"
)

// stage5ParamList wraps elems as the concrete input-signature list
// ParseFnParams walks.
func stage5ParamList(elems ...Value) Value {
	return NewList(elems)
}

// stage5Pair builds the implicit single-pair map `{name: typeVal}` — the
// standard typed-param form the production parser produces.
func stage5Pair(name string, typeVal Value) Value {
	om := NewOrderedMap()
	om.Set(name, typeVal)
	return NewImplicitMap(om)
}

// TestParseFnParamsSugarAndAnnotationArms drives the sugar-marker
// lowering in a param's type slot, the paren-annotation evaluation
// (success and failure), and the invalid-named-type error return.
func TestParseFnParamsSugarAndAnnotationArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// A sugar marker lowers to its expansion first; the resulting word
	// names no type, so the named-param resolution error fires.
	r.BindSugarWord(SugarUsurp, "stage5-usurp")
	sugarVal := NewSugar(SugarInfo{Kind: SugarUsurp})
	_, _, perr := ParseFnParams(r, stage5ParamList(stage5Pair("x", sugarVal)))
	if perr == nil || !strings.Contains(perr.Error(), `invalid type for "x"`) {
		t.Fatalf("sugar-lowered non-type must error, got %v", perr)
	}

	// Paren annotation evaluating to one type: adopted as the slot type.
	params, _, perr := ParseFnParams(r, stage5ParamList(
		stage5Pair("x", NewParenExpr([]Value{NewTypeLiteral(TInteger)}))))
	if perr != nil || len(params) != 1 || params[0].Name != "x" || !params[0].Type.Equal(TInteger) {
		t.Fatalf("paren annotation must adopt the evaluated type, got %+v, %v", params, perr)
	}

	// A failing paren annotation is a def-time error.
	r.RegisterNativeFunc(NativeFunc{
		Name: "stage-five-perr",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				return nil, reg.BoruError("type_error", "stage5 annotation boom", "stage-five-perr")
			}),
			Returns:    []*Type{},
			BarrierPos: 0,
		}},
	})
	_, _, perr = ParseFnParams(r, stage5ParamList(
		stage5Pair("x", NewParenExpr([]Value{NewWord("stage-five-perr")}))))
	if perr == nil || !strings.Contains(perr.Error(), `invalid type for "x"`) {
		t.Fatalf("failing paren annotation must error, got %v", perr)
	}
}

// TestParseFnParamsDisjunctOptional pins the None-including disjunct
// annotation: the param turns optional and its type collapses to the
// first non-None alternative.
func TestParseFnParamsDisjunctOptional(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	dis := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewNone()})
	params, _, perr := ParseFnParams(r, stage5ParamList(stage5Pair("x", dis)))
	if perr != nil || len(params) != 1 {
		t.Fatalf("disjunct param: got %+v, %v", params, perr)
	}
	p := params[0]
	if p.Name != "x" || !p.Optional || !p.Type.Equal(TInteger) {
		t.Fatalf("None-including disjunct must be optional Integer, got %+v", p)
	}
}

// TestParseFnParamsElementForms covers the remaining element kinds: the
// non-implicit map pattern, the bare type-name word, concrete literal
// patterns, dotted (Reach) annotations, and the invalid-element error.
func TestParseFnParamsElementForms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Non-implicit concrete map: an unnamed structural TMap pattern.
	om := NewOrderedMap()
	om.Set("a", NewTypeLiteral(TInteger))
	om.Set("b", NewTypeLiteral(TString))
	params, _, perr := ParseFnParams(r, stage5ParamList(NewMap(om)))
	if perr != nil || len(params) != 1 || !params[0].Type.Equal(TMap) || params[0].Pattern == nil {
		t.Fatalf("plain map param: got %+v, %v", params, perr)
	}

	// Bare type-name word, plus the three concrete literal patterns.
	params, _, perr = ParseFnParams(r, stage5ParamList(
		NewWord("Integer"), NewInteger(3), NewBoolean(true), NewString("s")))
	if perr != nil || len(params) != 4 {
		t.Fatalf("mixed forms: got %+v, %v", params, perr)
	}
	if !params[0].Type.Equal(TInteger) || params[0].Pattern != nil {
		t.Fatalf("bare word must be an unnamed Integer param, got %+v", params[0])
	}
	if !params[1].Type.Equal(TInteger) || params[1].Pattern == nil {
		t.Fatalf("integer literal must anchor a pattern, got %+v", params[1])
	}
	if !params[2].Type.Equal(TBoolean) || params[2].Pattern == nil {
		t.Fatalf("boolean literal must anchor a pattern, got %+v", params[2])
	}
	if !params[3].Type.Equal(TString) || params[3].Pattern == nil {
		t.Fatalf("string literal must anchor a pattern, got %+v", params[3])
	}

	// Dotted annotation: `mat:Pkg.Matrix` reaching a bound type literal.
	// ApplyReach lowers to `recv dot key`, so a minimal `dot` word must
	// exist (the lang layer's, shrunk to the map-get contract:
	// args[0] = key forward, args[1] = receiver from the stack).
	r.RegisterNativeFunc(NativeFunc{
		Name: "dot",
		Signatures: []Signature{{
			Args: []*Type{TAny, TMap},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				key, _ := AsAtom(args[0])
				m, merr := AsMap(args[1])
				if merr != nil {
					return nil, reg.BoruError("type_error", "dot: receiver is not a map", "dot")
				}
				v, ok := m.Get(key)
				if !ok {
					return nil, nil
				}
				return []Value{v}, nil
			}),
			Returns:    []*Type{TAny},
			BarrierPos: 1,
		}},
	})
	exports := NewOrderedMap()
	exports.Set("Matrix", NewTypeLiteral(TList))
	r.Defs.Push("Stage5Pkg", NewMap(exports))
	defer r.Defs.Pop("Stage5Pkg")
	recv := NewOrderedMap()
	recv.Set("mat", NewWord("Stage5Pkg"))
	reach := NewValueRaw(TReach, ReachInfo{
		Receiver: []Value{NewImplicitMap(recv)},
		Segments: []ReachSeg{{KeyLit: NewAtom("Matrix")}},
	})
	params, _, perr = ParseFnParams(r, stage5ParamList(reach))
	if perr != nil || len(params) != 1 || params[0].Name != "mat" || !params[0].Type.Equal(TList) {
		t.Fatalf("dotted annotation must resolve through the binding, got %+v, %v", params, perr)
	}

	// A dotted annotation that reaches nothing is an invalid parameter.
	badReach := NewValueRaw(TReach, ReachInfo{
		Receiver: []Value{NewWord("stage5-unbound-pkg")},
		Segments: []ReachSeg{{KeyLit: NewAtom("Matrix")}},
	})
	if _, _, perr = ParseFnParams(r, stage5ParamList(badReach)); perr == nil ||
		!strings.Contains(perr.Error(), "dotted annotation") {
		t.Fatalf("unresolvable dotted annotation must error, got %v", perr)
	}

	// A bound base whose reached export is NOT a type literal declines too.
	exports.Set("NotAType", NewInteger(1))
	nonLiteral := NewValueRaw(TReach, ReachInfo{
		Receiver: []Value{NewWord("Stage5Pkg")},
		Segments: []ReachSeg{{KeyLit: NewAtom("NotAType")}},
	})
	if _, _, perr = ParseFnParams(r, stage5ParamList(nonLiteral)); perr == nil ||
		!strings.Contains(perr.Error(), "dotted annotation") {
		t.Fatalf("non-literal dotted annotation must error, got %v", perr)
	}

	// An element of no recognised kind is an invalid parameter.
	if _, _, perr = ParseFnParams(r, stage5ParamList(NewList(nil))); perr == nil ||
		!strings.Contains(perr.Error(), "invalid parameter") {
		t.Fatalf("list element must error, got %v", perr)
	}
}

// TestParseFnReturnsArms covers the single-value output forms (typed,
// pattern-carrying, erroring), the empty list, and a list element
// carrying a value pattern.
func TestParseFnReturnsArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	types, pats, rerr := ParseFnReturns(r, NewTypeLiteral(TInteger))
	if rerr != nil || len(types) != 1 || !types[0].Equal(TInteger) || pats != nil {
		t.Fatalf("single type output: got %v %v %v", types, pats, rerr)
	}
	types, pats, rerr = ParseFnReturns(r, NewInteger(5))
	if rerr != nil || len(types) != 1 || !types[0].Equal(TInteger) || len(pats) != 1 || pats[0] == nil {
		t.Fatalf("single pattern output: got %v %v %v", types, pats, rerr)
	}
	if _, _, rerr = ParseFnReturns(r, NewWord("stage5-not-a-type")); rerr == nil {
		t.Fatal("unknown single output type must error")
	}
	types, pats, rerr = ParseFnReturns(r, NewList(nil))
	if rerr != nil || types != nil || pats != nil {
		t.Fatalf("empty output list: got %v %v %v", types, pats, rerr)
	}
	types, pats, rerr = ParseFnReturns(r, NewList([]Value{NewInteger(5), NewTypeLiteral(TString)}))
	if rerr != nil || len(types) != 2 || !types[0].Equal(TInteger) || !types[1].Equal(TString) {
		t.Fatalf("mixed output list: got %v %v %v", types, pats, rerr)
	}
	if len(pats) != 2 || pats[0] == nil || pats[1] != nil {
		t.Fatalf("only the literal element carries a pattern, got %v", pats)
	}
}

// TestResolveSigTypeArms covers the name-by-String arm, the TypeDef
// binding arms (plain and record-bodied), the value-bound def-type
// fallback, direct class-type / bounded-type / disjunct values, every
// scalar-pattern kind, and the typed/plain container arms.
func TestResolveSigTypeArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// String-spelled type name.
	tt, pat, serr := ResolveSigType(r, NewString("Integer"))
	if serr != nil || !tt.Equal(TInteger) || pat != nil {
		t.Fatalf("string name: got %v %v %v", tt, pat, serr)
	}

	// A capitalised-def type binding (DefEntry.TypeDef) with a plain body.
	r.Defs.PushType("Stage5T", TInteger, NewTypeLiteral(TInteger))
	defer r.Defs.Pop("Stage5T")
	tt, pat, serr = ResolveSigType(r, NewWord("Stage5T"))
	if serr != nil || !tt.Equal(TInteger) || pat != nil {
		t.Fatalf("TypeDef binding: got %v %v %v", tt, pat, serr)
	}
	// lookupTypeNameInRegistry reaches the same binding by name.
	if got, lerr := lookupTypeNameInRegistry(r, "Stage5T"); lerr != nil || !got.Equal(TInteger) {
		t.Fatalf("registry type-name lookup: got %v %v", got, lerr)
	}

	// A TypeDef binding whose body is a Record: structural pattern attached.
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	r.Defs.PushType("Stage5R", TMap, NewRecordType(fields))
	defer r.Defs.Pop("Stage5R")
	tt, pat, serr = ResolveSigType(r, NewWord("Stage5R"))
	if serr != nil || !tt.Equal(TMap) || pat == nil {
		t.Fatalf("record TypeDef binding: got %v %v %v", tt, pat, serr)
	}

	// A VALUE-bound record body (no TypeDef): the LookupDefType fallback.
	r.Defs.Push("Stage5V", NewRecordType(fields))
	defer r.Defs.Pop("Stage5V")
	tt, pat, serr = ResolveSigType(r, NewWord("Stage5V"))
	if serr != nil || !tt.Equal(TMap) || pat == nil {
		t.Fatalf("value-bound record: got %v %v %v", tt, pat, serr)
	}
	if LookupDefType(nil, "Stage5V") != nil {
		t.Fatal("nil registry LookupDefType must be nil")
	}

	// A class-type VALUE arriving directly (the paren annotation path).
	cv := NewClassType(TInteger, ClassTypeInfo{Fields: NewOrderedMap(), Name: "Class/Stage5"})
	tt, pat, serr = ResolveSigType(r, cv)
	if serr != nil || !tt.Equal(TInteger) || pat != nil {
		t.Fatalf("class type value: got %v %v %v", tt, pat, serr)
	}

	// Bounded Type (`Map/t`): slot type TType with the bound as pattern.
	tt, pat, serr = ResolveSigType(r, NewBoundedType(TMap))
	if serr != nil || !tt.Equal(TType) || pat == nil {
		t.Fatalf("bounded type: got %v %v %v", tt, pat, serr)
	}

	// Inline disjunct: TAny with the union as pattern.
	tt, pat, serr = ResolveSigType(r, NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)}))
	if serr != nil || !tt.Equal(TAny) || pat == nil {
		t.Fatalf("inline disjunct: got %v %v %v", tt, pat, serr)
	}

	// Scalar value patterns, one per kind arm.
	for _, tc := range []struct {
		v    Value
		kind *Type
	}{
		{NewInteger(5), TInteger},
		{NewFloat(1.5), TFloat},
		{NewBoolean(true), TBoolean},
		{NewDepScalar(DepGT, NewString("a")), TString},
		{NewDepScalar(DepGT, NewAtom("kw")), TAtom},
	} {
		tt, pat, serr = ResolveSigType(r, tc.v)
		if serr != nil || !tt.Equal(tc.kind) || pat == nil {
			t.Fatalf("scalar pattern %s: got %v %v %v", tc.v.String(), tt, pat, serr)
		}
	}

	// Typed and plain containers.
	tt, pat, serr = ResolveSigType(r, NewTypedMap(NewTypeLiteral(TInteger)))
	if serr != nil || !tt.Equal(TMap) || pat == nil {
		t.Fatalf("typed map: got %v %v %v", tt, pat, serr)
	}
	tt, pat, serr = ResolveSigType(r, NewTypedList(NewTypeLiteral(TInteger)))
	if serr != nil || !tt.Equal(TList) || pat == nil {
		t.Fatalf("typed list: got %v %v %v", tt, pat, serr)
	}
	plainList := NewList([]Value{NewTypeLiteral(TInteger)})
	tt, pat, serr = ResolveSigType(r, plainList)
	if serr != nil || !tt.Equal(TList) || pat == nil {
		t.Fatalf("plain list: got %v %v %v", tt, pat, serr)
	}

	// ResolveDefType's Options arm.
	tt, pat, serr = ResolveDefType(r, NewOptionsType(fields))
	if serr != nil || !tt.Equal(TMap) || pat == nil || !IsOptionsType(*pat) {
		t.Fatalf("options def type: got %v %v %v", tt, pat, serr)
	}
}

// TestResolveTypeNameTable pins the well-known name table arms.
func TestResolveTypeNameTable(t *testing.T) {
	for _, tc := range []struct {
		name string
		want *Type
	}{
		{"Any", TAny},
		{"None", TNone},
		{"Never", TNever},
		{"Number", TNumber},
		{"Integer", TInteger},
		{"Float", TFloat},
		{"String", TString},
		{"Boolean", TBoolean},
		{"List", TList},
		{"Function", TFunction},
		{"Map", TMap},
	} {
		got, err := ResolveTypeName(tc.name)
		if err != nil || !got.Equal(tc.want) {
			t.Fatalf("ResolveTypeName(%q) = %v, %v", tc.name, got, err)
		}
	}
}
