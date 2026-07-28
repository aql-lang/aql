package test

import (
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// `aql check` is documented as "type-check without running", but it runs
// imported module bodies for real. Two deliberate decisions compose to
// produce that:
//
//   - every `import` signature is registered RunInCheck, because the
//     checker cannot type `Mod.v` without the module's actual exports;
//   - check mode is deliberately NOT propagated into the module
//     sub-registry, because carrier-stripping destroys the concrete string
//     literals a body uses as export names and map keys.
//
// So a module body's effects — prints, file writes, network calls —
// execute with full ambient authority during a type check. And because
// file modules are never cached, a default run (which pre-flight checks
// first) executes each body twice per importer.
//
// Neither decision is wrong on its own, and neither is changed here. What
// was missing is that any of it was visible. These tests pin the info
// diagnostic that reports it.

func countCode(res lang.CheckResult, code string) int {
	n := 0
	for _, d := range res.Diagnostics {
		if d.Code == code {
			n++
		}
	}
	return n
}

func TestCheckReportsModuleBodyExecution(t *testing.T) {
	res := checkDiag(t, `import module [ export "N" {w: 2} ] end
N.w`)
	if got := countCode(res, "module_body_executed_in_check"); got != 1 {
		t.Fatalf("one module body ran, want 1 diagnostic, got %d: %v",
			got, diagCodes(res))
	}
	d, _ := diagWithCode(res, "module_body_executed_in_check")
	if d.Severity != lang.SeverityInfo {
		t.Errorf("severity = %s, want info — the program is not wrong, the "+
			"behaviour was merely invisible", d.Severity)
	}
	// The detail has to say WHY, or a reader will file it as a bug in
	// `import` rather than recognise the two decisions behind it.
	for _, frag := range []string{"import", "check"} {
		if !strings.Contains(d.Detail, frag) {
			t.Errorf("detail %q should mention %q", d.Detail, frag)
		}
	}
}

// TestCheckModuleExecutionCountIsNotDeduped pins the count, which is the
// actual finding: effects multiply by IMPORTERS, not by a fixed 2. A
// deduped diagnostic would report "a module body ran" and hide that it ran
// three times.
func TestCheckModuleExecutionCountIsNotDeduped(t *testing.T) {
	res := checkDiag(t, `import module [ export "A" {v: 1} ] end
import module [ export "B" {v: 2} ] end
import module [ export "C" {v: 3} ] end
A.v`)
	if got := countCode(res, "module_body_executed_in_check"); got != 3 {
		t.Fatalf("three module bodies ran, want 3 diagnostics, got %d: %v",
			got, diagCodes(res))
	}
}

// TestCheckWithoutModulesIsSilent is the negative half: the diagnostic must
// cost nothing for the overwhelming majority of programs, which import no
// module at all. An advisory that fires on everything is noise, and noise
// is how a real finding gets ignored.
func TestCheckWithoutModulesIsSilent(t *testing.T) {
	for _, src := range []string{
		`1 add 2`,
		`def f fn [[x:Integer][Integer][x mul 2]] f 5`,
		`do [1 div 0] error [drop 9]`,
	} {
		res := checkDiag(t, src)
		if got := countCode(res, "module_body_executed_in_check"); got != 0 {
			t.Errorf("%q imports nothing but drew %d module-execution "+
				"diagnostic(s): %v", src, got, diagCodes(res))
		}
	}
}
