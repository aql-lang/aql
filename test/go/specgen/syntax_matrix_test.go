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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	lang "github.com/aql-lang/aql/lang/go"
)

const (
	matrixPath      = "syntax-matrix.tsv"
	passingPath     = "syntax-matrix-passing.tsv"
	failCheckPath   = "syntax-matrix-fail-check.tsv"
	failCompilePath = "syntax-matrix-fail-compile.tsv"
	failRuntimePath = "syntax-matrix-fail-runtime.tsv"
)

// Floors for the three minimal-failing-prefix files, so a regression that
// empties or mislabels them can't pass vacuously. Measured at 9031
// type-check, 3436 compile and 936 runtime fails over the length-4
// matrix; pinned below the live counts as headroom.
const (
	minCheckFailRows   = 9000
	minCompileFailRows = 3400
	minRuntimeFailRows = 900
)

// minPassingRows floors the passing subset so a regression that empties
// it (or that shrinks the interpret/check/compile intersection) can't
// pass vacuously. Measured at 27712 over the length-4 matrix; pinned
// below the live count as headroom.
const minPassingRows = 27000

// minCompiledRows is the floor for the compiler-parity gate: at least
// this many non-error rows must take the bytecode-compiled path, or the
// parity assertion is vacuous (a compiler that refused everything would
// pass with zero comparisons). Measured at 27712 over the length-4
// matrix; pinned a little below the live count as headroom against
// incidental jitter. RAISE it when the compilable subset widens; only
// lower it with a documented reason.
const minCompiledRows = 27000

// matrixRow is one parsed data row of the matrix.
type matrixRow struct {
	line     int
	input    string
	expected string // canonical value, or "ERROR:<class>"
	n        int    // element count (1..4), parsed from the note column
}

// readMatrix parses the full matrix file into data rows.
func readMatrix(t *testing.T) []matrixRow { return readRows(t, matrixPath) }

// readRows parses a generated tsv (full matrix or passing subset) into
// data rows, skipping the comment/blank header lines.
func readRows(t *testing.T, path string) []matrixRow {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v (regenerate with `make spec-gen`)", path, err)
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
			t.Fatalf("%s:L%d: malformed row %q", path, n, line)
		}
		// The note column ("N-elem …") leads with the element count; default
		// 0 (treated as "short", always fully covered) if absent.
		elems := 0
		if len(parts) >= 3 {
			if note := strings.TrimSpace(parts[2]); len(note) > 0 && note[0] >= '1' && note[0] <= '9' {
				elems = int(note[0] - '0')
			}
		}
		rows = append(rows, matrixRow{line: n, input: strings.TrimSpace(parts[0]), expected: strings.TrimSpace(parts[1]), n: elems})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	if len(rows) == 0 {
		t.Fatalf("%s has no data rows", path)
	}
	return rows
}

// The generator's own evaluation recipe (run), error-class extraction
// (errorClass), and frozen clock (specClock) are reused verbatim from
// main.go, so a generated row reproduces here byte-for-byte.

// The matrix grows as len(alphabet)^maxLen, so at length 4 it is ~137k
// rows. Each gate therefore fans its rows across NumCPU workers (every
// row builds an isolated lang/registry instance with no shared mutable
// state — the same concurrency the langspec suite's
// TestSpecCompiledConcurrentRowsRaceFree relies on). Workers report via
// failSink (goroutine-safe t.Errorf, capped to avoid flooding output on
// a systemic regression); they must never call t.Fatalf / t.FailNow.

// failSink funnels worker-goroutine failures to t.Errorf, capping the
// number actually printed while counting them all.
type failSink struct {
	t   *testing.T
	n   int64
	max int64
}

func (s *failSink) fail(format string, args ...any) {
	if atomic.AddInt64(&s.n, 1) <= s.max {
		s.t.Errorf(format, args...)
	}
}

func (s *failSink) total() int64 { return atomic.LoadInt64(&s.n) }

// report logs the failure tally and notes any suppressed-from-print
// overflow so the count is never silently hidden.
func (s *failSink) report() {
	if n := s.total(); n > s.max {
		s.t.Errorf("%d failures total; %d shown above, %d suppressed", n, s.max, n-s.max)
	}
}

// forEachRow runs work on every row across NumCPU worker goroutines.
func forEachRow(rows []matrixRow, work func(r matrixRow)) {
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	ch := make(chan matrixRow, workers*2)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range ch {
				work(r)
			}
		}()
	}
	for _, r := range rows {
		ch <- r
	}
	close(ch)
	wg.Wait()
}

// TestSyntaxMatrixInterpreter re-evaluates every row and asserts the
// interpreter reproduces the recorded expectation exactly — the frozen
// deterministic snapshot.
func TestSyntaxMatrixInterpreter(t *testing.T) {
	rows := readMatrix(t)
	sink := &failSink{t: t, max: 50}
	forEachRow(rows, func(r matrixRow) {
		out, err := run(r.input)
		var got string
		if err != nil {
			got = errorClass(err)
		} else {
			got = eng.Canon(out)
		}
		if got != r.expected {
			sink.fail("L%d %q: interpreter gave %q, spec records %q", r.line, r.input, got, r.expected)
		}
	})
	sink.report()
	t.Logf("interpreter: %d rows, %d mismatches", len(rows), sink.total())
}

// TestSyntaxMatrixCompilerParity asserts the bytecode compiler agrees
// with the interpreter on every non-error row it accepts.
func TestSyntaxMatrixCompilerParity(t *testing.T) {
	rows := readMatrix(t)
	var compiled, errorRows int64
	sink := &failSink{t: t, max: 50}
	forEachRow(rows, func(r matrixRow) {
		if strings.HasPrefix(r.expected, "ERROR:") {
			atomic.AddInt64(&errorRows, 1)
			return
		}

		ac, err := lang.New()
		if err != nil {
			sink.fail("lang.New: %v", err)
			return
		}
		ac.SetClock(specClock)
		gotC, wasCompiled, errC := ac.RunCompiled(r.input)
		if !wasCompiled {
			return // outside the compilable subset — interpreter fallback covers it
		}
		atomic.AddInt64(&compiled, 1)

		ai, err := lang.New()
		if err != nil {
			sink.fail("lang.New: %v", err)
			return
		}
		ai.SetClock(specClock)
		gotI, errI := ai.Run(r.input)

		if (errC != nil) != (errI != nil) {
			sink.fail("L%d %q: error divergence compiled=%v interpreted=%v", r.line, r.input, errC, errI)
			return
		}
		if errC != nil {
			return
		}
		if renderAny(gotC) != renderAny(gotI) {
			sink.fail("L%d %q: compiled=%q interpreted=%q", r.line, r.input, renderAny(gotC), renderAny(gotI))
		}
	})
	sink.report()
	t.Logf("compiler parity: %d rows compiled (%d error rows skipped), %d mismatches", compiled, errorRows, sink.total())
	if compiled < minCompiledRows {
		t.Errorf("only %d rows took the compiled path (floor %d) — the compiler regressed to refusing the matrix",
			compiled, minCompiledRows)
	}
}

// Checker-gate sampling. The checker is the costliest path (~1.7ms per
// Check even with instance reuse), so at length 4 an exhaustive
// twice-over-everything pass is ~7min — too slow for `make test`. The
// gate is therefore scoped to what each stride controls:
//
//   - longNoPanicStride: length-4 rows get the no-panic + classification
//     check on every Nth row; length<=3 rows (all 3-grams and shorter)
//     are ALWAYS checked. So no-panic coverage is exhaustive through
//     length 3 and a 1/8 sample at length 4 — and the length-4 space is
//     already covered exhaustively by the interpreter and compiler gates,
//     which is where row-specific result bugs would surface anyway.
//   - determinismStride: among the rows it checks, the gate re-runs a
//     ~1/64 sample a second time and compares. Determinism is a global
//     property, not per-row, so a sparse but diverse sample suffices; on
//     a reused instance the re-run also asserts check idempotency (state
//     accumulation would diverge on the second pass).
const (
	longNoPanicStride = 8
	determinismStride = 64
)

// TestSyntaxMatrixCheckerDeterministic asserts the type checker never
// panics and is deterministic, over the sampled scope described above.
// It does not gate on accuracy — the curated false-positive / soundness
// ratchets live in the langspec suite — only on the trust properties
// this matrix is responsible for: no-crash and determinism.
//
// Because the alphabet contains NO state-mutating word (no
// def/set/import/macro/var — every atom is a literal, a stack shuffle,
// or a pure operator), a check leaves the registry untouched, so each
// worker reuses ONE checker instance across all its rows (this is what
// makes the gate affordable; the determinism re-run guards the reuse).
func TestSyntaxMatrixCheckerDeterministic(t *testing.T) {
	rows := readMatrix(t)
	var checked, clean, flagged, sampled int64
	sink := &failSink{t: t, max: 50}

	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	ch := make(chan matrixRow, workers*2)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := lang.New()
			if err != nil {
				sink.fail("lang.New: %v", err)
				return
			}
			a.SetClock(specClock)
			for r := range ch {
				if r.n >= 4 && r.line%longNoPanicStride != 0 {
					continue // length-4 no-panic coverage is sampled
				}
				atomic.AddInt64(&checked, 1)
				fa := checkWith(sink, a, r.input) // no-panic + classify
				if strings.HasPrefix(fa, "errors=0 ") {
					atomic.AddInt64(&clean, 1)
				} else {
					atomic.AddInt64(&flagged, 1)
				}
				if r.line%determinismStride == 0 {
					atomic.AddInt64(&sampled, 1)
					if fb := checkWith(sink, a, r.input); fa != fb {
						sink.fail("L%d %q: checker non-deterministic:\n  run1=%s\n  run2=%s", r.line, r.input, fa, fb)
					}
				}
			}
		}()
	}
	for _, r := range rows {
		ch <- r
	}
	close(ch)
	wg.Wait()

	sink.report()
	t.Logf("checker: %d/%d rows checked (%d clean, %d flagged), %d determinism-sampled, %d failures",
		checked, len(rows), clean, flagged, sampled, sink.total())
}

// checkWith runs one row through the supplied checker instance and
// renders a stable fingerprint (error count + residual stack types) used
// to compare two runs for determinism. A panic is recovered into a
// sentinel so the no-panic property is asserted as a normal test
// failure, never a crash.
func checkWith(sink *failSink, a *lang.AQL, input string) (fp string) {
	defer func() {
		if rec := recover(); rec != nil {
			sink.fail("checker PANICKED on %q: %v", input, rec)
			fp = fmt.Sprintf("PANIC:%v", rec)
		}
	}()
	res, err := a.Check(input)
	if err != nil {
		return "parse-or-run-error:" + errorClass(err)
	}
	return fmt.Sprintf("errors=%d warnings=%d [%s]", res.Summary.Errors, res.Summary.Warnings, strings.Join(res.Stack, " "))
}

// hasCheckError reports whether a CompileCheck/Check result carries any
// error-severity diagnostic (the checker rejected the program).
func hasCheckError(res lang.CheckResult) bool {
	for _, d := range res.Diagnostics {
		if d.Severity == lang.SeverityError {
			return true
		}
	}
	return false
}

// TestSyntaxMatrixFailFrontier re-verifies the two minimal-failing-prefix
// files and, with them, the "compiler runs the checker first" contract.
//
//   - Every row of syntax-matrix-fail-check.tsv must be REJECTED BY THE
//     CHECKER (a parse/run error or an error-severity diagnostic) AND the
//     compiler must emit NO Program for it. The second clause is the
//     contract: a program the checker rejects is never lowered to
//     bytecode, because CompileCheck runs the checker first and refuses on
//     any error diagnostic. Any check-rejected prefix that still produced
//     a Program would be a violation.
//   - Every row of syntax-matrix-fail-compile.tsv must PASS THE CHECKER
//     (no error diagnostic) yet still produce NO Program — a genuine
//     compile-stage refusal, disjoint from the check-fail set.
//   - Every row of syntax-matrix-fail-runtime.tsv must PASS THE CHECKER
//     AND COMPILE to a Program, yet ERROR when interpreted — a runtime
//     fault the static stages cannot see, disjoint from both other sets.
//
// Counts are floored so no file can silently collapse.
func TestSyntaxMatrixFailFrontier(t *testing.T) {
	checkRows := readRows(t, failCheckPath)
	compileRows := readRows(t, failCompilePath)
	runtimeRows := readRows(t, failRuntimePath)

	// fail-check: checker rejects, and the compiler emits no Program.
	var checkVerified, compiledDespiteReject int64
	csink := &failSink{t: t, max: 50}
	forEachRow(checkRows, func(r matrixRow) {
		a, err := lang.New()
		if err != nil {
			csink.fail("lang.New: %v", err)
			return
		}
		a.SetClock(specClock)
		prog, _, res, errc := a.CompileCheck(r.input)
		if errc == nil && !hasCheckError(res) {
			csink.fail("L%d %q: in fail-check but the checker did NOT reject it", r.line, r.input)
			return
		}
		if prog != nil {
			atomic.AddInt64(&compiledDespiteReject, 1)
			csink.fail("L%d %q: checker rejected it yet a Program was emitted — compiler did not run the checker first", r.line, r.input)
			return
		}
		atomic.AddInt64(&checkVerified, 1)
	})
	csink.report()

	// fail-compile: checker clean, but the compiler still emits no Program.
	var compileVerified int64
	psink := &failSink{t: t, max: 50}
	forEachRow(compileRows, func(r matrixRow) {
		a, err := lang.New()
		if err != nil {
			psink.fail("lang.New: %v", err)
			return
		}
		a.SetClock(specClock)
		prog, _, res, errc := a.CompileCheck(r.input)
		if errc != nil || hasCheckError(res) {
			psink.fail("L%d %q: in fail-compile but the checker rejected it (belongs in fail-check)", r.line, r.input)
			return
		}
		if prog != nil {
			psink.fail("L%d %q: in fail-compile but it DID compile to a Program", r.line, r.input)
			return
		}
		atomic.AddInt64(&compileVerified, 1)
	})
	psink.report()

	// fail-runtime: checker clean AND compiles, yet errors when interpreted.
	var runtimeVerified int64
	rsink := &failSink{t: t, max: 50}
	forEachRow(runtimeRows, func(r matrixRow) {
		a, err := lang.New()
		if err != nil {
			rsink.fail("lang.New: %v", err)
			return
		}
		a.SetClock(specClock)
		prog, _, res, errc := a.CompileCheck(r.input)
		if errc != nil || hasCheckError(res) {
			rsink.fail("L%d %q: in fail-runtime but the checker rejected it (belongs in fail-check)", r.line, r.input)
			return
		}
		if prog == nil {
			rsink.fail("L%d %q: in fail-runtime but it did NOT compile (belongs in fail-compile)", r.line, r.input)
			return
		}
		if _, errRun := a.Run(r.input); errRun == nil {
			rsink.fail("L%d %q: in fail-runtime but it ran without error", r.line, r.input)
			return
		}
		atomic.AddInt64(&runtimeVerified, 1)
	})
	rsink.report()

	t.Logf("fail-check: %d/%d verified (checker rejects + compiler refuses); checker-runs-first violations: %d",
		checkVerified, len(checkRows), compiledDespiteReject)
	t.Logf("fail-compile: %d/%d verified (checker clean + compiler refuses)", compileVerified, len(compileRows))
	t.Logf("fail-runtime: %d/%d verified (checker clean + compiles + errors at run)", runtimeVerified, len(runtimeRows))

	if compiledDespiteReject != 0 {
		t.Errorf("%d check-rejected prefixes still compiled — the compiler did NOT run the checker first", compiledDespiteReject)
	}
	if checkVerified < minCheckFailRows {
		t.Errorf("only %d type-check-fail prefixes verified (floor %d)", checkVerified, minCheckFailRows)
	}
	if compileVerified < minCompileFailRows {
		t.Errorf("only %d compile-fail prefixes verified (floor %d)", compileVerified, minCompileFailRows)
	}
	if runtimeVerified < minRuntimeFailRows {
		t.Errorf("only %d runtime-fail prefixes verified (floor %d)", runtimeVerified, minRuntimeFailRows)
	}
}

// TestSyntaxMatrixPassing re-verifies the curated passing subset
// (syntax-matrix-passing.tsv): every row must clear ALL THREE pipelines —
// interpret to the recorded value, type-check with zero errors, and
// compile to a result identical to the interpreter's. This is the
// invariant the extractor (specgen -extract) builds the file on; the test
// is its guard, so the file can't silently drift out of agreement with
// the engine. None of these rows is an error row (an error row can never
// pass interpret), and the count is floored so the subset can't collapse
// to empty unnoticed.
func TestSyntaxMatrixPassing(t *testing.T) {
	rows := readRows(t, passingPath)
	var verified int64
	sink := &failSink{t: t, max: 50}
	forEachRow(rows, func(r matrixRow) {
		if strings.HasPrefix(r.expected, "ERROR:") {
			sink.fail("L%d %q: error row present in the passing subset", r.line, r.input)
			return
		}
		// 1. interpret → recorded canonical value (the frozen result).
		out, err := run(r.input)
		if err != nil {
			sink.fail("L%d %q: passing row failed to interpret: %v", r.line, r.input, err)
			return
		}
		if got := eng.Canon(out); got != r.expected {
			sink.fail("L%d %q: interpreter gave %q, passing row records %q", r.line, r.input, got, r.expected)
			return
		}
		// 2. check → zero error-severity diagnostics.
		a, err := lang.New()
		if err != nil {
			sink.fail("lang.New: %v", err)
			return
		}
		a.SetClock(specClock)
		res, errChk := a.Check(r.input)
		if errChk != nil {
			sink.fail("L%d %q: passing row failed to check: %v", r.line, r.input, errChk)
			return
		}
		if res.Summary.Errors != 0 {
			sink.fail("L%d %q: passing row has %d checker error(s)", r.line, r.input, res.Summary.Errors)
			return
		}
		// 3. compile → accepted AND identical to the interpreter.
		gotC, wasCompiled, errC := a.RunCompiled(r.input)
		if !wasCompiled {
			sink.fail("L%d %q: passing row did not compile", r.line, r.input)
			return
		}
		gotI, errI := a.Run(r.input)
		if errC != nil || errI != nil {
			sink.fail("L%d %q: run error compiled=%v interpreted=%v", r.line, r.input, errC, errI)
			return
		}
		if renderAny(gotC) != renderAny(gotI) {
			sink.fail("L%d %q: compiled=%q interpreted=%q", r.line, r.input, renderAny(gotC), renderAny(gotI))
			return
		}
		atomic.AddInt64(&verified, 1)
	})
	sink.report()
	t.Logf("passing subset: %d/%d rows verified through interpret+check+compile, %d failures",
		verified, len(rows), sink.total())
	if verified < minPassingRows {
		t.Errorf("only %d passing rows verified (floor %d) — the passing subset shrank or the extractor regressed",
			verified, minPassingRows)
	}
}
