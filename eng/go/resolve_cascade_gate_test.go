package eng

import "testing"

// Gate tests for the canonical type-name cascade's less-travelled arms
// (ADR-012 rule 4) — each pins an arm the spec suites reach only
// through composed behaviour, paired with its negative.

// ResolveFieldType's builtin arm: a bare `{a:Integer}` field arrives
// as a Word post-opacity and resolves to the canonical literal; an
// unknown name passes through untouched (the caller's concern).
func TestResolveFieldTypeBuiltinArm(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	got := ResolveFieldType(r, NewWord("Integer"))
	if !IsBareTypeNode(got) || got.ID != TInteger.ID {
		t.Fatalf("ResolveFieldType(word(Integer)) = %s, want the Integer literal", got)
	}
	raw := NewWord("NoSuchTypeName")
	if got := ResolveFieldType(r, raw); !IsWord(got) {
		t.Fatalf("unknown name must pass through, got %s", got)
	}
}

// ResolveTypeLiteralDef's legacy bare-name arm: a Class-shaped value
// installed under a type's user-facing name (the RegisterResource-era
// shape) is picked up from the def store; an unbound name returns the
// literal unchanged.
func TestResolveTypeLiteralDefBareNameClassArm(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	fields := NewOrderedMap()
	fields.Set("kind", NewTypeLiteral(TString))
	cls := NewClassType(TInteger, ClassTypeInfo{Name: "Integer", Fields: fields})
	InstallDef(r, "Integer", cls)
	got := ResolveTypeLiteralDef(NewTypeLiteral(TInteger), r)
	if !IsClassType(got) {
		t.Fatalf("bare-name class binding must win, got %s", got)
	}
	// Negative: no binding → the literal passes through.
	if got := ResolveTypeLiteralDef(NewTypeLiteral(TFloat), r); !IsBareTypeNode(got) || got.ID != TFloat.ID {
		t.Fatalf("unbound literal must pass through, got %s", got)
	}
}

// ResolveSigChildParam's user-type body arm and the nested-unchanged
// short-circuit.
func TestResolveSigChildParamArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if _, err := r.DefineType("Foo", NewTypeLiteral(TInteger)); err != nil {
		t.Fatalf("DefineType: %v", err)
	}
	// User-type child resolves to the binding's body at sig install.
	got := ResolveSigChildParam(r, NewTypedList(NewWord("Foo")))
	ci, cerr := AsChildType(got)
	if cerr != nil || IsWord(ci.Child) {
		t.Fatalf("user-type child must resolve, got %s", got)
	}
	// Nested container whose inner child resolves to nothing: the
	// whole pattern is returned unchanged.
	raw := NewTypedList(NewTypedList(NewWord("NoSuchTypeName")))
	if got := ResolveSigChildParam(r, raw); !ExactEqual(got, raw) {
		t.Fatalf("unresolvable nested child must pass through, got %s", got)
	}
}
