// Guard tests for the FnSig/NativeSig argument-ordering unification
// (design/SIG-ORDER-REFACTOR.0.md). These pin behavior that must
// survive the refactor (top-first via matchSignature) and lock in the
// fix for the module-closure branch in execFnDefLiteral.
//
// Pre-refactor state:
//   - AQL named-param fns dispatch top-first via matchSignature (PASS).
//   - AQL unnamed-param fns dispatch top-first via matchSignature (PASS).
//   - Module wrappers with heterogeneous-type Params that mirror the
//     inner native's Args (top-first) FAIL to dispatch because the
//     module-closure branch in execFnDefLiteral re-matches via
//     execFnDefSigStackMatch under a bottom-first convention.
//
// Post-refactor:
//   - All four cases pass; module wrappers no longer need their Params
//     reversed.
package test

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
)

func setupRandReg(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	if err := modules.InstallRandExports(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func runSrc(t *testing.T, r *native.Registry, src string) ([]native.Value, error) {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	e := native.NewTop(r)
	return e.Run(vals)
}

// TestSigOrder_NamedAqlFn_TopFirst pins the current behavior: an AQL
// fn with named heterogeneous params binds the FIRST source param to
// the TOP of the outer stack. Must pass before AND after the refactor.
func TestSigOrder_NamedAqlFn_TopFirst(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	// def f fn [[a:Integer b:String] ...] "hello" 42 f
	// Stack at call: ["hello", 42]. Top=42 (Integer). matchSignature
	// is top-first, so a=42, b="hello". Body pushes a then b.
	res, runErr := runSrc(t, r, `def f fn [[a:Integer b:String] [Integer String] [a b]]  "hello" 42 f`)
	if runErr != nil {
		t.Fatalf("dispatch failed: %v", runErr)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 values, got %d (%v)", len(res), res)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 42 {
		t.Errorf("res[0] = %v, want 42 (a bound to top)", res[0])
	}
	if s, _ := res[1].AsConcreteString(); s != "hello" {
		t.Errorf("res[1] = %v, want \"hello\" (b bound to deeper)", res[1])
	}
}

// TestSigOrder_NamedAqlFn_RejectsBottomFirst confirms the inverse: a
// call that would only succeed under bottom-first matching is
// correctly rejected. Same invariant — must hold pre and post.
func TestSigOrder_NamedAqlFn_RejectsBottomFirst(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	// 42 "hello" f puts String on top. sig[0]=Integer should reject.
	_, runErr := runSrc(t, r, `def f fn [[a:Integer b:String] [Integer String] [a b]]  42 "hello" f`)
	if runErr == nil {
		t.Fatal("expected signature_error; got success")
	}
}

// TestSigOrder_UnnamedAqlFn_TopFirst pins unnamed-param AQL fns are
// also top-first via matchSignature on the compiled Signatures. args.0
// references the i-th sig position counted from the stack top.
func TestSigOrder_UnnamedAqlFn_TopFirst(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	// def h fn [[Integer String] [Integer String] [args.0 args.1]] "x" 5 h
	// Stack: ["x", 5]. Top=5. args.0=5, args.1="x".
	res, runErr := runSrc(t, r, `def h fn [[Integer String] [Integer String] [args.0 args.1]]  "x" 5 h`)
	if runErr != nil {
		t.Fatalf("dispatch failed: %v", runErr)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 values, got %d (%v)", len(res), res)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 5 {
		t.Errorf("res[0] = %v, want 5 (args.0 = top)", res[0])
	}
	if s, _ := res[1].AsConcreteString(); s != "x" {
		t.Errorf("res[1] = %v, want \"x\" (args.1 = deeper)", res[1])
	}
}

// TestSigOrder_ModuleWrapper_NaturalParams pins module-wrapper
// dispatch on top-first sig order. The wrapper's FnSig.Params match
// the inner native's NativeSig.Args order (top-first per the
// canonical forward call form `rand.string CHARSET LENGTH`).
// execFnDefLiteral's trivial-delegation short-circuit routes the call
// straight to the inner native via execMatch.
func TestSigOrder_ModuleWrapper_NaturalParams(t *testing.T) {
	r := setupRandReg(t)
	// Forward form: `rand.string "abc" 10` → charset="abc", length=10.
	res, runErr := runSrc(t, r, `def s (rand.with-seed 1)  (s.string "abc" 10)`)
	if runErr != nil {
		t.Fatalf("dispatch failed: %v", runErr)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 string, got %d (%v)", len(res), res)
	}
	s, err := res[0].AsConcreteString()
	if err != nil {
		t.Fatalf("not a string: %v", err)
	}
	if len(s) != 10 {
		t.Errorf("len=%d, want 10", len(s))
	}
}
