package core

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached error arms in fn_def.go — the ParseFnReturns / ParseFnParams
// error propagation and the full-path type-name recognition. Per
// design/TEST-SEAMS.10.md.

import "testing"

func TestW8ParseFnDefBadReturn(t *testing.T) {
	r := w8reg(t)
	// Output sig mixes a real type (forcing the type-return path) with an
	// unknown type-name word, so ParseFnReturns errors.
	list := []Value{
		NewList([]Value{NewTypeLiteral(TInteger)}),
		NewList([]Value{NewTypeLiteral(TInteger), NewWord("__W8Bad__")}),
		NewList([]Value{NewInteger(1)}),
	}
	if _, err := ParseFnDef(r, list); err == nil {
		t.Fatal("an unknown return type must error")
	}
}

func TestW8ParseFnUndefSpecBadParam(t *testing.T) {
	r := w8reg(t)
	list := []Value{
		NewList([]Value{NewWord("__W8NoType__")}),
		NewList([]Value{NewTypeLiteral(TInteger)}),
	}
	if _, err := ParseFnUndefSpec(r, list); err == nil {
		t.Fatal("an unknown param type must error")
	}
}

func TestW8IsSigTypeNameFullPath(t *testing.T) {
	// A full lattice path is not in the short-name table, but resolves via
	// ResolveTypePath.
	if !isSigTypeName(nil, "Scalar/Number/Integer") {
		t.Fatal("a full type path must be recognised as a sig type name")
	}
}
