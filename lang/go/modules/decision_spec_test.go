package modules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// TestDecisionSpec loads decision_spec.aql as an AQL module, runs the
// spec through aql:test's pure-AQL runner, and asserts every case
// passes. The spec mirrors decision_test.go but expressed
// declaratively — proof that the spec system can fully replace the
// hand-written Go assertion suite for data-shaped decision tests.
func TestDecisionSpec(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(".", "decision_spec.aql"))
	if err != nil {
		t.Fatal(err)
	}

	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	r.Modules.Resolver = Resolve

	// Install decision exports as a `decision` def so `decision.eval-cond/q`
	// resolves inside the spec subjects.
	if err := InstallDecisionExports(r); err != nil {
		t.Fatal(err)
	}

	// Install the test module so the spec's `"aql:test" import` resolves.
	// We pre-install rather than rely on the import resolver because
	// decision_spec.aql is loaded as a top-level program here.
	testDesc, err := BuildTestModule(r)
	if err != nil {
		t.Fatal(err)
	}
	for name, exp := range testDesc.Exports {
		r.Defs.Push(name, native.NewMap(exp))
	}

	// Parse and run the spec module body. The body's `def spec {...}`
	// installs `spec` in the local scope and `export` records it.
	tokens, err := parser.Parse(string(source))
	if err != nil {
		t.Fatalf("parse decision_spec.aql: %v", err)
	}
	if _, err := native.NewTop(r).Run(tokens); err != nil {
		t.Fatalf("run decision_spec.aql: %v", err)
	}

	// Now invoke the spec runner.
	runSrc := `spec test.run-spec`
	runTokens, _ := parser.Parse(runSrc)
	if _, err := native.NewTop(r).Run(runTokens); err != nil {
		t.Fatalf("run-spec: %v", err)
	}

	// Summary check.
	summarySrc := `test.summary`
	summaryTokens, _ := parser.Parse(summarySrc)
	result, err := native.NewTop(r).Run(summaryTokens)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := native.AsMap(result[0])
	total, _ := m.Get("total")
	passed, _ := m.Get("passed")
	failed, _ := m.Get("failed")
	tn, _ := native.AsInteger(total)
	pn, _ := native.AsInteger(passed)
	fn, _ := native.AsInteger(failed)
	if fn != 0 {
		// Pretty-print failures via report module so the failures appear
		// inline with go test output rather than just a count.
		desc, _ := BuildReportModule(r)
		for n, exp := range desc.Exports {
			r.Defs.Push(n, native.NewMap(exp))
		}
		rep, _ := parser.Parse(`test.results report.table`)
		repRes, _ := native.NewTop(r).Run(rep)
		if len(repRes) > 0 {
			s, _ := native.AsString(repRes[0])
			t.Logf("decision spec table:\n%s", s)
		}
		t.Errorf("decision spec: total=%d passed=%d failed=%d, want all-pass", tn, pn, fn)
	}
	if tn == 0 {
		t.Error("decision spec ran zero cases — spec module is empty?")
	}
}
