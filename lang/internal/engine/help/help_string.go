package help

func init() {
	register(&Entry{
		Word:    "upper",
		Summary: "Convert a string to uppercase.",
		Description: "Converts every character in the input string to its uppercase equivalent " +
			"using Go's strings.ToUpper (Unicode-aware). Works on both string and atom values.",
		Notes: []string{
			"Also accepts atoms: the word foo upper produces 'FOO'.",
		},
	})

	register(&Entry{
		Word:    "lower",
		Summary: "Convert a string to lowercase.",
		Description: "Converts every character in the input string to its lowercase equivalent " +
			"using Go's strings.ToLower (Unicode-aware). Works on both string and atom values.",
		Notes: []string{
			"Also accepts atoms: the word FOO lower produces 'foo'.",
		},
	})

	register(&Entry{
		Word:    "concat",
		Summary: "Concatenate list elements into a single string.",
		Description: "Joins all elements of a list into a single string. Each element is " +
			"converted to its string representation. Use the options map to specify a " +
			"separator (sep) or to skip empty/nullish parts.",
		Notes: []string{
			"None values are converted to empty strings unless skipNullish is true.",
			"Options: sep (string), skipEmpty (bool), skipNullish (bool).",
		},
	})

	register(&Entry{
		Word:    "split",
		Summary: "Split a string into a list of parts.",
		Description: "Splits the input string by the separator. By default uses literal matching " +
			"and discards empty parts. Set mode to \"shell\" to use glob-style pattern " +
			"matching as the delimiter.",
		Notes: []string{
			"Empty parts are dropped by default; set keepEmpty: true to keep them.",
			"Options: cs, mode, lim, keepEmpty, trimParts, u, norm.",
		},
	})

	register(&Entry{
		Word:    "trim",
		Summary: "Trim whitespace or specific characters from a string.",
		Description: "Removes leading and trailing whitespace from the input string. Use the " +
			"options map to trim specific characters (chars) or to trim from only one " +
			"side (side: \"left\" or \"right\").",
		Notes: []string{
			"Default side is \"both\". When chars is set with cs: \"insensitive\", both cases are trimmed.",
			"Options: side (left/right/both), chars, cs, u, norm.",
		},
	})

	register(&Entry{
		Word:    "contains",
		Summary: "Test whether a string contains a search term.",
		Description: "Returns true if the input string contains the search string. Supports " +
			"case-insensitive matching, whole-word matching, anchored matching " +
			"(startsWith/endsWith), and shell-pattern matching.",
		Notes: []string{
			"anchored: true is equivalent to anchored: \"both\" (exact match).",
			"wholeWord requires matches to be at word boundaries.",
			"Options: cs, mode, anchored, wholeWord, u, norm.",
		},
	})

	register(&Entry{
		Word:    "indexof",
		Summary: "Find the index of a search term in a string.",
		Description: "Returns the byte index of the first (or last) occurrence of the search " +
			"term in the input string. Returns -1 if not found.",
		Notes: []string{
			"Returns byte offsets, not rune offsets.",
			"Options: cs, mode, from, occ (\"first\"/\"last\"), u, norm.",
		},
	})

	register(&Entry{
		Word:    "replace",
		Summary: "Replace occurrences of a search term in a string.",
		Description: "Replaces occurrences of the search string with the replacement string. " +
			"By default replaces only the first occurrence. Use scope: \"all\" to replace all.",
		Notes: []string{
			"scope defaults to \"first\"; set to \"all\" for global replace.",
			"count limits the maximum number of replacements when scope is \"all\".",
			"Options: cs, mode, scope, from, count, u, norm.",
		},
	})

	register(&Entry{
		Word:    "changecase",
		Summary: "Apply a casing transformation to a string.",
		Description: "Transforms the case of a string according to the selected style. " +
			"Defaults to \"lower\". Available styles: lower, upper, capitalize (first char), " +
			"title (first char of each word), sentence (first char after lowering all), fold.",
		Notes: []string{
			"fold is an approximation using toLower; for true Unicode case folding, use a locale-aware library.",
			"Options: style, u, norm, loc.",
		},
	})

	register(&Entry{
		Word:    "normalize",
		Summary: "Normalize Unicode and optionally clean whitespace and line endings.",
		Description: "Applies Unicode normalization (default NFC) and optionally trims " +
			"surrounding whitespace, collapses internal whitespace runs, and normalizes " +
			"line endings.",
		Notes: []string{
			"Whitespace collapsing preserves newlines; only spaces and tabs are collapsed.",
			"Options: form (NFC/NFD/NFKC/NFKD), trim, collapseWs, eol (preserve/lf/crlf).",
		},
	})

	register(&Entry{
		Word:    "repeat",
		Summary: "Repeat a string a fixed number of times.",
		Description: "Returns the input string repeated count times. Optionally insert a " +
			"separator between repetitions using the sep option.",
		Notes: []string{
			"Count must be non-negative; negative values produce an error.",
			"Options: sep.",
		},
	})

	register(&Entry{
		Word:    "pad",
		Summary: "Pad a string to a desired length.",
		Description: "Pads the input string to reach the target length. By default pads on " +
			"the right with spaces. Use options to pad left, both sides, or with a " +
			"custom fill string.",
		Notes: []string{
			"If the input already meets or exceeds the target length, it is returned unchanged unless trunc is true.",
			"Options: side (left/right/both), fill, trunc.",
		},
	})

	register(&Entry{
		Word:    "match",
		Summary: "Match a pattern and return a structured result.",
		Description: "Searches for the pattern in the input and returns a map with fields: " +
			"ok (bool), ms (list of match maps), fst (first match), lst (last match), n (count). " +
			"Each match map has m (matched text), i (start index), e (end index).",
		Notes: []string{
			"Returns a map, not a boolean. Use .ok to get the boolean result.",
			"In shell mode, uses glob matching (* ? [...]).",
			"Options: cs, mode, scope (first/all), u, norm.",
		},
	})

	register(&Entry{
		Word:    "escape",
		Summary: "Escape a string for safe use in shells and text tools.",
		Description: "Escapes special characters in the input for the target environment. " +
			"Supports sh, bash, sed, awk, and grep targets. Optionally wraps the result " +
			"in single or double quotes.",
		Notes: []string{
			"Default target is sh (POSIX shell).",
			"Options: tgt (sh/bash/sed/awk/grep), quote (none/single/double).",
		},
	})
}
