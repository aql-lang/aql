package eng

// Seam-6b cluster B2: unit tests for previously-unreached blocks in
// fn_params.go — the non-concrete input-sig guard, the implicit-map
// optional/`?`-suffix and /q error arms, the whole colon-delimited
// single-Word param form, the type-value branches of ResolveSigType
// (surface, schema, record, typed list/map child errors, TAny tail),
// LookupDefType's def-stack arms, ResolveDefType's tail, and
// ResolveTypeName("Never"). Direct in-package calls throughout.

import (
	"strings"
	"testing"
)

// s6b2ImplicitParamMap builds the `{key: v}` implicit-pair map form the
// production parser produces for a typed param.
func s6b2ImplicitParamMap(key string, v Value) Value {
	om := NewOrderedMap()
	om.Implicit = true
	om.Set(key, v)
	return NewMap(om)
}

func TestS6b2ParseFnParamsNonConcreteInputSig(t *testing.T) {
	r := newTestRegistry(t)
	if _, _, err := ParseFnParams(r, NewCarrier(TList)); err == nil ||
		!strings.Contains(err.Error(), "concrete list") {
		t.Errorf("carrier input sig must error with the concrete-list message, got %v", err)
	}
}

func TestS6b2ParseFnParamsImplicitMapQuestionSuffixKey(t *testing.T) {
	r := newTestRegistry(t)
	sig := NewList([]Value{s6b2ImplicitParamMap("s6b2x?", NewTypeLiteral(TInteger))})
	params, _, err := ParseFnParams(r, sig)
	if err != nil {
		t.Fatalf("ParseFnParams: %v", err)
	}
	if len(params) != 1 || params[0].Name != "s6b2x" || !params[0].Optional {
		t.Errorf("`s6b2x?` key must yield an optional param named s6b2x, got %+v", params)
	}
}

func TestS6b2ParseFnParamsParenAnnotationMultiValue(t *testing.T) {
	r := newTestRegistry(t)
	paren := NewParenExpr([]Value{NewInteger(1), NewInteger(2)})
	sig := NewList([]Value{s6b2ImplicitParamMap("s6b2y", paren)})
	if _, _, err := ParseFnParams(r, sig); err == nil ||
		!strings.Contains(err.Error(), "must produce one type") {
		t.Errorf("multi-valued paren annotation must be a def-time error, got %v", err)
	}
}

func TestS6b2ParseFnParamsNamedQuoteAtomErrors(t *testing.T) {
	r := newTestRegistry(t)

	// Atom in the type slot that names NO type: def-time error.
	sig := NewList([]Value{s6b2ImplicitParamMap("s6b2z", NewAtom("S6b2NotAType"))})
	if _, _, err := ParseFnParams(r, sig); err == nil ||
		!strings.Contains(err.Error(), "invalid type") {
		t.Errorf("unknown atom type must error, got %v", err)
	}

	// Atom names a real type but the param name is invalid.
	sig = NewList([]Value{s6b2ImplicitParamMap("Bad", NewAtom("Integer"))})
	if _, _, err := ParseFnParams(r, sig); err == nil {
		t.Error("invalid word name in a /q param must error")
	}

	// Positive twin: valid name + valid atom type is a quote param.
	sig = NewList([]Value{s6b2ImplicitParamMap("s6b2q", NewAtom("Integer"))})
	params, _, err := ParseFnParams(r, sig)
	if err != nil || len(params) != 1 || !params[0].Quote {
		t.Errorf("named /q param must parse quoted, got %+v (%v)", params, err)
	}
}

func TestS6b2ParseFnParamsImplicitMapBadName(t *testing.T) {
	r := newTestRegistry(t)
	sig := NewList([]Value{s6b2ImplicitParamMap("Bad", NewTypeLiteral(TInteger))})
	if _, _, err := ParseFnParams(r, sig); err == nil ||
		!strings.Contains(err.Error(), "function spec") {
		t.Errorf("capitalised param name must error, got %v", err)
	}
}

func TestS6b2ParseFnParamsInvalidNonImplicitMapParam(t *testing.T) {
	r := newTestRegistry(t)
	// A typed map whose ParenExpr child evaluates to two values: the
	// non-implicit map arm's ResolveSigType call must surface the error.
	badChild := NewParenExpr([]Value{NewInteger(1), NewInteger(2)})
	tm := NewTypedMap(badChild)
	sig := NewList([]Value{tm})
	if _, _, err := ParseFnParams(r, sig); err == nil ||
		!strings.Contains(err.Error(), "invalid map param") {
		t.Errorf("typed-map param with a bad child expr must error, got %v", err)
	}
}

func TestS6b2ParseFnParamsColonFormFull(t *testing.T) {
	r := newTestRegistry(t)
	sig := NewList([]Value{
		NewWord("s6b2a:Integer"),
		NewWord("s6b2b?:String"),
		NewWord("s6b2c:Integer/q"),
	})
	params, _, err := ParseFnParams(r, sig)
	if err != nil {
		t.Fatalf("ParseFnParams colon form: %v", err)
	}
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}
	if params[0].Name != "s6b2a" || !params[0].Type.Equal(TInteger) || params[0].Optional || params[0].Quote {
		t.Errorf("plain colon param wrong: %+v", params[0])
	}
	if params[1].Name != "s6b2b" || !params[1].Optional {
		t.Errorf("`?`-suffixed colon param must be optional: %+v", params[1])
	}
	if params[2].Name != "s6b2c" || !params[2].Quote {
		t.Errorf("`/q`-suffixed colon param must be a quote param: %+v", params[2])
	}
}

func TestS6b2ParseFnParamsColonFormErrors(t *testing.T) {
	r := newTestRegistry(t)

	// Unknown type name in the colon form.
	sig := NewList([]Value{NewWord("s6b2d:S6b2Nope")})
	if _, _, err := ParseFnParams(r, sig); err == nil ||
		!strings.Contains(err.Error(), "invalid type") {
		t.Errorf("colon form with unknown type must error, got %v", err)
	}

	// Invalid param name in the colon form.
	sig = NewList([]Value{NewWord("Bad:Integer")})
	if _, _, err := ParseFnParams(r, sig); err == nil {
		t.Error("colon form with capitalised name must error")
	}
}

func TestS6b2ParseFnParamsBareWordUnknownType(t *testing.T) {
	r := newTestRegistry(t)
	sig := NewList([]Value{NewWord("s6b2notatype")})
	if _, _, err := ParseFnParams(r, sig); err == nil ||
		!strings.Contains(err.Error(), "invalid type") {
		t.Errorf("bare unknown type-name word must error, got %v", err)
	}
}

// --- ResolveSigType type-value branches ------------------------------------

func TestS6b2ResolveSigTypeSurfaceValue(t *testing.T) {
	r := newTestRegistry(t)
	surf := NewSurfaceType(TSurface, &SurfaceInfo{Name: "S6b2Surf", Required: NewOrderedMap()})
	typ, pat, err := ResolveSigType(r, surf)
	if err != nil || pat != nil {
		t.Fatalf("surface value: err=%v pat=%v", err, pat)
	}
	if typ == nil || typ.ID != TSurface.ID {
		t.Errorf("surface param type = %v, want the surface node", typ)
	}
}

func TestS6b2ResolveSigTypeSchemaValue(t *testing.T) {
	r := newTestRegistry(t)
	node := r.Types.MintType("S6b2Box", TMap)
	schema := NewTypeSchema(node, &TypeSchemaInfo{})
	typ, pat, err := ResolveSigType(r, schema)
	if err != nil || pat != nil {
		t.Fatalf("schema value: err=%v pat=%v", err, pat)
	}
	if typ == nil || typ.ID != node.ID {
		t.Errorf("schema param type = %v, want the minted schema node", typ)
	}
}

func TestS6b2ResolveSigTypeRecordValueDirect(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	rec := NewRecordType(fields)
	typ, pat, err := ResolveSigType(r, rec)
	if err != nil {
		t.Fatalf("record value: %v", err)
	}
	if typ == nil || !typ.Equal(TMap) || pat == nil {
		t.Errorf("record param must be TMap + structural pattern, got %v / %v", typ, pat)
	}
}

func TestS6b2ResolveSigTypeTypedContainerChildErrors(t *testing.T) {
	r := newTestRegistry(t)
	badChild := NewParenExpr([]Value{NewInteger(1), NewInteger(2)})

	if _, _, err := ResolveSigType(r, NewTypedMap(badChild)); err == nil {
		t.Error("typed map with a multi-valued child expr must error")
	}
	if _, _, err := ResolveSigType(r, NewTypedList(badChild)); err == nil {
		t.Error("typed list with a multi-valued child expr must error")
	}
}

func TestS6b2ResolveSigTypeUnclassifiedTail(t *testing.T) {
	r := newTestRegistry(t)
	fn := NewFunction(FnDefInfo{})
	typ, pat, err := ResolveSigType(r, fn)
	if err != nil || pat != nil || typ == nil || !typ.Equal(TAny) {
		t.Errorf("unclassified value must fall to TAny: %v / %v / %v", typ, pat, err)
	}
}

// --- LookupDefType / ResolveDefType / ResolveTypeName -----------------------

func TestS6b2LookupDefTypeArms(t *testing.T) {
	r := newTestRegistry(t)

	// Type-stack binding with a NON-bare body (a record): returned as-is.
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	recBody := NewRecordType(fields)
	r.Defs.PushType("S6b2RecTy", TMap, recBody)
	got := LookupDefType(r, "S6b2RecTy")
	if got == nil || !IsRecordType(*got) {
		t.Errorf("type-stack record body must return the body, got %v", got)
	}

	// Def-stack value binding that is NOT a type body: nil.
	r.Defs.Push("s6b2val", NewInteger(7))
	if got := LookupDefType(r, "s6b2val"); got != nil {
		t.Errorf("plain value binding must not be a def type, got %v", got)
	}

	// Def-stack bare type literal: canonicalized lattice node.
	r.Defs.Push("S6b2Alias", NewTypeLiteral(TInteger))
	got = LookupDefType(r, "S6b2Alias")
	if got == nil || !IsBareTypeNode(*got) || got.ID != TInteger.ID {
		t.Errorf("bare-literal def binding must canonicalize to Integer, got %v", got)
	}
}

func TestS6b2ResolveDefTypeFallthrough(t *testing.T) {
	r := newTestRegistry(t)
	typ, pat, err := ResolveDefType(r, NewInteger(5))
	if err != nil || pat != nil || typ == nil || !typ.Equal(TAny) {
		t.Errorf("non-type body must fall to TAny: %v / %v / %v", typ, pat, err)
	}
}

func TestS6b2ResolveTypeNameNever(t *testing.T) {
	typ, err := ResolveTypeName("Never")
	if err != nil || typ == nil || typ.ID != TNever.ID {
		t.Errorf("ResolveTypeName(Never) = %v (%v), want TNever", typ, err)
	}
}
