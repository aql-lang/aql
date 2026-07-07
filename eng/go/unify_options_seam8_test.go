package eng

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached branches in unify_options.go. The family handler and its
// helpers (optionsDefault / optionsBaseType) are pure — driven by direct
// calls with crafted Options / Map / Disjunct values. Per
// design/TEST-SEAMS.10.md.

import "testing"

func w8opts(pairs ...Value) Value {
	m := NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		key, _ := AsString(pairs[i])
		m.Set(key, pairs[i+1])
	}
	return NewOptionsType(m)
}

func TestW8UnifyOptionsPairSuccess(t *testing.T) {
	a := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	b := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	got, err := unifyOptionsFamily(a, Shape(a), b, Shape(b))
	if err != nil {
		t.Fatalf("two compatible Options must unify: %v", err)
	}
	if !IsOptionsType(got) {
		t.Fatalf("result should be an Options type, got %v", got)
	}
}

func TestW8UnifyOptionsPairFieldBagError(t *testing.T) {
	// Same field-count, different key → unifyFieldBags reports the key
	// missing on the right (also covers unify_map.go's missing-key arm).
	a := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	b := w8opts(NewString("y"), NewTypeLiteral(TInteger))
	if _, err := unifyOptionsFamily(a, Shape(a), b, Shape(b)); err == nil {
		t.Fatal("Options with mismatched keys must fail to unify")
	}
}

func TestW8UnifyOptionsBareMapLiteral(t *testing.T) {
	// Options vs a bare Map type literal preserves the Options schema.
	opts := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	lit := NewTypeLiteral(TMap)
	got, err := unifyOptionsFamily(opts, Shape(opts), lit, Shape(lit))
	if err != nil {
		t.Fatalf("Options vs Map literal: %v", err)
	}
	if !IsOptionsType(got) {
		t.Fatalf("expected Options schema preserved, got %v", got)
	}
}

func TestW8UnifyOptionsRejectsStructuralMap(t *testing.T) {
	// Options only unifies with a plain concrete Map, never a Record.
	opts := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	rf := NewOrderedMap()
	rf.Set("x", NewTypeLiteral(TInteger))
	rec := NewRecordType(rf)
	if !IsConcrete(rec) {
		t.Fatal("precondition: record type value must be concrete for this arm")
	}
	if _, err := unifyOptionsFamily(opts, Shape(opts), rec, Shape(rec)); err == nil {
		t.Fatal("Options must not unify with a Record")
	}
}

func TestW8UnifyOptionsConcreteOnRight(t *testing.T) {
	// sa==ShapeOptions branch with a concrete map on the right: the
	// present key unifies against its Options field constraint.
	opts := w8opts(NewString("x"), NewTypeLiteral(TInteger))
	cm := NewOrderedMap()
	cm.Set("x", NewInteger(7))
	concrete := NewMap(cm)
	got, err := unifyOptionsFamily(opts, Shape(opts), concrete, Shape(concrete))
	if err != nil {
		t.Fatalf("Options vs concrete map: %v", err)
	}
	m, _ := AsMap(got)
	if v, ok := m.Get("x"); !ok {
		t.Fatal("expected key x in result")
	} else if n, _ := AsInteger(v); n != 7 {
		t.Fatalf("expected x=7, got %d", n)
	}
}

// --- optionsDefault ---------------------------------------------------------

func TestW8OptionsDefaultDisjunctConcreteAlt(t *testing.T) {
	// No None alternative, but a concrete alternative → it is the default.
	d := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewInteger(42)})
	got, ok := optionsDefault(d)
	if !ok {
		t.Fatal("disjunct with a concrete alternative must yield a default")
	}
	if n, _ := AsInteger(got); n != 42 {
		t.Fatalf("expected default 42, got %d", n)
	}
}

func TestW8OptionsDefaultDisjunctNoConcrete(t *testing.T) {
	// No None and no concrete alternative → no default.
	d := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	if _, ok := optionsDefault(d); ok {
		t.Fatal("disjunct of bare type literals has no default")
	}
}

func TestW8OptionsDefaultNoneValue(t *testing.T) {
	// A bare None literal defaults to itself.
	got, ok := optionsDefault(NewTypeLiteral(TNone))
	if !ok {
		t.Fatal("None must be its own default")
	}
	if !IsNoneShape(got) {
		t.Fatalf("expected None default, got %v", got)
	}
}

// --- optionsBaseType --------------------------------------------------------

func TestW8OptionsBaseType(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		want *Type
	}{
		{"float", NewFloat(1.5), TFloat},
		{"string", NewString("s"), TString},
		{"boolean", NewBoolean(true), TBoolean},
		{"map", NewMap(NewOrderedMap()), TMap},
		{"list", NewList(nil), TList},
		{"none", NewNone(), TNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := optionsBaseType(c.v); !got.Equal(c.want) {
				t.Fatalf("optionsBaseType(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
	// Default arm: an Atom is not one of the well-known bases, so its
	// own Parent is returned unchanged.
	atom := NewAtom("a")
	if got := optionsBaseType(atom); !got.Equal(atom.Parent) {
		t.Fatalf("default arm should return the value's Parent, got %v", got)
	}
}
