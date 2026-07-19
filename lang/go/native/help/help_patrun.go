package help

func init() {
	register(&Entry{
		Word:    "patrun",
		Summary: "Create an empty typed Patrun — a pattern→value dispatch table.",
		Description: "`patrun T` returns a fresh, mutable Patrun whose every stored/matched value is declared to " +
			"be a T. Register rules with `add {pattern} value pm` (the value must be a T) and look them up with " +
			"`find {subject} pm`, which returns the value of the most specific pattern that subset-matches the " +
			"subject (as `T | None`). Match-values in the pattern are Scalars compared by string coercion (1 and " +
			"\"1\" share a rule). The stored value is often a function (`patrun Function`), making a Patrun a " +
			"dispatch table. Vendors github.com/rjrodger/patrun.",
		Examples: []string{
			`def r (patrun String) add {a:1} "A" r find {a:1 z:9} r ; # => 'A'`,
		},
	})

	register(&Entry{
		Word:    "find",
		Summary: "Look up the most specific matching rule in a Patrun.",
		Description: "`find {subject} pm` returns the value of the most specific pattern that subset-matches the " +
			"subject — more matched keys win, unknown subject keys are ignored — or None if none match. On a " +
			"specificity tie the alphabetically earlier pattern wins. `find {subject} pm {exact:true}` requires " +
			"the matched pattern's keys to equal the subject's.",
		Examples: []string{
			`def r (patrun String) add {a:1} "A" r add {a:1 b:1} "B" r find {a:1 b:1} r ; # => 'B'`,
			`find {x:9} r ; # => None`,
		},
	})

	register(&Entry{
		Word:    "patterns",
		Summary: "List a Patrun's registered rules.",
		Description: "`patterns pm` returns the registered rules as a list of `{pattern value}` maps, in " +
			"registration order.",
		Examples: []string{
			`def r (patrun String) add {a:1} "A" r patterns r ; # => [{pattern:{a:1} value:'A'}]`,
		},
	})
}
