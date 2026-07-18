package modules

func init() {
	registerDocs("aql:sift", map[string]string{
		"define":   "Register a spec-map as a named parse kind: define NAME SPEC.",
		"parse":    "Parse a source with an inline spec-map or a registered kind name.",
		"kinds":    "List the atoms of every sift-registered kind (families, presets, user defines).",
		"families": "List the six format-family atoms: kv, blocks, columns, dsv, fixed, pattern.",
		"spec":     "The registered spec-map for a kind name (introspection): spec NAME.",
		"detect":   "The kind a path or argv resolves to via the detection table, or None.",
		"check":    "Validate a spec-map without registering; a list of {code detail} issues, empty when valid.",
	})
}
