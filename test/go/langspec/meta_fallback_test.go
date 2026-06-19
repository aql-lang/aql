// The re-scoped P7 gate (design/aql-bytecode-completion.0.md §3): the literal
// "every row compiles" goal is unreachable while the corpus exercises words
// that are inherently dynamic/reflective — they parse and run runtime-
// constructed code, re-step tape-coupled fns, or read per-call state the VM
// frame doesn't keep. Those are DELIBERATELY interpreted via a confined island,
// not a coverage failure.
//
// This file curates that allowlist (metaFallbackWords) and the boundary gate
// (TestMetaFallbackBoundary): every refused/islanded spec row is classified as
// META (attributable to an allowlisted word — an EXPECTED fallback) or NON-META
// (remaining COMPILABLE work). The non-meta count is the honest finish-line
// ratchet: when it reaches 0, "only meta falls back" holds and the unbounded
// whole-program fallback can be narrowed (§P7). The total refusalCeiling
// (compiled_coverage_test.go) stays as the coarse ratchet; this splits it into
// the part that should still shrink versus the part that never will.
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

// metaFallbackWords is the curated allowlist of inherently-dynamic / reflective
// words: a refused or islanded row that uses one is an EXPECTED, deliberate
// interpreter fallback, never a coverage gap. Each entry states why it cannot
// compile to runtime-independent bytecode. Attribution is by the refusal-reason
// word (module-wrapped natives like the test harness surface here) OR a source
// token (the usurp family surfaces as a /u-suffixed token whose recorded reason
// names the inner arithmetic word, not the modifier).
var metaFallbackWords = map[string]string{
	// Runtime-source execution — the program isn't known until run time.
	"vm-run":      "runs runtime-constructed source in a sub-engine",
	"vm-run-with": "runs runtime-constructed source in a sub-engine",
	"parse":       "parses runtime source text into a program",

	// The property-test harness generates inputs and runs candidate programs.
	"test-test":       "property-test harness runs candidate programs",
	"test-prop":       "property-test harness runs candidate programs",
	"test-check-prop": "property-test harness runs candidate programs",
	"test-check":      "property-test harness runs candidate programs",
	"test-describe":   "property-test harness scaffolding",
	"test-skip":       "property-test harness scaffolding",

	// Code-as-data / reflection — rewrite or render the program at run time.
	"macroexpand":                "rewrites code at run time",
	"minilang-register":          "registers a mini-language (code) at run time",
	"minilang-register-compiled": "registers a mini-language compile hook (code) at run time",
	"parselang-register":         "registers a parser (code) at run time",
	"codequote":                  "captures code as data",
	"canon":                      "renders a value as canonical source (code-as-data)",
	"reify":                      "reflects a value back to its constructing form",

	// The usurp / dispatch-modifier family returns a re-stepped, tape-coupled
	// modified fn the VM cannot push (the `/u`, `/s`, `/f`, `/N` sugar desugars
	// to these). Their refusal reason names the inner word (e.g. sub), so these
	// are matched by the usurpSuffix token check below, not by reason word.
	"usurp":        "re-steps a tape-coupled modified fn",
	"stack-args":   "re-dispatch modifier (re-steps a modified fn)",
	"forward-args": "re-dispatch modifier (re-steps a modified fn)",
	"force-arity":  "re-dispatch modifier (re-steps a modified fn)",

	// Per-call / async state the compiled frame deliberately does not maintain.
	"args":     "reads the per-call args stack the VM frame doesn't keep",
	"__pa":     "the per-call args-stack pop the VM frame doesn't keep",
	"timeout":  "async runtime scheduling",
	"interval": "async runtime scheduling",
	"await":    "async runtime scheduling",

	// Token splice — re-steps spliced tokens at the pointer.
	"word": "splices tokens that re-step at the pointer",

	// Runtime randomization over a dynamic shape.
	"rand-map-from": "runtime randomization over a dynamic shape",
}

// usurpSuffix matches a token carrying a usurp-family modifier suffix
// (`f/u`, `f/ur`, `f/us`, `x/s`, `x/f`, `x/3`) — the sugar for usurp /
// stack-args / forward-args / force-arity. `/q` (quote) and `/r` (ref) are
// deliberately excluded: they are NOT meta (a quoted atom / reffed value bakes).
var usurpSuffix = regexp.MustCompile(`/(ur|us|u|s|f|[0-9]+)$`)

// metaReasonWord extracts the offending word a refusal reason names, for the
// reason-word arm of attribution (module-wrapped meta natives surface here).
func metaReasonWord(reason string) string {
	for _, p := range []string{
		"code-body word ", "quoted-operand word ", "context-dependent word ",
		"compile-time word ", "full-stack word ", "unannotated or opaque word ",
	} {
		if strings.HasPrefix(reason, p) {
			r := strings.TrimPrefix(reason, p)
			r = strings.TrimSuffix(r, " (Stage 2)")
			if i := strings.IndexByte(r, ' '); i > 0 {
				r = r[:i]
			}
			return r
		}
	}
	return ""
}

// metaTokens splits a source row into coarse tokens for the source-token arm.
func metaTokens(src string) []string {
	return strings.FieldsFunc(src, func(r rune) bool {
		return strings.ContainsRune(" \t\n()[]{}.,;", r)
	})
}

// metaAttribution returns the allowlist reason a refused/islanded row is a
// DELIBERATE meta fallback, or ("", false) if it is remaining compilable work.
func metaAttribution(source, reason string) (string, bool) {
	if w := metaReasonWord(reason); w != "" {
		if why, ok := metaFallbackWords[w]; ok {
			return w + " — " + why, true
		}
	}
	for _, tk := range metaTokens(source) {
		if why, ok := metaFallbackWords[tk]; ok {
			return tk + " — " + why, true
		}
		if usurpSuffix.MatchString(tk) {
			return tk + " — usurp-family modifier (/u /s /f /N): re-steps a modified fn", true
		}
	}
	return "", false
}

// nonMetaCeiling is the count of refused/islanded rows NOT attributable to a
// metaFallbackWords member — the remaining COMPILABLE work. This is the
// finish-line ratchet: it must only ever go DOWN, and when it reaches 0 the
// re-scoped P7 gate ("only meta falls back") holds. Never raise it.
// Reconciled to the live actual (was a long-stale 201, set when refusals
// numbered in the hundreds and never lowered as the ratchet fell); the
// inert-reach code-body fix cleared the last non-meta `rand-map-from` row, the
// catch frame cleared 2 non-meta `error [handler]` rows (42 → 40),
// value-def-locals in fn units cleared the 4 `error [… case …]` rows (40 → 36),
// and dropping RecordClosureCall's dynamic-input pre-decline cleared the caught
// dynamic-error `error [get code]` row (36 → 35).
// 35 -> 29: the cross-fn break/continue compilation (OpFlowBreak/OpFlowContinue
// + the 0-value-if zeroOut and AnalyseFnBody phantom-strip enablers) cleared the
// 2 recursion.tsv §9 rows, and the ceiling re-tightens to the live actual after
// earlier landings left it slack.
const nonMetaCeiling = 29

// TestMetaFallbackBoundary classifies every refused/islanded spec value row as
// META (an expected, allowlisted interpreter fallback) or NON-META (remaining
// compilable work), and ratchets the non-meta count downward. A NEW refusal
// from a non-meta word raises the count and fails; clearing real work lowers
// the ceiling toward the P7 finish line.
func TestMetaFallbackBoundary(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}
	var meta, nonMeta int
	metaHits := map[string]int{}
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
			a := newDifferentialInstance(t)
			prog, reason, _, cerr := a.CompileCheck(strings.TrimSpace(parts[0]))
			if cerr != nil || reason == "check diagnostics" {
				continue
			}
			islanded := prog != nil && strings.Contains(prog.Disassemble(), "FALLBACK")
			if prog != nil && !islanded {
				continue // fully native — not a fallback row
			}
			if why, ok := metaAttribution(parts[0], reason); ok {
				meta++
				metaHits[strings.SplitN(why, " — ", 2)[0]]++
			} else {
				nonMeta++
			}
		}
		f.Close()
	}

	t.Logf("meta-fallback boundary: %d META (allowlisted, deliberate) + %d NON-META (remaining compilable work)", meta, nonMeta)
	type kv struct {
		w string
		n int
	}
	hist := make([]kv, 0, len(metaHits))
	for w, n := range metaHits {
		hist = append(hist, kv{w, n})
	}
	sort.Slice(hist, func(i, j int) bool {
		if hist[i].n != hist[j].n {
			return hist[i].n > hist[j].n
		}
		return hist[i].w < hist[j].w
	})
	for _, h := range hist {
		t.Logf("  meta %3d  %s", h.n, h.w)
	}

	if nonMeta > nonMetaCeiling {
		t.Errorf("non-meta refusals/islands %d exceed ceiling %d — remaining compilable work regressed", nonMeta, nonMetaCeiling)
	}
}
