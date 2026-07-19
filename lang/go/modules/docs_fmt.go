package modules

func init() {
	registerDocs("aql:fmt", map[string]string{
		"format":          "Pretty-print AQL source text into canonical layout.",
		"format-markdown": "Reformat AQL in ```aql fences and <!-- aqlfmt --> regions of Markdown.",
		"format-html":     "Reformat AQL in <!-- aqlfmt --> regions of an HTML document.",
		"render":          "Lay out a declarative document tree (text/line/group/indent) to a width.",
		"kind":            "Classify a node for rule dispatch: a $kind-tagged Map's tag, else 'map' / 'list' / 'scalar'.",
		"children":        "The child sequence a rule recurses over: a Map's {$kind:'entry' key value} entries, a List's elements, else [].",
	})
}
