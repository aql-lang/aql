package modules

func init() {
	registerDocs("aql:vm", map[string]string{
		"run":         "Run code in a sandboxed sub-engine, returning the result.",
		"run-sandbox": "Run code under the 'sandbox' profile (pure computation).",
		"run-compute": "Run code under the 'compute' profile (arithmetic).",
		"run-with":    "Run code under an explicit policy supplied as a data map.",
		"parse":       "Parse AQL source to a quoted token list without running it (loud parse_error on malformed source).",
		"check":       "Static type-check source without running it, returning a { ok errors warnings diagnostics } report.",
		"compile":     "Compile source to bytecode without running it, returning a { ok reason sites } report.",
	})
}
