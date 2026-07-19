package modules

func init() {
	registerDocs("aql:fmt", map[string]string{
		"format": "Pretty-print AQL source text into canonical layout.",
	})
}
