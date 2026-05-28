// Stage 6 of PBT-PLAN.0.md: end-to-end demos exercising the PBT
// framework (test.check-prop + the shrink reducer) against the
// aql:decision module. These are the proof-of-life that the whole
// stack works together — a generator that produces random decision
// inputs, a property that asserts an invariant, and a negative-
// control test that proves the shrinker collapses failing inputs to
// minimal counterexamples.
package modules

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// decisionPbtRegistry returns a registry with aql:test, aql:rand,
// and aql:decision all installed flat — what the user gets after
// importing each module at the top level.
func decisionPbtRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	if err := InstallTestExports(r); err != nil {
		t.Fatalf("install test: %v", err)
	}
	if err := InstallRandExports(r); err != nil {
		t.Fatalf("install rand: %v", err)
	}
	if err := InstallDecisionExports(r); err != nil {
		t.Fatalf("install decision: %v", err)
	}
	return r
}

func runDecisionPbt(t *testing.T, r *native.Registry, src string) native.Value {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v\n--- src ---\n%s", err, src)
	}
	res, err := native.NewTop(r).Run(vals)
	if err != nil {
		t.Fatalf("run: %v\n--- src ---\n%s", err, src)
	}
	if len(res) == 0 {
		t.Fatalf("no result\n--- src ---\n%s", src)
	}
	return res[len(res)-1]
}

func resultField(t *testing.T, m native.Value, key string) native.Value {
	t.Helper()
	om, _ := native.AsMap(m)
	v, ok := om.Get(key)
	if !ok {
		t.Fatalf("result missing field %q in %s", key, m.String())
	}
	return v
}

// TestDecisionPBT_EvalCondAlwaysReturnsBoolean is a sanity demo:
// for any random condition + input, eval-cond returns SOMETHING
// (no errors, no None). This exercises the whole pipeline:
// rand-seeded gen → decision module dispatch → property evaluation.
func TestDecisionPBT_EvalCondAlwaysReturnsBoolean(t *testing.T) {
	r := decisionPbtRegistry(t)
	src := `
		test.check-prop "eval-cond-returns-boolean"
		  [
		    # Generator: random condition checking age against a
		    # random threshold, paired with a random input.
		    def threshold (r.int 0 100)
		    def age-val (r.int 0 100)
		    {
		      input: {age: age-val}
		      cond:  {field: "age" op: "gt" value: threshold}
		    }
		  ]
		  [
		    # Property: eval-cond produces a Boolean (true or false).
		    # We don't care which — just that the pipeline works.
		    # Sub-expressions wrapped in parens so forward-collection
		    # doesn't bleed across the eq/or calls.
		    def pkg (args.0)
		    def result ((pkg "input" get) (pkg "cond" get) decision.eval-cond)
		    (result true eq) (result false eq) or
		  ]
		  50 1 0
	`
	res := runDecisionPbt(t, r, src)
	okV := resultField(t, res, "ok")
	ok, _ := okV.AsConcreteBoolean()
	if !ok {
		errV := resultField(t, res, "error")
		failingV := resultField(t, res, "failing-input")
		t.Fatalf("eval-cond-returns-boolean property failed unexpectedly\n  error: %s\n  failing-input: %s",
			errV.String(), failingV.String())
	}
}

// TestDecisionPBT_GreaterThanSelfIsAlwaysFalse asserts the
// mathematical invariant `X > X = false` via eval-cond.
//
// gen: random V; cond {field:"v",op:"gt",value:V}; input {v:V}.
// property: eval-cond(cond, input) == false (since V > V is false).
//
// This passes for any V — a real invariant the PBT framework can
// confirm holds across the iteration space.
func TestDecisionPBT_GreaterThanSelfIsAlwaysFalse(t *testing.T) {
	r := decisionPbtRegistry(t)
	src := `
		test.check-prop "gt-self-is-false"
		  [
		    def v (r.int 0 100)
		    {
		      input: {v: v}
		      cond:  {field: "v" op: "gt" value: v}
		    }
		  ]
		  [
		    def pkg (args.0)
		    def result ((pkg "input" get) (pkg "cond" get) decision.eval-cond)
		    # invariant: V > V is false, so eval-cond should return false.
		    result false eq
		  ]
		  100 1 0
	`
	res := runDecisionPbt(t, r, src)
	okV := resultField(t, res, "ok")
	ok, _ := okV.AsConcreteBoolean()
	if !ok {
		errV := resultField(t, res, "error")
		failingV := resultField(t, res, "failing-input")
		t.Fatalf("gt-self-is-false should hold for all V — somehow failed\n  error: %s\n  failing-input: %s",
			errV.String(), failingV.String())
	}
}

// TestDecisionPBT_NegativeControl_GtZero is the shrinker demo.
//
// A deliberately-buggy property — "the random value is > 99" —
// fails on every input EXCEPT exactly 99. The framework's evaluator
// detects the first failure with whatever value the rand draws;
// the SHRINKER then collapses it. With value-level shrinking +
// integer halving + n-1 step-down, the shrunk-input should land at
// 99 (the minimum non-violator of `< 99`... wait that doesn't
// shrink — let me think) — actually the property is `v > 99` which
// fails for v ≤ 99 (every value in [0,100)); the shrinker should
// collapse any failing v to 0.
func TestDecisionPBT_NegativeControl_ShrinksFailingInput(t *testing.T) {
	r := decisionPbtRegistry(t)
	src := `
		test.check-prop "values-are-large"
		  [
		    # Generator: random integer in [0, 100).
		    r.int 0 100
		  ]
		  [
		    # Property (buggy): value must be > 99. Fails on any
		    # value 0..99 inclusive. The shrinker should converge
		    # on 0 as the minimum failing value.
		    99 gt
		  ]
		  50 1 200
	`
	res := runDecisionPbt(t, r, src)
	okV := resultField(t, res, "ok")
	if ok, _ := okV.AsConcreteBoolean(); ok {
		t.Fatal("negative-control property unexpectedly passed — every value in [0,100) should fail `> 99`")
	}
	shrunkV := resultField(t, res, "shrunk-input")
	shrunk, err := shrunkV.AsConcreteInteger()
	if err != nil {
		t.Fatalf("shrunk-input not Integer: %v (raw: %s)", err, shrunkV.String())
	}
	if shrunk != 0 {
		t.Errorf("shrunk-input=%d, want 0 (minimum value in [0,100) failing `> 99`)", shrunk)
	}
	// Confirm shrunk-source is non-empty (the reducer ran).
	srcV := resultField(t, res, "shrunk-source")
	if s, _ := srcV.AsConcreteString(); s == "" {
		t.Error("shrunk-source is empty — reducer didn't run or pretty-print is broken")
	}
}
