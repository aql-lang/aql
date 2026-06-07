package modules

func init() {
	registerDocs("aql:io", map[string]string{
		"read":     "Read the contents of a file.",
		"write":    "Write a string to a file, returning its path.",
		"printstr": "Write formatted text to output, leaving no value.",
		"stdin":    "Standard-input handle.",
		"stdout":   "Standard-output handle.",
		"stderr":   "Standard-error handle.",
		"trace":    "Run a list as a traced sub-program, returning its result.",
		"folder":   "Create a directory, returning its Path.",
	})
}
