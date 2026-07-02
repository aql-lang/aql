// The check-vs-run FALSE-POSITIVE differential gate — the checker twin of
// TestPropertyDifferential. Over the same generated-program corpus, a program
// the checker REJECTS (error-severity diagnostics, after the same
// forward-ref rescue the CLI applies) must not run cleanly in the
// interpreter: a static error on a successfully-running program is a checker
// false positive. The spec-corpus FP pin (check_accuracy_test.go,
// pinnedFalsePositives = 0) only sees hand-written rows; this gate sweeps
// the generator's shapes — heterogeneous collections, computed containers,
// fn values, context stores — where the `[1 2 "s"] each [1 add]` FP class
// lived undetected until the checker review probed it by hand.
package langspec

import (
	"math/rand"
	"os"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// pinnedCheckRunDivergent is the measured count of generated programs the
// checker rejects that nevertheless run cleanly, over the DETERMINISTIC
// default corpus (2 seeds x 1500). The residual class is DEAD-BRANCH
// analysis — a static error inside an unselected `case`/`if` arm the runtime
// never executes (`(4 case [4 4 Integer ({a:4} get a) …])` errors its
// Integer arm; the scrutinee picks the first) — which is static checking
// working as intended, not a false positive. The pin may only DECREASE; a
// rise means a NEW check-vs-run divergence class appeared (the
// `[1 2 "s"] each [1 add]` heterogeneous-element FP was exactly such a
// class, invisible to the spec-corpus FP pin) and must be triaged, never
// waved through.
const pinnedCheckRunDivergent = 104 // measured June 2026; all 104 contain a case/if (hand-verified: zero non-branch hits)

// TestCheckRunFalsePositive generates the property fuzzer's program corpus
// and ratchets the count of check-rejected-but-running programs.
// Deterministic default budget (the same seeds every run, so a failure
// always reproduces); deepen with AQL_FUZZ_SEEDS / AQL_FUZZ_ITERS exactly
// like TestPropertyDifferential (the pin applies only at the default
// budget — a cranked run reports without gating).
func TestCheckRunFalsePositive(t *testing.T) {
	if testing.Short() {
		t.Skip("check-run fp fuzz: skipped in -short")
	}
	const depth = 4
	iters := fuzzEnv("AQL_FUZZ_ITERS", 1500)
	seeds := fuzzEnv("AQL_FUZZ_SEEDS", 2)
	var rejected, divergent int
	var samples []string
	for seed := 1; seed <= seeds; seed++ {
		r := rand.New(rand.NewSource(int64(seed)))
		for i := 0; i < iters; i++ {
			root, scope := genProgram(r, depth)
			src := render(root, scope)
			if !checkRejects(t, src) {
				continue
			}
			rejected++
			ai := newDifferentialInstance(t)
			if _, err := ai.Run(src); err == nil {
				divergent++
				if os.Getenv("AQL_LOG_DIVERGENT") != "" {
					t.Logf("DIVERGENT s%d i%d: %s", seed, i, src)
				}
				if len(samples) < 5 {
					samples = append(samples, src)
				}
			}
		}
	}
	t.Logf("check-run fp fuzz: %d seeds x %d programs, %d check-rejected, %d run clean (pin %d)",
		seeds, iters, rejected, divergent, pinnedCheckRunDivergent)
	if iters != 1500 || seeds != 2 {
		// A cranked exploratory run (AQL_FUZZ_*) reports without gating —
		// the pin is calibrated to the deterministic default corpus.
		for _, s := range samples {
			t.Logf("  divergent sample: %s", s)
		}
		return
	}
	if divergent > pinnedCheckRunDivergent {
		for _, s := range samples {
			t.Logf("  divergent sample: %s", s)
		}
		t.Errorf("check-rejected-but-running programs rose to %d (pin %d) — a NEW class of "+
			"check-vs-run divergence appeared; triage the samples (a genuine checker false "+
			"positive, or a new legitimately-dead-branch shape) before raising the pin",
			divergent, pinnedCheckRunDivergent)
	} else if divergent < pinnedCheckRunDivergent {
		t.Logf("check-run divergent count improved to %d — lower pinnedCheckRunDivergent to lock it in", divergent)
	}
}

// checkRejects reports whether the checker rejects src with an
// error-severity diagnostic. A check-run error (parse failure, handler
// error) is NOT a rejection here — the interpreter fails the same way, so
// there is nothing to differentiate.
func checkRejects(t *testing.T, src string) bool {
	t.Helper()
	ac := newDifferentialInstance(t)
	cr, err := ac.Check(src)
	if err != nil {
		return false
	}
	for _, d := range cr.Diagnostics {
		if d.Severity == eng.SeverityError {
			return true
		}
	}
	return false
}
