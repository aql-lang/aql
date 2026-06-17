package modules

func init() {
	registerDocs("aql:parselang", map[string]string{
		"register": "Install an AQL fn as a new parser: ParseLang.register <name> <fn>. " +
			"Every fn signature must start with the standard prefix [source:String opts:Map …].",
		"kinds": "List the registered parser-kind atoms.",
		"source": "Resolve a `parse` source value to its String: a String passes through, " +
			"a {src:'…'} map yields its src, a {file:…} map is not yet supported.",
	})
}
