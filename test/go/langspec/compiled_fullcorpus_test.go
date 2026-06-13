// Stage-5 compile-or-fallback gate (design/aql-bytecode-plan.0.md
// §Stage 5: "every program either compiles or falls back, so the whole
// suite must pass in compiled mode"). Where the span-level differential
// gate (compiled_differential_test.go) checks ONLY the rows the emitter
// accepts, this gate runs EVERY value row through RunCompiled — which
// compiles what it can and SILENTLY falls back to the interpreter for
// the rest — and asserts the result matches the interpreter row-for-row.
//
// Zero divergences is the bar. The fallback path no longer
// double-executes check-pass side effects: RunCompiled snapshots the
// registry's mutable scopes before the check pass and rolls them back
// (Registry.SnapshotForCompile / RestoreForCompile) when it falls back,
// so the interpreter runs on pristine state — no re-mint, re-import, or
// re-run Test spec.
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecCompiledOrFallback(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var rows, compiledPath, mismatches int
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
			// Error rows are covered by the dedicated error-taxonomy
			// pins; this gate is VALUE-row compile-or-fallback parity.
			if strings.HasPrefix(strings.TrimSpace(parts[1]), "ERROR:") {
				continue
			}
			rows++

			ac := newDifferentialInstance(t)
			gotC, wasCompiled, errC := ac.RunCompiled(input)
			if wasCompiled {
				compiledPath++
			}
			ai := newDifferentialInstance(t)
			gotI, errI := ai.Run(input)

			if (errC != nil) != (errI != nil) {
				mismatches++
				t.Errorf("%s:L%d (wasCompiled=%v): %s\n  error divergence: compiled=%v interpreted=%v",
					e.Name(), lineNum, wasCompiled, input, errC, errI)
				continue
			}
			if errC != nil {
				continue
			}
			if renderAny(gotC) != renderAny(gotI) {
				mismatches++
				t.Errorf("%s:L%d (wasCompiled=%v): %s\n  compiled=%q interpreted=%q",
					e.Name(), lineNum, wasCompiled, input, renderAny(gotC), renderAny(gotI))
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	t.Logf("compile-or-fallback: %d value rows, %d compiled, %d mismatches", rows, compiledPath, mismatches)
	if mismatches != 0 {
		t.Errorf("%d compile-or-fallback divergences — every program must compile or fall back to an identical result", mismatches)
	}
}
