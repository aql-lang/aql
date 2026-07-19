package modules

func init() {
	registerDocs("aql:fmt", map[string]string{
		"format":          "Pretty-print AQL source text into canonical layout.",
		"format-markdown": "Reformat AQL in ```aql fences and <!-- aqlfmt --> regions of Markdown.",
		"format-html":     "Reformat AQL in <!-- aqlfmt --> regions of an HTML document.",
	})
}
