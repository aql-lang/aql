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
// wrong — it laundered unfinished compiler work as impossibility. `args.N`
// proved it concretely: once called "context-dependent, needs an args stack the
// VM frame doesn't keep," it now compiles to a plain PUSH_LOCAL N — the params
// ARE the frame locals. `with-decimal` was the same move. The remaining tier-2
// words are more of the same, just deeper.)
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
		"REDUCIBLE (mostly COMPILED): usurp is a static arg permutation; its wrapper re-dispatch runs in check mode so the carrier compiler compiles the reversed original call (core_ref.go). Residual: usurp combined with quote/codequote or a non-fn target"},
	{"quote", regexp.MustCompile(`\b(code)?quote\b`),
		"REDUCIBLE: code-as-data; needs the compiler to bake the quoted form as a constant token-list value"},
	{"word", regexp.MustCompile(`\bword\b`),
		"REDUCIBLE (mostly COMPILED): the __SP splice now expands inline at the use site (carrier.go). Residual: a few splices whose inlined body reaches an un-compilable word"},
	{"macroexpand", regexp.MustCompile(`\bmacroexpand\b`),
		"REDUCIBLE (static cases COMPILED): the macro expands at compile time and the token list bakes as a code-as-data const (carrier.go). Residual: expansions with a non-Word/un-bakeable member (parens, type nodes)"},
	{"minilang", regexp.MustCompile(`\bminilang`),
		"REDUCIBLE: registers a sublanguage; static registrations could compile, only runtime-input parsing is interpreter-bound"},
	{"flex", regexp.MustCompile(`\bflex\b`),
		"REDUCIBLE: reference-semantics container; needs reference cells in the VM value model (currently by-value)"},
	{"canon", regexp.MustCompile(`\bcanon\b`),
		"REDUCIBLE: canonicalisation is a pure function; just unimplemented in the VM"},
	// `args.N` was here ("REDUCIBLE: a frame-local read"); it is now COMPILED —
	// AnalyseFnBody exposes the params as the args projection and
	// tryFoldStaticIndex folds `get N args` to PUSH_LOCAL N (carrier.go). It is
	// the concrete proof that the tier-2 words are reducible, not irreducible.
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
	interpreterOnlyCeiling = 3  // Vm.run / Vm.run-with — execute runtime-computed code. NOW 0: the corpus's last tier-1 row, Vm.run(canon [1 {a:none} x/q]), compiled once OpMakeList let the canon list assemble. Kept at 3 for headroom — a Vm.run of a genuinely RUNTIME string would re-populate it
	reducibleCeiling       = 48 // Test/Assert 21, quote 10, usurp 9, flex 6, minilang/parselang 8 (+3 from merged PR #142) — each a named, reducible compiler/VM TODO; 54 -> 52 when structural type-pattern operands started baking (two rows that refused at a reducible word past the now-compiled pattern dropped out); 52 -> 51 when dead value-defs stopped refusing (a reducible-word row past a now-dropped dead binding fell out); 51 -> 48 when arity/param/return introspection over a baked fn value compiled (those rows left the reducible bucket)
	computeRefusalCeiling  = 85 // operand-provenance cascades, code-body DSL words, Stage-1 lowering residuals, dynamic in/out, 5 islands, user-fn dispatch (+8 from merged PR #142 minilang/parselang corpus rows that fall back faithfully; +1 patrun.tsv — a mutable Patrun dispatch / `add` 3-arg-overload row falls back, the bytecode fix is tracked on a separate branch); 166 -> 137: structural type-pattern operands (`{a:Integer}`, `[Integer String]`, `[Resource Entity]`) now bake as const members, so static is/typeof/size over them compiles; 137 -> 126: generic schema bakes as a const, unblocking make/is/typeof/residual over top-level generic types; 126 -> 122: schema body check admits a type-parameter Word, so typed-list-field generic schemas (`{items:[:T]}`) bake too; 122 -> 119: a function-signature value bakes as pure descriptor data, unblocking fnsig residual/typeof and fnsig-bodied generic schemas; 119 -> 100: a surface type bakes as a const (shared canonical descriptor; the compiled path keeps check-pass mints so it never goes stale), unblocking the surface type-algebra cluster; 100 -> 97: typed-def object construction records its skipped make event, giving the bound instance make-equivalent provenance; 97 -> 90: a single-result value-def referenced zero times drops its dead result (OpDrop) rather than refusing as an unconsumed call result; 90 -> 85: a no-capture fn value bakes as a const so it stands as data (residual, map member, introspection operand), with an auto-dispatch-boundary guard keeping a fn-ahead-of-args residual on the fallback path
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
