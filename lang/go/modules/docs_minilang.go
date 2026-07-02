package modules

func init() {
	registerDocs("aql:minilang", map[string]string{
		"lang_re": "Go regular-expression match: `mini re <pattern>` — subject from the stack, " +
			"match structure {ok ms fst lst n} out; each match is {m i e g}. opts: {limit:I}.",
		"lang_bf": "Brainfuck: `mini bf <program>` — output String; `,` reads the stack input " +
			"(filter form) or opts.in. opts: {in:S, steps:I execution budget}.",
		"lang_gex": "Glob-expression select: `mini gex <pattern>` (or `+gex/<pattern>/`) — " +
			"anchored whole-subject match (`*` any run, `?` one char, `**`/`*?` literal `*`/`?`). " +
			"Returns the stack subject if it matches else None; filters a List to matching " +
			"elements and a Map to entries whose key matches.",
		"lang_m": "Traditional maths formula: `mini m '<formula>' {vars}` — evaluate an " +
			"arithmetic expression (operators + - * / % ^, unary +/-, parens; ^ is " +
			"right-associative and binds tighter than * /) whose variables are bound by the " +
			"named params. Numeric coercion follows AQL: all-integer operands stay Integer " +
			"(division truncates like `div`), any Float promotes to Float. github.com/tabnas/expr/go.",
		"lang_jp": "JSONPath query: `<doc> mini jp '<path>'` — run a JSONPath (e.g. " +
			"$.store.book[*].title) over the stack subject (a Node, Object, Array, Table or " +
			"Record) and return the matched nodes as a List. A Map/Record subject needs an " +
			"explicit {} opts. github.com/ohler55/ojg.",
		"lang_jq": "jq filter: `<doc> mini jq '<filter>'` — run a jq filter (e.g. .items[].name) " +
			"over the stack subject (same shapes as jp) and return its output stream as a List. " +
			"github.com/itchyny/gojq.",
		"lang_xp": "XPath query: `<xml> mini xp '<path>'` — run an XPath (e.g. //book/title, " +
			"//@id, count(//item)) over the stack Node/Xml subject and return a List: matched " +
			"nodes for a node-set (an element as its Node/Xml value, an attribute/text node as a " +
			"String), or a one-element list for a scalar count/string/boolean result. " +
			"github.com/antchfx/xpath.",
		"lang_hb": "Hex Bytes literal: `+hb/deadbeef/` (≡ mini hb 'deadbeef') — decode an " +
			"even-length hex string to a Bytes value. Whitespace and `_` in the source are " +
			"ignored, so `+hb/de_ad_be_ef/` groups for readability. A bad/odd-length source " +
			"raises mini_parse_error.",
		"lang_bb": "Binary Bytes literal: `+bb/01001100/` (≡ mini bb '01001100') — decode a " +
			"string of 0/1 bits (a multiple of 8, MSB-first per byte) to a Bytes value. " +
			"Whitespace and `_` are ignored. A non-binary digit or a non-multiple-of-8 length " +
			"raises mini_parse_error.",
		"register": "Install an AQL fn as a new mini-language: MiniLang.register <name> <fn>. " +
			"Every fn signature must start with the standard prefix [src:String opts:Map …].",
		"kinds": "List the registered mini-language kind atoms.",
		"register-compiled": "Add an expansion-time compile hook (a macro) to a kind: " +
			"MiniLang.register-compiled <name> (macro [[src opts] [ quote [ … ] ]]). The kind must " +
			"already have a transducer; `mini` runs the compiler at the call site and splices its tokens.",
		"run-re": "Internal: the compiled-`re` consumer — matches a precompiled-pattern carrier " +
			"(from the re compile hook) against the stack subject.",
	})
}
