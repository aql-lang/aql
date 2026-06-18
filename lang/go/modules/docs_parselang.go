package modules

func init() {
	registerDocs("aql:parselang", map[string]string{
		"parse_ini": "Built-in INI parser: `parse ini <text>` — decodes INI text to a Map " +
			"(github.com/tabnas/ini/go). Top-level `key = value` lines become Map fields, a " +
			"`[section]` header nests a Map (dotted `[a.b]` nests further), and a recognised " +
			"boolean decodes to a Boolean. Malformed input raises parse_syntax_error.",
		"register": "Install an AQL fn as a new parser: ParseLang.register <name> <fn>. " +
			"Every fn signature must start with the standard prefix [source:String opts:Map …].",
		"kinds": "List the registered parser-kind atoms.",
		"source": "Resolve a `parse` source value to its String: a String passes through, " +
			"a {src:'…'} map yields its src, a {file:…} map is not yet supported.",
	})
}
