package modules

func init() {
	registerDocs("boru:minilang", map[string]string{
		"lang_re": "Go regular-expression match: `mini re <pattern>` — subject from the stack, " +
			"match structure {ok ms fst lst n} out; each match is {m i e g}. opts: {limit:I}.",
		"lang_bf": "Brainfuck: `mini bf <program>` — output String; `,` reads the stack input " +
			"(filter form) or opts.in. opts: {in:S, steps:I execution budget}.",
		"lang_gex": "Glob-expression select: `mini gex <pattern>` (or `+gex/<pattern>/`) — " +
			"anchored whole-subject match (`*` any run, `?` one char, `**`/`*?` literal `*`/`?`). " +
			"Returns the stack subject if it matches else None; filters a List to matching " +
			"elements and a Map to entries whose key matches.",
		"lang_math": "Traditional maths formula: `mini math '<formula>' {vars}` — evaluate an " +
			"arithmetic expression (operators + - * / % ^, unary +/-, parens; ^ is " +
			"right-associative and binds tighter than * /) whose variables are bound by the " +
			"named params. Numeric coercion follows boru: all-integer operands stay Integer " +
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
		"lang_sp": "Structure-path (XPath-style) over boru structure: `<map-or-list> mini sp '<path>' {}` — run an XPath " +
			"(e.g. /name, //title, //item[p>20]/p, count(//item), sum(//v)) over the stack Map/List " +
			"subject and return a List. Same engine as xp but over native boru data: a Map is an " +
			"element per key, a List repeated `item` elements, a scalar the element's text. A matched " +
			"element comes back as its source value, a text/scalar result as a String/Number/Boolean. " +
			"A Map subject needs an explicit {} opts (a bare Map is claimed as opts). github.com/antchfx/xpath.",
		"lang_hb": "Hex Bytes literal: `+hb/deadbeef/` (≡ mini hb 'deadbeef') — decode an " +
			"even-length hex string to a Bytes value. Whitespace and `_` in the source are " +
			"ignored, so `+hb/de_ad_be_ef/` groups for readability. A bad/odd-length source " +
			"raises mini_parse_error.",
		"lang_bb": "Binary Bytes literal: `+bb/01001100/` (≡ mini bb '01001100') — decode a " +
			"string of 0/1 bits (a multiple of 8, MSB-first per byte) to a Bytes value. " +
			"Whitespace and `_` are ignored. A non-binary digit or a non-multiple-of-8 length " +
			"raises mini_parse_error.",
		"lang_micron": "Micron literal: `+m:alice@example.com` (≡ mini micron '…' ≡ mini m '…') — " +
			"parse the source with the ONE merged tabnas grammar built from each builtin Micron leaf's literal grammar " +
			"(Emailon, then Urlon, then Pathon) — the matching shape decides the type. Pathon accepts any " +
			"whitespace-free source, so it is the catch-all (`+m:a/b` is a Pathon). The leaf set is " +
			"FIXED — the merge is not extensible. URL sources " +
			"contain `:` and `/` — pick a delimiter outside the source: `+m|https://x.com/a`.",
		"lang_m": "Short form of the micron kind — see lang_micron. `+m:alice@example.com` " +
			"parses as an Emailon; `+m|https://x.com/a` as an Urlon; `+m:a/b` as a Pathon.",
		"micron": "TOMBSTONE — raises mini_registry_frozen. The +m literal grammar is fixed to the " +
			"builtin Micron leaves (Emailon, Urlon, Pathon); the user-shape hook was removed. A user " +
			"Micron TYPE (def Nameon refine Micron {…}) still works with make — parse custom sources " +
			"with a parser fn value; only the literal sugar is builtin-only.",
		"register": "TOMBSTONE — raises mini_registry_frozen. The mini kind namespace is fixed " +
			"(built-in kinds only); pass a custom mini-language as a Function value instead: " +
			"def myl (fn [[src:String opts:Map] [Any] [...]])  mini myl '...' — Go hosts build " +
			"one with NewMiniLangFn.",
		"Re": "The named type of a `re` partial — the Function a parked `+re/…/` (or " +
			"`def f (+re/…/)`) produces. `typeof` still reports Function (a member type is a " +
			"constraint, like a DepScalar); use `is` (`f/r is (MiniLang.Re)`) or a typed fn param " +
			"(`fn [[m:(MiniLang.Re) s:String] …]`) to require specifically a regexp matcher. " +
			"Every built-in filter kind has one: Re, Gex, Jp, Jq, Xp, Sp.",
		"Gex": "The named type of a `gex` partial (a stored glob-expression matcher) — " +
			"see MiniLang.Re for the convention.",
		"Jp": "The named type of a `jp` partial (a stored JSONPath query) — " +
			"see MiniLang.Re for the convention.",
		"Jq": "The named type of a `jq` partial (a stored jq filter) — " +
			"see MiniLang.Re for the convention.",
		"Xp": "The named type of an `xp` partial (a stored XPath query) — " +
			"see MiniLang.Re for the convention.",
		"Sp": "The named type of an `sp` partial (a stored structure-path query) — " +
			"see MiniLang.Re for the convention.",
		"kinds": "List the (fixed) mini-language kind atoms.",
		"register-compiled": "TOMBSTONE — raises mini_registry_frozen. The boru compile-hook " +
			"surface died with the frozen kind namespace; pass a custom mini-language as a " +
			"Function value (mini <fn> '...') and memoize any expensive compile inside the fn.",
		"run-re": "Internal: the compiled-`re` consumer — matches a precompiled-pattern carrier " +
			"(from the re compile hook) against the stack subject.",
	})

	// A mini-language is reached through `mini <kind> <src>`, not through
	// the lang_* export the signature names — and the RESULT is a match
	// map, not the matched text. Results are from verified
	// lang/spec/module-minilang.tsv rows.
	registerExamples("boru:minilang", map[string][]string{
		"kinds": {`MiniLang.kinds                                   ;# [re/q bf/q gex/q math/q hb/q bb/q micron/q m/q jp/q jq/q xp/q sp/q]`},
		"lang_re": {
			`def r ("AbcD" mini re '[a-z]+')`,
			`r.fst.m                                          ;# 'bc' — .fst is the first match, .m its text`,
			`("a1b2c3" mini re '\\d').n                       ;# 3 — how many matched`,
			`("a1b2c3" mini re '\\d' {limit:2}).n             ;# 2`,
			`("zz" mini re 'a+').ok                           ;# false`,
			`(('abc-123' mini re '([a-z]+)-(\\d+)')).fst.g    ;# ['abc' '123'] — capture groups`,
			`;# The SUBJECT comes first and the pattern second, so a`,
			`;# pipeline reads left to right. +re/[a-z]+/ is the literal form.`,
		},
		"run-re": {
			`MiniLang.run-re compiled {} subject             ;# the runner behind a COMPILED re`,
			`;# Reached through MiniLang.register-compiled, not called directly:`,
			`;# for a one-off match write "subject mini re pattern".`,
		},
		"lang_bf": {
			`mini bf '++++++++[>++++++++<-]>+.'                ;# 'A'`,
			`"hi" mini bf ',.,.'                              ;# 'hi' — the subject is stdin`,
			`mini bf ',.+.' {in:'A'}                          ;# 'AB' — or pass it as an option`,
		},
		"lang_gex":    {`"path/to/x" mini gex 'path/*/x'                  ;# glob-style expressions`},
		"lang_math":   {`mini math '1 + 2 * 3'                            ;# 7 — infix arithmetic`},
		"lang_jp":     {`data mini jp '$.a.b[0]'                          ;# JSONPath`},
		"lang_jq":     {`data mini jq '.a | length'                       ;# jq`},
		"lang_xp":     {`doc mini xp '//item/@id'                         ;# XPath`},
		"lang_sp":     {`data mini sp 'a.b.0'                             ;# struct paths`},
		"lang_micron": {`mini micron '1kb'                               ;# the micron scalar syntax`},
		"micron": {
			`MiniLang.micron                                  ;# the builtin Micron leaf set`,
			`;# The +m literal grammar is FIXED to these; a custom Micron`,
			`;# type is built with "def Nameon refine Micron {…}" and make.`,
		},
		"register": {
			`def poly (fn [[src:String opts:Map] [Integer] [`,
			`  ((opts.x pow 2) add (3 mul opts.y))`,
			`]])`,
			`mini poly 'x^2 + 3*y' {x:10, y:2}                ;# 106 — any fn of that shape is a mini-language`,
			`;# A 3-param fn [[src opts subject]] also receives the subject.`,
		},
		"register-compiled": {
			`MiniLang.register-compiled mykind compile-fn     ;# compile once per source, then run per subject`,
		},
	})
}
