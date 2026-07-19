package modules

func init() {
	registerDocs("aql:parselang", map[string]string{
		"parse_ini": "Built-in INI parser: `parse ini <text>` — decodes INI text to a Map " +
			"(github.com/tabnas/ini/go). Top-level `key = value` lines become Map fields, a " +
			"`[section]` header nests a Map (dotted `[a.b]` nests further), and a recognised " +
			"boolean decodes to a Boolean. Malformed input raises parse_syntax_error.",
		"parse_json":     "Built-in JSON parser: `parse json <text>` → the decoded top level (Map, List or scalar). github.com/tabnas/json/go.",
		"parse_jsonic":   "Built-in jsonic parser: `parse jsonic <text>` — relaxed JSON (unquoted keys, optional commas) → the decoded value. github.com/tabnas/jsonic/go.",
		"parse_json5":    "Built-in JSON5 parser: `parse json5 <text>` — JSON5 (comments, hex, trailing commas) → the decoded value. github.com/tabnas/json5/go.",
		"parse_jsonc":    "Built-in JSONC parser: `parse jsonc <text>` — JSON with comments → the decoded value. github.com/tabnas/jsonc/go.",
		"parse_csv":      "Built-in CSV parser: `parse csv <text>` → a List of rows, each a List of fields (numeric fields decode to numbers). github.com/tabnas/csv/go.",
		"parse_toml":     "Built-in TOML parser: `parse toml <text>` → a Map. github.com/tabnas/toml/go.",
		"parse_yaml":     "Built-in YAML parser: `parse yaml <text>` → the decoded value (typically a Map). github.com/tabnas/yaml/go.",
		"parse_xml":      "Built-in XML parser: `parse xml <text>` → a Node/Xml element ({tag, attr, cren}) — the same value an embedded `<tag>…</tag>` literal produces. github.com/tabnas/xml/go.",
		"parse_zon":      "Built-in ZON parser: `parse zon <text>` — Zig Object Notation → the decoded value. github.com/tabnas/zon/go.",
		"parse_markdown": "Built-in Markdown parser: `parse markdown <text>` → a List of blocks. github.com/tabnas/markdown/go.",
		"parse_feed":     "Built-in feed parser: `parse feed <text>` — RSS/Atom → a normalised atom-shaped Map. github.com/tabnas/feed/go.",
		"parse_aontu": "Built-in aontu parser: `parse aontu <text>` — the upstream Go port of " +
			"github.com/rjrodger/aontu, a CUE-inspired unification config dialect → a Node of Maps " +
			"and Lists. Top-level `k:v` entries and the colon-chain `a:b:c:1` build Maps, `[…]` builds " +
			"Lists, duplicate keys and `&` deep-merge, bare kind words (string/number/integer/boolean/top) " +
			"constrain scalars, an absolute `$.a` reference resolves from the root and a relative `.k` " +
			"reference resolves a sibling key. A conflict, unresolvable reference or a value that does " +
			"not fully resolve to concrete data raises parse_syntax_error.",
		"register": "TOMBSTONE — raises parse_registry_frozen: the parse kind namespace is fixed " +
			"and registration was removed. Pass a custom parser as a Function value instead " +
			"(def myp (fn [[source:String opts:Map] [Any] [...]])  parse myp '...').",
		"kinds": "List the (fixed) built-in parser-kind atoms.",
		"source": "Resolve a `parse` source value to its String: a String passes through, " +
			"a {src:'…'} map yields its src, a {file:…} map is not yet supported.",
	})
}
