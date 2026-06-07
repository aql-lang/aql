package modules

func init() {
	registerDocs("aql:vm", map[string]string{
		"run":         "Run code in a sandboxed sub-engine, returning the result.",
		"run-sandbox": "Run code under the 'sandbox' profile (pure computation).",
		"run-compute": "Run code under the 'compute' profile (arithmetic).",
		"run-with":    "Run code under an explicit policy supplied as a data map.",
	})
}
