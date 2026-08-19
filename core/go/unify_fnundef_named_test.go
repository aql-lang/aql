package core

import "testing"

// Named fn-shape types — the FnUndefUnifier that lets DISPATCH consult
// the same structural rule `is` already consulted (unify_fnundef_named.go).
// These are core-level unit tests: the surface spelling
// (`def IntToStr fnsig Integer String`) is a `basic` word, so core's own
// suite drives the unifier directly.

// intToStrSpec is the shape `Integer -> String`.
func intToStrSpec() FnSigSpec {
	return FnSigSpec{
		Params:  []FnParam{{Name: "x", Type: TInteger}},
		Returns: []*Type{TString},
	}
}

// fnValueWith builds a function VALUE carrying one signature.
func fnValueWith(name string, params []FnParam, returns []*Type) Value {
	return NewFunction(FnDefInfo{
		Name:       name,
		Signatures: []Signature{{Params: params, Returns: returns}},
	})
}

func TestFnUndefUnifierMatchesOnShape(t *testing.T) {
	def := MintTestType("FunctionSignature/FnUIntToStr")
	installFnUndefUnifier(def, []FnSigSpec{intToStrSpec()}, "FnUIntToStr")

	b := def.Behavior()
	fu, ok := b.(*FnUndefUnifier)
	if !ok {
		t.Fatalf("Behavior = %T, want *FnUndefUnifier", b)
	}
	// ContentMembership is the marker declaring that membership is a
	// property of the value's signatures, not its lattice position.
	fu.ContentMembership()

	good := fnValueWith("good", []FnParam{{Name: "x", Type: TInteger}}, []*Type{TString})
	if !fu.Match(good, def) {
		t.Error("Integer -> String must satisfy the Integer -> String shape")
	}

	// Wrong direction: String -> Integer is not substitutable.
	bad := fnValueWith("bad", []FnParam{{Name: "s", Type: TString}}, []*Type{TInteger})
	if fu.Match(bad, def) {
		t.Error("String -> Integer must NOT satisfy Integer -> String")
	}

	// Wrong arity.
	two := fnValueWith("two",
		[]FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		[]*Type{TString})
	if fu.Match(two, def) {
		t.Error("a 2-parameter fn must NOT satisfy a 1-parameter shape")
	}

	// A non-function carries no signatures at all.
	if fu.Match(NewInteger(42), def) {
		t.Error("a non-function must NOT satisfy an fn shape")
	}
}

// Variance: parameters are contravariant, returns covariant. A function
// accepting MORE and returning LESS is substitutable; the reverse is not.
func TestFnUndefUnifierVariance(t *testing.T) {
	spec := FnSigSpec{
		Params:  []FnParam{{Name: "x", Type: TInteger}},
		Returns: []*Type{TNumber},
	}
	def := MintTestType("FunctionSignature/FnUIntToNum")
	installFnUndefUnifier(def, []FnSigSpec{spec}, "FnUIntToNum")
	fu, _ := def.Behavior().(*FnUndefUnifier)

	wider := fnValueWith("wider", []FnParam{{Name: "x", Type: TNumber}}, []*Type{TInteger})
	if !fu.Match(wider, def) {
		t.Error("Number -> Integer must satisfy Integer -> Number (contra/co)")
	}

	narrower := fnValueWith("narrower", []FnParam{{Name: "x", Type: TInteger}}, []*Type{TNumber})
	revDef := MintTestType("FunctionSignature/FnUNumToInt")
	installFnUndefUnifier(revDef, []FnSigSpec{{
		Params:  []FnParam{{Name: "x", Type: TNumber}},
		Returns: []*Type{TInteger},
	}}, "FnUNumToInt")
	revFu, _ := revDef.Behavior().(*FnUndefUnifier)
	if revFu.Match(narrower, revDef) {
		t.Error("Integer -> Number must NOT satisfy Number -> Integer")
	}
}

// A bare type literal is the TYPE itself, not an inhabitant, so it
// passes through to the wrapped Behavior — mirroring DisjunctUnifier.
func TestFnUndefUnifierBareTypeNodeDelegates(t *testing.T) {
	def := MintTestType("FunctionSignature/FnUBare")
	installFnUndefUnifier(def, []FnSigSpec{intToStrSpec()}, "FnUBare")
	fu, _ := def.Behavior().(*FnUndefUnifier)

	// The delegated walk decides; the point under test is that the
	// signature check is NOT applied to a bare node (which carries no
	// signatures and would otherwise always fail).
	got := fu.Match(NewTypeLiteral(def), def)
	want := baseBehavior(fu.prev).Match(NewTypeLiteral(def), def)
	if got != want {
		t.Errorf("bare type node: got %v, want the delegated %v", got, want)
	}
}

// A carrier is an abstract value with no signatures to inspect. Admit it
// when it could still be a function — the sound over-approximation, so
// the checker does not reject a program the runtime accepts.
func TestFnUndefUnifierAdmitsFunctionCarriers(t *testing.T) {
	def := MintTestType("FunctionSignature/FnUCarrier")
	installFnUndefUnifier(def, []FnSigSpec{intToStrSpec()}, "FnUCarrier")
	fu, _ := def.Behavior().(*FnUndefUnifier)

	if !fu.Match(NewCarrier(TFunction), def) {
		t.Error("a Function carrier must be admitted (could still be a match)")
	}
	if !fu.Match(NewCarrier(TAny), def) {
		t.Error("an Any carrier must be admitted (could still be a function)")
	}
	if fu.Match(NewCarrier(TInteger), def) {
		t.Error("an Integer carrier can never be a function — reject")
	}
}

// InstallType's FnUndef branch: `def IntToStr (fnsig …)` must mint under
// the body's lattice AND attach the unifier, so dispatch works. Without
// the branch the node is minted bare and every function is rejected.
func TestInstallTypeFnUndefAttachesUnifier(t *testing.T) {
	r := newTestRegistry(t)
	body := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{intToStrSpec()}})
	if err := InstallType(r, "FnUInstalled", body); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	entry, ok := r.Defs.TopEntry("FnUInstalled")
	if !ok || entry.TypeDef == nil {
		t.Fatal("FnUInstalled was not installed as a type")
	}
	def := entry.TypeDef
	fu, ok := def.Behavior().(*FnUndefUnifier)
	if !ok {
		t.Fatalf("Behavior = %T, want *FnUndefUnifier — dispatch would reject every fn", def.Behavior())
	}
	if fu.typeName != "FnUInstalled" {
		t.Errorf("typeName = %q, want FnUInstalled", fu.typeName)
	}
	good := fnValueWith("g", []FnParam{{Name: "x", Type: TInteger}}, []*Type{TString})
	if !fu.Match(good, def) {
		t.Error("installed type must admit a matching function")
	}
}
