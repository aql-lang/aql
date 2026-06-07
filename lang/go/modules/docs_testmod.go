package modules

func init() {
	registerDocs("aql:test", map[string]string{
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
		"skip":           "Record a parked property without running it.",
		"results":        "Return the accumulated TestResult table.",
		"report":         "Render a one-line-per-case String summary of results.",
		"summary":        "Tally results into a total/passed/failed map.",
		"fail-count":     "Return the number of failed cases in the active run.",
		"reset":          "Clear the active run's accumulated results.",
		// Assert namespace
		"equal":     "Assert that two values are equal.",
		"not-equal": "Assert that two values differ.",
		"ok":        "Assert that a value is truthy.",
		"match":     "Assert that a string contains a substring.",
		"throws":    "Assert that evaluating a body raises an error.",
	})
}
