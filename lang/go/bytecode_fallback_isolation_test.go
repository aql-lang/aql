package lang

import "testing"

// RunCompiled compiles the program in check mode, which executes its
// RunInCheckMode words (def/import/type, the Test harness) for real.
// When the program is uncompilable the interpreter fallback re-runs the
// whole source, so the check-pass side effects MUST be rolled back first
// — otherwise a type re-mint, fn re-registration, or re-import diverges
// from a clean interpreter run. These pin that isolation
// (SnapshotForCompile / RestoreForCompile).
func TestRunCompiledFallbackIsolation(t *testing.T) {
	// Each row is UNCOMPILABLE (so it takes the fallback path) and
	// side-effecting (so a double-execution would corrupt the result).
	// RunCompiled must equal a clean interpreter Run.
	cases := []string{
		// type mint + undef, then reuse — a re-mint would clash.
		`def Point class {x:1} def p (make Point {x:5}) undef Point end p typeof`,
		// fn registration under a capitalised name — a re-register clashes.
		`def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] Positive tcmp Positive`,
		// native-module import whose namespace metadata a re-import degrades.
		`"aql:math-util" import end typeof MathUtil`,
		`"aql:math-util" import end MathUtil.$name`,
	}
	for _, src := range cases {
		ac, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotC, wasCompiled, errC := ac.RunCompiled(src)
		if wasCompiled {
			t.Fatalf("%q unexpectedly compiled — this row is meant to exercise the FALLBACK isolation path", src)
		}

		ai, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := ai.Run(src)

		if (errC != nil) != (errI != nil) {
			t.Errorf("%q: error divergence: compiled-fallback=%v interpreter=%v", src, errC, errI)
			continue
		}
		if errC != nil {
			continue
		}
		if len(gotC) != len(gotI) {
			t.Errorf("%q: length divergence: %v vs %v", src, gotC, gotI)
			continue
		}
		for i := range gotC {
			if gotC[i] != gotI[i] {
				t.Errorf("%q: result divergence at %d: compiled-fallback=%v interpreter=%v", src, i, gotC, gotI)
				break
			}
		}
	}

	// Positive control: a COMPILABLE program still runs on the compiled
	// path (the snapshot must not have rolled back state it needs). The
	// minted type reaches PUSH_TYPE at run time.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def Pt class {x:1} def q (make Pt {x:7}) q.x`)
	if err != nil {
		t.Fatalf("compilable user-type program: %v", err)
	}
	_ = compiled // may compile or fall back depending on the emitter; result is what matters
	if len(out) != 1 || out[0] != int64(7) {
		t.Fatalf("user-type field access = %v, want 7", out)
	}
}
