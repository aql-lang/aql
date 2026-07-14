package lang

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

// check-prop's gen/property bodies compile as same-program stored-param-body
// units (Signature.StoredBodies → compileStoredParamBody) and run nested on
// the VM via InvokeCallback. The sharp pin: ITERATION COUNT ADDS NO
// INTERPRETER ENTRIES — every unattributed entry left is module-load AQL
// (invariant in the run count; the Phase 10 attribution seam owns those).
// The fn-scope ${frame-local} guard (TestCheckPropInterpStringFnScopeRefuses)
// is the standing negative: stored-param-body compiles are module-scope only.
func TestCheckPropIterationsAddNoInterpEntries(t *testing.T) {
	entryCensus := func(runs int) map[string]int {
		t.Helper()
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		a.SetOutput(&bytes.Buffer{})
		var mu sync.Mutex
		counts := map[string]int{}
		disarm := a.ArmInterpEntryHook(func(e InterpEntry) {
			if e.Attribution == "" {
				mu.Lock()
				counts[e.Seam]++
				mu.Unlock()
			}
		})
		defer disarm()
		src := fmt.Sprintf("import \"aql:test\" end\ndef res (Test.check-prop \"x\" [r.int 1 9] [ 0 gte ] %d 1 0)\nres get \"ok\"", runs)
		if _, err := a.RunCompiledStrict(src); err != nil {
			t.Fatalf("run (%d iterations): %v", runs, err)
		}
		mu.Lock()
		defer mu.Unlock()
		out := make(map[string]int, len(counts))
		for k, v := range counts {
			out[k] = v
		}
		return out
	}
	few, many := entryCensus(2), entryCensus(60)
	if fmt.Sprintf("%v", few) != fmt.Sprintf("%v", many) {
		t.Errorf("interpreter entries scale with the iteration count — the bodies are NOT running as units:\n  2 runs:  %v\n  60 runs: %v", few, many)
	}
	// The per-iteration seams specifically must contribute nothing: before
	// the stored-param-body units the census grew by one CallAQL per body
	// per iteration.
	if few["CallAQL"] != 0 || many["CallAQL"] != 0 {
		t.Errorf("per-iteration CallAQL entries present (2 runs: %d, 60 runs: %d) — the throwaway-sig path is back", few["CallAQL"], many["CallAQL"])
	}
}

// TestCheckPropCompiledParity — the compiled and interpreted pipelines agree
// on check-prop results across the pass / fail / generator-error shapes (the
// units must preserve CallAQL frame semantics exactly).
func TestCheckPropCompiledParity(t *testing.T) {
	cases := []string{
		`import "aql:test" end
def res (Test.check-prop "pass" [r.int 1 9] [ 0 gte ] 6 1 0)
(res get "ok") (res get "runs")`,
		`import "aql:test" end
def res2 (Test.check-prop "fail" [r.int 1 9] [ drop false ] 6 1 0)
(res2 get "ok") (res2 get "runs")`,
		`import "aql:test" end
def res (Test.check-prop "gen-raises" [raise bad_input "boom"] [ 0 gte ] 3 1 0)
(res get "ok")`,
	}
	for _, src := range cases {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		a.SetOutput(&bytes.Buffer{})
		gotC, compiled, err := a.RunCompiled(src)
		if err != nil {
			t.Fatalf("RunCompiled: %v\nsrc: %s", err, src)
		}
		if !compiled {
			t.Fatalf("check-prop program must run compiled\nsrc: %s", src)
		}
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		b.SetOutput(&bytes.Buffer{})
		gotI, err := b.Run(src)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
			t.Errorf("parity: compiled %v != interp %v\nsrc: %s", gotC, gotI, src)
		}
	}
}

// TestCheckPropRefusedBodyFallsBackSound — a gen body the stored-param
// compile declines (a capitalised type install doesn't lower in a closure
// unit) falls through to the standing NoEvalArgs replay-hazard gates, which
// refuse the program: the interpreter fallback runs it with parity — slow,
// not wrong, exactly the do-registry-replay discipline.
func TestCheckPropRefusedBodyFallsBackSound(t *testing.T) {
	const src = `import "aql:test" end
def res (Test.check-prop "hazard" [def Big Integer 9] [ 0 gte ] 2 1 0)
res get "ok"`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	a.SetOutput(&bytes.Buffer{})
	gotC, compiled, err := a.RunCompiled(src)
	if err != nil {
		t.Fatalf("RunCompiled: %v", err)
	}
	if compiled {
		t.Fatal("the replay-hazard gen body must refuse the compile (the interpreter owns it)")
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	b.SetOutput(&bytes.Buffer{})
	gotI, err := b.Run(src)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
		t.Errorf("parity: compiled %v != interp %v", gotC, gotI)
	}
}
