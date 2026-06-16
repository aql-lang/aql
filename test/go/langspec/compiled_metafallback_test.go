// The re-scoped P7 gate (design/aql-bytecode-completion.0.md §3).
//
// The literal P7 — every spec value row compiles to a fallback-free Program —
// is NOT reachable, by design: the corpus exercises irreducibly dynamic /
// reflective words (Vm.run executes runtime-constructed source; the Test/Assert
// harness generates and runs candidate programs; macroexpand rewrites code;
// codequote/quote are code-as-data; flex/canon are reflective; usurp re-steps
// tape-coupled values; args reads the per-call args stack). Those ARE the
// interpreter — there is nothing to ahead-of-time compile.
//
// So P7 is re-scoped to: all REAL COMPUTE native, with an explicit, enumerated
// allowlist for the meta words. This gate is the objective measure of THAT
// goal. It partitions every refused or islanded spec value row into:
//
//   - META — the source uses an allowlisted meta word (metaFallbackWords); the
//     refusal/island is by design and the interpreter owns it.
//   - ERROR-ROW — the checker deliberately refuses so the interpreter surfaces a
//     runtime error (return-count mismatch, unpack of a missing key, orphan
//     gen); §3 step 3 dispositions these as allowlisted.
//   - COMPUTE GAP — everything else: the real frontier, ratcheted DOWN toward 0.
//
// The downward ratchet is computeRefusalCeiling. When it reaches 0 this becomes
// the strict "only meta (and error rows) fall back" gate, and the unbounded
// whole-program fallback in RunCompiled can be narrowed to the allowlisted meta
// spans (§3 step 4) while the OpFallback island machinery stays as the hybrid's
// confined interpreter seam.
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

// metaFallbackWords is the curated allowlist: a source pattern plus the one-line
// reason that word cannot ahead-of-time compile. A refused/islanded row whose
// SOURCE matches a pattern is attributed to meta. The patterns are deliberately
// narrow (e.g. `args.` the accessor, not `forward-args`; `Vm.run` not every
// `run`) so a genuine compute gap is never masked. Curated conservatively —
// expand only with a documented rationale, since a too-wide allowlist hides
// compute frontiers from the ratchet.
var metaFallbackWords = []struct {
	name    string
	pattern *regexp.Regexp
	why     string
}{
	{"usurp", regexp.MustCompile(`\busurp\b|/u[rs]?(\b|$)`),
		"re-steps tape-coupled values on the interpreter tape; the VM cannot push them (incl. the /u /ur /us ref synthetics)"},
	{"quote", regexp.MustCompile(`\b(code)?quote\b`),
		"code-as-data: yields its operand's unevaluated form, which has no runtime value to bake"},
	{"macroexpand", regexp.MustCompile(`\bmacroexpand\b`),
		"rewrites code at run time (macro expansion); the program is not known until then"},
	{"minilang", regexp.MustCompile(`\bminilang`),
		"registers and runs a sublanguage at run time (reflective)"},
	{"flex", regexp.MustCompile(`\bflex\b`),
		"reference-semantics mutable container; the aliasing the VM's value model cannot track"},
	{"canon", regexp.MustCompile(`\bcanon\b`),
		"canonicalises a value reflectively"},
	{"args", regexp.MustCompile(`\bargs\.`),
		"reads the interpreter's per-call args stack; the VM frame binds params to locals and keeps no args stack"},
	{"Vm.run", regexp.MustCompile(`\bVm\.run`),
		"executes runtime-constructed source in a sub-engine — this IS the interpreter, by definition"},
	{"Test/Assert", regexp.MustCompile(`\b(Test|Assert)\.`),
		"the test / property harness generates inputs and runs candidate programs at run time"},
}

// metaAttribution returns the meta word a refused/islanded row's source is
// attributable to (and why), or ok=false when the row is a genuine non-meta gap.
func metaAttribution(src string) (name, why string, ok bool) {
	for _, m := range metaFallbackWords {
		if m.pattern.MatchString(src) {
			return m.name, m.why, true
		}
	}
	return "", "", false
}

// errorRowReason reports whether a refusal reason is a deliberately-interpreted
// runtime-error surface: the checker refuses so the interpreter raises the
// matching taxonomy (a return-count mismatch, an unpack of a missing key, an
// orphan gen). §3 step 3 dispositions these as allowlisted rather than demanding
// the VM raise every such taxonomy.
func errorRowReason(reason string) bool {
	return strings.Contains(reason, "suppressed a runtime error") ||
		strings.Contains(reason, "body value count differs") ||
		strings.Contains(reason, "without exactly one declared return")
}

// computeRefusalCeiling is the downward ratchet at the heart of the re-scoped
// P7 gate: the maximum number of refused-or-islanded spec value rows allowed to
// be GENUINE COMPUTE gaps (neither meta-attributable nor an error-row). It must
// reach 0 before the unbounded whole-program fallback can be narrowed. Lower it
// monotonically as compute clusters compile natively; never raise it.
const computeRefusalCeiling = 295 // lambda higher-order args landed; remaining: operand-provenance cascades (get/is/typeof/make over unmaterialisable producers, 140), code-body DSL words (select/case/reach/with-decimal/..., 82), Stage-1 lowering residuals (~24), dynamic in/out (~24), 9 islands, user-fn dispatch (5)

func TestOnlyMetaFallsBack(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var metaCount, errorRows, computeGap int
	metaByWord := map[string]int{}
	computeByReason := map[string]int{}
	var sampleGaps []string

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
			// Refused, or an islanded Program (re-enters the interpreter): a row
			// that is NOT fully native. Attribute it.
			if name, _, ok := metaAttribution(input); ok {
				metaCount++
				metaByWord[name]++
				continue
			}
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
			if len(sampleGaps) < 25 {
				sampleGaps = append(sampleGaps, input)
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner: %v", err)
		}
	}

	t.Logf("re-scoped P7 partition: %d meta-attributable, %d error-row, %d COMPUTE GAP (ceiling %d)",
		metaCount, errorRows, computeGap, computeRefusalCeiling)
	t.Logf("meta-attributable by word:")
	for _, w := range sortedKeys(metaByWord) {
		t.Logf("  meta %4d  %s", metaByWord[w], w)
	}
	t.Logf("compute gaps by reason (the frontier to clear):")
	for _, r := range sortedKeys(computeByReason) {
		t.Logf("  gap  %4d  %s", computeByReason[r], r)
	}

	if computeGap > computeRefusalCeiling {
		for _, s := range sampleGaps {
			t.Logf("  e.g. %s", s)
		}
		t.Errorf("compute gaps %d exceed ceiling %d — a real-compute row regressed to refusing (it is NOT attributable to a meta word or an error-row)",
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
