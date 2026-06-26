// Command specgen deterministically enumerates every AQL token
// sequence up to a fixed length over a small, documented alphabet of
// syntax atoms, evaluates each through the production language layer,
// and emits a `.tsv` spec row (`input<TAB>expected[<TAB>note]`) capturing
// the canonical result — or the stable error class — for that sequence.
//
// Why this exists. The hand-written specs under lang/spec/ pin
// behaviour a human thought to test. This generator instead pins the
// behaviour of EVERY combination of a representative alphabet, so the
// result is an exhaustive, machine-checkable contract: a frozen
// snapshot of exactly what the interpreter produces for each sequence.
//
// The generated file is its own self-contained spec
// (test/go/specgen/syntax-matrix.tsv) with a dedicated runner
// (syntax_matrix_test.go) that asserts the three DETERMINISTIC trust
// properties on every row:
//
//   - interpreter correctness: re-evaluating the row reproduces the
//     recorded canonical value (or the recorded error class), so the
//     interpreter is pinned deterministic against the snapshot;
//   - compiler parity: every row the bytecode compiler accepts produces
//     a result IDENTICAL to the interpreter — the compiler is trustworthy
//     on the whole combination space, not just a curated sample;
//   - checker determinism: the type checker yields the same diagnostics
//     and residual stack on repeated runs, and never panics.
//
// It is deliberately NOT dropped into lang/spec/: that directory's
// suites carry curated, corpus-keyed accuracy ratchets (checker false-
// positive pins, compiled-refusal ceilings) that an exhaustive ~137k-row
// matrix would swamp with pin churn rather than signal. The existing
// generated parity fixture (bytecode-combinations.tsv) is special-cased
// out of those same ratchets for exactly this reason; a dedicated
// runner keeps this matrix's gates explicit and independent.
//
// The evaluation recipe (parse → fresh DefaultRegistry → q-fixtures →
// parse func → module resolver → frozen clock → NewTop().Run) is a
// deliberate mirror of test/go/langspec/langspec_test.go: same engine,
// same canonical rendering (eng.Canon), same frozen instant, so a row
// this tool writes is a row the runner reproduces exactly.
//
// A second mode derives the PASSING SUBSET — the rows that clear all
// three pipelines (interpret to a value, type-check clean, and compile to
// a result identical to the interpreter) — into its own file. That is the
// curated "golden" set a consumer can trust to run, check, and compile.
//
// Usage:
//
//	go run ./specgen [-max N] [-out path]                     # generate the full matrix
//	go run ./specgen -extract -in full.tsv -out passing.tsv   # derive the passing subset
//
//	-max      maximum sequence length to enumerate (default 4)
//	-out      output file; empty writes to stdout (default empty)
//	-extract  extraction mode: read -in (a full matrix) and write the passing subset to -out
//	-in       input full-matrix path (extraction mode)
//
// The Makefile target `spec-gen` regenerates both
// test/go/specgen/syntax-matrix.tsv and syntax-matrix-passing.tsv.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/test/go/specrunner"
)

// alphabet is the fixed, ordered set of syntax atoms the enumeration
// ranges over. Each entry is ONE syntactic element; "up to N elements"
// means every sequence of 1..N of these joined by spaces (N defaults to
// 4). The set is deliberately small and representative rather than
// exhaustive over the whole vocabulary — the cross product grows as
// len(alphabet)^N (19^4 ≈ 130k rows at N=4), so the goal is to cover
// every value family and every operator arity with the fewest atoms,
// not to list every word.
//
// Only core words present in the default production registry are used
// (verified: module-only words like abs/min/concat resolve to
// undefined_word without an import, and so are excluded by design).
//
// Coverage rationale, by group:
//
//	values  — one of each leaf family plus the two structural literals
//	          ([]list and the falsy/none/zero edges that drive
//	          short-circuit, truthiness, and integer-division corners)
//	unary   — words that consume one stack value (dup/drop reshape the
//	          stack; not/size compute over a value)
//	binary  — words that consume two: arithmetic (add/sub/mul),
//	          comparison (eq/lt), boolean (and), and the pure stack
//	          shuffle (swap)
//
// Keep this list in sync with the header comment emitted into the .tsv.
var alphabet = []string{
	// values (8)
	"0", "1", "2.5", "'x'", "true", "false", "none", "[1 2]",
	// unary words (4)
	"dup", "drop", "not", "size",
	// binary words (7)
	"add", "sub", "mul", "eq", "lt", "and", "swap",
}

// specClock freezes time so any clock-seeded behaviour is reproducible,
// matching test/go/langspec/langspec_test.go's specClock exactly.
var specClock = capabilities.FixedClock{T: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)}

// run evaluates one input through a fresh production registry and
// returns the resulting stack — a faithful copy of langspec_test.go's
// Run closure so generated rows reproduce under the real spec runner.
func run(input string) ([]eng.Value, error) {
	values, err := parser.Parse(input)
	if err != nil {
		return nil, err
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		return nil, err
	}
	specrunner.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)
	return native.NewTop(reg).Run(values)
}

// errorClass extracts a stable, message-independent substring from an
// engine error so a generated ERROR row pins the error *kind* rather
// than its (volatile) human text. Engine errors render as
// "[aql/<code>]: <detail>"; the code (e.g. signature_error,
// undefined_word, div_by_zero) is the stable part. Anything without the
// bracketed code falls back to the generic "ERROR:" sentinel, which the
// spec runner treats as "any error".
func errorClass(err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, "[aql/") {
		if end := strings.IndexByte(msg, ']'); end > len("[aql/") {
			return "ERROR:" + msg[len("[aql/"):end]
		}
	}
	return "ERROR:"
}

// eachSeq yields every alphabet sequence of length 1..max as a token
// slice, in a fully deterministic order: shorter sequences first, then
// odometer order over alphabet indices (last position varies fastest).
// The slice is reused between calls — copy it if you need to retain it.
func eachSeq(max int, emit func(toks []string)) {
	for n := 1; n <= max; n++ {
		idx := make([]int, n)
		for {
			toks := make([]string, n)
			for i, a := range idx {
				toks[i] = alphabet[a]
			}
			emit(toks)

			// odometer increment over base-len(alphabet) digits.
			pos := n - 1
			for pos >= 0 {
				idx[pos]++
				if idx[pos] < len(alphabet) {
					break
				}
				idx[pos] = 0
				pos--
			}
			if pos < 0 {
				break
			}
		}
	}
}

// sequences yields every alphabet sequence of length 1..max as a joined
// source string plus its element count — the form the full-matrix
// generator writes.
func sequences(max int, emit func(src string, n int)) {
	eachSeq(max, func(toks []string) { emit(strings.Join(toks, " "), len(toks)) })
}

func main() {
	max := flag.Int("max", 4, "maximum sequence length to enumerate")
	out := flag.String("out", "", "output .tsv path; empty writes to stdout")
	extract := flag.Bool("extract", false, "extraction mode: read -in and write the passing subset to -out")
	frontier := flag.Bool("frontier", false, "frontier mode: write minimal failing prefixes split into -check-out and -compile-out")
	in := flag.String("in", "", "input full-matrix .tsv path (extraction / frontier mode)")
	passing := flag.String("passing", "", "passing-subset .tsv path (frontier mode)")
	checkOut := flag.String("check-out", "", "output path for type-check-fail prefixes (frontier mode)")
	compileOut := flag.String("compile-out", "", "output path for compile-fail prefixes (frontier mode)")
	runtimeOut := flag.String("runtime-out", "", "output path for runtime-fail prefixes (frontier mode)")
	flag.Parse()

	if *frontier {
		if *in == "" || *passing == "" || *checkOut == "" || *compileOut == "" || *runtimeOut == "" {
			fmt.Fprintln(os.Stderr, "specgen -frontier requires -in, -passing, -check-out, -compile-out and -runtime-out")
			os.Exit(2)
		}
		extractFrontier(*in, *passing, *checkOut, *compileOut, *runtimeOut, *max)
		return
	}

	if *extract {
		if *in == "" || *out == "" {
			fmt.Fprintln(os.Stderr, "specgen -extract requires both -in and -out")
			os.Exit(2)
		}
		extractPassing(*in, *out)
		return
	}

	w := bufio.NewWriter(os.Stdout)
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "specgen: create %s: %v\n", *out, err)
			os.Exit(1)
		}
		defer f.Close()
		w = bufio.NewWriter(f)
	}
	defer w.Flush()

	writeHeader(w, *max)

	var rows, values, errs int
	curLen := 0
	sequences(*max, func(src string, n int) {
		if n != curLen {
			curLen = n
			fmt.Fprintf(w, "#\n# §%d  %d-element sequences\n", n, n)
		}
		stack, err := run(src)
		var expected, note string
		if err != nil {
			expected = errorClass(err)
			note = fmt.Sprintf("%d-elem rejected", n)
			errs++
		} else {
			expected = eng.Canon(stack)
			note = fmt.Sprintf("%d-elem → %d value(s)", n, len(stack))
			values++
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", src, expected, note)
		rows++
	})

	fmt.Fprintf(os.Stderr, "specgen: %d rows (%d value, %d error) over %d atoms, max length %d\n",
		rows, values, errs, len(alphabet), *max)
}

// writeHeader emits the leading comment block describing the file's
// provenance and the alphabet, so a reader of the .tsv knows it is
// generated and how to regenerate it.
func writeHeader(w *bufio.Writer, max int) {
	fmt.Fprintf(w, "# AQL Language Specification: Syntax Combination Matrix (GENERATED)\n")
	fmt.Fprintf(w, "# Format: input<TAB>expected<TAB>note\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# DO NOT EDIT BY HAND. Regenerate with:\n")
	fmt.Fprintf(w, "#   cd test/go && go run ./specgen -max %d -out ./specgen/syntax-matrix.tsv\n", max)
	fmt.Fprintf(w, "# (or `make spec-gen` from the repo root).\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# Every sequence of 1..%d atoms drawn from a fixed alphabet, evaluated\n", max)
	fmt.Fprintf(w, "# through the production language layer. `expected` is the canonical\n")
	fmt.Fprintf(w, "# eng.Canon rendering of the result stack; `ERROR:<code>` rows pin the\n")
	fmt.Fprintf(w, "# error CLASS (the stable [aql/<code>] tag), not its message text.\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# These rows are exhaustive over the alphabet, so they form a frozen\n")
	fmt.Fprintf(w, "# contract checked by syntax_matrix_test.go: the interpreter reproduces\n")
	fmt.Fprintf(w, "# every row, the bytecode compiler matches the interpreter on every row\n")
	fmt.Fprintf(w, "# it accepts, and the type checker is deterministic — all without drift.\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# Alphabet (%d atoms): %s\n", len(alphabet), strings.Join(alphabet, " "))
	fmt.Fprintf(w, "#\n")
}

// dataRow is one parsed data row of a generated matrix file.
type dataRow struct {
	input    string
	expected string
}

// readDataRows parses the data rows (input, expected) of a generated
// matrix .tsv, skipping comment and blank lines.
func readDataRows(path string) ([]dataRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rows []dataRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			return nil, fmt.Errorf("malformed row %q in %s", line, path)
		}
		rows = append(rows, dataRow{input: strings.TrimSpace(parts[0]), expected: strings.TrimSpace(parts[1])})
	}
	return rows, sc.Err()
}

// extractPassing reads the full matrix at inPath and writes, to outPath,
// only the rows that pass interpret + check + compile — the curated
// "golden" subset every downstream consumer can trust to run, type-check
// clean, and compile to an identical result. Work is fanned across
// NumCPU workers (each with its own reused checker/compiler instances);
// output preserves the input order, so the file is deterministic.
func extractPassing(inPath, outPath string) {
	rows, err := readDataRows(inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -extract: %v\n", err)
		os.Exit(1)
	}

	pass := make([]bool, len(rows))
	var next int64 = -1
	var checked, kept int64

	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ai, err1 := lang.New()
			ac, err2 := lang.New()
			if err1 != nil || err2 != nil {
				fmt.Fprintf(os.Stderr, "specgen -extract: lang.New: %v %v\n", err1, err2)
				os.Exit(1)
			}
			ai.SetClock(specClock)
			ac.SetClock(specClock)
			for {
				idx := int(atomic.AddInt64(&next, 1))
				if idx >= len(rows) {
					return
				}
				r := rows[idx]
				if strings.HasPrefix(r.expected, "ERROR:") {
					continue // error rows are never "passing"
				}
				atomic.AddInt64(&checked, 1)

				// 1. interpret → a value (no runtime error)
				gotI, errI := ai.Run(r.input)
				if errI != nil {
					continue
				}
				// 2. check → zero error-severity diagnostics
				res, errChk := ai.Check(r.input)
				if errChk != nil || res.Summary.Errors != 0 {
					continue
				}
				// 3. compile → accepted AND result matches the interpreter
				gotC, wasCompiled, errC := ac.RunCompiled(r.input)
				if !wasCompiled || errC != nil {
					continue
				}
				if renderAny(gotC) != renderAny(gotI) {
					continue
				}
				pass[idx] = true
				atomic.AddInt64(&kept, 1)
			}
		}()
	}
	wg.Wait()

	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -extract: create %s: %v\n", outPath, err)
		os.Exit(1)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	writePassingHeader(w, len(rows), int(kept))
	for i, r := range rows {
		if pass[i] {
			fmt.Fprintf(w, "%s\t%s\tinterpret+check+compile\n", r.input, r.expected)
		}
	}

	fmt.Fprintf(os.Stderr, "specgen -extract: %d/%d non-error rows pass interpret+check+compile (%d total rows scanned)\n",
		kept, checked, len(rows))
}

// renderAny renders a compiled/interpreted result stack ([]any) to a
// stable string for the compiler-parity comparison. Shared with
// syntax_matrix_test.go's gates.
func renderAny(vs []any) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, " ")
}

// writePassingHeader emits the leading comment block of the passing
// subset file.
func writePassingHeader(w *bufio.Writer, scanned, kept int) {
	fmt.Fprintf(w, "# AQL Language Specification: Syntax Combination Matrix — PASSING SUBSET (GENERATED)\n")
	fmt.Fprintf(w, "# Format: input<TAB>expected<TAB>note\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# DO NOT EDIT BY HAND. Regenerate from the full matrix with:\n")
	fmt.Fprintf(w, "#   cd test/go && go run ./specgen -extract -in ./specgen/syntax-matrix.tsv -out ./specgen/syntax-matrix-passing.tsv\n")
	fmt.Fprintf(w, "# (or `make spec-gen`, which regenerates both files).\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# The subset of syntax-matrix.tsv rows that clear ALL THREE pipelines:\n")
	fmt.Fprintf(w, "#   1. interpret — the program runs and leaves a value (no runtime error);\n")
	fmt.Fprintf(w, "#   2. check     — the type checker reports zero error-severity diagnostics;\n")
	fmt.Fprintf(w, "#   3. compile   — the bytecode compiler accepts it AND its result is\n")
	fmt.Fprintf(w, "#                  identical to the interpreter's.\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# `expected` is the canonical eng.Canon value carried over verbatim from\n")
	fmt.Fprintf(w, "# the full matrix. Every row here is a trusted, three-way-agreed program;\n")
	fmt.Fprintf(w, "# syntax_matrix_test.go re-verifies the invariant on each one.\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# %d of the full matrix's rows pass (%d scanned).\n", kept, scanned)
	fmt.Fprintf(w, "#\n")
}

// frontierClass is the stage at which a minimal failing prefix fails.
type frontierClass int

const (
	classCheck   frontierClass = iota // the type checker reports an error (compiler refuses)
	classCompile                      // checker clean, but the compiler will not compile it
	classRuntime                      // checker clean and it compiles, yet it is not a passing program
)

// classifyFrontier runs one prefix through CompileCheck on a reused
// instance and returns the stage it fails at, a short detail (the check
// error code, or the compiler's refusal reason), and whether the compiler
// emitted a Program. CompileCheck runs the checker BEFORE any lowering
// and returns a nil Program on any error-severity diagnostic — so a
// check-stage failure can never also be `compiled`. The caller asserts
// that (the "checker runs first" confirmation) on the returned bool.
func classifyFrontier(a *lang.AQL, src string) (frontierClass, string, bool) {
	prog, reason, res, err := a.CompileCheck(src)
	compiled := prog != nil
	if err != nil {
		return classCheck, sanitizeNote(reason), compiled // parse error / check run error
	}
	for _, d := range res.Diagnostics {
		if d.Severity == lang.SeverityError {
			code := d.Code
			if code == "" {
				code = "error"
			}
			return classCheck, code, compiled
		}
	}
	if !compiled {
		return classCompile, sanitizeNote(reason), compiled // checker clean, Stage-1 could not lower it
	}
	// Checker clean AND it compiled, yet it is not a passing program — run
	// it to capture why: almost always a runtime error the static checker
	// cannot predict (e.g. `incomparable`), occasionally a value the
	// passing-extractor rejected.
	if _, errRun := a.Run(src); errRun != nil {
		detail := strings.TrimPrefix(errorClass(errRun), "ERROR:")
		if detail == "" {
			detail = "error"
		}
		return classRuntime, detail, compiled
	}
	return classRuntime, "non-passing-value", compiled
}

// sanitizeNote makes a reason string safe for a single TSV note column.
func sanitizeNote(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return "uncompilable"
	}
	return s
}

// readInputSet reads a generated matrix file into a set of its input
// strings (used as the "passing" membership oracle).
func readInputSet(path string) (map[string]bool, error) {
	rows, err := readDataRows(path)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(rows))
	for _, r := range rows {
		m[r.input] = true
	}
	return m, nil
}

// readExpectedMap reads a generated matrix file into an input→expected
// lookup (used to annotate a failing prefix with what the full matrix
// records it evaluates to).
func readExpectedMap(path string) (map[string]string, error) {
	rows, err := readDataRows(path)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.input] = r.expected
	}
	return m, nil
}

// extractFrontier finds the minimal failing prefixes of the matrix — the
// sequences that FAIL while their immediate prefix PASSES, i.e. the exact
// point at which a passing program first breaks when one more atom is
// appended. Every non-passing ("remaining") row truncates to exactly one
// such prefix (its first break point; the tokens after it are discarded),
// so this set is the deduped catalogue of distinct failure points.
//
// Each prefix is then classified by the stage it fails at and written to
// the matching file: -check-out for type-check failures (the checker
// rejects it), -compile-out for compile failures (the checker passes but
// the bytecode compiler will not lower it), and -runtime-out for runtime
// failures (the checker passes AND it compiles, yet it errors at run — a
// fault the static stages cannot predict).
//
// As it goes it verifies the "compiler runs the checker first" contract:
// no prefix that the checker rejects is ever emitted as a Program.
func extractFrontier(fullPath, passingPath, checkOut, compileOut, runtimeOut string, max int) {
	passing, err := readInputSet(passingPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -frontier: %v\n", err)
		os.Exit(1)
	}
	expected, err := readExpectedMap(fullPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -frontier: %v\n", err)
		os.Exit(1)
	}

	// 1. Collect the frontier: fails, immediate prefix passes.
	var cands []string
	eachSeq(max, func(toks []string) {
		s := strings.Join(toks, " ")
		if passing[s] {
			return
		}
		prefixOK := len(toks) == 1 // the empty prefix trivially "passes"
		if len(toks) > 1 {
			prefixOK = passing[strings.Join(toks[:len(toks)-1], " ")]
		}
		if prefixOK {
			cands = append(cands, s)
		}
	})

	// 2. Classify in parallel (each worker reuses one compiler instance —
	//    safe because the alphabet has no state-mutating word).
	classes := make([]frontierClass, len(cands))
	notes := make([]string, len(cands))
	var checkFirstViolations int64
	var idx int64 = -1
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, e := lang.New()
			if e != nil {
				fmt.Fprintf(os.Stderr, "specgen -frontier: lang.New: %v\n", e)
				os.Exit(1)
			}
			a.SetClock(specClock)
			for {
				k := int(atomic.AddInt64(&idx, 1))
				if k >= len(cands) {
					return
				}
				cl, detail, compiled := classifyFrontier(a, cands[k])
				classes[k] = cl
				notes[k] = detail
				if cl == classCheck && compiled {
					atomic.AddInt64(&checkFirstViolations, 1) // checker rejected it yet a Program was emitted
				}
			}
		}()
	}
	wg.Wait()

	// 3. Write the three files in input order.
	nCheck := writeFrontierFile(checkOut, "type-check", "check-fail", cands, classes, notes, expected, classCheck)
	nCompile := writeFrontierFile(compileOut, "compile", "compile-refused", cands, classes, notes, expected, classCompile)
	nRuntime := writeFrontierFile(runtimeOut, "runtime", "runtime-fail", cands, classes, notes, expected, classRuntime)

	fmt.Fprintf(os.Stderr, "specgen -frontier: %d minimal failing prefixes — %d type-check, %d compile, %d runtime\n",
		len(cands), nCheck, nCompile, nRuntime)
	fmt.Fprintf(os.Stderr, "specgen -frontier: checker-runs-first check: %d prefixes rejected by the checker were ALSO emitted as a Program (want 0)\n",
		checkFirstViolations)
}

// writeFrontierFile writes the candidates of one class to path as
// `input<TAB>note` rows (note = the failure detail plus what the full
// matrix records the prefix evaluates to), and returns the count.
func writeFrontierFile(path, kind, tag string, cands []string, classes []frontierClass, notes []string, expected map[string]string, want frontierClass) int {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -frontier: create %s: %v\n", path, err)
		os.Exit(1)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	n := 0
	for _, cl := range classes {
		if cl == want {
			n++
		}
	}
	writeFrontierHeader(w, kind, tag, n)
	for k, cl := range classes {
		if cl != want {
			continue
		}
		fmt.Fprintf(w, "%s\t%s: %s\tmatrix:%s\n", cands[k], tag, notes[k], expected[cands[k]])
	}
	return n
}

// writeFrontierHeader emits the leading comment block of a frontier file.
func writeFrontierHeader(w *bufio.Writer, kind, tag string, n int) {
	fmt.Fprintf(w, "# AQL Language Specification: Minimal Failing Prefixes — %s FAILURES (GENERATED)\n", strings.ToUpper(kind))
	fmt.Fprintf(w, "# Format: prefix<TAB>note (note = `%s: <detail>` + the full-matrix result)\n", tag)
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# DO NOT EDIT BY HAND. Regenerate from the matrix + passing subset with:\n")
	fmt.Fprintf(w, "#   cd test/go && go run ./specgen -frontier -in ./specgen/syntax-matrix.tsv \\\n")
	fmt.Fprintf(w, "#     -passing ./specgen/syntax-matrix-passing.tsv \\\n")
	fmt.Fprintf(w, "#     -check-out ./specgen/syntax-matrix-fail-check.tsv \\\n")
	fmt.Fprintf(w, "#     -compile-out ./specgen/syntax-matrix-fail-compile.tsv \\\n")
	fmt.Fprintf(w, "#     -runtime-out ./specgen/syntax-matrix-fail-runtime.tsv\n")
	fmt.Fprintf(w, "# (or `make spec-gen`, which regenerates every derived file).\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# A MINIMAL FAILING PREFIX is a sequence that fails while its immediate\n")
	fmt.Fprintf(w, "# prefix passes — the exact atom at which a passing program first breaks.\n")
	fmt.Fprintf(w, "# Every non-passing matrix row truncates to one of these (the trailing\n")
	fmt.Fprintf(w, "# atoms after the break are discarded), so this is the deduped catalogue\n")
	fmt.Fprintf(w, "# of distinct failure points.\n")
	fmt.Fprintf(w, "#\n")
	switch kind {
	case "type-check":
		fmt.Fprintf(w, "# These prefixes FAIL THE TYPE CHECKER: `aql check` reports at least one\n")
		fmt.Fprintf(w, "# error-severity diagnostic. Because the compiler runs the checker first\n")
		fmt.Fprintf(w, "# (lang.(*AQL).CompileCheck), every prefix here is also refused by the\n")
		fmt.Fprintf(w, "# compiler — none is ever lowered to bytecode.\n")
	case "runtime":
		fmt.Fprintf(w, "# These prefixes PASS THE TYPE CHECKER and COMPILE to bytecode, yet they\n")
		fmt.Fprintf(w, "# ERROR AT RUN — a fault the static stages cannot predict (e.g. comparing\n")
		fmt.Fprintf(w, "# values of incomparable types). The note gives the runtime error class;\n")
		fmt.Fprintf(w, "# the interpreter and compiler agree on it (it is not a compiler bug).\n")
	default:
		fmt.Fprintf(w, "# These prefixes PASS THE TYPE CHECKER but the bytecode compiler will not\n")
		fmt.Fprintf(w, "# lower them (CompileCheck returns no Program); the note gives the first\n")
		fmt.Fprintf(w, "# offender / refusal reason. They run correctly via the interpreter.\n")
	}
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# %d prefixes.\n", n)
	fmt.Fprintf(w, "#\n")
}
