// Check-accuracy ratchet (design/checker-accuracy-review.0.md §5).
//
// Runs `aql check` semantics (Registry.Check.Begin + a normal engine
// run) over every row of the production language spec at lang/spec/
// and counts two things:
//
//   - FALSE POSITIVES: rows whose expectation is a VALUE (the program
//     is correct and runs) but where the checker reports an
//     error-severity diagnostic or a hard error. Target: 0.
//   - UNFLAGGED ERROR ROWS: rows whose expectation is ERROR:* but
//     where the checker is silent. This will never reach 0 — many
//     spec errors are value-dependent (overflow, missing keys,
//     division by zero) and are the runtime's job — so the count is
//     a trend metric, not a target.
//
// Both counts are pinned and may only DECREASE (a ratchet): a change
// that pushes either count above its pin is a checker-accuracy
// regression and fails this test. When a fix lowers a count, lower
// the pin in the same commit so the gain is locked in.
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/test/go/specrunner"
)

// The ratchet pins. Lower them when a checker improvement lands;
// never raise them without a documented decision.
const (
	pinnedFalsePositives     = 120 // value rows the checker wrongly errors on (June 2026; was 119 — +1 module-minilang §7 register-compiled-then-use row, like the MiniLang.register rows the checker can't see the runtime register for; was 114 — +5 module-parselang register-then-parse rows; was 132 — macro installs in check mode; was 122 — make returns a value carrier of the made type, not the type literal)
	pinnedUnflaggedErrorRows = 139 // ERROR rows the checker is silent on (June 2026; was 136 — +3 module-minilang §7 runtime-only errors: mini_no_transducer / mini_bad_compiler / mini_bad_name; was 131 — +5 module-parselang runtime-only errors; was 132)
)

func TestCheckAccuracyRatchet(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var falsePositives, unflagged, valueRows, errorRows int

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		// bytecode-combinations.tsv is a compiled-vs-interpreter PARITY
		// fixture (validated by the differential / whole-corpus / spec
		// gates), not a checker-accuracy spec — its rows deliberately
		// exercise dynamic/island shapes the gradual checker widens, so
		// it must not move the false-positive / soundness baselines.
		if e.Name() == "bytecode-combinations.tsv" {
			continue
		}
		path := filepath.Join(specDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue // malformed rows are TestSpecProd's problem
			}
			input := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(parts[1])
			expectError := strings.HasPrefix(expected, "ERROR:")

			flagged := checkFlagsError(t, input)

			if expectError {
				errorRows++
				if !flagged {
					unflagged++
				}
				continue
			}
			valueRows++
			if flagged {
				falsePositives++
				t.Logf("FALSE POSITIVE %s:L%d: %s", e.Name(), lineNum, input)
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	t.Logf("check-accuracy: %d/%d value rows falsely flagged; %d/%d error rows unflagged",
		falsePositives, valueRows, unflagged, errorRows)

	if falsePositives > pinnedFalsePositives {
		t.Errorf("false positives rose to %d (pin %d) — the checker now wrongly rejects correct spec rows",
			falsePositives, pinnedFalsePositives)
	} else if falsePositives < pinnedFalsePositives {
		t.Logf("false positives improved to %d — lower pinnedFalsePositives to lock it in", falsePositives)
	}
	if unflagged > pinnedUnflaggedErrorRows {
		t.Errorf("unflagged error rows rose to %d (pin %d) — the checker lost coverage",
			unflagged, pinnedUnflaggedErrorRows)
	} else if unflagged < pinnedUnflaggedErrorRows {
		t.Logf("unflagged error rows improved to %d — lower pinnedUnflaggedErrorRows to lock it in", unflagged)
	}
}

// checkFlagsError runs one spec row in check mode against a fresh
// production registry (the same setup as runSpecProd) and reports
// whether the checker flags it: a parse failure, a hard run error, or
// any error-severity diagnostic.
func checkFlagsError(t *testing.T, input string) bool {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		return true // parse errors are flagged by definition
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	specrunner.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)

	done := reg.Check.Begin()
	_, runErr := native.NewTop(reg).Run(values)
	diags := reg.Check.Diagnostics
	done()

	if runErr != nil {
		return true
	}
	for _, d := range diags {
		if d.Severity == eng.SeverityError {
			return true
		}
	}
	return false
}

// ---- type-soundness differential (checker-accuracy-review.0.md §5,
// follow-on): for every value row that BOTH checks clean and runs,
// the runtime result's type must be covered by the checked carrier —
// `typeof(actual) ⊑ checked`. This is the assertion that catches
// wrong-TYPE checker bugs (A1, A4), which the value-pinning ratchet
// cannot see. Violations are pinned and may only decrease.

const pinnedTypeSoundnessViolations = 14 // was 15 — do [body] now reports the body's full residual stack (matching doListHandler) instead of only the last value; was 16 — pop/shift TList sigs now declare their true 2-value Returns ([TList, TAny]); was 22 — corrected six wrong aql:time-util inner-native Returns (weeks/days/until/since → CalDuration, tz-offset → String, total-ms → Float) to match their handlers; was 159 — typeCovered now applies the runtime `is Type` membership rule to type-as-value actuals (typeof / type-algebra / make-of-a-type / record shapes), which a raw actual.Parent.ConformsTo misjudged (a type literal's Parent is the DENOTED type's lattice parent), removing 127 spurious flags and exposing the genuine residue (wrong module-time Returns annotations, do/for/pop arity, dynamic method dispatch)

func TestCheckTypeSoundness(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var violations, compared int

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		// bytecode-combinations.tsv is a compiled-vs-interpreter PARITY
		// fixture (validated by the differential / whole-corpus / spec
		// gates), not a checker-accuracy spec — its rows deliberately
		// exercise dynamic/island shapes the gradual checker widens, so
		// it must not move the false-positive / soundness baselines.
		if e.Name() == "bytecode-combinations.tsv" {
			continue
		}
		path := filepath.Join(specDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(f)
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
			expected := strings.TrimSpace(parts[1])
			if strings.HasPrefix(expected, "ERROR:") {
				continue
			}

			checked, flagged := checkRow(t, input)
			if flagged {
				continue // counted by the FP ratchet, not here
			}
			actual, ok := runRow(t, input)
			if !ok {
				continue // runtime-environment rows (fixtures etc.)
			}
			compared++
			if !stackTypeCovered(checked, actual) {
				violations++
				t.Logf("TYPE UNSOUND %s:L%d: %s\n  checked=%s actual=%s",
					e.Name(), lineNum, input, stackTypes(checked), stackTypes(actual))
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	t.Logf("type-soundness: %d violations across %d compared rows", violations, compared)
	if violations > pinnedTypeSoundnessViolations {
		t.Errorf("type-soundness violations rose to %d (pin %d)", violations, pinnedTypeSoundnessViolations)
	} else if violations < pinnedTypeSoundnessViolations {
		t.Logf("violations improved to %d — lower pinnedTypeSoundnessViolations to lock it in", violations)
	}
}

// checkRow runs one row in check mode and returns the residual
// carrier stack plus whether the checker flagged it.
func checkRow(t *testing.T, input string) ([]eng.Value, bool) {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		return nil, true
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	specrunner.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)

	done := reg.Check.Begin()
	out, runErr := native.NewTop(reg).Run(values)
	diags := reg.Check.Diagnostics
	done()
	if runErr != nil {
		return nil, true
	}
	for _, d := range diags {
		if d.Severity == eng.SeverityError {
			return nil, true
		}
	}
	return out, false
}

// runRow executes one row at runtime; ok=false when the row needs an
// environment this harness doesn't provide (it errored at runtime
// although the spec expects a value — fixtures, network, files).
func runRow(t *testing.T, input string) ([]eng.Value, bool) {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		return nil, false
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	specrunner.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)
	out, runErr := native.NewTop(reg).Run(values)
	if runErr != nil {
		return nil, false
	}
	return out, true
}

// stackTypeCovered checks the runtime stack against the checked
// carrier stack position-for-position from the TOP. A checked stack
// may be longer (None padding from branch joins); a runtime stack
// longer than the checked one is a violation.
func stackTypeCovered(checked, actual []eng.Value) bool {
	if len(actual) > len(checked) {
		return false
	}
	for i := 0; i < len(actual); i++ {
		c := checked[len(checked)-1-i]
		a := actual[len(actual)-1-i]
		if !typeCovered(c, a) {
			return false
		}
	}
	return true
}

// typeCovered reports whether one checked carrier admits the runtime
// value's type.
func typeCovered(checked, actual eng.Value) bool {
	if checked.Dynamic {
		return true
	}
	if eng.IsDisjunct(checked) {
		di, err := eng.AsDisjunct(checked)
		if err != nil {
			return false
		}
		for _, alt := range di.Alternatives {
			if typeCovered(alt, actual) {
				return true
			}
		}
		return false
	}
	node := checked.Parent
	if eng.IsBareTypeNode(checked) {
		node = eng.ValueType(checked)
	}
	if node == nil {
		return false
	}
	// A type-as-value actual — the result of `typeof`, type algebra
	// (`exclude`/`extract`/`tor`), `make`-of-a-type, a record shape left
	// on the stack — is a VALUE THAT IS A TYPE. Its runtime membership is
	// the `is Type` rule (native_type.go), NOT actual.Parent.ConformsTo:
	// a type literal's Parent is the DENOTED type's lattice parent (the
	// `Integer` literal has Parent == Number, the `Resource` literal has
	// Parent == Object), so a raw Parent.ConformsTo misjudges every such
	// value as not-a-Type. The checker correctly types these as the
	// meta-type `Type`; cover them when the checked node admits `Type`
	// (checked == Type/Any), or — when the checker was precise enough to
	// produce a specific type literal — when the actual's denoted node
	// conforms to it.
	if actualIsTypeValue(actual) {
		if eng.TType.ConformsTo(node) {
			return true
		}
		if dn := eng.ValueType(actual); dn != nil && dn.ConformsTo(node) {
			return true
		}
	}
	return actual.Parent.ConformsTo(node)
}

// actualIsTypeValue mirrors the runtime `is Type` membership rule
// (native_type.go isHandler): a value is itself a type when it is a
// bare type node, a structural type body, an (explicit-map) record
// shape, or a Type/-rooted value (Function / Disjunct / Enum). Used by
// typeCovered so a `typeof`/type-algebra result is judged by its
// type-membership, not by its denoted type's lattice parent.
func actualIsTypeValue(v eng.Value) bool {
	return eng.IsBareTypeNode(v) || eng.IsTypeBody(v) ||
		eng.IsRecordShape(v) || v.Parent.ConformsTo(eng.TType)
}

func stackTypes(vs []eng.Value) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.Parent.Leaf()
		if v.Dynamic {
			parts[i] = "dynamic(" + parts[i] + ")"
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// ---- Any-frontier metric (the "narrow types instead of Any" goal):
// counts clean value rows whose residual carrier stack still contains an
// Any-bounded carrier — strict Carry<Any> or dynamic(Any), the two shapes
// where the checker gave up to Any instead of a narrow type. (A dynamic
// carrier with a real bound — dynamic(Integer) — is NOT counted; it has a
// useful type.) Pinned as a ratchet (only decreases) so return-annotation
// / ReturnsFn narrowing work is measured and an Any-widening regression is
// caught. Unlike type-soundness this is a PRECISION metric, not a
// soundness one: a high count is imprecise, not unsound.

const pinnedAnyFrontierRows = 208 // was 217 — filter narrows to its input collection type (List/Map subset) instead of Any; was 241 (case branch-join), 286 (item #4 field access); see TestCheckAnyFrontier

func TestCheckAnyFrontier(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var anyRows, valueRows int
	byFile := map[string]int{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		if e.Name() == "bytecode-combinations.tsv" {
			continue
		}
		f, err := os.Open(filepath.Join(specDir, e.Name()))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 || strings.HasPrefix(strings.TrimSpace(parts[1]), "ERROR:") {
				continue
			}
			checked, flagged := checkRow(t, strings.TrimSpace(parts[0]))
			if flagged {
				continue
			}
			valueRows++
			if residualHasAnyFrontier(checked) {
				anyRows++
				byFile[e.Name()]++
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner: %v", err)
		}
	}

	t.Logf("any-frontier: %d/%d clean value rows end with an Any carrier", anyRows, valueRows)
	for _, kv := range topFiles(byFile, 10) {
		t.Logf("  %3d  %s", kv.n, kv.name)
	}
	if anyRows > pinnedAnyFrontierRows {
		t.Errorf("Any-frontier rows rose to %d (pin %d) — a word widened to Any where it previously narrowed",
			anyRows, pinnedAnyFrontierRows)
	} else if anyRows < pinnedAnyFrontierRows {
		t.Logf("Any-frontier improved to %d — lower pinnedAnyFrontierRows to lock it in", anyRows)
	}
}

// residualHasAnyFrontier reports whether any carrier in a residual stack
// is Any-bounded (strict Any carrier or dynamic(Any)) — the "gave up to
// Any" shape the frontier metric counts.
func residualHasAnyFrontier(stk []eng.Value) bool {
	for _, v := range stk {
		if v.Parent != nil && v.Parent.Equal(eng.TAny) {
			return true
		}
	}
	return false
}

type fileCount struct {
	name string
	n    int
}

// topFiles returns the n spec files with the most Any-frontier rows,
// most first — a worklist of where narrowing would pay off most.
func topFiles(byFile map[string]int, n int) []fileCount {
	out := make([]fileCount, 0, len(byFile))
	for name, c := range byFile {
		out = append(out, fileCount{name, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].name < out[j].name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
