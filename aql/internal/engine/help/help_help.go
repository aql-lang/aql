package help

func init() {
	register(&Entry{
		Word:    "help",
		Summary: "Show help for an AQL word.",
		Signatures: []string{
			"[] -> []",
			"[atom] -> []",
			"[string] -> []",
		},
		Description: "With no argument, prints a summary of the help word itself. " +
			"Given a word name (as an atom or string), prints detailed help " +
			"including description, signatures, examples, and notes.",
		Examples: []string{
			`help                 => (prints help about help)`,
			`add help             => (prints help about add)`,
			`"concat" help        => (prints help about concat)`,
		},
		Notes: []string{
			"Help is available for all built-in words.",
		},
	})
}
