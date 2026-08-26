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
// 568 of 7568 corpus rows at the baseline. The shapes are structured, not
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
const diagnosticParityCeiling = 568 // 568 (2026-08-26, Stage-1 baseline) -> 0 (Stage 8)

// diagKey renders a diagnostic's identity for set comparison: the code and
// the word it is about. Detail text is deliberately excluded — it embeds
// positions and inferred types that legitimately differ in phrasing
// between passes; what must not differ is WHICH findings exist.
func diagKey(d lang.CheckDiagnostic) string {
	return d.Code + "/" + d.Word
}

func diagSet(ds []lang.CheckDiagnostic) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, diagKey(d))
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

	var rows, diverged int
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
			_, _, armed, cerr := ac.CompileCheck(input)
			if cerr != nil {
				continue
			}

			p, c := diagSet(plain.Diagnostics), diagSet(armed.Diagnostics)
			if strings.Join(p, ",") == strings.Join(c, ",") {
				continue
			}
			diverged++
			shape := "plain=" + strings.Join(p, "|") + " armed=" + strings.Join(c, "|")
			byShape[shape]++
			if len(examples) < 5 {
				examples = append(examples, e.Name()+":L"+itoa(lineNum)+"  "+firstNRunes(input, 60))
			}
		}
		f.Close()
	}

	t.Logf("diagnostic parity: %d rows, %d diverged between plain check and compile-armed check", rows, diverged)
	for _, ex := range examples {
		t.Logf("  e.g. %s", ex)
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
