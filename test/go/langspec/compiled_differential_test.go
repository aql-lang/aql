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

// minCompiledRows is the floor: at least this many spec rows must
// take the compiled path for the gate to be meaningful. Raise it as
// later stages widen the compilable subset; never lower it without a
// documented decision.
const minCompiledRows = 610 // raised with Stage-3 completion: mutual tails + closures (614 compiled June 2026)

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
			ac, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			gotC, wasCompiled, errC := ac.RunCompiled(input)
			if !wasCompiled {
				continue // beyond the current stage's subset
			}
			compiled++

			// Interpreter path on another fresh instance.
			ai, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
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
