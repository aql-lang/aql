package modules

import (
	_ "embed"
	"sort"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

//go:embed cli_test.aql
var cliTestAQL string

// cliCoverAllow lists cli.aql rows that cli_test.aql cannot reach — the AQL
// analogue of //covergate:allow. Each entry is asserted to actually be
// uncovered, so the list cannot rot.
//
// Every entry is inside `Cli.main`, and that is not an accident: it is the
// reason design/CLI-PROGRAMS.1.md §2 splits the surface. `Cli.main` ends in
// `IO.exit`, and `aql test`'s runner treats a raised error — which is what an
// exit is — as a file-level failure that ends the file. A test that exercised
// the --help arm would kill its own suite before reaching the --version arm.
// The pure half (parse / dispatch / usage), which is where the flag grammar
// lives, is covered exhaustively; what is left here is the three-line shell,
// small enough that reading it IS the verification.
var cliCoverAllow = map[int]string{
	// cli-main-do — the shell. Every arm ends in IO.exit.
	673: "cli-main-do: the dispatch call whose result the exiting arms below consume",
	674: "cli-main-do: prints d.out, then an arm below exits",
	675: "cli-main-do: prints d.err, then an arm below exits",
	676: "cli-main-do: the help/version/error arm — IO.exit ends the test FILE",
	677: "cli-main-do: builds the handler context, reached only on the run arm",
	678: "cli-main-do: applies the handler, reached only on the run arm",
	679: "cli-main-do: the handler's exit code — IO.exit ends the test FILE",
	// cli-w-main — the two exported signatures, each a one-line delegation to
	// cli-main-do. The argv-less one additionally reads IO.args, which is
	// always empty under `aql test`.
	684: "Cli.main (spec) (h): delegates to cli-main-do, which exits",
	685: "Cli.main (spec) (argv) (h): delegates to cli-main-do, which exits",
}

// TestCliAQLCoverage runs the aql:test suite for cli under the coverage hook
// and asserts (1) every case passes and (2) every executable row of cli.aql is
// covered, save the allowlisted exiting arms.
func TestCliAQLCoverage(t *testing.T) {
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
	runOn("suite", cliTestAQL)
	fails, _ := runOn("fail-count", `Test.fail-count`).AsConcreteInteger()
	report := runOn("report", `Test.report`).String()
	rep, merr := native.AsMap(runOn("coverage", `Test.coverage "aql:cli"`))
	if merr != nil {
		t.Fatalf("coverage report is not a map")
	}

	if fails != 0 {
		t.Fatalf("%d cli test case(s) failed:\n%s", fails, report)
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
	t.Logf("cli.aql coverage: %d/%d rows (%.1f%%), %d uncovered", covered, total, pct, len(uncovered))

	srcLines := strings.Split(cliSource, "\n")
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
		if _, ok := cliCoverAllow[row]; !ok {
			unexpected = append(unexpected, "  "+itoa(row)+": "+line(row))
		}
	}
	var stale []int
	for row := range cliCoverAllow {
		if !uncovSet[row] {
			stale = append(stale, row)
		}
	}
	sort.Ints(stale)

	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		t.Fatalf("%d cli.aql row(s) uncovered and not allowlisted — add a test (or allowlist a proven-unreachable arm):\n%s",
			len(unexpected), strings.Join(unexpected, "\n"))
	}
	for _, row := range stale {
		t.Errorf("cliCoverAllow[%d] is now covered — remove it (arm reached: %q)", row, line(row))
	}
}
