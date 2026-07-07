package native

import (
	"strings"
	"testing"
)

// --- genHandler pure arg guards (direct) ---

func TestW8GenHandlerNonConcreteList(t *testing.T) {
	r := w8Reg(t)
	// A bare List type literal is not a concrete list.
	if _, err := genHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r); err == nil {
		t.Fatal("gen: non-concrete list arg must error")
	}
}

func TestW8GenHandlerEmptyList(t *testing.T) {
	r := w8Reg(t)
	if _, err := genHandler([]Value{NewList(nil)}, nil, nil, r); err == nil {
		t.Fatal("gen: empty parameter list must error")
	}
}

// --- genHandler paren-entry paths (parser-driven) ---

func TestW8GenParenEntryPaths(t *testing.T) {
	// Each entry: source + whether an error is expected.
	cases := []struct {
		src     string
		wantErr bool
	}{
		// paren entry does not start with a parameter name
		{`def X gen [(5)] refine Record [v:Integer]`, true},
		// lowercase parameter name inside a paren entry
		{`def X gen [(t extends Integer)] refine Record [v:Integer]`, true},
		// entry expression itself errors when run
		{`def X gen [(T extends undefinedword123)] refine Record [v:T]`, true},
		// entry produces more than one value
		{`def X gen [(T Integer)] refine Record [v:T]`, true},
		// entry produces a single value that is neither a GenParam nor a
		// bare same-named placeholder
		{`def X gen [(T drop 5)] refine Record [v:T]`, true},
		// `(T)` — parens around a bare parameter: valid
		{`def X gen [(T)] refine Record [v:T]`, false},
		// extends bound is not a type
		{`def X gen [(T extends 5)] refine Record [v:T]`, true},
		// extends + default chain: valid
		{`def X gen [(T extends Integer default 0)] refine Record [v:T]`, false},
	}
	for _, c := range cases {
		_, err := w8Run(t, c.src)
		if c.wantErr && err == nil {
			t.Errorf("%q: expected error, got nil", c.src)
		}
		if !c.wantErr && err != nil {
			t.Errorf("%q: unexpected error: %v", c.src, err)
		}
	}
}

// --- extendsHandler / defaultBareHandler placeholder guards (direct) ---

func TestW8ExtendsHandlerNotPlaceholder(t *testing.T) {
	r := w8Reg(t)
	// args[1] is an Integer, not a gen placeholder -> TypeParamName == "".
	_, err := extendsHandler([]Value{NewTypeLiteral(TInteger), NewInteger(1)}, nil, nil, r)
	if err == nil {
		t.Fatal("extends: non-placeholder left side must error")
	}
}

func TestW8DefaultBareHandlerNotPlaceholder(t *testing.T) {
	r := w8Reg(t)
	_, err := defaultBareHandler([]Value{NewInteger(0), NewInteger(1)}, nil, nil, r)
	if err == nil {
		t.Fatal("default: non-placeholder left side must error")
	}
}

// --- ofHandler paths ---

func TestW8OfHandlerNonConcreteArgList(t *testing.T) {
	r := w8Reg(t)
	// args[0] a bare List literal -> not concrete.
	_, err := ofHandler([]Value{NewTypeLiteral(TList), NewTypeLiteral(TType)}, nil, nil, r)
	if err == nil {
		t.Fatal("of: non-concrete arg list must error")
	}
}

func TestW8OfTypeWrongArity(t *testing.T) {
	// `Type of [Integer String]` — two bounds is an arity error.
	if _, err := w8Run(t, `Type of [Integer String]`); err == nil {
		t.Fatal("of: Type with two bounds must error")
	}
}

func TestW8OfTypeBoundNominalNamed(t *testing.T) {
	// A nominal named type (whose body is a bare node) instantiates via
	// NewBoundedType(def) rather than NewBoundedTypeBody.
	out, err := w8Run(t, `def Foo Integer  Foo/t`)
	if err != nil {
		t.Fatalf("Foo/t: %v", err)
	}
	if len(out) != 1 || !strings.Contains(out[0].String(), "Foo") {
		t.Errorf("Foo/t should render a Type<Foo>, got %v", Canon(out))
	}
}

func TestW8OfTypeBoundNameFromWord(t *testing.T) {
	r := w8Reg(t)
	if _, err := w8RunOn(t, r, `def Foo Integer`); err != nil {
		t.Fatalf("def Foo: %v", err)
	}
	// Hand-build the `Type of [Foo]` call with Foo as a WORD element so the
	// boundName-from-Word branch runs.
	argList := NewList([]Value{NewWord("Foo")})
	out, err := ofHandler([]Value{argList, NewTypeLiteral(TType)}, nil, nil, r)
	if err != nil {
		t.Fatalf("of Type [word Foo]: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
}

func TestW8OfBareSchemaNode(t *testing.T) {
	r := w8Reg(t)
	if _, err := w8RunOn(t, r, `def Box gen [T] refine Record [value:T]`); err != nil {
		t.Fatalf("def Box: %v", err)
	}
	node := r.LookupTypeName("Box")
	if node == nil {
		t.Fatal("Box type node not found")
	}
	// Head is the bare schema NODE (Data==nil) rather than its TypeSchema
	// value, so the IsBareTypeNode + SchemaInfoOf branch runs.
	argList := NewList([]Value{NewTypeLiteral(TInteger)})
	out, err := ofHandler([]Value{argList, NewTypeLiteral(node)}, nil, nil, r)
	if err != nil {
		t.Fatalf("of bare schema node: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
}
