package core

import (
	"strings"
	"testing"
)

// Unit pins for the output-slot rework (design/FN-OUTPUT-SIG.0.md): the
// `name:Type` return unwrap, the declaration-span fallback that makes the
// two spellings of one declaration diagnose alike, and the ADR-017 value
// renderer. The end-to-end behaviour lives in lang/spec/fn-triple.tsv §7;
// these cover the arms a spec row cannot reach.

// implicitMap builds the value a `name:Type` pair lowers to.
func implicitMap(pairs ...Value) Value {
	om := NewOrderedMap()
	om.Implicit = true
	for i := 0; i+1 < len(pairs); i += 2 {
		k, _ := AsString(pairs[i])
		om.Set(k, pairs[i+1])
	}
	return NewMap(om)
}

func TestUnwrapNamedReturnImplicitPair(t *testing.T) {
	sig := implicitMap(NewString("i"), NewTypeLiteral(TInteger))
	got, err := unwrapNamedReturn(sig)
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if !IsBareTypeNode(got) || !ValueType(got).Equal(TInteger) {
		t.Fatalf("unwrapped = %v, want the Integer type literal", got)
	}
}

func TestUnwrapNamedReturnLeavesEverythingElse(t *testing.T) {
	// A non-map value is returned untouched.
	word := NewWord("Integer")
	if got, err := unwrapNamedReturn(word); err != nil || !IsWord(got) {
		t.Errorf("a word must pass through: %v, %v", got, err)
	}
	// An EXPLICIT map declares a Map-typed return — not a named one — so
	// it must survive the unwrap intact.
	om := NewOrderedMap()
	om.Set("i", NewTypeLiteral(TInteger))
	explicit := NewMap(om)
	got, err := unwrapNamedReturn(explicit)
	if err != nil {
		t.Fatalf("explicit map: %v", err)
	}
	if !got.Parent.Equal(TMap) {
		t.Errorf("an explicit map must pass through, got %v", got)
	}
	// A Map-typed value with no payload takes the same early return.
	if got, err := unwrapNamedReturn(NewTypeLiteral(TMap)); err != nil || !IsBareTypeNode(got) {
		t.Errorf("a bare Map type node must pass through: %v, %v", got, err)
	}
}

func TestUnwrapNamedReturnErrors(t *testing.T) {
	two := implicitMap(
		NewString("i"), NewTypeLiteral(TInteger),
		NewString("j"), NewTypeLiteral(TString),
	)
	if _, err := unwrapNamedReturn(two); err == nil ||
		!strings.Contains(err.Error(), "exactly one key") {
		t.Errorf("a two-key return map must error, got %v", err)
	}

	bad := implicitMap(NewString("2bad"), NewTypeLiteral(TInteger))
	if _, err := unwrapNamedReturn(bad); err == nil {
		t.Error("an invalid return name must error")
	}
}

func TestParseFnReturnsNamedPair(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Bare pair, and the same pair wrapped in a list: one declaration,
	// two spellings, one reading.
	pair := implicitMap(NewString("i"), NewWord("Integer"))
	for _, sig := range []Value{pair, NewList([]Value{pair})} {
		types, pats, perr := ParseFnReturns(r, sig)
		if perr != nil {
			t.Fatalf("ParseFnReturns(%v): %v", sig, perr)
		}
		if len(types) != 1 || !types[0].Equal(TInteger) {
			t.Errorf("types = %v, want [Integer]", types)
		}
		if pats != nil {
			t.Errorf("a named type return carries no pattern, got %v", pats)
		}
	}

	// The error arm propagates out of the list walk.
	twoKey := implicitMap(
		NewString("i"), NewWord("Integer"),
		NewString("j"), NewWord("String"),
	)
	if _, _, perr := ParseFnReturns(r, NewList([]Value{twoKey})); perr == nil {
		t.Error("a bad named return inside a list must propagate")
	}
}

func TestSigDeclPos(t *testing.T) {
	// A slot with its own position answers with it.
	own := NewList([]Value{NewWord("Integer")})
	own.SetPos(SrcPos{Row: 3, Col: 7})
	if p := sigDeclPos(own); p.Row != 3 || p.Col != 7 {
		t.Errorf("own position = %+v, want 3:7", p)
	}

	// A `name:Type` pair has none, so the span falls to the declared type
	// inside it — the fix that stops the pair spelling losing the
	// declaration span the list spelling gets.
	typ := NewWord("Integer")
	typ.SetPos(SrcPos{Row: 1, Col: 22})
	if p := sigDeclPos(implicitMap(NewString("i"), typ)); p.Row != 1 || p.Col != 22 {
		t.Errorf("map descent = %+v, want 1:22", p)
	}

	// A positionless LIST descends to its first positioned element, and
	// skips the unpositioned ones ahead of it.
	nested := NewList([]Value{NewWord("String"), implicitMap(NewString("i"), typ)})
	if p := sigDeclPos(nested); p.Row != 1 || p.Col != 22 {
		t.Errorf("list descent = %+v, want 1:22", p)
	}

	// Nothing positioned anywhere: unknown, and attachDeclSpan drops the
	// span rather than pointing at 0:0.
	if p := sigDeclPos(NewList([]Value{NewWord("Integer")})); p.Row != 0 {
		t.Errorf("positionless sig = %+v, want the zero SrcPos", p)
	}
	if p := sigDeclPos(NewTypeLiteral(TInteger)); p.Row != 0 {
		t.Errorf("bare type node = %+v, want the zero SrcPos", p)
	}
}

func TestReturnCountErrorTextShowsValues(t *testing.T) {
	// ADR-017: the values ride in the detail.
	got := ReturnCountErrorText("f", 1, 2, []Value{NewInteger(1), NewString("x")})
	if !strings.Contains(got, "got 2 — [1 'x']") {
		t.Errorf("detail = %q, want the values shown", got)
	}
	// No values is a real answer: the bare count, no empty brackets.
	if got := ReturnCountErrorText("f", 1, 0, nil); !strings.HasSuffix(got, "got 0") {
		t.Errorf("valueless detail = %q, want the bare count", got)
	}
}

func TestDiagValueListAbbreviates(t *testing.T) {
	if got := diagValueList(nil); got != "[]" {
		t.Errorf("empty = %q", got)
	}
	vals := make([]Value, diagMaxListHead+3)
	for i := range vals {
		vals[i] = NewInteger(int64(i))
	}
	got := diagValueList(vals)
	if !strings.Contains(got, "… (3 more)") {
		t.Errorf("over-limit run must elide its tail, got %q", got)
	}
	if strings.Contains(got, strings.TrimSpace(NewInteger(int64(diagMaxListHead)).String())+" ") {
		t.Errorf("elided values must not be rendered, got %q", got)
	}
}

func TestResolveSigTypeStringShapeGate(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// A String naming a real type still resolves as a NAME.
	if got, pat, rerr := ResolveSigType(r, NewString("Integer")); rerr != nil ||
		pat != nil || !got.Equal(TInteger) {
		t.Errorf("'Integer' = %v/%v/%v, want the Integer type", got, pat, rerr)
	}
	// A String that could never be a type name is the literal itself.
	got, pat, rerr := ResolveSigType(r, NewString("ok"))
	if rerr != nil || pat == nil || !got.Equal(TString) {
		t.Fatalf("'ok' = %v/%v/%v, want a String literal pattern", got, pat, rerr)
	}
	if s, aerr := AsString(*pat); aerr != nil || s != "ok" {
		t.Errorf("'ok' pattern = %v (%v)", *pat, aerr)
	}
	// A MISSPELLED type name has the shape of one, so it stays a loud
	// error rather than silently becoming a literal that matches nothing.
	if _, _, rerr := ResolveSigType(r, NewString("Integr")); rerr == nil {
		t.Error("'Integr' must stay an unknown-type error, not become a literal")
	}
	// A word is always a name, whatever its shape.
	if _, _, rerr := ResolveSigType(r, NewWord("ok")); rerr == nil {
		t.Error("a bare lowercase word in a type slot must error")
	}
}

func TestLooksLikeTypeName(t *testing.T) {
	for _, c := range []struct {
		name string
		want bool
	}{
		{"Integer", true},
		{"Scalar/Number/Integer", true},
		{"ok", false},
		{"Scalar/number", false},
		{"42", false},
		{"", false},
	} {
		if got := looksLikeTypeName(c.name); got != c.want {
			t.Errorf("looksLikeTypeName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
