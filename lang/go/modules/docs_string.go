package modules

func init() {
	registerDocs("aql:string-util", map[string]string{
		"upper":      "Uppercase a string.",
		"lower":      "Lowercase a string.",
		"concat":     "Join a list of values into one string.",
		"split":      "Split a string on a separator into a list.",
		"trim":       "Strip leading and trailing whitespace.",
		"contains":   "Report whether a string contains a substring.",
		"indexof":    "Position of a substring, -1 if absent.",
		"replace":    "Replace occurrences of a substring.",
		"changecase": "Recase a string (lower/upper/camel/snake/...).",
		"normalize":  "Unicode-normalise (NFC) and tidy whitespace.",
		"repeat":     "Repeat a string a given number of times.",
		"pad":        "Pad a string to a width with a fill character.",
		"match":      "Match a pattern in a string, returning the matches.",
		"escape":     "Escape a string for a target syntax (shell/sed/...).",
	})
}
