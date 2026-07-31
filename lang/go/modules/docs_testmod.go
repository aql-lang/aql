package modules

func init() {
	registerDocs("boru:test", map[string]string{
		// Test namespace
		"test":           "Define and run a test case, recording its outcome.",
		"it":             "Alias for test; define and run a single test case.",
		"describe":       "Group nested tests under a descriptive label.",
		"spec":           "Build a TestSpec from cases, a subject, and a name.",
		"spec-with-subs": "Build a TestSpec that also carries nested sub-specs.",
		"case":           "Build a TestCase from expected output, input, and name.",
		"invoke":         "Run a subject word against an input list.",
		"run-spec":       "Run a TestSpec, checking every case against its subject.",
		"prop":           "Build a PropertySpec with a generator and property.",
		"check-prop":     "Run a property's generator/property loop directly.",
		"run-property":   "Run a PropertySpec map, returning its property result.",
		"skip":           "Park a test case or property as skipped without running its body.",
		"results":        "Return the accumulated TestResult table.",
		"report":         "Render a one-line-per-case String summary of results.",
		"summary":        "Tally results into a total/passed/failed map.",
		"fail-count":     "Return the number of failed cases in the active run.",
		"reset":          "Clear the active run's accumulated results.",
		"cover":          "Run a body with line coverage recorded for the modules it exercises.",
		"coverage":       "Report covered/total/percent/uncovered rows for a covered module id.",
		// Assert namespace
		"equal":     "Assert that two values are equal.",
		"not-equal": "Assert that two values differ.",
		"ok":        "Assert that a value is truthy.",
		"match":     "Assert that a string contains a substring.",
		"throws":    "Assert that evaluating a body raises an error.",
	})

	// The assertion words take their operands in the boru swap order
	// (actual, then expected), and the runner words take a BODY — neither
	// of which the generated permutations can show. Results are from
	// verified lang/spec/module-test.tsv rows.
	registerExamples("boru:test", map[string][]string{
		"test": {
			`Test.test "adds" [(1 add 1) 2 Assert.equal]`,
			`Test.fail-count                                  ;# 0`,
		},
		"it": {`Test.it "adds" [(1 add 1) 2 Assert.equal]        ;# an alias for Test.test`},
		"describe": {
			`Test.describe "math" [`,
			`  Test.test "adds" [(1 add 1) 2 Assert.equal]`,
			`]`,
		},
		"equal": {
			`(1 add 1) 2 Assert.equal                         ;# ACTUAL first, expected second`,
		},
		"not-equal": {`1 2 Assert.not-equal`},
		"ok":        {`5 Assert.ok                                      ;# truthy; none/false/0 fail`},
		"match":     {`Assert.match "ell" "hello"                       ;# substring: NEEDLE first`},
		"throws": {
			`[1 0 div] Assert.throws                          ;# passes — the BODY must raise`,
			`[1] Assert.throws                                ;# fails — it did not raise`,
		},
		"summary": {
			`Test.test "ok" [true Assert.ok]`,
			`Test.summary                                     ;# {total:1 passed:1 failed:0}`,
		},
		"fail-count": {`Test.fail-count                                  ;# 0 — the exit code a runner wants`},
		"results":    {`Test.results size                                ;# one entry per test`},
		"report":     {`Test.report                                      ;# the human-readable run report`},
		"reset":      {`Test.reset                                       ;# clear the accumulated results`},
		"invoke": {
			`def double fn [[n:Integer] [Integer] [n 2 mul]]`,
			`[3] Test.invoke double/q                         ;# 6 — call by NAME with an argument list`,
		},
		"spec": {
			`def s {name: "doubling"  subject: double/q  cases: [`,
			`  {name: "d3" in: [3] out: 6}`,
			`  {name: "d0" in: [0] out: 0}`,
			`]  subs: []}`,
			`s Test.run-spec`,
			`Test.summary                                     ;# {total:2 passed:2 failed:0}`,
			`;# A spec is DATA: one table drives many cases.`,
		},
		"run-spec": {`s Test.run-spec                                 ;# run a spec table (see Test.spec)`},
		"case":     {`(Test.case 6 [3] "d3") get "name"                ;# 'd3' — build one case: out, in, name`},
		"cover":    {`Test.cover [ … ]                                 ;# record which words the body exercised`},
		"coverage": {`Test.coverage                                    ;# the accumulated coverage report`},
	})
}
