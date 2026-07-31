package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// exitCodeAsync drives fn with osExit swapped to a stub that records
// the code and kills only the CALLING goroutine (runtime.Goexit) — an
// exit inside a worker goroutine must not take the test process down
// with it, and the mode function then drains normally through wg.Wait.
// Returns -1 when nothing exited. fn itself runs on the test goroutine,
// so callers must hand the mode function valid output paths (an exit on
// the test goroutine would end the test as a failure).
func exitCodeAsync(t *testing.T, fn func()) int {
	t.Helper()
	prev := osExit
	t.Cleanup(func() { osExit = prev })
	var code int64 = -1
	osExit = func(c int) {
		atomic.StoreInt64(&code, int64(c))
		runtime.Goexit()
	}
	fn()
	return int(atomic.LoadInt64(&code))
}

func writeTSV(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// run()'s registry-construction arm: only drivable through the seam
// (native.DefaultRegistry cannot fail on a healthy build).
func TestRunRegistryFailure(t *testing.T) {
	prev := newNativeRegistry
	t.Cleanup(func() { newNativeRegistry = prev })
	newNativeRegistry = func(...func(*eng.Registry)) (*eng.Registry, error) {
		return nil, errors.New("registry boom")
	}
	if _, err := run("1"); err == nil || !strings.Contains(err.Error(), "registry boom") {
		t.Errorf("run with failing registry: got %v, want registry boom", err)
	}
}

// Worker-goroutine construction failures across all three parallel
// modes, plus the workers<1 floor (numCPU stubbed to 0).
func TestWorkerLangNewFailures(t *testing.T) {
	prevCPU, prevNew := numCPU, langNew
	t.Cleanup(func() { numCPU, langNew = prevCPU, prevNew })
	numCPU = func() int { return 0 } // also drives the single-worker floor
	langNew = func() (*lang.BORU, error) { return nil, errors.New("no lang") }

	dir := t.TempDir()
	passing := writeTSV(t, dir, "passing.tsv", "#kept: 1 of 1 scanned (max-len 1)\n1\t1\n")
	out := func(n string) string { return filepath.Join(dir, n) }

	if got := exitCodeAsync(t, func() { extractPassing(passing, out("p.tsv"), 4) }); got != 1 {
		t.Errorf("extractPassing worker lang.New failure exited %d, want 1", got)
	}
	if got := exitCodeAsync(t, func() {
		extractFrontier(passing, out("c.tsv"), out("k.tsv"), out("r.tsv"), 1)
	}); got != 1 {
		t.Errorf("extractFrontier worker lang.New failure exited %d, want 1", got)
	}
	if got := exitCodeAsync(t, func() {
		extendFive(passing, passing, out("5p.tsv"), out("5c.tsv"), out("5k.tsv"), out("5r.tsv"), "")
	}); got != 1 {
		t.Errorf("extendFive worker lang.New failure exited %d, want 1", got)
	}
}

// extendFive's worker also constructs a native registry (for canonical
// expected values); its failure arm exits 1.
func TestExtendFiveRegistryFailure(t *testing.T) {
	prevCPU, prevReg := numCPU, newNativeRegistry
	t.Cleanup(func() { numCPU, newNativeRegistry = prevCPU, prevReg })
	numCPU = func() int { return 0 }
	newNativeRegistry = func(...func(*eng.Registry)) (*eng.Registry, error) {
		return nil, errors.New("no registry")
	}

	dir := t.TempDir()
	passing := writeTSV(t, dir, "passing.tsv", "#kept: 1 of 1 scanned (max-len 1)\n1\t1\n")
	out := func(n string) string { return filepath.Join(dir, n) }
	if got := exitCodeAsync(t, func() {
		extendFive(passing, passing, out("5p.tsv"), out("5c.tsv"), out("5k.tsv"), out("5r.tsv"), "")
	}); got != 1 {
		t.Errorf("extendFive registry failure exited %d, want 1", got)
	}
}

// extractPassing's compiled-result-divergence arm: the compiler
// "succeeds" but returns a different value than the interpreter, so the
// row must be dropped from the passing set.
func TestExtractPassingCompiledDivergence(t *testing.T) {
	prev := runCompiled
	t.Cleanup(func() { runCompiled = prev })
	runCompiled = func(*lang.BORU, string) ([]any, bool, error) {
		return []any{"divergent"}, true, nil
	}

	dir := t.TempDir()
	in := writeTSV(t, dir, "matrix.tsv", "#matrix\n1\t1\t1\n")
	outPath := filepath.Join(dir, "passing.tsv")
	extractPassing(in, outPath, 4)

	rows, err := readDataRows(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("divergent row kept in passing set: %+v", rows)
	}
}

// classifyFrontier's empty-diagnostic-code fallback and extractFrontier's
// checker-runs-first violation counter — both only drivable through the
// compileCheck seam (the real compiler never emits a Program alongside an
// error-severity diagnostic, and every real diagnostic carries a code).
func TestFrontierCheckerFirstViolation(t *testing.T) {
	real, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	prog, _, _, err := real.CompileCheck("1 2 add")
	if err != nil || prog == nil {
		t.Fatalf("fixture program did not compile: %v", err)
	}

	prevCPU, prevCC := numCPU, compileCheck
	t.Cleanup(func() { numCPU, compileCheck = prevCPU, prevCC })
	numCPU = func() int { return 0 }
	compileCheck = func(*lang.BORU, string) (*lang.Program, string, lang.CheckResult, error) {
		res := lang.CheckResult{Diagnostics: []lang.CheckDiagnostic{{Severity: lang.SeverityError}}}
		return prog, "", res, nil
	}

	dir := t.TempDir()
	passing := writeTSV(t, dir, "passing.tsv", "#kept: 1 of 1 scanned (max-len 1)\n1\t1\n")
	checkOut := filepath.Join(dir, "check.tsv")
	extractFrontier(passing, checkOut,
		filepath.Join(dir, "compile.tsv"), filepath.Join(dir, "runtime.tsv"), 1)

	// Every candidate (each single atom except the passing "1") lands in
	// the check file with the "error" fallback code.
	n := 0
	err = forEachDataRow(checkOut, func(input, expected, note string, _ int) {
		n++
		if expected != "error" && note != "error" {
			t.Errorf("row %q classified as (%q, %q), want the \"error\" fallback code", input, expected, note)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := len(alphabet) - 1; n != want {
		t.Errorf("check file has %d rows, want %d", n, want)
	}
}

func TestFreshDivergenceArms(t *testing.T) {
	t.Run("lang.New failure", func(t *testing.T) {
		prev := langNew
		t.Cleanup(func() { langNew = prev })
		langNew = func() (*lang.BORU, error) { return nil, errors.New("no lang") }
		if note, real := freshDivergence("1"); real || note != "" {
			t.Errorf("freshDivergence with failing lang.New = (%q, %v), want (\"\", false)", note, real)
		}
	})
	t.Run("error divergence", func(t *testing.T) {
		prev := runCompiled
		t.Cleanup(func() { runCompiled = prev })
		runCompiled = func(*lang.BORU, string) ([]any, bool, error) {
			return nil, true, errors.New("compiled boom")
		}
		note, real := freshDivergence("1")
		if !real || !strings.Contains(note, "error-divergence") {
			t.Errorf("freshDivergence error arm = (%q, %v), want error-divergence note", note, real)
		}
	})
	t.Run("value divergence", func(t *testing.T) {
		prev := runCompiled
		t.Cleanup(func() { runCompiled = prev })
		runCompiled = func(*lang.BORU, string) ([]any, bool, error) {
			return []any{"zzz"}, true, nil
		}
		note, real := freshDivergence("1")
		if !real || !strings.Contains(note, "interpreted=") {
			t.Errorf("freshDivergence mismatch arm = (%q, %v), want interpreted/compiled note", note, real)
		}
	})
}

// canonOf's parse- and run-error arms are unreachable through the
// extendFive worker (CompileCheck has already parsed and vetted the
// candidate), so they are pinned directly.
func TestCanonOfErrorArms(t *testing.T) {
	reg, err := newNativeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if got := canonOf(reg, "]"); got != "" {
		t.Errorf("canonOf(unparsable) = %q, want \"\"", got)
	}
	if got := canonOf(reg, "drop"); got != "" {
		t.Errorf("canonOf(run-error) = %q, want \"\"", got)
	}
	if got := canonOf(reg, "1"); got == "" {
		t.Error("canonOf(\"1\") = \"\", want the canonical value")
	}
}

// A reused-instance divergence that SURVIVES the fresh re-verification is
// a genuine compiler mismatch: the candidate lands in the mismatch bucket
// and the summary prints the divergence note.
func TestExtendFiveMismatchPath(t *testing.T) {
	prevCPU, prevRC := numCPU, runCompiled
	t.Cleanup(func() { numCPU, runCompiled = prevCPU, prevRC })
	numCPU = func() int { return 1 }
	runCompiled = func(*lang.BORU, string) ([]any, bool, error) {
		return []any{"zzz"}, true, nil
	}

	dir := t.TempDir()
	passing := writeTSV(t, dir, "passing.tsv", "#kept: 2 of 2 scanned (max-len 4)\n1\t1\n1 1\t1 1\n")
	len123 := writeTSV(t, dir, "len123.tsv", "#kept: 1 of 1 scanned (max-len 3)\n1\t1\n")
	mismatch := filepath.Join(dir, "mismatch.tsv")
	extendFive(passing, len123,
		filepath.Join(dir, "5p.tsv"), filepath.Join(dir, "5c.tsv"),
		filepath.Join(dir, "5k.tsv"), filepath.Join(dir, "5r.tsv"), mismatch)

	n := 0
	if err := forEachDataRow(mismatch, func(string, string, string, int) { n++ }); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("no fresh-verified divergences written to the mismatch file")
	}
}
