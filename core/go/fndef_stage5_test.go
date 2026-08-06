package core

import (
	"strings"
	"testing"
)

// Stage-5 coverage for fn_def.go: the ParseFnDef happy paths (triple
// walking, input-sig wrapping, return-by-value splicing), the
// ParseFnUndefSpec pair walker, and the signature-context type
// classifiers (OutputSigIsConcreteReturns / IsSigTypeValue /
// isSigTypeName / OutputSigValues).

func fndefStage5List(elems ...Value) Value { return NewList(elems) }

func TestFnDefStage5ParseFnDefTriples(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	list := []Value{
		// Triple 1: list input, type-word output, list body.
		fndefStage5List(NewWord("n:Integer")), NewWord("Integer"), fndefStage5List(NewWord("n")),
		// Triple 2: NON-list input (auto-wrapped to a one-element list),
		// concrete single-value output (return-by-value), non-list body.
		NewWord("Integer"), NewInteger(42), NewWord("zz-b"),
		// Triple 3: concrete LIST output (multi return-by-value).
		fndefStage5List(NewWord("m:Integer")), fndefStage5List(NewInteger(1), NewString("ok")), fndefStage5List(NewWord("m")),
	}
	info, perr := ParseFnDef(r, list)
	if perr != nil {
		t.Fatalf("ParseFnDef: %v", perr)
	}
	if len(info.Signatures) != 3 {
		t.Fatalf("want 3 sigs, got %d", len(info.Signatures))
	}

	s0 := info.Signatures[0]
	if len(s0.Params) != 1 || s0.Params[0].Name != "n" || !s0.Params[0].Type.Equal(TInteger) {
		t.Errorf("sig0 params = %+v", s0.Params)
	}
	if len(s0.Returns) != 1 || !s0.Returns[0].Equal(TInteger) {
		t.Errorf("sig0 returns = %v", s0.Returns)
	}
	if len(s0.Body()) != 1 {
		t.Errorf("sig0 body = %v", s0.Body())
	}

	// Triple 2: the wrapped input yields one unnamed Integer param; the
	// concrete output splices [End 42] after the body and forces the
	// static returns to [Any].
	s1 := info.Signatures[1]
	if len(s1.Params) != 1 || s1.Params[0].Name != "" || !s1.Params[0].Type.Equal(TInteger) {
		t.Errorf("sig1 params = %+v", s1.Params)
	}
	b1 := s1.Body()
	if len(b1) != 3 || !IsEnd(b1[1]) {
		t.Fatalf("sig1 body = %v (want [zz-b End 42])", b1)
	}
	if n, aerr := AsInteger(b1[2]); aerr != nil || n != 42 {
		t.Errorf("sig1 spliced return = %v (%v)", b1[2], aerr)
	}
	if len(s1.Returns) != 1 || !s1.Returns[0].Equal(TAny) {
		t.Errorf("sig1 returns = %v", s1.Returns)
	}

	// Triple 3: two concrete return values spliced after End.
	s2 := info.Signatures[2]
	b2 := s2.Body()
	if len(b2) != 4 || !IsEnd(b2[1]) {
		t.Fatalf("sig2 body = %v (want [m End 1 'ok'])", b2)
	}
	if len(s2.Returns) != 2 || !s2.Returns[0].Equal(TAny) || !s2.Returns[1].Equal(TAny) {
		t.Errorf("sig2 returns = %v", s2.Returns)
	}

	// A parameter that names no type is a ParseFnParams error.
	_, perr = ParseFnDef(r, []Value{
		fndefStage5List(NewWord("zz-not-a-type")), NewWord("Integer"), fndefStage5List(),
	})
	if perr == nil || !strings.Contains(perr.Error(), "invalid type") {
		t.Errorf("bad param type must error, got %v", perr)
	}

	// An empty list parses to an empty FnDefInfo.
	empty, perr := ParseFnDef(r, nil)
	if perr != nil || len(empty.Signatures) != 0 {
		t.Errorf("empty list: %+v, %v", empty, perr)
	}
}

func TestFnDefStage5ParseFnUndefSpec(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	info, uerr := ParseFnUndefSpec(r, []Value{
		// Pair 1: NON-list input (auto-wrapped).
		NewWord("Integer"), NewWord("Integer"),
		// Pair 2: named param, list output.
		fndefStage5List(NewWord("s:String")), fndefStage5List(NewWord("String")),
	})
	if uerr != nil {
		t.Fatalf("ParseFnUndefSpec: %v", uerr)
	}
	if len(info.Sigs) != 2 {
		t.Fatalf("want 2 sig specs, got %d", len(info.Sigs))
	}
	if len(info.Sigs[0].Params) != 1 || !info.Sigs[0].Params[0].Type.Equal(TInteger) ||
		len(info.Sigs[0].Returns) != 1 || !info.Sigs[0].Returns[0].Equal(TInteger) {
		t.Errorf("spec0 = %+v", info.Sigs[0])
	}
	if len(info.Sigs[1].Params) != 1 || info.Sigs[1].Params[0].Name != "s" ||
		len(info.Sigs[1].Returns) != 1 || !info.Sigs[1].Returns[0].Equal(TString) {
		t.Errorf("spec1 = %+v", info.Sigs[1])
	}

	// The ParseFnReturns error arm.
	_, uerr = ParseFnUndefSpec(r, []Value{
		fndefStage5List(NewWord("Integer")), NewWord("zz-not-a-type"),
	})
	if uerr == nil {
		t.Error("an unresolvable output type must error")
	}

	// The ParseFnParams error arm.
	_, uerr = ParseFnUndefSpec(r, []Value{
		fndefStage5List(NewWord("zz-not-a-type")), NewWord("Integer"),
	})
	if uerr == nil {
		t.Error("an unresolvable input type must error")
	}
}

func TestFnDefStage5OutputSigClassifiers(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Empty list output sig: NOT concrete returns.
	if OutputSigIsConcreteReturns(r, fndefStage5List()) {
		t.Error("empty output list must not be concrete returns")
	}
	// All-concrete list: concrete returns.
	if !OutputSigIsConcreteReturns(r, fndefStage5List(NewInteger(1), NewString("ok"))) {
		t.Error("all-concrete list must be concrete returns")
	}
	// A type inside the list defeats concreteness.
	if OutputSigIsConcreteReturns(r, fndefStage5List(NewInteger(1), NewWord("Integer"))) {
		t.Error("a type element must defeat concrete returns")
	}
	// Non-list forms: a concrete value is concrete, a type value is not.
	if !OutputSigIsConcreteReturns(r, NewInteger(7)) {
		t.Error("a single concrete value must be concrete returns")
	}
	if OutputSigIsConcreteReturns(r, NewWord("Integer")) {
		t.Error("a single type word must not be concrete returns")
	}

	// OutputSigValues: list form returns the elements, single-value
	// form wraps.
	vals := OutputSigValues(fndefStage5List(NewInteger(1), NewString("ok")))
	if len(vals) != 2 {
		t.Errorf("list output values = %v", vals)
	}
	vals = OutputSigValues(NewInteger(7))
	if len(vals) != 1 {
		t.Errorf("single output values = %v", vals)
	}
	if n, aerr := AsInteger(vals[0]); aerr != nil || n != 7 {
		t.Errorf("wrapped value = %v (%v)", vals[0], aerr)
	}
}

func TestFnDefStage5IsSigTypeValueArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// A Disjunct VALUE is a type.
	dis := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	if !IsSigTypeValue(r, dis) {
		t.Error("a disjunct must classify as a sig type")
	}
	// Word / Atom / String naming a kernel type.
	if !IsSigTypeValue(r, NewWord("Integer")) {
		t.Error("a type-name word must classify as a sig type")
	}
	// An Atom takes the name branch but its payload is AtomPayload, not
	// StrPayload — AsString yields "" and no type name matches.
	if IsSigTypeValue(r, NewAtom("Integer")) {
		t.Error("an atom must not classify as a sig type (AsString yields no name)")
	}
	if !IsSigTypeValue(r, NewString("String")) {
		t.Error("a type-name string must classify as a sig type")
	}
	// Names that resolve nowhere are not types (with and without a
	// registry — the nil-registry LookupTypeName short-circuit).
	if IsSigTypeValue(r, NewWord("zz-not-a-type")) {
		t.Error("an unbound word must not classify as a sig type")
	}
	if IsSigTypeValue(nil, NewString("zz-not-a-type")) {
		t.Error("nil-registry unknown string must not classify as a sig type")
	}
	// A plain concrete value is not a type.
	if IsSigTypeValue(r, NewInteger(3)) {
		t.Error("a concrete integer must not classify as a sig type")
	}
	// A user-defined type binding is recognised through the registry.
	r.Defs.PushType("ZzStage5T", TInteger, NewTypeLiteral(TInteger))
	if !IsSigTypeValue(r, NewWord("ZzStage5T")) {
		t.Error("a def'd type name must classify as a sig type")
	}
	r.Defs.Pop("ZzStage5T")
}

func TestFnDefStage5IsSigTypeValueDottedReach(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// ApplyReach lowers to `recv dot key` — supply the minimal map-get
	// `dot` word (args[0] = key forward, args[1] = receiver from stack).
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
	if rerr := r.Err(); rerr != nil {
		t.Fatalf("registration: %v", rerr)
	}
	exports := NewOrderedMap()
	exports.Set("Matrix", NewTypeLiteral(TList))
	r.Defs.Push("ZzStage5Pkg", NewMap(exports))
	defer r.Defs.Pop("ZzStage5Pkg")

	reach := NewValueRaw(TReach, ReachInfo{
		Receiver: []Value{NewWord("ZzStage5Pkg")},
		Segments: []ReachSeg{{KeyLit: NewAtom("Matrix")}},
	})
	if !IsSigTypeValue(r, reach) {
		t.Error("a dotted reach to a bound type literal must classify as a sig type")
	}
	// A reach that resolves to nothing falls through to false.
	miss := NewValueRaw(TReach, ReachInfo{
		Receiver: []Value{NewWord("zz-stage5-unbound")},
		Segments: []ReachSeg{{KeyLit: NewAtom("Matrix")}},
	})
	if IsSigTypeValue(r, miss) {
		t.Error("an unresolvable reach must not classify as a sig type")
	}
}
