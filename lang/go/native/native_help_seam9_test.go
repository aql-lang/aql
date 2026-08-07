package native

import (
	"testing"

	"github.com/boru-lang/boru/parser/go"
)

// Seam-9 coverage (W9_nativeC) for native_help.go: the dynamic-help hook
// arms, the makeDynamicEval ParseFunc/parse-error arms, and the
// inferReturns / FnDefFuncInfo edge arms — driven directly (FS-hermetic).

func TestW9EnableDynamicHelpHookArms(t *testing.T) {
	// info == nil arm (17.18,19.4): an unregistered, non-def name.
	r := seam5Reg(t)
	EnableDynamicHelp(r)
	r.OnRegisterHook("this-name-is-not-registered-anywhere")

	// eval == nil arm (21.18,23.4): a registered word but no ParseFunc, so
	// makeDynamicEval returns nil.
	r2, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	EnableDynamicHelp(r2)
	r2.OnRegisterHook("add") // add resolves (info != nil) but eval is nil
}

func TestW9MakeDynamicEval(t *testing.T) {
	// ParseFunc == nil → nil (32.24,34.3).
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if makeDynamicEval(r) != nil {
		t.Fatal("makeDynamicEval must return nil when ParseFunc is unset")
	}

	// The eval closure surfaces a parse error (37.17,39.4).
	r.SetParseFunc(parser.Parse)
	eval := makeDynamicEval(r)
	if eval == nil {
		t.Fatal("makeDynamicEval must return a func when ParseFunc is set")
	}
	if _, err := eval("("); err == nil {
		t.Fatal("eval of unparseable source must return the parse error")
	}
}

func TestW9BuildFuncInfoPlainDef(t *testing.T) {
	// A plain def (not a fn) returns a FuncInfo carrying only its help
	// Entry (216.23,221.4).
	r := seam5Reg(t)
	if _, err := seam5Run(r, `def myval 42`); err != nil {
		t.Fatalf("def: %v", err)
	}
	info := BuildFuncInfo(r, "myval")
	if info == nil || info.Name != "myval" {
		t.Fatalf("BuildFuncInfo(def) = %v", info)
	}
}

func TestW9BuildQualifiedFuncInfoNonFn(t *testing.T) {
	// A module export that is a plain value (not a function) yields nil
	// from the qualified-describe path (141.9,143.3).
	r := seam5Reg(t)
	if _, err := seam5Run(r, `import module [export "Foo" {a:1}]`); err != nil {
		t.Fatalf("import module: %v", err)
	}
	if info := BuildQualifiedFuncInfo(r, "Foo.a"); info != nil {
		t.Fatalf("BuildQualifiedFuncInfo(non-fn export) = %v, want nil", info)
	}
}

func TestW9FnDefFuncInfoInferReturns(t *testing.T) {
	// A core word whose first sig carries no authored Returns takes the
	// inferReturns else branch (188.9,190.4).
	r := seam5Reg(t)
	fn := r.Lookup("add")
	if fn == nil {
		t.Fatal("add must be registered")
	}
	info := FnDefFuncInfo("add", fn)
	if info == nil || len(info.Sigs) == 0 {
		t.Fatalf("FnDefFuncInfo(add) = %v", info)
	}
}

func TestW9FnDefForwardEligibleAllStack(t *testing.T) {
	// Every sig pinned to BarrierPos 0 → not forward-eligible (208.2,208.14).
	fn := &FnDefInfo{Signatures: []Signature{{BarrierPos: 0}, {BarrierPos: 0}}}
	if fnDefForwardEligible(fn) {
		t.Fatal("all-BarrierPos-0 fn must not be forward-eligible")
	}
	// Sanity: a nonzero barrier flips it true.
	fn2 := &FnDefInfo{Signatures: []Signature{{BarrierPos: 0}, {BarrierPos: 2}}}
	if !fnDefForwardEligible(fn2) {
		t.Fatal("a nonzero-barrier sig must be forward-eligible")
	}
}

func TestW9InferReturnsHelpers(t *testing.T) {
	// inferExact: math-pi / math-e constant words (398.27,399.41).
	if got := inferExact("math-pi", Signature{}); len(got) != 1 || got[0] != "Scalar/Number/Float" {
		t.Fatalf("inferExact(math-pi) = %v", got)
	}
	if got := inferExact("math-e", Signature{}); len(got) != 1 {
		t.Fatalf("inferExact(math-e) = %v", got)
	}

	// inferArithReturns: wrong arity → nil (472.30,474.3).
	if got := inferArithReturns("add", Signature{}); got != nil {
		t.Fatalf("inferArithReturns(non-2-arg) = %v, want nil", got)
	}
	// inferArithReturns: add with two Scalar args → String (478.55,480.3).
	scalarSig := Signature{Args: []*Type{TScalar, TScalar}}
	if got := inferArithReturns("add", scalarSig); len(got) != 1 || got[0] != "Scalar/String" {
		t.Fatalf("inferArithReturns(add, Scalar Scalar) = %v", got)
	}
}
