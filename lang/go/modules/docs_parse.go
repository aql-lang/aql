package modules

func init() {
	registerDocs("aql:parse", map[string]string{
		"grammar": "Mint a fresh parser-grammar builder: `Parse.grammar` → a Grammar value. " +
			"Chain Parse.abnf / Parse.rule / Parse.token / Parse.matcher / Parse.action onto it, " +
			"then finalize with Parse.register.",
		"abnf": "Add an ABNF grammar to a builder: `Parse.abnf <grammar> <abnf-src> <opts?>`. " +
			"opts: {start:'rule', tag:'name', builtins:Bool, marks:Bool}. The install is deferred " +
			"to Parse.register so semantic actions (Parse.action) are baked in. github.com/tabnas/abnf/go.",
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
		"register": "Finalize a builder and register it as a `parse <name>` kind: `Parse.register <name> <grammar>`. " +
			"After import of aql:parselang, run it via the macro: `parse <name> '<source>'`.",
	})
}
