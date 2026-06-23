package help

func init() {
	register(&Entry{
		Word:    "quote",
		Summary: "Quote the next token as data: a word becomes an atom, a list stays unevaluated.",
		Description: "Prevents evaluation. `quote add` is the atom `add`; `quote [1 add 2]` is the unevaluated " +
			"list `[1 add 2]`. The `/q` word suffix (`add/q`) is the canonical short form.",
		Examples: []string{
			`quote add   ;# => add   (an atom, not a call)`,
		},
	})

	register(&Entry{
		Word:    "unquote",
		Summary: "Inside a macro template, evaluate an argument and splice the result into the quoted output.",
		Description: "The dual of quote, used in macro bodies: `unquote x` evaluates x and inserts the resulting " +
			"value into the surrounding `quote` template — this is how a macro builds code from its arguments.",
	})

	register(&Entry{
		Word:    "splice",
		Summary: "Wrap a value in a splice marker so its elements spread in place when stepped.",
		Description: "Like word, splice defers expansion: when the marker reaches the engine pointer it is replaced " +
			"by its payload's elements, which then re-step against the live stack. Used to build code that splices " +
			"into argument positions.",
	})

	register(&Entry{
		Word:    "word",
		Summary: "Bind a body that splices its elements inline at every reference (Forth-style).",
		Description: "`def name word [body]` makes name expand its body in place wherever it is used, rather than " +
			"binding the list as a value. The splice fires when the reference is stepped, so it sees current bindings.",
		Examples: []string{`def double word [dup add]  5 double   ;# => 10`},
	})

	register(&Entry{
		Word:    "macro",
		Summary: "Define a hygienic macro: AQL code that rewrites code before it runs.",
		Description: "`def name (macro [[params] [template]])` — the template, built with quote / unquote / splice, " +
			"expands at each call site. Macros add new syntax in AQL itself; hygiene (via gensym) keeps temporaries " +
			"from colliding with user names.",
	})

	register(&Entry{
		Word:        "macroexpand",
		Summary:     "Expand a macro call one step, returning the rewritten code as data.",
		Description: "Shows what a macro produces without running it — the macro author's debugging tool.",
	})

	register(&Entry{
		Word:    "gensym",
		Summary: "Generate a fresh, unique symbol — for hygienic macro temporaries.",
		Description: "Each call returns a new name (e.g. tmp$g1, tmp$g2, …) guaranteed not to clash with any other " +
			"binding, so macro-introduced locals stay hygienic.",
	})

	register(&Entry{
		Word:    "canon",
		Summary: "Render any value as canonical, round-trippable AQL source.",
		Description: "The textual inverse of parsing: `canon v` produces the canonical source form of a value — " +
			"normalising sugar (dotted access, `/q`, `=>`) to its underlying words. Useful for serialising code or " +
			"data back to source.",
	})

	register(&Entry{
		Word:    "mini",
		Summary: "Call an embedded mini-language: mini <kind> <src> <opts?>.",
		Description: "Expands to the standard call `MiniLang.lang_<kind> <src> <opts> end` at the call site — " +
			"kind-declared inputs come from the stack, results are whatever the kind leaves. Kinds live in the " +
			"aql:minilang module (import it first; MiniLang.kinds lists them; register your own with " +
			"MiniLang.register). An unknown kind is an expansion-time error. Note backslashes in quoted strings " +
			"need doubling ('\\\\d'); backtick strings are backslash-safe.",
		Examples: []string{
			`import "aql:minilang"  def r ("AbcD" mini re '[a-z]+') end  r.fst.m   ;# => 'bc'`,
			`import "aql:minilang"  mini bf '++++++++[>++++++++<-]>+.'   ;# => 'A'`,
		},
	})

	register(&Entry{
		Word:    "parse",
		Summary: "Call a named parser: parse <kind> <opts?> <source>.",
		Description: "The sibling of `mini`: expands to the standard call " +
			"`ParseLang.parse_<kind> <source> <opts> end` at the call site. The `source` is the " +
			"required LAST argument — a String or a {src:…} Source map — and `opts` is the optional " +
			"middle one. A parser returns whatever the language yields (an AST, a transduction, …), " +
			"typed Any. Parsers live in the aql:parselang module (import it first; ParseLang.kinds " +
			"lists them; register your own with ParseLang.register or the Go RegisterParser host API). " +
			"An unknown kind is an expansion-time error.",
		Examples: []string{
			`import "aql:parselang"  parse calc 'x + y'        ;# => the parser's AST`,
			`import "aql:parselang"  parse calc {src:'x + y'}  ;# a {src:…} source map`,
		},
	})

	register(&Entry{
		Word:    "emit",
		Summary: "Emit a value to a string: emit <kind> <opts?> <data> (kind optional — defaults to the value's natural format).",
		Description: "The symmetric inverse of `parse`: expands to the standard call " +
			"`EmitLang get emit_<kind> <data> <opts> end` at the call site. The `data` is the " +
			"required LAST argument and `opts` is the optional middle one. The kind is OPTIONAL — " +
			"omit it and `emit` picks the value's natural format (Map/List/scalar/Error → json, " +
			"Table → csv, Xml → xml). Built-in kinds: json, jsonic, yaml, csv, tsv, toml, ini, xml " +
			"(opts {pretty:true} / {indent:N} for json/jsonic/yaml/xml; {separation:sep} for csv). " +
			"Every emitter traverses the value through the canonical walk engine — one code path " +
			"for all formats. Emitters live in the aql:emitlang module (import it first; " +
			"EmitLang.kinds lists them; register your own with EmitLang.register). An unknown " +
			"explicit kind is an expansion-time error; a shape a format cannot represent (e.g. " +
			"`emit toml [1 2]`, `emit xml {a:1}`) raises emit_unsupported.",
		Examples: []string{
			`import "aql:emitlang"  emit {a:1 b:[2 3]}            ;# => compact json (natural)`,
			`import "aql:emitlang"  emit json {pretty:true} {a:1} ;# => indented json`,
		},
	})
}
