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
// Usage:
//
//	go run ./specgen [-max N] [-out path]
//
//	-max  maximum sequence length to enumerate (default 4)
//	-out  output file; empty writes to stdout (default empty)
//
// The Makefile target `spec-gen` regenerates test/go/specgen/syntax-matrix.tsv.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
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

// sequences yields every alphabet sequence of length 1..max, in a fully
// deterministic order: shorter sequences first, then odometer order over
// alphabet indices (last position varies fastest). The callback receives
// the joined source string and the element count.
func sequences(max int, emit func(src string, n int)) {
	for n := 1; n <= max; n++ {
		idx := make([]int, n)
		for {
			toks := make([]string, n)
			for i, a := range idx {
				toks[i] = alphabet[a]
			}
			emit(strings.Join(toks, " "), n)

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

func main() {
	max := flag.Int("max", 4, "maximum sequence length to enumerate")
	out := flag.String("out", "", "output .tsv path; empty writes to stdout")
	flag.Parse()

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
