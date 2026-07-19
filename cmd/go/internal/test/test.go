// Package test implements `aql test` — discover *_test.aql suites, run each on
// the bytecode compiler by DEFAULT (the normal, fast execution mode; falls back
// to the interpreter only when a file is uncompilable), print aql:test's
// per-case report, and exit non-zero if any case failed or any file errored.
//
// With --coverage the runner arms the engine's line-coverage hook BEFORE each
// file runs, so a `*_test.aql` that imports a user module (`import "./mod.aql"`)
// records that module's executed source rows (feature C tags the module because
// the hook is armed at import time). After the run it reports each imported
// module's line coverage.
//
// Coverage is measured in whatever engine mode the suite runs — compiled by
// default. The bytecode VM folds some source positions (e.g. a trailing
// bare-word return), so compiled coverage is a SUBSET of the interpreter's:
// pair --coverage with --no-compile for the interpreter's line-granular
// coverage when the goal is to drive a module to 100%.
package test

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/pathutil"
	"github.com/aql-lang/aql/cmd/go/internal/permsflags"
	"github.com/aql-lang/aql/cmd/go/internal/run"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
)

// walkDir and newAQL are test seams (design/TEST-SEAMS.10.md): filepath.WalkDir
// never surfaces a read error under the test's root uid, and lang.New's only
// error path (registry init) is unreachable in a healthy build, so both
// error arms are driven by swapping these in a test.
var (
	walkDir = filepath.WalkDir
	newAQL  = lang.New
)

type cmd struct{}

// New returns the test subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "test" }
func (*cmd) Synopsis() string { return "discover and run *_test.aql suites (compiled by default)" }

func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(stderr)
	registry := fs.String("r", "", "registry path")
	coverage := fs.Bool("coverage", false, "measure imported-user-module line coverage; print a summary and write an HTML report")
	coverageDir := fs.String("coverage-dir", "coverage", "directory for the HTML coverage report written with --coverage")
	compileFlag := fs.Bool("compile", false, "execute via the bytecode compiler when possible; silent interpreter fallback (the default; also enabled by AQL_COMPILE)")
	noCompileFlag := fs.Bool("no-compile", false, "run each suite on the interpreter instead of the default bytecode compiler (also enabled by AQL_NO_COMPILE)")
	forceCompileFlag := fs.Bool("force-compile", false, "REQUIRE the bytecode compiler — a suite that is not compilable errors instead of falling back")
	var pf permsflags.Flags
	permsflags.Register(fs, &pf)
	if err := fs.Parse(args); err != nil {
		return 1
	}

	pol, err := pf.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	targets := fs.Args()
	if len(targets) == 0 {
		targets = []string{"."}
	}
	files, err := discover(targets)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if len(files) == 0 {
		fmt.Fprintln(stderr, "no *_test.aql files found")
		return 1
	}

	o := run.OptionsFor(pathutil.Expand(*registry), 0, pol)
	mode := run.ResolveCompileMode(*compileFlag, *forceCompileFlag, *noCompileFlag)

	var accum *covAccum
	if *coverage {
		accum = newCovAccum()
	}
	var passed, failed int
	anyErr := false
	for _, f := range files {
		p, fl, errored := runFile(stdout, stderr, f, o, mode, accum)
		passed += p
		failed += fl
		if errored {
			anyErr = true
		}
	}
	fmt.Fprintf(stdout, "\n=== %d files: %d passed, %d failed ===\n", len(files), passed, failed)
	if accum != nil {
		accum.report(stdout)
		index, err := accum.writeHTML(*coverageDir)
		if err != nil {
			fmt.Fprintf(stderr, "warning: coverage report: %s\n", err)
		} else if index != "" {
			fmt.Fprintf(stdout, "coverage report: %s\n", index)
		}
	}
	if failed > 0 || anyErr {
		return 1
	}
	return 0
}

// discover expands each target into test files: a file target is taken
// verbatim (run even if it lacks the _test.aql suffix — an explicit request),
// a directory is walked recursively for *_test.aql. Results are de-duplicated
// and sorted for a stable run order. A target that does not exist is an error.
func discover(targets []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, t := range targets {
		t = pathutil.Expand(t)
		info, err := os.Stat(t)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(t)
			continue
		}
		if err := walkDir(t, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, "_test.aql") {
				add(path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// runFile runs one suite and prints its report. When accum is non-nil the run
// is coverage-armed and the suite's imported-module coverage is folded into it.
// It returns the suite's passed and failed case counts, plus an errored flag set
// when the file could not be read, initialised, run, or its results read — an
// errored suite counts no cases but forces a non-zero final exit.
func runFile(stdout, stderr io.Writer, path string, o lang.Options, mode run.CompileMode, accum *covAccum) (passed, failed int, errored bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s: %s\n", path, err)
		return 0, 0, true
	}
	a, err := newAQL(o)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s: init: %s\n", path, err)
		return 0, 0, true
	}
	// Route the framework's streams to the runner's writers: a test body's
	// `print` and aql:test's loud per-case `FAIL` line go to the runner's
	// stdout/stderr rather than leaking to os.Stderr.
	a.SetOutput(stdout)
	a.NativeRegistry().ErrOutput = stderr
	if accum != nil {
		disarm := modules.ArmCoverageCollector(a.NativeRegistry())
		defer disarm()
	}
	fmt.Fprintf(stdout, "# %s\n", path)
	if err := runSource(a, string(data), mode); err != nil {
		fmt.Fprintf(stderr, "error: %s: %s\n", path, err)
		return 0, 0, true
	}
	_, passed, failed, report, err := readOutcome(a)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s: reading results: %s\n", path, err)
		return 0, 0, true
	}
	fmt.Fprintln(stdout, report)
	if accum != nil {
		accum.collect(a.NativeRegistry())
	}
	return passed, failed, false
}

// runSource executes src on the instance in the selected engine mode. The
// default (CompileTry) is a.Run — bytecode when compilable, silent interpreter
// fallback otherwise. A recorded test failure does NOT error the run (the
// framework records it and continues); only a genuine runtime/parse error does.
func runSource(a *lang.AQL, src string, mode run.CompileMode) error {
	switch mode {
	case run.CompileForce:
		_, err := a.RunCompiledStrict(src)
		return err
	case run.CompileOff:
		_, err := a.RunInterp(src)
		return err
	default:
		_, err := a.Run(src)
		return err
	}
}

// readOutcome reads aql:test's accumulated results off the (already-run)
// instance: `Test.summary Test.report` leaves a {total,passed,failed} map and
// the human report string on the stack in that order. The helpers below skip
// whichever value they don't consume, so one read yields both.
func readOutcome(a *lang.AQL) (total, passed, failed int, report string, err error) {
	vals, err := a.RunInterpValues("Test.summary Test.report")
	if err != nil {
		return 0, 0, 0, "", err
	}
	total, passed, failed = tallyFromSummary(vals)
	report = stringFromStack(vals)
	return total, passed, failed, report, nil
}

// tallyFromSummary extracts {total,passed,failed} from the first concrete map
// on a residual stack, tolerating non-map values (skipped) so a malformed read
// never panics.
func tallyFromSummary(vals []native.Value) (total, passed, failed int) {
	for _, v := range vals {
		m, err := native.AsMap(v)
		if err != nil {
			continue
		}
		total = mapInt(m, "total")
		passed = mapInt(m, "passed")
		failed = mapInt(m, "failed")
	}
	return total, passed, failed
}

// stringFromStack returns the last string value on a residual stack ("" when
// none), skipping non-string values.
func stringFromStack(vals []native.Value) string {
	out := ""
	for _, v := range vals {
		s, err := native.AsString(v)
		if err != nil {
			continue
		}
		out = s
	}
	return out
}

// mapInt reads an integer field, defaulting to 0 for an absent or non-integer
// value (branch-free: the ignored errors collapse to the zero value).
func mapInt(m native.ReadMap, key string) int {
	v, _ := m.Get(key)
	n, _ := native.AsInteger(v)
	return int(n)
}

// coverageLine renders one module's coverage summary, appending the uncovered
// line numbers when any remain. Pure (no I/O) so both arms are unit-testable.
func coverageLine(id string, covered, total int, uncovered []int) string {
	line := fmt.Sprintf("  cover %s  %.1f%% (%d/%d)", id, percent(covered, total), covered, total)
	if len(uncovered) > 0 {
		line += "  uncovered: " + joinInts(uncovered)
	}
	return line + "\n"
}

// joinInts renders a row-number list as "3, 6, 7".
func joinInts(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, ", ")
}

// percent is covered/total as a percentage, defined as 100 for an empty
// denominator (a module with no executable rows is vacuously fully covered).
func percent(covered, total int) float64 {
	if total == 0 {
		return 100
	}
	return float64(covered) / float64(total) * 100
}
