package modules

func init() {
	registerDocs("aql:report", map[string]string{
		"value":  "Render any value as its single-line print form.",
		"record": "Render a map as a vertical key:value aligned block.",
		"table":  "Render a list of maps as an aligned grid.",
		"list":   "Render a list as indexed one-per-line entries.",
	})
}
