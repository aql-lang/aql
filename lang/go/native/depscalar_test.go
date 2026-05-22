package native

import (
	"testing"
)

// --- General dependent types: DepScalar over any scalar base ---
//
// `Decimal gte 1.5`, `String lt "z"`, `Boolean eq true`, `Atom eq foo`
// each construct a DepScalar value living under Type/Dependent/Dep<X>
// where <X> is the leaf of the base type. The same machinery as
// DepInteger applies: the DepScalar matches the base type's lattice
// ancestors, and unifying it with a concrete value of the base type
// runs the comparison and returns the concrete value on success.

// --- Construction across base types ---

func TestNewDepScalarDecimal(t *testing.T) {
	d := NewDepScalar(DepGTE, NewDecimal(1.5))
	if d.Parent.String() != "Type/Dependent/DepDecimal" {
		t.Errorf("Parent = %s, want Type/Dependent/DepDecimal", d.Parent.String())
	}
	info, err := d.AsDepScalar()
	if err != nil {
		t.Fatalf("AsDepScalar: %v", err)
	}
	if info.Lo == nil || !info.Lo.Inclusive {
		t.Errorf("Lo = %+v, want inclusive lower bound (GTE)", info.Lo)
	}
	if info.Hi != nil {
		t.Errorf("Hi = %+v, want nil for single-bound DepScalar", info.Hi)
	}
	bv, _ := AsDecimal(info.Lo.Value)
	if bv != 1.5 {
		t.Errorf("Lo.Value = %v, want 1.5", bv)
	}
}

func TestNewDepScalarString(t *testing.T) {
	d := NewDepScalar(DepLT, NewString("z"))
	if d.Parent.String() != "Type/Dependent/DepString" {
		t.Errorf("Parent = %s, want Type/Dependent/DepString", d.Parent.String())
	}
	info, _ := d.AsDepScalar()
	if info.Hi == nil || info.Hi.Inclusive {
		t.Errorf("Hi = %+v, want strict upper bound (LT)", info.Hi)
	}
	if info.Lo != nil {
		t.Errorf("Lo = %+v, want nil for single-bound DepScalar", info.Lo)
	}
	bv, _ := AsString(info.Hi.Value)
	if bv != "z" {
		t.Errorf("Hi.Value = %q, want \"z\"", bv)
	}
}

func TestNewDepScalarBoolean(t *testing.T) {
	d := NewDepScalar(DepGTE, NewBoolean(true))
	if d.Parent.String() != "Type/Dependent/DepBoolean" {
		t.Errorf("Parent = %s, want Type/Dependent/DepBoolean", d.Parent.String())
	}
}

func TestNewDepScalarAtom(t *testing.T) {
	d := NewDepScalar(DepGTE, NewAtom("hello"))
	if d.Parent.String() != "Type/Dependent/DepAtom" {
		t.Errorf("Parent = %s, want Type/Dependent/DepAtom", d.Parent.String())
	}
}

// --- Lattice subtyping for each base ---

func TestDepDecimalMatchesDecimalAncestors(t *testing.T) {
	d := NewDepScalar(DepGTE, NewDecimal(0.0))
	for _, anc := range []*Type{TDecimal, TNumber, TScalar, TAny} {
		if !d.Parent.Matches(anc) {
			t.Errorf("DepDecimal does not match ancestor %s", anc)
		}
	}
}

func TestDepStringMatchesStringAncestors(t *testing.T) {
	d := NewDepScalar(DepLT, NewString("m"))
	for _, anc := range []*Type{TString, TScalar, TAny} {
		if !d.Parent.Matches(anc) {
			t.Errorf("DepString does not match ancestor %s", anc)
		}
	}
	// DepString must NOT match Number or Boolean.
	for _, foreign := range []*Type{TNumber, TInteger, TBoolean} {
		if d.Parent.Matches(foreign) {
			t.Errorf("DepString unexpectedly matches %s", foreign)
		}
	}
}

func TestDepAtomMatchesAtom(t *testing.T) {
	d := NewDepScalar(DepGTE, NewAtom("foo"))
	if !d.Parent.Matches(TAtom) {
		t.Errorf("DepAtom does not match TAtom")
	}
	if !d.Parent.Matches(TScalar) {
		t.Errorf("DepAtom does not match TScalar")
	}
}

// DepInteger continues to work via the new general path (sanity).
func TestDepIntegerStillMatchesInteger(t *testing.T) {
	d := NewDepScalar(DepGTE, NewInteger(10))
	for _, anc := range []*Type{TInteger, TNumber, TScalar, TAny} {
		if !d.Parent.Matches(anc) {
			t.Errorf("DepInteger no longer matches ancestor %s", anc)
		}
	}
}

// --- Unify across base types ---

func TestUnifyDepDecimal(t *testing.T) {
	d := NewDepScalar(DepGTE, NewDecimal(1.5))
	cases := []struct {
		val    float64
		expect bool
	}{
		{1.5, true},
		{2.0, true},
		{1.499, false},
		{0.0, false},
	}
	for _, tc := range cases {
		got, ok := Unify(NewDecimal(tc.val), d)
		if ok != tc.expect {
			t.Errorf("Unify(%v, Decimal gte 1.5) = %v; want %v", tc.val, ok, tc.expect)
		}
		if ok {
			f, _ := AsDecimal(got)
			if f != tc.val {
				t.Errorf("Unify result = %v, want %v", f, tc.val)
			}
		}
	}
}

func TestUnifyDepString(t *testing.T) {
	d := NewDepScalar(DepLT, NewString("z"))
	cases := []struct {
		val    string
		expect bool
	}{
		{"a", true},
		{"y", true},
		{"z", false},  // strict <
		{"za", false}, // lex order
	}
	for _, tc := range cases {
		_, ok := Unify(NewString(tc.val), d)
		if ok != tc.expect {
			t.Errorf("Unify(%q, String lt \"z\") = %v; want %v", tc.val, ok, tc.expect)
		}
	}
}

func TestUnifyDepBoolean(t *testing.T) {
	// `gte true` keeps only true (false < true).
	d := NewDepScalar(DepGTE, NewBoolean(true))
	if _, ok := Unify(NewBoolean(true), d); !ok {
		t.Errorf("Unify(true, Boolean gte true) failed; want success")
	}
	if _, ok := Unify(NewBoolean(false), d); ok {
		t.Errorf("Unify(false, Boolean gte true) succeeded; want failure")
	}
}

func TestUnifyDepAtom(t *testing.T) {
	d := NewDepScalar(DepGTE, NewAtom("m"))
	if _, ok := Unify(NewAtom("z"), d); !ok {
		t.Errorf("Unify('z', Atom gte 'm') failed; want success (lex)")
	}
	if _, ok := Unify(NewAtom("a"), d); ok {
		t.Errorf("Unify('a', Atom gte 'm') succeeded; want failure")
	}
}

// Cross-type unification fails: DepString vs Integer can't compare.
func TestUnifyDepRejectsCrossType(t *testing.T) {
	ds := NewDepScalar(DepLT, NewString("z"))
	if _, ok := Unify(NewInteger(5), ds); ok {
		t.Error("Unify(5, DepString) succeeded; want failure (cross-type)")
	}
	di := NewDepScalar(DepGTE, NewInteger(10))
	if _, ok := Unify(NewString("x"), di); ok {
		t.Error("Unify(\"x\", DepInteger) succeeded; want failure (cross-type)")
	}
}

// --- AQL-level construction across base types ---

func TestRunDecimalGTEReturnsDepDecimal(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewTypeLiteral(TDecimal),
		NewWord("gte"),
		NewDecimal(1.5),
	})
	if len(result) != 1 || result[0].Parent.String() != "Type/Dependent/DepDecimal" {
		t.Fatalf("Decimal gte 1.5: got %v, want DepDecimal", result)
	}
}

func TestRunStringLTReturnsDepString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewTypeLiteral(TString),
		NewWord("lt"),
		NewString("z"),
	})
	if len(result) != 1 || result[0].Parent.String() != "Type/Dependent/DepString" {
		t.Fatalf("String lt \"z\": got %v, want DepString", result)
	}
}

// `Atom gte 'm'` constructs a DepAtom. Use NewAtom directly because
// the parser would treat `m` as a Word.
func TestRunAtomGTEReturnsDepAtom(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewTypeLiteral(TAtom),
		NewWord("gte"),
		NewAtom("m"),
	})
	if len(result) != 1 || result[0].Parent.String() != "Type/Dependent/DepAtom" {
		t.Fatalf("Atom gte 'm': got %v, want DepAtom", result)
	}
}

// Mismatched bound type: `Integer gte 1.5` (Decimal bound) must error
// rather than silently building a DepInteger with a Decimal bound.
func TestDepConstructorRejectsMismatchedBound(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)
	_, err = e.Run([]Value{
		NewTypeLiteral(TInteger),
		NewWord("gte"),
		NewDecimal(1.5),
	})
	if err == nil {
		t.Fatal("expected type-mismatch error from Integer gte 1.5")
	}
}

// `is` overload across base types still works.
func TestIsCheckWithDepDecimal(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewDecimal(2.0),
		NewWord("is"),
		NewOpenParen(),
		NewTypeLiteral(TDecimal),
		NewWord("gte"),
		NewDecimal(1.5),
		NewCloseParen(),
	})
	got, _ := AsBoolean(result[0])
	if !got {
		t.Errorf("2.0 is (Decimal gte 1.5) = false; want true")
	}
}

func TestIsCheckWithDepString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewString("apple"),
		NewWord("is"),
		NewOpenParen(),
		NewTypeLiteral(TString),
		NewWord("lt"),
		NewString("banana"),
		NewCloseParen(),
	})
	got, _ := AsBoolean(result[0])
	if !got {
		t.Errorf("\"apple\" is (String lt \"banana\") = false; want true")
	}
}
