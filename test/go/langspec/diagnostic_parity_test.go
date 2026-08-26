// Diagnostic parity between the two analysis passes — the gate NUR103
// showed was missing.
//
// design/FULL-COMPILATION.0.md section 6.9(4) states the alignment
// property the whole design rests on, and its clause (b) is that
// diagnostics stay identical whether or not a program is being compiled.
// The static pass is one abstracted interpreter over carriers; its verdict
// on a program is a property of the program, not of what the caller
// intends to do with the verdict.
//
// It is not currently true. NUR103 records a program that `boru check`
// reports clean and the compile pass refuses with `undefined_word` — a
// divergence a user cannot diagnose, because the tool they would reach for
// says their program is fine. Nothing measured how widespread that is,
// because both passes were only ever compared against the RUNTIME, never
// against each other.
//
// This walks the corpus running both passes over each row and counts the
// rows whose diagnostic sets differ. Like the other censuses it ratchets:
// the count may only fall, and it reaches zero when section 6.9(3)'s
// !Compiling forks are collapsed or proven neutral.
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// diagnosticParityCeiling is the number of corpus rows whose diagnostics
// differ between a plain check and a compile-armed check. Monotone DOWN
// only; 0 when the two passes cannot disagree (Stage 8).
//
// 318 of 7568 corpus rows at the baseline, 317 of 7569 after the first row
// was fixed, counting FINDINGS only; a
// further 260 rows differ on informational advisories, tracked separately
// because some of those are pass-specific BY DESIGN (see diagSet). The shapes are structured, not
// noise, and they run in BOTH directions:
//   - plain-only findings: no_signature suppressed while compiling (a
//     documented fork, check_recovery.go), unreachable_branch, and the
//     module_body_executed_in_check info;
//   - armed-only findings: redundant_guard, case_not_exhaustive, and —
//     the NUR103 shape — undefined_word on programs plain check calls
//     clean;
//   - armed DUPLICATES: 57 rows where one plain diagnostic becomes two.
//
// Some are deliberate; the design requires each to be collapsed or proven
// diagnostic-neutral (section 6.9(3)), which is what drives this to zero.
const diagnosticParityCeiling = 317 // 318 (2026-08-26, Stage-1 baseline) -> 317 (NUR103 record-field fix) -> 0 (Stage 8)

// armedOnlyCeiling is the sharpest of the three classes: rows the plain
// check calls clean and the compile-armed pass finds fault with. It is the
// one a user CANNOT diagnose, because the tool they would reach for
// reports the program fine — NUR103's shape. Monotone DOWN only.
//
// The other two classes are less severe and tracked in the log rather than
// gated: 41 rows where the armed pass drops a diagnostic but REFUSES, so
// the finding still reaches the user through the refusal reason (the
// documented no_signature suppression), and 272 where a check finding
// vanishes and the program compiles anyway — the checker being stricter
// than the compiler, which is a false-positive surface rather than a
// silent-acceptance one.
// 5 -> 4, 2026-08-26: `edge-dispatch-3.tsv:L56` — a field read from a
// STRUCTURAL-RECORD parameter — is fixed. The inline record pattern's field
// type words now resolve at sig install (ResolveSigRecordFields), so the
// schema-bearing param carrier no longer narrows the read to dynamic(Word),
// and the step loop no longer dispatches a word-typed CARRIER as a nameless
// token. NUR103 has the full trace; its `h2` half is a different defect and
// is not among these four.
const armedOnlyCeiling = 4 // 5 (2026-08-26) -> 4 (NUR103 record-field fix) -> 0 (Stage 8)

// diagKey renders a diagnostic's identity for set comparison: the code and
// the word it is about. Detail text is deliberately excluded — it embeds
// positions and inferred types that legitimately differ in phrasing
// between passes; what must not differ is WHICH findings exist.
func diagKey(d lang.CheckDiagnostic) string {
	return d.Code + "/" + d.Word
}

// diagSet collects the FINDINGS — errors and warnings. Informational
// entries are excluded on purpose, because some of them are advisories
// about the ANALYSIS rather than findings about the program, and those
// legitimately differ between passes. The load-bearing example is
// `module_body_executed_in_check`, which warns that `boru check` executed
// a module body the user did not ask it to run; under compilation that
// execution IS the program's own, so there is nothing to advise about and
// the emitter is explicitly scoped to a pure check pass
// (lang/go/native/native_module_module.go). That is section 6.9(3)'s
// "proven diagnostic-neutral" disposition, not a defect, and a gate that
// counted it would be demanding the wrong thing.
//
// Info-only divergence is still counted and reported separately, so a NEW
// informational fork cannot hide behind this exclusion.
func diagSet(ds []lang.CheckDiagnostic) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		if d.Severity == "info" || d.Severity == "" {
			continue
		}
		out = append(out, diagKey(d))
	}
	sort.Strings(out)
	return out
}

// infoSet is the excluded half, tracked so it cannot drift unwatched.
func infoSet(ds []lang.CheckDiagnostic) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		if d.Severity == "info" || d.Severity == "" {
			out = append(out, diagKey(d))
		}
	}
	sort.Strings(out)
	return out
}

func TestDiagnosticParityAcrossPasses(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var rows, diverged, infoDiverged int
	var armedOnly, carriedByRefusal, lostUnderCompile int
	var armedOnlyRows []string
	byShape := map[string]int{}
	var examples []string

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, err := os.Open(filepath.Join(specDir, e.Name()))
		if err != nil {
			t.Fatalf("open %s: %v", e.Name(), err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			input := strings.TrimSpace(parts[0])
			rows++

			ap := newDifferentialInstance(t)
			plain, perr := ap.Check(input)
			if perr != nil {
				continue // a pass that cannot run is not a parity question
			}
			ac := newDifferentialInstance(t)
			prog, _, armed, cerr := ac.CompileCheck(input)
			if cerr != nil {
				continue
			}
			refused := prog == nil

			if strings.Join(infoSet(plain.Diagnostics), ",") != strings.Join(infoSet(armed.Diagnostics), ",") {
				infoDiverged++
			}
			p, c := diagSet(plain.Diagnostics), diagSet(armed.Diagnostics)
			if strings.Join(p, ",") == strings.Join(c, ",") {
				continue
			}
			diverged++
			// Classify by what the USER sees, which is the property that
			// matters. A finding the armed pass drops is not lost if that
			// pass REFUSED — the refusal reason carries it, which is
			// exactly why no_signature is suppressed while compiling
			// (check/go/check_recovery.go: emitting it there would mask the
			// specific reason as the generic sentinel). What is serious is a
			// finding only ONE lane surfaces at all.
			switch {
			case len(c) > len(p):
				armedOnly++ // the NUR103 class: clean to `boru check`, refused by the compiler
				// Few enough to name. Listing them is the difference between
				// a ratchet and a worklist.
				armedOnlyRows = append(armedOnlyRows,
					e.Name()+":L"+itoa(lineNum)+"  "+strings.Join(c, "|")+"  "+firstNRunes(input, 70))
			case refused:
				carriedByRefusal++ // dropped as a diagnostic, still reported as a refusal
			default:
				lostUnderCompile++ // `boru check` errors that vanish AND the program compiles
			}
			shape := "plain=" + strings.Join(p, "|") + " armed=" + strings.Join(c, "|")
			byShape[shape]++
			if len(examples) < 5 {
				examples = append(examples, e.Name()+":L"+itoa(lineNum)+"  "+firstNRunes(input, 60))
			}
		}
		f.Close()
	}

	t.Logf("diagnostic parity: %d rows, %d diverged on FINDINGS, %d on info-only advisories", rows, diverged, infoDiverged)
	t.Logf("  by user impact: %d armed-only (clean to check, refused compiling — the NUR103 class), %d carried by the refusal reason instead, %d lost under compilation (check errors that vanish while the program compiles)",
		armedOnly, carriedByRefusal, lostUnderCompile)
	for _, ex := range examples {
		t.Logf("  e.g. %s", ex)
	}
	for _, r := range armedOnlyRows {
		t.Logf("  armed-only: %s", r)
	}
	if armedOnly > armedOnlyCeiling {
		t.Errorf("armed-only findings %d exceed ceiling %d — programs that `boru check` calls clean and the compiler refuses, which a user cannot diagnose (NUR103)",
			armedOnly, armedOnlyCeiling)
	}
	if diverged > diagnosticParityCeiling {
		t.Errorf("diagnostic parity: %d rows exceed ceiling %d — the checker's verdict depends on who is asking (NUR103). Top shapes:\n%s",
			diverged, diagnosticParityCeiling, topShapes(byShape, 8))
	}
}

// topShapes renders the n most frequent divergence shapes. The full map
// is hundreds of entries; a failure needs the pattern, not the census.
func topShapes(m map[string]int, n int) string {
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
	if len(keys) > n {
		keys = keys[:n]
	}
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = "  " + itoa(m[k]) + "x  " + k
	}
	return strings.Join(lines, "\n")
}

// firstNRunes truncates for log lines without splitting a rune.
func firstNRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
