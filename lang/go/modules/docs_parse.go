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
}
