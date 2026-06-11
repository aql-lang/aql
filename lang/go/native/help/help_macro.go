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
}
