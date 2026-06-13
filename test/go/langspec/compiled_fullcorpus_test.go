// Stage-5 compile-or-fallback gate (design/aql-bytecode-plan.0.md
// §Stage 5: "every program either compiles or falls back, so the whole
// suite must pass in compiled mode"). Where the span-level differential
// gate (compiled_differential_test.go) checks ONLY the rows the emitter
// accepts, this gate runs EVERY value row through RunCompiled — which
// compiles what it can and SILENTLY falls back to the interpreter for
// the rest — and compares to the interpreter row-for-row.
//
// All but a pinned set of rows match. The remaining divergences are a
// documented, PRE-EXISTING RunCompiled limitation that is independent of
// the span-fallback islands this stage adds: CompileCheck executes the
// program in check mode, so its RunInCheckMode words (def/import/type/
// macro and the Test harness) leave real side effects on the instance
// registry; when the program is UNCOMPILABLE the interpreter fallback
// re-runs the whole source and double-applies those effects (a type
// re-mint, a fn re-registration, a re-import, a re-run Test spec). Every
// pinned case is one of these, and every one is on the FALLBACK path
// (wasCompiled=false). A faithful fix needs registry-state isolation
// between the check pass and the fallback run (the naive fork loses
// module-import fidelity); it is tracked as a Stage-5 follow-on.
//
// This gate is therefore a RATCHET (the repo's check-accuracy baseline
// pattern): the divergence set must be EXACTLY the pinned inputs. A new
// divergence — a real compile-or-fallback parity regression — fails the
// gate; fixing a pinned case also fails it (tighten the baseline).
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// knownFallbackDoubleExec pins the value rows that diverge ONLY because
// the interpreter fallback re-executes check-pass side effects (see the
// file comment). Keyed by the exact spec input. Every entry is a
// wasCompiled=false row. Shrink this set when the double-execution
// isolation follow-on lands; never grow it without a documented reason.
var knownFallbackDoubleExec = map[string]bool{
	`def Point class {x:1} def p (make Point {x:5}) undef Point end p typeof`:                                                                                                                                                              true,
	`def Point class {x:1} def p (make Point {x:5}) undef Point end p get x`:                                                                                                                                                               true,
	`def Point class {x:1} def p (make Point {x:5}) undef Point end p set x 9 end p.x`:                                                                                                                                                     true,
	`def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] Positive tcmp Positive`:                                                                                                                                                  true,
	`def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] def Negative fn [n:Integer Integer [if (n lt 0) [n] [None]]] Negative tcmp Positive`:                                                                                     true,
	`def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] def Negative fn [n:Integer Integer [if (n lt 0) [n] [None]]] Positive tcmp Negative`:                                                                                     true,
	`def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] Positive tcmp Function`:                                                                                                                                                  true,
	`def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] Function tcmp Positive`:                                                                                                                                                  true,
	`"aql:test" import end  def double fn [[n:Integer] [Integer] [n 2 mul]] end def s {name: "doubling" subject: double/q cases: [{name: "d3" in: [3] out: 6} {name: "d0" in: [0] out: 0}] subs: []} end s Test.run-spec end Test.summary`: true,
	`"aql:array-util" import  def people [{name:"ada" age:36} {name:"bob" age:20}]  each $.name (ArrayUtil.sortby $.age people)`:                                                                                                           true,
}

func TestSpecCompiledOrFallback(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var rows, compiledPath int
	gotDiverge := map[string]bool{}

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

			diverged := (errC != nil) != (errI != nil) ||
				(errC == nil && renderAny(gotC) != renderAny(gotI))
			if !diverged {
				continue
			}
			gotDiverge[input] = true
			if !knownFallbackDoubleExec[input] {
				t.Errorf("%s:L%d NEW compile-or-fallback divergence (wasCompiled=%v):\n  %s\n  compiled=%q err=%v | interpreted=%q err=%v",
					e.Name(), lineNum, wasCompiled, input, renderAny(gotC), errC, renderAny(gotI), errI)
			} else if wasCompiled {
				// A pinned case is supposed to diverge on the FALLBACK
				// path only — if it now compiled, the soundness story
				// changed and the pin is stale.
				t.Errorf("%s:L%d pinned double-exec row now takes the COMPILED path — re-evaluate:\n  %s", e.Name(), lineNum, input)
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	// Ratchet: every pinned row must still be diverging. A pin that no
	// longer diverges means the double-execution follow-on (or a related
	// fix) landed — tighten the baseline by removing it.
	var fixed []string
	for input := range knownFallbackDoubleExec {
		if !gotDiverge[input] {
			fixed = append(fixed, input)
		}
	}
	sort.Strings(fixed)
	for _, input := range fixed {
		t.Errorf("pinned double-exec row no longer diverges (remove it from knownFallbackDoubleExec):\n  %s", input)
	}

	t.Logf("compile-or-fallback: %d value rows, %d compiled, %d pinned fallback divergences",
		rows, compiledPath, len(knownFallbackDoubleExec))
}
