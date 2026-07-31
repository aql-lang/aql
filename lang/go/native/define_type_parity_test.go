package native

import (
	"testing"

	"github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
)

// installBORUType runs `def Name <bodySrc>` and returns the minted *Type,
// so a Go-defined type can be compared against its BORU twin.
func installBORUType(t *testing.T, r *Registry, name, bodySrc string) *eng.Type {
	t.Helper()
	toks, err := parser.Parse("def " + name + " " + bodySrc)
	if err != nil {
		t.Fatalf("parse def %s: %v", name, err)
	}
	if _, err := NewTop(r).Run(toks); err != nil {
		t.Fatalf("install BORU type %s: %v", name, err)
	}
	got := r.LookupTypeName(name)
	if got == nil {
		t.Fatalf("BORU type %s did not install", name)
	}
	return got
}

// assertParity checks that a BORU-defined type and a Go-defined type
// answer `is` identically for every probe value.
func assertParity(t *testing.T, label string, boruT, goT *eng.Type, probes []Value) {
	t.Helper()
	for _, v := range probes {
		if a, g := v.Is(boruT), v.Is(goT); a != g {
			t.Errorf("%s: membership disagrees for %s: BORU=%v Go=%v", label, v.String(), a, g)
		}
	}
}

// TestDefineTypeGoBoruParity is the construction-axis convergence proof:
// types built from Go via (*Registry).DefineType / DefineEnum — which
// route through the SAME InstallType the `def` word uses — behave
// identically to their BORU `def` twins under `is`. Covers a value enum,
// a type union, and a negation.
func TestDefineTypeGoBoruParity(t *testing.T) {
	probes := []Value{
		NewInteger(3), NewInteger(0), NewString("x"), NewString(""),
		NewBoolean(true), NewAtom("red"), NewAtom("green"), NewAtom("blue"),
	}

	t.Run("value enum (tor of atoms)", func(t *testing.T) {
		r, _ := DefaultRegistry()
		boru := installBORUType(t, r, "ColorA", "(red/q tor green/q)")
		// Go twin: the same closed set of atom values.
		go_, err := r.DefineEnum("ColorG", eng.NewAtom("red"), eng.NewAtom("green"))
		if err != nil {
			t.Fatalf("DefineEnum: %v", err)
		}
		assertParity(t, "atom-enum", boru, go_, probes)
		// Spot-check the verdicts are the intended ones.
		if !NewAtom("red").Is(go_) || NewAtom("blue").Is(go_) || NewInteger(3).Is(go_) {
			t.Error("Go atom enum gave an unexpected verdict")
		}
	})

	t.Run("type union (tor of types)", func(t *testing.T) {
		r, _ := DefaultRegistry()
		boru := installBORUType(t, r, "NumOrStrA", "(Integer tor String)")
		go_, err := r.DefineEnum("NumOrStrG",
			eng.NewTypeLiteral(eng.TInteger), eng.NewTypeLiteral(eng.TString))
		if err != nil {
			t.Fatalf("DefineEnum: %v", err)
		}
		assertParity(t, "type-union", boru, go_, probes)
		if !NewInteger(3).Is(go_) || !NewString("x").Is(go_) || NewBoolean(true).Is(go_) {
			t.Error("Go type union gave an unexpected verdict")
		}
	})

	t.Run("negation (tnot)", func(t *testing.T) {
		r, _ := DefaultRegistry()
		boru := installBORUType(t, r, "NotStrA", "(tnot String)")
		go_, err := r.DefineType("NotStrG", eng.NewNegation(eng.NewTypeLiteral(eng.TString)))
		if err != nil {
			t.Fatalf("DefineType: %v", err)
		}
		assertParity(t, "negation", boru, go_, probes)
		if NewString("x").Is(go_) || !NewInteger(3).Is(go_) {
			t.Error("Go negation gave an unexpected verdict")
		}
	})
}

// TestDefineTypeRejectsBadName confirms DefineType surfaces InstallType's
// validation rather than minting a half-built node: a type name must be
// capitalised, exactly as the `def` word requires.
func TestDefineTypeRejectsBadName(t *testing.T) {
	r, _ := DefaultRegistry()
	if _, err := r.DefineType("lowercase", eng.NewTypeLiteral(eng.TInteger)); err == nil {
		t.Error("DefineType accepted a lowercase type name")
	}
}
