package modules

func init() {
	registerDocs("aql:struct-util", map[string]string{
		"clone":     "Deep-copy a value.",
		"getpath":   "Read the value at a dotted path.",
		"setpath":   "Write a value at a path, returning a new structure.",
		"inject":    "Resolve backtick `$`-path references against a store.",
		"merge":     "Deep, index-wise merge (later argument wins).",
		"walk":      "Collect leaf path/value pairs.",
		"items":     "Map entries as [key value] pairs.",
		"transform": "Shape data via a spec.",
		"validate":  "Check data against a shape spec.",
		"selector":  "Query-select children matching a spec.",
		"jsonify":   "Serialise a value to JSON text (class instances carry $class; user $-keys escape to $$).",
		"parse":     "Decode jsonic/JSON text to data (the complement of jsonify) — data context, loud parse_error on malformed input.",
		"reify":     "Hydrate a class instance from JSON text or a Node — explicit class or tor-union target.",
		"nodify":    "Project a value to its Node/Scalar form.",
	})
}
