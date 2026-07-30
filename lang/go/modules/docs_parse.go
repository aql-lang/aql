package modules

func init() {
	registerDocs("aql:parse", map[string]string{
		"grammar": "Mint a fresh parser-grammar builder: `Parse.grammar` → a Grammar value. " +
			"Chain Parse.abnf / Parse.rule / Parse.token / Parse.matcher / Parse.action onto it, " +
			"then finalize with Parse.parser.",
		"abnf": "Add an ABNF grammar to a builder: `Parse.abnf <grammar> <abnf-src> <opts?>`. " +
			"opts: {start:'rule', tag:'name', builtins:Bool, marks:Bool}. The install is deferred " +
			"to Parse.parser so semantic actions (Parse.action) are baked in. github.com/tabnas/abnf/go.",
		"rule": "Add a declarative grammar rule (the Map-subtype form): `Parse.rule <grammar> <name> <spec>`. " +
			"spec mirrors Parse.RuleSpec {open:[alt…] close:[alt…]}, each alternate a Parse.AltSpec " +
			"{s p r b a g n}. An inline Function in `a` is attached as a semantic action.",
		"token": "Register a fixed lexer token: `Parse.token <grammar> <name> <literal>` — e.g. " +
			"`Parse.token g '#PL' '+'` lexes '+' as the #PL token.",
		"matcher": "Register a custom lex matcher backed by an AQL fn: `Parse.matcher <grammar> <name> <priority> <fn>`. " +
			"The fn receives the unconsumed source String and returns None (no match) or " +
			"{src:String tin:String? val:Any?} — src is the matched prefix, tin the emitted token (default '#TX').",
		"action": "Attach a semantic action (a mark) backed by an AQL fn: `Parse.action <grammar> <ref> <fn>`. " +
			"ref is @rule:phase (bo/ao/bc/ac) or @rule:o|c:MARK. The fn receives the rule's current node " +
			"and returns its replacement — the bridge that lets a parser emit custom AQL data types.",
		"spec": "Apply a WHOLE declarative grammar in one call, mirroring the tabnas GrammarSpec " +
			"document: `Parse.spec <grammar> {options rule ref v abnf matcher}`. options is the tabnas " +
			"OptionsMap (fixed:{token:{'#T':'@'}} declares fixed tokens, rule:{start:'name'} the start " +
			"rule, plus space/line/text/number/… lexing options) — applied first, as tabnas does. " +
			"ref:{'@name': fn-or-list} is the named-action table (serves ABNF marks and rule-alt " +
			"a:'@name' references). rule:{name:{open:[alt…] close:[alt…]}} is the GrammarRuleSpec shape " +
			"(see Parse.RuleSpec/Parse.AltSpec). v gates the builtin config-schema version. The two " +
			"AQL extensions: abnf (a String, a {src start tag builtins marks} map, or a list of either) " +
			"and matcher:{name:{priority fn}}. Everything defers to Parse.parser and composes with " +
			"the chained builder words. An unknown section is a loud parse_bad_spec.",
		"parser": "Finalize a builder into a ParseLang Function VALUE: `Parse.parser <grammar>`. " +
			"Bind it (`def calc (Parse.parser g)`) or pass it directly, then run it via the " +
			"`parse` value form: `parse calc '<source>'` / `parse (Parse.parser g) '<source>'`.",
	})

	// Building a parser is a SEQUENCE — make a grammar, attach actions and
	// rules, then realise it — and no single word's signature can show
	// that order. Results are from verified lang/spec/module-parse.tsv
	// rows. (This is the grammar builder; to parse a KNOWN format reach
	// for aql:parselang instead.)
	registerExamples("aql:parse", map[string][]string{
		"grammar": {
			`import "aql:parselang"`,
			`def g Parse.grammar                              ;# a fresh grammar to build up`,
			`Parse.action g '@op:o:INC' ([nd:Any] => [7])     ;# what a matched rule produces`,
			`Parse.abnf g 'op = "inc" / "dec"' {start:'op'}   ;# the syntax, as ABNF`,
			`def op (Parse.parser g)                          ;# realise it`,
			`parse op 'inc'                                   ;# 7`,
		},
		"abnf":   {`Parse.abnf g 'op = "inc" / "dec"' {start:'op'}   ;# start: names the entry rule`},
		"action": {`Parse.action g '@op:o:INC' ([nd:Any] => [{k:'inc'}])   ;# a fn, or a list of fns applied in order`},
		"parser": {`def op (Parse.parser g)                          ;# grammar to callable parser; then "parse op src"`},
		"token":  {`Parse.token g '#PL' '+'                          ;# a fixed token, so '+' lexes on its own`},
		"rule":   {`Parse.rule g extra {open:[{s:'#PL'}]}            ;# a hand-written rule alternate`},
		"matcher": {
			`Parse.matcher g skip 1000000 ([s:String] => [None])   ;# a custom lexer matcher, by priority`,
		},
		"spec": {
			`Parse.spec g {`,
			`  ref:  {'@op:o:INC': ([nd:Any] => [7])}`,
			`  abnf: {src:'op = "inc" / "dec"'  start:'op'}`,
			`}`,
			`parse (Parse.parser g) 'inc'                     ;# 7`,
			`;# One DATA map instead of the call sequence above — the whole`,
			`;# grammar becomes something you can store and ship.`,
		},
	})
}
