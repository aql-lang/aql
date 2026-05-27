package help

func init() {
	register(&Entry{
		Word:    "help",
		Summary: "Show help for an AQL word.",
		Description: "With no argument, prints a summary of the help word itself. " +
			"Given a word name (as a word, atom, or string), prints detailed help " +
			"including description, signatures, examples, and notes. " +
			"Has forward arg collection, so 'add help' works directly.",
		Notes: []string{
			"Help is available for all built-in words.",
			"Use word form (add help) for any registered word.",
			"Use string form (\"name\" help) as an alternative.",
		},
	})
}
