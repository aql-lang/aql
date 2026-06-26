package main

// Self-contained runner for the generated syntax-combination matrix
// (syntax-matrix.tsv, produced by this package's generator). It pins the
// three deterministic trust properties the matrix exists to defend —
// stated in the package doc comment of main.go:
//
//   1. Interpreter correctness (TestSyntaxMatrixInterpreter): every row
//      re-evaluates to the recorded canonical value, or to the recorded
//      error class. This is the frozen snapshot — if the interpreter's
//      output for any of the ~7k sequences drifts, this fails. (Identity
//      with the generator's own recipe is the point: the spec is exactly
//      what the interpreter produces, captured deterministically.)
//
//   2. Compiler parity (TestSyntaxMatrixCompilerParity): every non-error
//      row the bytecode compiler accepts must produce a result IDENTICAL
//      to the interpreter. A floor on the accepted-row count keeps the
//      gate honest (a compiler that silently refused everything would
//      pass vacuously). This is the same differential contract the
//      langspec suite enforces, applied to the whole combination space.
//
//   3. Checker determinism (TestSyntaxMatrixCheckerDeterministic): the
//      type checker yields byte-identical diagnostics and residual stack
//      on repeated runs of the same row, and never panics. Determinism is
//      the trust property here — the curated accuracy/soundness ratchets
//      live in the langspec suite, against the hand-written corpus.
//
// The matrix is regenerated, not edited; keep these gates in lockstep
// with the generator's evaluation recipe (a divergence would surface as
// an interpreter-correctness failure on the next regeneration).

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	lang "github.com/aql-lang/aql/lang/go"
)

const matrixPath = "syntax-matrix.tsv"

// minCompiledRows is the floor for the compiler-parity gate: at least
// this many non-error rows must take the bytecode-compiled path, or the
// parity assertion is vacuous (a compiler that refused everything would
// pass with zero comparisons). Measured at 1992 over the current
// alphabet; pinned a little below the live count as headroom against
// incidental jitter. RAISE it when the compilable subset widens; only
// lower it with a documented reason.
const minCompiledRows = 1800

// matrixRow is one parsed data row of the matrix.
type matrixRow struct {
	line     int
	input    string
	expected string // canonical value, or "ERROR:<class>"
}

// readMatrix parses the generated tsv into data rows, skipping the
// comment/blank header lines.
func readMatrix(t *testing.T) []matrixRow {
	t.Helper()
	f, err := os.Open(matrixPath)
	if err != nil {
		t.Fatalf("open %s: %v (regenerate with `make spec-gen`)", matrixPath, err)
	}
	defer f.Close()

	var rows []matrixRow
	scanner := bufio.NewScanner(f)
	// Rows can be long once the alphabet grows; lift the default 64K line cap.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	n := 0
	for scanner.Scan() {
		n++
		line := strings.TrimRight(scanner.Text(), " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			t.Fatalf("%s:L%d: malformed row %q", matrixPath, n, line)
		}
		rows = append(rows, matrixRow{line: n, input: strings.TrimSpace(parts[0]), expected: strings.TrimSpace(parts[1])})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", matrixPath, err)
	}
	if len(rows) == 0 {
		t.Fatalf("%s has no data rows", matrixPath)
	}
	return rows
}

// The generator's own evaluation recipe (run), error-class extraction
// (errorClass), and frozen clock (specClock) are reused verbatim from
// main.go, so a generated row reproduces here byte-for-byte.

// TestSyntaxMatrixInterpreter re-evaluates every row and asserts the
// interpreter reproduces the recorded expectation exactly — the frozen
// deterministic snapshot.
func TestSyntaxMatrixInterpreter(t *testing.T) {
	rows := readMatrix(t)
	for _, r := range rows {
		out, err := run(r.input)
		var got string
		if err != nil {
			got = errorClass(err)
		} else {
			got = eng.Canon(out)
		}
		if got != r.expected {
			t.Errorf("L%d %q: interpreter gave %q, spec records %q", r.line, r.input, got, r.expected)
		}
	}
	t.Logf("interpreter: %d rows reproduced", len(rows))
}

// TestSyntaxMatrixCompilerParity asserts the bytecode compiler agrees
// with the interpreter on every non-error row it accepts.
func TestSyntaxMatrixCompilerParity(t *testing.T) {
	rows := readMatrix(t)
	var compiled, errorRows int
	for _, r := range rows {
		if strings.HasPrefix(r.expected, "ERROR:") {
			errorRows++
			continue
		}

		ac, err := lang.New()
		if err != nil {
			t.Fatalf("lang.New: %v", err)
		}
		ac.SetClock(specClock)
		gotC, wasCompiled, errC := ac.RunCompiled(r.input)
		if !wasCompiled {
			continue // outside the compilable subset — interpreter fallback covers it
		}
		compiled++

		ai, err := lang.New()
		if err != nil {
			t.Fatalf("lang.New: %v", err)
		}
		ai.SetClock(specClock)
		gotI, errI := ai.Run(r.input)

		if (errC != nil) != (errI != nil) {
			t.Errorf("L%d %q: error divergence compiled=%v interpreted=%v", r.line, r.input, errC, errI)
			continue
		}
		if errC != nil {
			continue
		}
		if renderAny(gotC) != renderAny(gotI) {
			t.Errorf("L%d %q: compiled=%q interpreted=%q", r.line, r.input, renderAny(gotC), renderAny(gotI))
		}
	}
	t.Logf("compiler parity: %d rows compiled (%d error rows skipped), 0 mismatches", compiled, errorRows)
	if compiled < minCompiledRows {
		t.Errorf("only %d rows took the compiled path (floor %d) — the compiler regressed to refusing the matrix",
			compiled, minCompiledRows)
	}
}

// TestSyntaxMatrixCheckerDeterministic asserts the type checker is
// deterministic (identical result on repeated runs) and never panics,
// over every row. It does not gate on accuracy — the curated false-
// positive / soundness ratchets live in the langspec suite — only on the
// trust property this matrix is responsible for: determinism.
func TestSyntaxMatrixCheckerDeterministic(t *testing.T) {
	rows := readMatrix(t)
	var clean, flagged int
	for _, r := range rows {
		a, b := checkOnce(t, r.input), checkOnce(t, r.input)
		if a != b {
			t.Errorf("L%d %q: checker non-deterministic:\n  run1=%s\n  run2=%s", r.line, r.input, a, b)
		}
		if strings.HasPrefix(a, "errors=0 ") {
			clean++
		} else {
			flagged++
		}
	}
	t.Logf("checker determinism: %d rows clean, %d flagged, 0 non-deterministic", clean, flagged)
}

// checkOnce runs one row through the checker and renders a stable
// fingerprint (error count + residual stack types) used to compare two
// runs for determinism. A panic is recovered into a sentinel so the
// no-panic property is asserted as a normal test failure, never a crash.
func checkOnce(t *testing.T, input string) (fp string) {
	t.Helper()
	defer func() {
		if rec := recover(); rec != nil {
			t.Errorf("checker PANICKED on %q: %v", input, rec)
			fp = fmt.Sprintf("PANIC:%v", rec)
		}
	}()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	a.SetClock(specClock)
	res, err := a.Check(input)
	if err != nil {
		return "parse-or-run-error:" + errorClass(err)
	}
	return fmt.Sprintf("errors=%d warnings=%d [%s]", res.Summary.Errors, res.Summary.Warnings, strings.Join(res.Stack, " "))
}

func renderAny(vs []any) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, " ")
}
