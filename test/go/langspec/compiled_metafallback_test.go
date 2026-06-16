// The re-scoped P7 gate (design/aql-bytecode-completion.0.md §3).
//
// The literal P7 — every spec value row compiles to a fallback-free Program —
// is not reachable for ONE narrow reason: a bytecode compiler is an
// ahead-of-time function `compile : Program -> Instructions`, and a handful of
// words EXECUTE CODE THAT IS COMPUTED AT RUNTIME (`Vm.run` of a runtime string).
// For those there is no static instruction sequence — "compiling" them is
// parse+compile+run at runtime, which is exactly what the OpFallback island
// (the embedded interpreter) does. That, and only that, is irreducible.
//
// EVERYTHING ELSE that refuses today is REDUCIBLE — it is refused by a specific,
// nameable limitation of THIS compiler/VM, not by any law. So the partition is
// three tiers, not "meta vs compute":
//
//   - tier 1, interpreter-only (interpreterOnlyWords): executes runtime-computed
//     code. The island is the correct, permanent home. Capped, not ratcheted.
//   - tier 2, reducible-not-yet-compiled (reducibleWords): refused by a named
//     missing compiler/VM feature (macroexpand = compile-time expand + bake;
//     word = splice inline; args.N = frame-local read; flex = reference cells;
//     …). These are TODOs, ratcheted toward 0 like any other work.
//   - the compute frontier: cascades, lowering residuals, DSL bodies, error
//     rows. Ratcheted by computeRefusalCeiling.
//
// (An earlier version of this file called tier 2 "irreducible meta." That was
// wrong — it laundered unfinished compiler work as impossibility. `with-decimal`
// proved the point by moving from "refusing code-body word" to a clean native
// compile; the tier-2 words are the same kind of move, just deeper.)
//
// When BOTH ratchets reach 0, only tier 1 falls back, and the unbounded
// whole-program fallback in RunCompiled can be narrowed to the tier-1 island
// spans (§3 step 4).
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

type allowEntry struct {
	name    string
	pattern *regexp.Regexp
	why     string
}

// interpreterOnlyWords (tier 1) execute code that is constructed at RUNTIME, so
// there is no ahead-of-time instruction sequence to emit: "compiling" them is a
// runtime parse+compile+run, i.e. the interpreter. This is the one genuinely
// irreducible category and the legitimate permanent home of the OpFallback
// island. Kept deliberately tiny — a NEW entry here is a claim of irreducibility
// that must be justified.
var interpreterOnlyWords = []allowEntry{
	{"Vm.run", regexp.MustCompile(`\bVm\.run`),
		"executes runtime-constructed source in a sub-engine — there is no static program to compile; this IS the interpreter"},
}

// reducibleWords (tier 2) refuse today because of a specific, NAMED limitation
// of this compiler/VM — not because they cannot be compiled. The `why` records
// exactly what compiling each would take. They are ratcheted toward 0
// (reducibleCeiling); none is a permanent exclusion.
var reducibleWords = []allowEntry{
	{"usurp", regexp.MustCompile(`\busurp\b|/u[rs]?(\b|$)`),
		"REDUCIBLE: re-steps tape-coupled values; needs the VM to model the usurp-modified dispatch instead of the interpreter tape (incl. /u /ur /us synthetics)"},
	{"quote", regexp.MustCompile(`\b(code)?quote\b`),
		"REDUCIBLE: code-as-data; needs the compiler to bake the quoted form as a constant token-list value"},
	{"word", regexp.MustCompile(`\bword\b`),
		"REDUCIBLE: Forth-style splice; needs compile-time inlining of the splice body at each use site (late binding then falls out of the normal def sequence)"},
	{"macroexpand", regexp.MustCompile(`\bmacroexpand\b`),
		"REDUCIBLE: Lisp-style — expand the macro at compile time (args/macro are static) and bake the resulting tokens"},
	{"minilang", regexp.MustCompile(`\bminilang`),
		"REDUCIBLE: registers a sublanguage; static registrations could compile, only runtime-input parsing is interpreter-bound"},
	{"flex", regexp.MustCompile(`\bflex\b`),
		"REDUCIBLE: reference-semantics container; needs reference cells in the VM value model (currently by-value)"},
	{"canon", regexp.MustCompile(`\bcanon\b`),
		"REDUCIBLE: canonicalisation is a pure function; just unimplemented in the VM"},
	{"args", regexp.MustCompile(`\bargs\.`),
		"REDUCIBLE: args.N is a frame-local read (params ARE locals 0..n-1); needs the emitter to fold get-N-of-args to PUSH_LOCAL N"},
	{"Test/Assert", regexp.MustCompile(`\b(Test|Assert)\.`),
		"REDUCIBLE: the test/property harness; the candidate bodies compile, the harness accumulates — only random input generation is runtime"},
}

// classify returns the tier a refused/islanded row's source is attributable to:
// "" (compute gap), or the matched tier-1/tier-2 word. Tier 1 is checked first
// so a `Vm.run (canon …)` row counts as interpreter-only, not reducible-canon.
func classify(src string) (tier int, name string) {
	for _, e := range interpreterOnlyWords {
		if e.pattern.MatchString(src) {
			return 1, e.name
		}
	}
	for _, e := range reducibleWords {
		if e.pattern.MatchString(src) {
			return 2, e.name
		}
	}
	return 0, ""
}

// errorRowReason reports whether a refusal is a deliberately-interpreted
// runtime-error surface: the checker refuses so the interpreter raises the
// matching taxonomy (a return-count mismatch, an unpack of a missing key, an
// orphan gen). §3 step 3 dispositions these as allowlisted.
func errorRowReason(reason string) bool {
	return strings.Contains(reason, "suppressed a runtime error") ||
		strings.Contains(reason, "body value count differs") ||
		strings.Contains(reason, "without exactly one declared return")
}

// Three downward ratchets. interpreterOnlyCeiling caps the ONE permanent island
// category (a new tier-1 word is an irreducibility claim that must be argued);
// reducibleCeiling and computeRefusalCeiling both ratchet toward 0 — when they
// reach it, only tier 1 falls back and the unbounded fallback can be narrowed.
const (
	interpreterOnlyCeiling = 3   // Vm.run / Vm.run-with — execute runtime-computed code
	reducibleCeiling       = 129 // usurp 43, word 30, Test/Assert 28, quote 14, flex 7, minilang 5, args 2 — each a named, reducible compiler/VM TODO
	computeRefusalCeiling  = 260 // operand-provenance cascades (140), code-body DSL words (47), Stage-1 lowering residuals (~24), dynamic in/out (~24), 9 islands, user-fn dispatch (5)
)

func TestOnlyMetaFallsBack(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var interp, reducible, errorRows, computeGap int
	tier1By := map[string]int{}
	tier2By := map[string]int{}
	computeByReason := map[string]int{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, err := os.Open(filepath.Join(specDir, e.Name()))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			input := strings.TrimSpace(parts[0])

			a := newDifferentialInstance(t)
			prog, reason, _, cerr := a.CompileCheck(input)
			switch {
			case cerr != nil, reason == "check diagnostics":
				continue // statically invalid in both engines — not the gate's concern
			case prog != nil && !strings.Contains(prog.Disassemble(), "FALLBACK"):
				continue // fully native
			}
			// Refused, or an islanded Program: not fully native. Classify.
			switch tier, name := classify(input); tier {
			case 1:
				interp++
				tier1By[name]++
			case 2:
				reducible++
				tier2By[name]++
			default:
				if errorRowReason(reason) {
					errorRows++
					continue
				}
				computeGap++
				r := normaliseReason(reason)
				if reason == "" {
					r = "island (OpFallback span)"
				}
				computeByReason[r]++
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner: %v", err)
		}
	}

	t.Logf("re-scoped P7 partition: %d interpreter-only (tier 1, permanent), %d reducible (tier 2, TODO), %d error-row, %d COMPUTE GAP",
		interp, reducible, errorRows, computeGap)
	t.Logf("tier 1 — interpreter-only (ceiling %d):", interpreterOnlyCeiling)
	for _, w := range sortedKeys(tier1By) {
		t.Logf("  interp %4d  %s", tier1By[w], w)
	}
	t.Logf("tier 2 — reducible, not yet compiled (ceiling %d):", reducibleCeiling)
	for _, w := range sortedKeys(tier2By) {
		t.Logf("  reduce %4d  %s", tier2By[w], w)
	}
	t.Logf("compute gaps by reason (ceiling %d):", computeRefusalCeiling)
	for _, r := range sortedKeys(computeByReason) {
		t.Logf("  gap    %4d  %s", computeByReason[r], r)
	}

	if interp > interpreterOnlyCeiling {
		t.Errorf("interpreter-only rows %d exceed cap %d — a NEW word claims irreducibility; justify it or compile it",
			interp, interpreterOnlyCeiling)
	}
	if reducible > reducibleCeiling {
		t.Errorf("reducible (tier-2) rows %d exceed ceiling %d — a reducible-but-unimplemented row regressed",
			reducible, reducibleCeiling)
	}
	if computeGap > computeRefusalCeiling {
		t.Errorf("compute gaps %d exceed ceiling %d — a real-compute row regressed to refusing",
			computeGap, computeRefusalCeiling)
	}
}

// sortedKeys returns a map's keys sorted by descending value then name, for a
// stable most-frequent-first histogram.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}
