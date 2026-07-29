// errorcodes_test.go gates REFERENCE.md's "Common codes" table against the
// codes the engine can actually mint.
//
// That table is a DISPATCH CONTRACT, not prose: readers write
//
//	do [risky] error [dot code case [not_found/q "…" bad_input/q "…"]]
//
// against it, so a documented code no site mints is not a typo — it is a
// `case` arm that can never fire, and the author has no way to discover
// that short of provoking the condition and printing the result. Seven of
// the table's twenty-three rows were exactly that when this gate was
// written (`type_mismatch`, `out_of_range`, `unify_fail`,
// `extend_user_type`, `io_error`, `cap_denied`, `cancelled`), plus one
// duplicated row. See design/verse-report-defects-investigation.0.md §E.
//
// There is no registry-side enumeration of codes to compare against —
// codes are string literals at ~700 construction sites across eng, lang
// and cmd — so the truth set is EXTRACTED from those sites. Two
// consequences worth stating:
//
//   - The gate is ONE-DIRECTIONAL on purpose. It proves every DOCUMENTED
//     code is mintable. It says nothing about the ~200 mintable codes the
//     table omits, because the table is "common codes", not a census.
//   - A code being mintable does NOT prove the row's DESCRIPTION is
//     right, only that the name is real. `type_mismatch` would have
//     failed here; a row that named `signature_error` but described the
//     wrong condition would not. That part still needs a reader.
package docexamples

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// codeSourceRoots are the trees searched for code-minting sites,
// relative to this package (repo root is two dirs up from
// test/go/docexamples).
var codeSourceRoots = []string{"eng/go", "lang/go", "cmd/go"}

// codeMintPatterns match every shape that ATTACHES an error code to an
// error or a diagnostic. Capture group 1 is the code.
//
// Deliberately NOT matched: a package-level constant that merely spells a
// code (`CodePermissionDenied = "aql/permission_denied"`). Counting
// declarations would have made this gate vacuous exactly where it is most
// needed — `policy` declares four such constants, its header says "the
// engine adapter copies these onto the produced AqlError", and until that
// adapter was written not one of them ever reached a user. A code is real
// when a site attaches it, not when a site names it.
//
// The `aql/` prefix stays optional in the struct-literal pattern because a
// code can be assigned from such a constant; AqlError stores the bare name
// and renders the prefix.
var codeMintPatterns = []*regexp.Regexp{
	// r.AqlError("x", …) / r.AqlErrorHint("x", …) / MakeAqlError("x", …)
	// / MakeAqlErrorAt / makeAqlError / makeAqlErrorAt.
	regexp.MustCompile(`\b(?:M|m)akeAqlError(?:At)?\(\s*"([a-z][a-z0-9_]*)"`),
	regexp.MustCompile(`\.AqlError(?:Hint)?\(\s*"([a-z][a-z0-9_]*)"`),
	// AqlError / Diagnostic struct literals: Code: "x".
	regexp.MustCompile(`\bCode:\s*"(?:aql/)?([a-z][a-z0-9_]*)"`),
	// Check-mode diagnostics: CheckAddUniqueDiagnostic(r, "x", …).
	regexp.MustCompile(`\bCheckAdd\w*Diagnostic\([^,)]*,\s*"([a-z][a-z0-9_]*)"`),
}

// mintableCodes walks the source roots and returns every code any site
// can attach to an error or diagnostic.
func mintableCodes(t *testing.T) map[string]string {
	t.Helper()
	found := map[string]string{}
	for _, root := range codeSourceRoots {
		dir := filepath.Join("..", "..", "..", root)
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, re := range codeMintPatterns {
				for _, m := range re.FindAllSubmatch(src, -1) {
					code := string(m[1])
					if _, seen := found[code]; !seen {
						found[code] = path
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	return found
}

// documentedCodes parses the "Common codes" table out of REFERENCE.md,
// in document order and WITH duplicates, so the caller can flag both
// phantoms and repeated rows.
func documentedCodes(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "REFERENCE.md"))
	if err != nil {
		t.Fatalf("reading REFERENCE.md: %v", err)
	}
	lines := strings.Split(string(src), "\n")
	// Anchor on the prose that introduces the table rather than on a
	// line number or on "| Code | Meaning |" — REFERENCE.md has two
	// other tables with that exact header, both of them EXIT codes.
	start := -1
	for i, ln := range lines {
		if strings.Contains(ln, "Common codes:") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("REFERENCE.md: no `Common codes:` table found — if the table " +
			"moved or was renamed, retarget this gate rather than deleting it")
	}
	// One code per row, by convention — a cell spelling two codes
	// (`read_error` / `write_error`) would be skipped silently, which is
	// the one way a row could slip past this gate. Split such rows.
	cell := regexp.MustCompile("^\\|\\s*`([a-z][a-z0-9_]*)`\\s*\\|")
	suspicious := regexp.MustCompile("^\\|\\s*`[a-z][a-z0-9_]*`[^|]")
	var out []string
	for _, ln := range lines[start:] {
		if m := cell.FindStringSubmatch(ln); m != nil {
			out = append(out, m[1])
			continue
		}
		if suspicious.MatchString(ln) {
			t.Errorf("codes-table row names more than one code in its first "+
				"cell, so this gate cannot read it — give each code its own "+
				"row:\n  %s", strings.TrimSpace(ln))
			continue
		}
		// Stop at the first blank line AFTER at least one row, so the
		// gap between the prose anchor and the table header doesn't end
		// the scan.
		if len(out) > 0 && strings.TrimSpace(ln) == "" {
			break
		}
	}
	return out
}

func TestReferenceErrorCodesAreMintable(t *testing.T) {
	mintable := mintableCodes(t)
	// A floor, so a regex that silently stops matching fails as a broken
	// gate rather than as ~20 simultaneous "phantom" reports pointing at
	// innocent rows.
	if len(mintable) < 100 {
		t.Fatalf("extracted only %d codes from %v — the mint patterns have "+
			"stopped matching; fix codeMintPatterns before reading any "+
			"failure below as a documentation bug", len(mintable), codeSourceRoots)
	}

	documented := documentedCodes(t)
	if len(documented) < 10 {
		t.Fatalf("parsed only %d rows out of REFERENCE.md's codes table — "+
			"the table's shape changed and this gate is no longer reading it",
			len(documented))
	}

	for _, code := range documented {
		if _, ok := mintable[code]; !ok {
			t.Errorf("REFERENCE.md documents `%s`, which NO site mints — a "+
				"`case %s/q` arm against it can never fire. Either the code "+
				"was renamed (find the real one and fix the row) or the "+
				"condition raises code-lessly (give it a code).", code, code)
		}
	}
}

// A duplicated row is its own small defect: the table is read as a
// checklist, and the same code described twice reads as two conditions.
func TestReferenceErrorCodeTableHasNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, code := range documentedCodes(t) {
		if seen[code] {
			t.Errorf("REFERENCE.md's codes table lists `%s` twice", code)
		}
		seen[code] = true
	}
}
