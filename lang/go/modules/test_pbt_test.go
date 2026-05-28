package modules

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// Helper: run AQL source and extract a top-of-stack Map field by key.
func runTestAndGetField(t *testing.T, r *native.Registry, src, key string) native.Value {
	t.Helper()
	res := runTestAQL(t, r, src)
	if len(res) == 0 {
		t.Fatalf("no result for %q", src)
	}
	m, _ := native.AsMap(res[len(res)-1])
	if m == nil {
		t.Fatalf("top of stack is not a map (was %s)", res[len(res)-1].Parent.String())
	}
	v, ok := m.Get(key)
	if !ok {
		t.Fatalf("result missing key %q in %s", key, res[len(res)-1].String())
	}
	return v
}

// TestCheckProp_AlwaysPasses confirms a property that's always true
// reports ok=true after `runs` iterations.
func TestCheckProp_AlwaysPasses(t *testing.T) {
	r := testRegistry(t)
	// Property: any drawn Integer in [0, 100) is >= 0.
	// gen body: draws one Integer.
	// property body: top-of-stack >= 0.
	src := `test.check-prop "non-negative" [r.int 0 100] [0 gte] 20 1 0`
	ok := runTestAndGetField(t, r, src, "ok")
	b, err := ok.AsConcreteBoolean()
	if err != nil {
		t.Fatalf("ok field not Boolean: %v", err)
	}
	if !b {
		t.Errorf("expected ok=true for always-true property")
	}
}

// TestCheckProp_FailsOnSpecificInput introduces a property that's
// very likely to fail within 100 iterations: "value > 0". The
// generator draws from [0, 100), so 0 will eventually appear and
// the property will report failure with the failing input.
func TestCheckProp_FailsOnSpecificInput(t *testing.T) {
	r := testRegistry(t)
	src := `test.check-prop "positive" [r.int 0 100] [0 gt] 200 1 0`
	res := runTestAQL(t, r, src)
	m, _ := native.AsMap(res[0])
	okV, _ := m.Get("ok")
	ok, _ := okV.AsConcreteBoolean()
	if ok {
		t.Fatal("expected failure for positive property (0 should be drawn within 200 runs)")
	}
	failing, _ := m.Get("failing-input")
	n, err := failing.AsConcreteInteger()
	if err != nil {
		t.Fatalf("failing-input not Integer: %v", err)
	}
	if n != 0 {
		t.Errorf("failing-input=%d, want 0 (the only value in [0,100) that fails `0 gt`)", n)
	}
}

// TestCheckProp_DeterministicAcrossRuns confirms same seed → same
// failing input. The Stage-5 reducer will rely on this.
func TestCheckProp_DeterministicAcrossRuns(t *testing.T) {
	// Property "value != 7" — fails the moment the gen draws 7.
	// With seed=42 + 200 iterations drawing from [0, 100), this
	// is very likely to fail somewhere; the iteration AND value at
	// which it fails must be identical across registries.
	draw := func() int64 {
		r := testRegistry(t)
		src := `test.check-prop "not-seven" [r.int 0 100] [7 neq] 200 42 0`
		res := runTestAQL(t, r, src)
		m, _ := native.AsMap(res[0])
		v, _ := m.Get("failing-input")
		n, _ := v.AsConcreteInteger()
		return n
	}
	a := draw()
	b := draw()
	if a != b {
		t.Errorf("seed=42 produced different failing inputs across registries: %d vs %d", a, b)
	}
}

// TestCheckProp_RunsCount confirms the result records the actual
// number of iterations executed (which equals `runs` for passing
// properties, and the iteration that failed for failing ones).
func TestCheckProp_RunsCount(t *testing.T) {
	r := testRegistry(t)
	src := `test.check-prop "always-true" [42] [true] 7 1 0`
	res := runTestAQL(t, r, src)
	m, _ := native.AsMap(res[0])
	v, _ := m.Get("runs")
	n, _ := v.AsConcreteInteger()
	if n != 7 {
		t.Errorf("runs=%d, want 7", n)
	}
}

// TestCheckProp_RejectsNonBooleanProperty: a property body that
// produces a non-Boolean value should fail with an error, not pass
// silently.
func TestCheckProp_RejectsNonBooleanProperty(t *testing.T) {
	r := testRegistry(t)
	src := `test.check-prop "bad-prop" [r.int 0 100] [42] 1 1 0`
	res := runTestAQL(t, r, src)
	m, _ := native.AsMap(res[0])
	okV, _ := m.Get("ok")
	ok, _ := okV.AsConcreteBoolean()
	if ok {
		t.Fatal("expected non-Boolean property body to fail")
	}
	errV, _ := m.Get("error")
	if native.IsNone(errV) {
		t.Errorf("error field should be populated when property returns non-Boolean")
	}
}

// TestRunProperty_ViaSpecMap confirms the AQL `run-property` fn
// destructures a PropertySpec map and dispatches the same way as
// the imperative `test.check-prop` call.
func TestRunProperty_ViaSpecMap(t *testing.T) {
	r := testRegistry(t)
	// Use test.prop (Go native) to build the spec — its NoEvalArgs
	// boundary preserves the quoted gen/property bodies so the
	// embedded list literal [r.int 0 100] survives without being
	// auto-evaluated at construction time.
	// Forward form for test.prop so sig positions align with the
	// canonical String/List/List declaration.
	src := `
		def p (test.prop "non-negative-via-spec" [r.int 0 100] [0 gte])
		p test.run-property
	`
	ok := runTestAndGetField(t, r, src, "ok")
	b, _ := ok.AsConcreteBoolean()
	if !b {
		t.Errorf("run-property should report ok=true for always-true property")
	}
	nameV := runTestAndGetField(t, r, src, "name")
	name, _ := nameV.AsConcreteString()
	if name != "non-negative-via-spec" {
		t.Errorf("result name=%q, want non-negative-via-spec", name)
	}
}

// TestCheckProp_ResultsAccumulated confirms PropertyResults flow
// into the same testRun results bucket as table-driven tests, so a
// single test.summary covers both.
func TestCheckProp_ResultsAccumulated(t *testing.T) {
	r := testRegistry(t)
	// One passing, one failing.
	runTestAQL(t, r, `test.check-prop "ok-prop" [r.int 0 10] [0 gte] 5 1 0`)
	runTestAQL(t, r, `test.check-prop "bad-prop" [r.int 0 10] [false] 5 1 0`)

	resV := runTestAQL(t, r, `test.results`)
	if len(resV) == 0 {
		t.Fatal("test.results returned nothing")
	}
	// Just confirm we have entries and at least one failure recorded.
	failV := runTestAQL(t, r, `test.fail-count`)
	if len(failV) == 0 {
		t.Fatal("test.fail-count returned nothing")
	}
	n, _ := failV[0].AsConcreteInteger()
	if n < 1 {
		t.Errorf("fail-count=%d, want >=1", n)
	}
}

// Stub to keep parser referenced when no other test in this file
// uses it directly.
var _ = parser.Parse
