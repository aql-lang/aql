// Compiled-mode differential gate (design/aql-bytecode-plan.0.md,
// ground rule "differential gate from day one"): every spec value
// row the Stage-1 emitter accepts must produce IDENTICAL results
// through the bytecode VM and the interpreter. The compiled-row
// count is pinned with a floor so a regression that silently refuses
// everything (vacuously passing the equality check) is caught.
package langspec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// newDifferentialInstance builds one side of the comparison with the
// SAME frozen clock the spec runner uses (langspec_test.go), so
// clock-seeded rows (rand, now) are deterministic and comparable
// across the two runs.
func newDifferentialInstance(t *testing.T) *lang.AQL {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	a.SetClock(specClock)
	return a
}

// minCompiledRows is the floor: at least this many spec rows must
// take the compiled path for the gate to be meaningful. Raise it as
// later stages widen the compilable subset; never lower it without a
// documented decision.
const minCompiledRows = 1825 // + F4 class/object make (plain-data const-bake) + get islands + fn-value boundary + is/typeof on make-result (type-operand ID-collision guard): 1636 compiled June 2026; + P5 multi/0-result calls: 1677 compiled; + multi-return / 0-return / anonymous-lambda fns (N-result fn units): 1742; + apply of a fn value (check-engine re-step): 1753; + unnamed-fn map/list members (method fields m.f): 1759; + 0-value-then if statement guard: 1760; + concrete-args dynamic-output core builtins (unify et al): 1765; + module inner natives (dot-access): 1813; + 3-arg operand shape (sig-0 result over consts, push+swap chain): 1820; + strict-disjunct type-algebra poly (is/tand over tnot-predicate): 1825 (the differential count is sensitive to test grouping — this is the strict CI-group value; the full make-test grouping runs a few higher)

func TestSpecCompiledDifferential(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var compiled, mismatches int

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

			// Compiled path on a fresh instance.
			ac := newDifferentialInstance(t)
			gotC, wasCompiled, errC := ac.RunCompiled(input)
			if !wasCompiled {
				continue // beyond the current stage's subset
			}
			compiled++

			// Interpreter path on another fresh instance.
			ai := newDifferentialInstance(t)
			gotI, errI := ai.Run(input)

			if (errC != nil) != (errI != nil) {
				mismatches++
				t.Errorf("%s:L%d: %s\n  error divergence: compiled=%v interpreted=%v",
					e.Name(), lineNum, input, errC, errI)
				continue
			}
			if errC != nil {
				continue
			}
			if renderAny(gotC) != renderAny(gotI) {
				mismatches++
				t.Errorf("%s:L%d: %s\n  compiled=%q interpreted=%q",
					e.Name(), lineNum, input, renderAny(gotC), renderAny(gotI))
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	t.Logf("compiled differential: %d rows compiled, %d mismatches", compiled, mismatches)
	if compiled < minCompiledRows {
		t.Errorf("only %d rows took the compiled path (floor %d) — the emitter regressed to refusing the corpus",
			compiled, minCompiledRows)
	}
}

func renderAny(vs []any) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, " ")
}
