package modules

func init() {
	registerDocs("aql:string-util", map[string]string{
		"upper":      "Uppercase a string.",
		"lower":      "Lowercase a string.",
		"concat":     "Join a list of values into one string.",
		"split":      "Split a string on a separator into a list (subject last).",
		"trim":       "Strip leading and trailing whitespace.",
		"contains":   "Report whether a haystack contains a needle (haystack last).",
		"indexof":    "Index of a needle in a haystack (haystack last), -1 if absent.",
		"replace":    "Replace occurrences of a substring (subject last).",
		"changecase": "Recase a string (lower/upper/camel/snake/...).",
		"normalize":  "Unicode-normalise (NFC) and tidy whitespace.",
		"repeat":     "Repeat a string a given number of times (count then string).",
		"pad":        "Pad a string to a width with a fill character.",
		"match":      "Find a pattern in a string, returning the matches (subject last).",
		"escape":     "Escape a string for a target syntax (shell/sed/...).",
	})
}
