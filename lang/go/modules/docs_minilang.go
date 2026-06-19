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
