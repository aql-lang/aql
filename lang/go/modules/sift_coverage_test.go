package modules

import (
	_ "embed"
	"sort"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

//go:embed sift_test.aql
var siftTestAQL string

// siftCoverAllow lists sift.aql rows that are STRUCTURALLY UNREACHABLE through
// the public Sift API — shadowed defense-in-depth guards a caller's earlier
// validation always intercepts (the AQL analog of //covergate:allow). Each
// entry is asserted to actually be uncovered, so it cannot rot.
//
// Both are terminal `else` arms of an exhaustive dispatch over a value that is
// validated against a FIXED SET one call earlier — the validator raises first,
// so the arm is dead through every public entry point:
//   - 136 sift-coerce-val's "unknown type": vtype is checked by sift-known-type
//     (against sift-type-set) at spec build — sift-w-parse-opts, sift-kv-types,
//     sift-fields-types — before sift-coerce reaches it.
//   - 809 sift-run-family's "unknown family": family is checked by
//     sift-known-family in sift-spec-from before sift-run-spec runs it.
var siftCoverAllow = map[int]string{
	136: "shadowed: sift-known-type (sift-type-set) validates vtype at spec build before sift-coerce-val",
	809: "shadowed: sift-known-family validates family in sift-spec-from before sift-run-spec calls sift-run-family",
}

// TestSiftAQLCoverage runs the aql:test suite for sift under the coverage hook
// and asserts (1) every case passes and (2) every executable row of sift.aql is
// covered, save the allowlisted unreachable guards.
func TestSiftAQLCoverage(t *testing.T) {
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
	// Run the suite (Test.cover [...]), then read fail-count + the coverage
	// report on the same registry so the cover collector persists.
	runOn("suite", siftTestAQL)
	fails, _ := runOn("fail-count", `Test.fail-count`).AsConcreteInteger()
	report := runOn("report", `Test.report`).String()
	rep, merr := native.AsMap(runOn("coverage", `Test.coverage "aql:sift"`))
	if merr != nil {
		t.Fatalf("coverage report is not a map")
	}

	if fails != 0 {
		t.Fatalf("%d sift test case(s) failed:\n%s", fails, report)
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
	t.Logf("sift.aql coverage: %d/%d rows (%.1f%%), %d uncovered", covered, total, pct, len(uncovered))

	// Every uncovered row must be allowlisted; every allowlist entry must be
	// genuinely uncovered (so the list can't rot).
	srcLines := strings.Split(siftSource, "\n")
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
		if _, ok := siftCoverAllow[row]; !ok {
			unexpected = append(unexpected, "  "+itoa(row)+": "+line(row))
		}
	}
	var stale []int
	for row := range siftCoverAllow {
		if !uncovSet[row] {
			stale = append(stale, row)
		}
	}
	sort.Ints(stale)

	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		t.Fatalf("%d sift.aql row(s) uncovered and not allowlisted — add a test (or allowlist a proven-unreachable guard):\n%s",
			len(unexpected), strings.Join(unexpected, "\n"))
	}
	for _, row := range stale {
		t.Errorf("siftCoverAllow[%d] is now covered — remove it (guard reached: %q)", row, line(row))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
