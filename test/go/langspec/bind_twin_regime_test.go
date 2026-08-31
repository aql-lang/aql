// bind_twin_regime_test.go is the ROLLBACK-AND-REPLAY lane — the first place
// §6.5's flip actually RUNS (design/FULL-COMPILATION.0.md; staged behind
// BORU_TWIN_REGIME so the default build stays byte-identical).
//
// With the regime armed, a compiled request no longer keeps the check pass's
// installs: lang rolls the runtime-visible bindings back to the pre-pass
// snapshot (core.RestoreBindingsForReplay), each placed OpBindTwin re-installs
// its recorded transition at its source position (core.ApplyBindTwin), and
// OpBindGlobal writes the runtime values in Push mode. The differential below
// is therefore the flip's own gate: every corpus row that compiles under the
// regime must produce results identical to a fresh interpreter — same values,
// same error/no-error — exactly the contract the keep-the-installs lane
// (compiled_differential_test.go) pins for today's default.
//
// The lane reports three counts — rows compiled under the regime, ordinary
// stage refusals, and REGIME-ONLY refusals (classified by the refusal
// reason the compile_refused error carries: the full-placement gate's
// "twin regime:" prefix) — and gates divergences at ZERO with no allowance.
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// minTwinRegimeRows is the regime lane's own compiled floor — measured at
// 6415 after the do-body adoption increment (the corpus-wide compiled
// population, sensitive to test grouping like minCompiledRows' caveat,
// hence the small headroom), against 6416 for the default recorder: the
// full-placement gate now costs exactly 1 row, the suspended-recorder
// each-body leaking def (bytecode-migrated.tsv:41), which has no stream
// home until the arm-resident-twin increment. The other four table-only
// rows (the do-body class) were recovered by ADOPTION, not by island
// accounting — instrumentation showed all five record through the CLOSURE
// path, so `do`'s spec flag (CallableSpec.BodyOnceKeepsDefs) licenses
// AdoptBodyTwins to place each suspended body twin as a real twin op
// after the call, replaying the captured entry (§6.5: replay, never
// re-execution). Raise the floor as the each-body row closes; never lower
// it.
const minTwinRegimeRows = 6405

// TestTwinRegimeSmoke pins the flip's mechanics on four shapes small enough
// to reason through by hand before the corpus lane runs: a concrete def
// (the twin replays the captured value), a computed def (the twin skips,
// Push-mode OpBindGlobal installs the runtime value), cross-request
// persistence on ONE instance (the replayed binding must be readable by the
// next request's check pass — keep-on-compile's contract, now delivered by
// replay), and a type install (the twin re-pushes the minted node).
func TestTwinRegimeSmoke(t *testing.T) {
	t.Setenv("BORU_TWIN_REGIME", "1")
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	out, err := a.RunCompiledStrict("def x 1  x add 2")
	if err != nil || len(out) != 1 || renderAny(out) != "3" {
		t.Fatalf("concrete def under the regime: %v, %v", out, err)
	}
	out, err = a.RunCompiledStrict("x add 10")
	if err != nil || renderAny(out) != "11" {
		t.Fatalf("cross-request read of a replayed binding: %v, %v", out, err)
	}
	out, err = a.RunCompiledStrict("def Flagq (refine Boolean)  def f:Flagq true  f")
	if err != nil || renderAny(out) != "true" {
		t.Fatalf("type install + typed def under the regime: %v, %v", out, err)
	}
}

// TestSpecCompiledDifferentialTwinRegime is TestSpecCompiledDifferential with
// the regime armed on the compiled side: regime-compiled vs fresh interpreter,
// divergence gate 0.
func TestSpecCompiledDifferentialTwinRegime(t *testing.T) {
	t.Setenv("BORU_TWIN_REGIME", "1")

	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var compiled, refused, regimeRefused, mismatches int

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		path := filepath.Join(specDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			input := strings.TrimSpace(parts[0])
			if strings.HasPrefix(strings.TrimSpace(parts[1]), "ERROR:") {
				continue
			}

			ac := newDifferentialInstance(t)
			gotC, wasCompiled, errC := ac.RunCompiled(input)
			if !wasCompiled {
				if errC != nil && strings.Contains(errC.Error(), "twin regime:") {
					regimeRefused++
				} else {
					refused++
				}
				continue
			}
			compiled++

			ai := newDifferentialInstance(t)
			gotI, errI := ai.RunInterp(input)

			if (errC != nil) != (errI != nil) {
				mismatches++
				t.Errorf("%s:L%d: %s\n  regime error divergence: compiled=%v interpreted=%v",
					e.Name(), lineNum, input, errC, errI)
				continue
			}
			if errC != nil {
				continue
			}
			if renderAny(gotC) != renderAny(gotI) {
				mismatches++
				t.Errorf("%s:L%d: %s\n  regime compiled=%q interpreted=%q",
					e.Name(), lineNum, input, renderAny(gotC), renderAny(gotI))
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	t.Logf("twin-regime differential: %d rows compiled, %d stage refusals, %d regime-only (placement) refusals, %d divergences",
		compiled, refused, regimeRefused, mismatches)
	if compiled < minTwinRegimeRows {
		t.Errorf("only %d rows compiled under the regime (floor %d) — the flip regressed the compilable subset",
			compiled, minTwinRegimeRows)
	}
}
