package help

func init() {
	register(&Entry{
		Word:    "help",
		Summary: "Show help for an AQL word.",
		Signatures: []string{
			"[] -> []",
			"[word] -> []",
			"[atom] -> []",
			"[string] -> []",
		},
		Description: "With no argument, prints a summary of the help word itself. " +
			"Given a word name (as a word, atom, or string), prints detailed help " +
			"including description, signatures, examples, and notes. " +
			"Has forward precedence, so 'add help' works directly.",
		Examples: []string{
			`help                           (prints help about help)`,
			`add help                       (prints help about add)`,
			`concat help                    (prints help about concat)`,
			`"split" help                   (prints help about split)`,
		},
		Notes: []string{
			"Help is available for all built-in words.",
			"Use word form (add help) for any registered word.",
			"Use string form (\"name\" help) as an alternative.",
		},
	})
}
