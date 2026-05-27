package test

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// AQL fns and lambdas use implicit lexical capture: at construction
// time the engine walks the body's bare-Word references, identifies
// which ones resolve to bindings made by an enclosing fn (params or
// local defs), and snapshots their current values into the FnDefInfo.
// At dispatch the captured names are installed as defs alongside per-
// call named params; the body sees its captures regardless of what
// happened to the outer scope. Module-global names and forward refs
// stay dynamic (resolved via registry at call time).
//
// The headline test is the factory pattern — `make-adder N` returning
// an inner closure that captures `N`. Today that pattern depended on
// the outer scope being live; the closure mechanism makes it work
// regardless.

func intResult(t *testing.T, vals []native.Value) int64 {
	t.Helper()
	if len(vals) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(vals), vals)
	}
	n, err := eng.AsInteger(vals[0])
	if err != nil {
		t.Fatalf("expected Integer, got %s: %v", vals[0].Parent.String(), err)
	}
	return n
}

// A — Factory pattern via fn.
func TestClosureFactoryViaFn(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def make-adder fn [[x:Integer] [Function] [fn [[y:Integer] [Integer] [x add y]]]]`,
		`def add5 (make-adder 5)`,
		`add5 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 8 {
		t.Errorf("add5 3 → %d, want 8", got)
	}
}

// B — Factory via afn / =>.
func TestClosureFactoryViaArrow(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def make-adder ([x:Integer] => [([y:Integer] => [x add y])])`,
		`def add5 (make-adder 5)`,
		`add5 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 8 {
		t.Errorf("add5 3 → %d, want 8", got)
	}
}

// Independent captures: two `make-adder` calls produce two closures
// that don't share state.
func TestClosureIndependentCaptures(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def make-adder ([x:Integer] => [([y:Integer] => [x add y])])`,
		`def add5 (make-adder 5)`,
		`def add20 (make-adder 20)`,
		`add5 1`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 6 {
		t.Errorf("add5 1 → %d, want 6", got)
	}
	out, err = runNativeSteps(t, nil, []string{
		`def make-adder ([x:Integer] => [([y:Integer] => [x add y])])`,
		`def add20 (make-adder 20)`,
		`add20 1`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 21 {
		t.Errorf("add20 1 → %d, want 21", got)
	}
}

// D — Module-level (top-level) names stay dynamic. `x` is bound at
// module scope; the closure's body references it but it should NOT be
// captured. Reassigning x after closure construction changes what the
// body sees.
func TestClosureModuleLevelStaysDynamic(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def x 1`,
		`def f ([] => [x])`,
		`def x 2`,
		`f`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 2 {
		t.Errorf("f after `def x 2` → %d, want 2 (module x is dynamic)", got)
	}
}

// E — Recursion via forward reference: at construction `fact` isn't
// bound (the `def fact …` completes after the fn handler returns), so
// it isn't captured. Runtime registry lookup finds it.
func TestClosureRecursionForwardRef(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]]`,
		`fact 5`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 120 {
		t.Errorf("fact 5 → %d, want 120", got)
	}
}

// F — Capture wins over module-level rebind. The inner def-local `n`
// is captured (enclosing-fn-local). After the outer fn returns, the
// closure's `n` is the captured one — module `n` is shadowed for the
// closure body's lookup.
func TestClosureShadowsModuleLevel(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def n 99`,
		`def f fn [[] [Function] [ def n 5  ([] => [n]) ]]`,
		`def g (f)`,
		`def n 7`,
		`g`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := intResult(t, out); got != 5 {
		t.Errorf("g → %d, want 5 (captured n=5 shadows module n)", got)
	}
}

// H — Top-level fn captures nothing. Inspect the FnDefInfo's
// Captured field directly.
func TestClosureTopLevelFnHasNoCaptures(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`fn [[x:Integer] [Integer] [x add 1]]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	fnDef, ok := out[0].Data.(eng.FnDefInfo)
	if !ok {
		t.Fatalf("payload type = %T, want FnDefInfo", out[0].Data)
	}
	if fnDef.Captured != nil {
		t.Errorf("top-level fn Captured = %v, want nil", fnDef.Captured)
	}
}

// Inner fn populates Captured. Construct the inner fn inside an outer
// fn body and verify the inner FnDefInfo has the outer param captured.
func TestClosureInnerFnHasCaptures(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def make ([x:Integer] => [([y:Integer] => [x add y])])`,
		`make 5`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	fnDef, ok := out[0].Data.(eng.FnDefInfo)
	if !ok {
		t.Fatalf("payload type = %T, want FnDefInfo", out[0].Data)
	}
	if len(fnDef.Captured) != 1 {
		t.Fatalf("expected 1 capture, got %d: %v", len(fnDef.Captured), fnDef.Captured)
	}
	if fnDef.Captured[0].Name != "x" {
		t.Errorf("captured name = %q, want x", fnDef.Captured[0].Name)
	}
	n, _ := eng.AsInteger(fnDef.Captured[0].Value)
	if n != 5 {
		t.Errorf("captured x = %d, want 5", n)
	}
}

// L — Param shadows capture. Inner fn has its own `x` param, distinct
// from outer's `x`. The inner body's `x` resolves to the param, not
// the capture.
func TestClosureParamShadowsCapture(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def make ([x:Integer] => [([x:String] => [x])])`,
		`(make 5) "hello"`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(out), out)
	}
	s, err := eng.AsString(out[0])
	if err != nil {
		t.Fatalf("expected String, got %s: %v", out[0].Parent.String(), err)
	}
	if s != "hello" {
		t.Errorf("got %q, want \"hello\"", s)
	}
}

// I — Captured-def cleanup safety: after `def add5 (make-adder 5)`,
// the outer make-adder's `x` is gone from the module scope (cleaned
// up by DefCleanup); only add5's captured `x` survives. No `x` leak.
func TestClosureNoOuterDefLeak(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	native.Register(reg)
	e := native.NewTop(reg)
	src := `def make-adder ([x:Integer] => [([y:Integer] => [x add y])])
def add5 (make-adder 5)`
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := e.Run(vals); err != nil {
		t.Fatalf("run: %v", err)
	}
	if reg.Defs.Depth("x") != 0 {
		t.Errorf("outer x leaked to module scope: depth = %d", reg.Defs.Depth("x"))
	}
}

// J — AnalyseFnBody installs captures so a body that references an
// enclosing-fn-local sees its captured type during check-mode
// inference. Direct test of the Phase 5 wire-up: call AnalyseFnBody
// with a synthetic body, an Integer-typed captured `x`, and verify
// the residual carrier infers Integer from `x add 1`.
func TestClosureCheckModeAnalyseFnBodyUsesCaptures(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	native.Register(reg)
	reg.Check.Mode = true
	defer func() { reg.Check.Mode = false }()

	// Body: x add 1 — references captured x, no params.
	body := []eng.Value{
		eng.NewWord("x"),
		eng.NewWord("add"),
		eng.NewInteger(1),
	}
	captures := []eng.CapturedBinding{
		{Name: "x", Value: eng.NewCarrier(eng.TInteger)},
	}

	result := eng.AnalyseFnBody(reg, "test", nil, body, nil, captures)
	if len(result) != 1 {
		t.Fatalf("got %d residual values, want 1: %v", len(result), result)
	}
	v := result[0]
	if !v.Parent.Equal(eng.TInteger) {
		t.Errorf("residual Parent = %s, want Integer (capture flowed through)", v.Parent.String())
	}

	// Confirm captures are NOT leaked: x should not exist at module
	// scope after AnalyseFnBody returns (it restores via Defs.Restore).
	if _, ok := reg.Defs.Top("x"); ok {
		t.Errorf("captured x leaked to outer scope after AnalyseFnBody")
	}
}

// Same test minus captures: body's reference to x stays undefined,
// AnalyseFnBody emits a diagnostic, no Integer inference.
func TestClosureCheckModeAnalyseFnBodyWithoutCaptures(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	native.Register(reg)
	reg.Check.Mode = true
	defer func() { reg.Check.Mode = false }()

	body := []eng.Value{
		eng.NewWord("x"),
		eng.NewWord("add"),
		eng.NewInteger(1),
	}
	// Same body, no captures — x is undefined in body's scope.
	result := eng.AnalyseFnBody(reg, "test", nil, body, nil, nil)
	// We expect either an empty/Any residual or a non-Integer carrier
	// because x doesn't resolve; the analyser falls back to lenient
	// undefined-word handling.
	if len(result) == 1 && result[0].Parent.Equal(eng.TInteger) {
		t.Errorf("residual unexpectedly inferred Integer without captures: %v", result)
	}
}

// K — args stays dynamic across the capture boundary. Inside a
// captured fn, `args.0` refers to that fn's own call args, not the
// surrounding scope's.
func TestClosureArgsStaysDynamic(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def outer ([a:Any] => [([] => [args])])`,
		`def captured-lam (outer 42)`,
		`captured-lam`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(out), out)
	}
	// captured-lam was invoked with no args, so its `args` should
	// be an empty list — not outer's [42].
	if !out[0].Parent.Equal(eng.TList) {
		t.Fatalf("expected list, got %s", out[0].Parent.String())
	}
	lst, _ := eng.AsList(out[0])
	if lst.Len() != 0 {
		t.Errorf("args inside captured fn = %s, want empty (its own call args, not outer's)", out[0].String())
	}
}
