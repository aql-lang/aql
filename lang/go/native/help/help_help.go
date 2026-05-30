package help

func init() {
	register(&Entry{
		Word:    "help",
		Summary: "Show an overview of AQL and how to get help.",
		Description: "Prints a short orientation to the language: how words and " +
			"values compose, how to define words, and how to use the `describe` " +
			"word to look up documentation for any built-in word. Takes no " +
			"arguments.",
		Notes: []string{
			"Use `describe <word>` (e.g. describe add) for docs on a specific word.",
			"In the REPL, `/help` prints this overview and `/describe <word>` looks one up.",
		},
	})

	register(&Entry{
		Word:    "describe",
		Summary: "Describe an AQL word: signatures, examples, and notes.",
		Description: "Given a word name (as a word, atom, or string), prints detailed " +
			"documentation including its description, signatures, examples, and " +
			"notes. With no argument, prints a reminder of how to call describe. " +
			"Has forward arg collection, so `describe add` works directly.",
		Notes: []string{
			"Use forward form (describe add) for any registered word.",
			"Use string form (\"name\" describe) as an alternative.",
			"Run `help` for a language overview.",
		},
	})
}
