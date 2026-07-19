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
		"lang_math": "Traditional maths formula: `mini math '<formula>' {vars}` — evaluate an " +
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
		"lang_micron": "Micron literal: `+m:alice@example.com` (≡ mini micron '…' ≡ mini m '…') — " +
			"parse the source with the ONE merged tabnas grammar built from each builtin Micron leaf's literal grammar " +
			"(Emailon, then Urlon, then Pathon) — the matching shape decides the type. Pathon accepts any " +
			"whitespace-free source, so it is the catch-all (`+m:a/b` is a Pathon). User Microns join " +
			"the merge via MiniLang.micron (see micron). URL sources " +
			"contain `:` and `/` — pick a delimiter outside the source: `+m|https://x.com/a`.",
		"lang_m": "Short form of the micron kind — see lang_micron. `+m:alice@example.com` " +
			"parses as an Emailon; `+m|https://x.com/a` as an Urlon; `+m:a/b` as a Pathon.",
		"micron": "OPT-IN user-Micron literal hook: `MiniLang.micron <Kind> <grammar> <fn>` registers " +
			"a literal shape for a USER-defined Micron kind (def Nameon refine Micron {\u2026}). The shape " +
			"is a whole tabnas GRAMMAR \u2014 a declarative spec Map (the same GrammarSpec document " +
			"Parse.spec accepts: `{options:{match:{token:{'#TK':'@/T-[0-9]+/'}}}} rule:{\u2026} ref:{\u2026}}`) " +
			"or an aql:parse Parse.grammar builder value (consumed, single-use). The grammar must " +
			"declare \u22651 match token \u2014 the merged `+m` dispatch is token-driven, so the gate token(s) " +
			"claim the kind's literal spans \u2014 and merges into the `+m` grammar BETWEEN the builtin " +
			"leaves and the Pathon catch-all (registration order): Emailon/Urlon keep their spans, " +
			"unclaimed spans still fall to Pathon. A token-only grammar's gates are auto-anchored to " +
			"the whole literal; a grammar with its own val alternates/rules owns recognition verbatim " +
			"(a claimed-then-rejected span is a loud mini_parse_error). The fn receives the grammar's " +
			"parse RESULT (the matched span String for a plain gate; whatever the rules/ref actions " +
			"produced otherwise) and must return one Kind instance \u2014 a failure or wrong-typed result " +
			"is loud. One shape per kind; token names must be unique across the merge.",
		"register": "TOMBSTONE — raises mini_registry_frozen. The mini kind namespace is fixed " +
			"(built-in kinds only); pass a custom mini-language as a Function value instead: " +
			"def myl (fn [[src:String opts:Map] [Any] [...]])  mini myl '...' — Go hosts build " +
			"one with NewMiniLangFn.",
		"Re": "The named type of a `re` partial — the Function a parked `+re/…/` (or " +
			"`def f (+re/…/)`) produces. `typeof` still reports Function (a member type is a " +
			"constraint, like a DepScalar); use `is` (`f/r is (MiniLang.Re)`) or a typed fn param " +
			"(`fn [[m:(MiniLang.Re) s:String] …]`) to require specifically a regexp matcher. " +
			"Every built-in filter kind has one: Re, Gex, Jp, Jq, Xp.",
		"Gex": "The named type of a `gex` partial (a stored glob-expression matcher) — " +
			"see MiniLang.Re for the convention.",
		"Jp": "The named type of a `jp` partial (a stored JSONPath query) — " +
			"see MiniLang.Re for the convention.",
		"Jq": "The named type of a `jq` partial (a stored jq filter) — " +
			"see MiniLang.Re for the convention.",
		"Xp": "The named type of an `xp` partial (a stored XPath query) — " +
			"see MiniLang.Re for the convention.",
		"kinds": "List the (fixed) mini-language kind atoms.",
		"register-compiled": "TOMBSTONE — raises mini_registry_frozen. The AQL compile-hook " +
			"surface died with the frozen kind namespace; pass a custom mini-language as a " +
			"Function value (mini <fn> '...') and memoize any expensive compile inside the fn.",
		"run-re": "Internal: the compiled-`re` consumer — matches a precompiled-pattern carrier " +
			"(from the re compile hook) against the stack subject.",
	})
}
