package modules

import (
	"sort"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// aqlcoverage_test.go — the shared AQL-LINE coverage gate for modules written
// in AQL (aql:sift, aql:cli).
//
// Nothing meta-tests that an AQL-authored module HAS such a gate — aql:vault-tui
// is AQL and has none — so the gate is opt-in per module, and this is the one
// implementation each opts into. Keeping it in one place is not only tidiness:
// two copies drifted the moment the second module needed a different message,
// and the linter's duplicate detector caught them.
//
// What it asserts: the module's own aql:test suite passes, AND every executable
// row of the module source is covered by it, save rows the module's allowlist
// names. Every allowlist entry must be genuinely uncovered, so the list cannot
// rot into a claim nobody re-checks.
func assertAQLCoverage(t *testing.T, coverID, moduleSource, testSource string, allow map[int]string) {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)

	runOn := func(label, aql string) native.Value {
		vals, perr := parser.Parse(aql)
		if perr != nil {
			t.Fatalf("parse %s: %v", label, perr)
		}
		out, rerr := native.NewTop(r).Run(vals)
		if rerr != nil {
			t.Fatalf("run %s: %v", label, rerr)
		}
		if len(out) == 0 {
			return native.Value{}
		}
		return out[len(out)-1]
	}
	// Run the suite, then read fail-count + the coverage report on the SAME
	// registry so the cover collector persists.
	runOn("suite", testSource)
	fails, _ := runOn("fail-count", `Test.fail-count`).AsConcreteInteger()
	report := runOn("report", `Test.report`).String()
	rep, merr := native.AsMap(runOn("coverage", `Test.coverage "`+coverID+`"`))
	if merr != nil {
		t.Fatalf("%s: coverage report is not a map", coverID)
	}
	if fails != 0 {
		t.Fatalf("%d %s test case(s) failed:\n%s", fails, coverID, report)
	}

	get := func(k string) native.Value { v, _ := rep.Get(k); return v }
	total, _ := get("total").AsConcreteInteger()
	covered, _ := get("covered").AsConcreteInteger()
	pct, _ := get("percent").AsConcreteFloat()
	uncovList, _ := native.AsList(get("uncovered"))
	var uncovered []int
	for _, v := range uncovList.Slice() {
		n, _ := v.AsConcreteInteger()
		uncovered = append(uncovered, int(n))
	}
	t.Logf("%s coverage: %d/%d rows (%.1f%%), %d uncovered", coverID, covered, total, pct, len(uncovered))

	srcLines := strings.Split(moduleSource, "\n")
	line := func(n int) string {
		if n >= 1 && n <= len(srcLines) {
			return strings.TrimSpace(srcLines[n-1])
		}
		return "?"
	}
	uncovSet := map[int]bool{}
	var unexpected []string
	for _, row := range uncovered {
		uncovSet[row] = true
		if _, ok := allow[row]; !ok {
			unexpected = append(unexpected, "  "+itoa(row)+": "+line(row))
		}
	}
	var stale []int
	for row := range allow {
		if !uncovSet[row] {
			stale = append(stale, row)
		}
	}
	sort.Ints(stale)

	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		t.Fatalf("%d %s row(s) uncovered and not allowlisted — add a test (or allowlist a proven-unreachable arm):\n%s",
			len(unexpected), coverID, strings.Join(unexpected, "\n"))
	}
	for _, row := range stale {
		t.Errorf("%s allowlist[%d] is now covered — remove it (arm reached: %q)", coverID, row, line(row))
	}
}
